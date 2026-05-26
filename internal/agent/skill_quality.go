package agent

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type skillQualityFeedbackValue struct {
	FeedbackType   string `json:"feedback_type"`
	MessageContent string `json:"message_content,omitempty"`
	Correction     string `json:"correction,omitempty"`
}

type skillQualityRegressionCase struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type skillQualityRegressionRun struct {
	Cases []skillQualityRegressionCase `json:"cases"`
}

// BuildSkillQualityScores combines concrete skill calls, feedback, and regression signals.
func BuildSkillQualityScores(ctx context.Context, metrics store.EvolutionMetricsStore, agentID uuid.UUID, since time.Time, toolAggs []store.ToolAggregate) ([]store.SkillQualityScore, error) {
	bySkill := map[string]*store.SkillQualityScore{}
	ensure := func(name string) *store.SkillQualityScore {
		name = strings.TrimSpace(strings.TrimPrefix(name, "skill:"))
		if name == "" {
			name = "unknown"
		}
		if existing := bySkill[name]; existing != nil {
			return existing
		}
		score := &store.SkillQualityScore{
			SkillName:    name,
			SuccessRate:  1,
			QualityScore: 100,
			RiskLevel:    "low",
		}
		bySkill[name] = score
		return score
	}

	for _, agg := range toolAggs {
		if !strings.HasPrefix(agg.ToolName, "skill:") {
			continue
		}
		score := ensure(agg.ToolName)
		score.CallCount = agg.CallCount
		score.SuccessRate = agg.SuccessRate
		score.AvgDurationMs = agg.AvgDurationMs
	}

	feedbackMetrics, err := metrics.QueryMetrics(ctx, agentID, store.MetricFeedback, since, 500)
	if err != nil {
		return nil, err
	}
	aliases := skillQualityAliases()
	for _, metric := range feedbackMetrics {
		var value skillQualityFeedbackValue
		if err := json.Unmarshal(metric.Value, &value); err != nil {
			value.FeedbackType = metric.MetricKey
		}
		if value.FeedbackType != "correction" && value.FeedbackType != "not_useful" {
			continue
		}
		text := normalizeSkillQualityText(strings.Join([]string{
			metric.MetricKey,
			value.MessageContent,
			value.Correction,
		}, " "))
		for skill, names := range aliases {
			if textMentionsAny(text, names) {
				ensure(skill).FeedbackCorrections++
			}
		}
	}

	regressionMetrics, err := metrics.QueryMetrics(ctx, agentID, store.MetricRegression, since, 100)
	if err != nil {
		return nil, err
	}
	for _, metric := range regressionMetrics {
		var run skillQualityRegressionRun
		if err := json.Unmarshal(metric.Value, &run); err != nil {
			continue
		}
		for _, item := range run.Cases {
			if item.Status != "failed" {
				continue
			}
			if skill := SkillFromRegressionCase(item.Name); skill != "" {
				ensure(skill).RegressionFailures++
			}
		}
	}

	out := make([]store.SkillQualityScore, 0, len(bySkill))
	for _, item := range bySkill {
		score := 100
		if item.CallCount > 0 {
			score -= int(math.Round((1 - item.SuccessRate) * 60))
		}
		score -= min(item.FeedbackCorrections*8, 32)
		score -= min(item.RegressionFailures*15, 45)
		if score < 0 {
			score = 0
		}
		item.QualityScore = score
		switch {
		case score < 70:
			item.RiskLevel = "high"
		case score < 85:
			item.RiskLevel = "medium"
		default:
			item.RiskLevel = "low"
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RiskLevel != out[j].RiskLevel {
			return riskRank(out[i].RiskLevel) > riskRank(out[j].RiskLevel)
		}
		if out[i].QualityScore != out[j].QualityScore {
			return out[i].QualityScore < out[j].QualityScore
		}
		if out[i].RegressionFailures != out[j].RegressionFailures {
			return out[i].RegressionFailures > out[j].RegressionFailures
		}
		if out[i].FeedbackCorrections != out[j].FeedbackCorrections {
			return out[i].FeedbackCorrections > out[j].FeedbackCorrections
		}
		return out[i].CallCount > out[j].CallCount
	})
	return out, nil
}

func skillQualityAliases() map[string][]string {
	return map[string][]string{
		"excel-type-identify":     {"excel-type-identify", "excel类型识别", "excel 类型识别"},
		"shipping-doc-processing": {"shipping-doc-processing", "船务清单处理", "船务清单", "ci", "epl"},
		"flow-order-query":        {"flow-order-query", "流转单查询", "流转单"},
		"package-weight-query":    {"package-weight-query", "产品包装重量查询", "产品包装重量", "重量表"},
		"label-generate":          {"label-generate", "标签生成", "标签"},
		"package-calculation":     {"package-calculation", "包装计算", "包装资料"},
	}
}

func normalizeSkillQualityText(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

func textMentionsAny(text string, names []string) bool {
	for _, name := range names {
		if name == "" {
			continue
		}
		if strings.Contains(text, normalizeSkillQualityText(name)) {
			return true
		}
	}
	return false
}

// SkillFromRegressionCase maps regression case names back to the responsible skill.
func SkillFromRegressionCase(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "core_skill_") {
		return strings.TrimPrefix(name, "core_skill_")
	}
	switch {
	case strings.HasPrefix(name, "shipping_doc_golden_"):
		return "shipping-doc-processing"
	case strings.HasPrefix(name, "flow_order_"):
		return "flow-order-query"
	case strings.HasPrefix(name, "package_weight_"):
		return "package-weight-query"
	case strings.HasPrefix(name, "package_materials_"):
		return "package-calculation"
	case strings.HasPrefix(name, "label_template_"):
		return "label-generate"
	default:
		return ""
	}
}

func riskRank(risk string) int {
	switch risk {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}
