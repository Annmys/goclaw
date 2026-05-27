package agent

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
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

type skillQualityTarget struct {
	Slug               string
	Name               string
	DisplayName        string
	Family             string
	Canonical          bool
	Aliases            []string
	Replaces           []string
	RegressionPrefixes []string
	normalizedPatterns []string
}

func newSkillQualityTarget(info store.SkillInfo) skillQualityTarget {
	target := skillQualityTarget{
		Slug:               strings.TrimSpace(info.Slug),
		Name:               strings.TrimSpace(info.Name),
		DisplayName:        strings.TrimSpace(info.DisplayName),
		Family:             strings.TrimSpace(info.Family),
		Canonical:          info.Canonical,
		Aliases:            append([]string(nil), info.Aliases...),
		Replaces:           append([]string(nil), info.Replaces...),
		RegressionPrefixes: append([]string(nil), info.RegressionPrefixes...),
	}
	target.normalizedPatterns = target.collectPatterns()
	return target
}

func (t skillQualityTarget) collectPatterns() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(values ...string) {
		for _, v := range values {
			for _, candidate := range splitSkillQualityPhrases(v) {
				if candidate == "" {
					continue
				}
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				out = append(out, candidate)
			}
		}
	}
	add(t.Slug, t.Name, t.DisplayName, t.Family)
	add(t.Aliases...)
	add(t.Replaces...)
	add(t.RegressionPrefixes...)
	return out
}

func (t skillQualityTarget) normalizedRegressionPrefixes() []string {
	seen := make(map[string]struct{})
	var out []string
	for _, p := range t.RegressionPrefixes {
		p = normalizeSkillQualityText(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (t skillQualityTarget) scoreTextMatch(text string) int {
	best := 0
	for _, pattern := range t.normalizedPatterns {
		if pattern == "" {
			continue
		}
		switch {
		case text == pattern:
			if best < 100 {
				best = 100
			}
		case strings.HasPrefix(text, pattern):
			if best < 90 {
				best = 90
			}
		case strings.Contains(text, pattern):
			if best < 70 {
				best = 70
			}
		}
	}
	if best == 0 && t.Canonical && t.Family != "" && strings.Contains(text, normalizeSkillQualityText(t.Family)) {
		best = 60
	}
	return best
}

func (t skillQualityTarget) scoreRegressionCase(name string) int {
	normalized := normalizeSkillQualityText(name)
	best := 0
	for _, prefix := range t.normalizedRegressionPrefixes() {
		switch {
		case normalized == prefix:
			if best < 120 {
				best = 120
			}
		case strings.HasPrefix(normalized, prefix):
			if best < 110 {
				best = 110
			}
		case strings.Contains(normalized, prefix):
			if best < 100 {
				best = 100
			}
		}
	}
	if best > 0 {
		return best
	}
	return t.scoreTextMatch(normalized)
}

// BuildSkillQualityScores combines concrete skill calls, feedback, and regression signals.
// It is data-driven: skill names, display names, families, aliases, and regression prefixes
// are all loaded from the current skill catalog instead of hardcoded business lists.
func BuildSkillQualityScores(ctx context.Context, metrics store.EvolutionMetricsStore, skillStore store.SkillStore, agentID uuid.UUID, since time.Time, toolAggs []store.ToolAggregate) ([]store.SkillQualityScore, error) {
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

	targets := buildSkillQualityTargets(ctx, skillStore)

	for _, agg := range toolAggs {
		if !strings.HasPrefix(agg.ToolName, "skill:") {
			continue
		}
		skillName := resolveSkillFromToolName(agg.ToolName, targets)
		score := ensure(skillName)
		score.CallCount = agg.CallCount
		score.SuccessRate = agg.SuccessRate
		score.AvgDurationMs = agg.AvgDurationMs
	}

	feedbackMetrics, err := metrics.QueryMetrics(ctx, agentID, store.MetricFeedback, since, 500)
	if err != nil {
		return nil, err
	}
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
		if skill := resolveSkillFromText(text, targets); skill != "" {
			ensure(skill).FeedbackCorrections++
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
			if skill := resolveSkillFromRegressionCase(item.Name, targets); skill != "" {
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

func buildSkillQualityTargets(ctx context.Context, skillStore store.SkillStore) []skillQualityTarget {
	if skillStore == nil {
		return nil
	}
	return buildSkillQualityTargetsFromList(skillStore.ListSkills(ctx))
}

func buildSkillQualityTargetsFromList(list []store.SkillInfo) []skillQualityTarget {
	if len(list) == 0 {
		return nil
	}
	byFamily := make(map[string]skillQualityTarget)
	var singles []skillQualityTarget
	for _, info := range list {
		target := newSkillQualityTarget(info)
		if target.Slug == "" && target.Name == "" {
			continue
		}
		familyKey := skills.Slugify(target.Family)
		if familyKey == "" || familyKey == "skill" {
			singles = append(singles, target)
			continue
		}
		if existing, ok := byFamily[familyKey]; ok {
			byFamily[familyKey] = pickPreferredSkillTarget(existing, target)
			continue
		}
		byFamily[familyKey] = target
	}
	out := make([]skillQualityTarget, 0, len(byFamily)+len(singles))
	for _, target := range byFamily {
		out = append(out, target)
	}
	out = append(out, singles...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Canonical != out[j].Canonical {
			return out[i].Canonical
		}
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func pickPreferredSkillTarget(a, b skillQualityTarget) skillQualityTarget {
	if a.Canonical != b.Canonical {
		if b.Canonical {
			return b
		}
		return a
	}
	if len(b.normalizedPatterns) > len(a.normalizedPatterns) {
		return b
	}
	if len(b.normalizedPatterns) == len(a.normalizedPatterns) && b.Slug < a.Slug {
		return b
	}
	return a
}

func resolveSkillFromToolName(toolName string, targets []skillQualityTarget) string {
	name := strings.TrimSpace(strings.TrimPrefix(toolName, "skill:"))
	if name == "" {
		return "unknown"
	}
	if resolved := resolveSkillFromText(name, targets); resolved != "" {
		return resolved
	}
	return name
}

func resolveSkillFromText(text string, targets []skillQualityTarget) string {
	normalized := normalizeSkillQualityText(text)
	bestSkill := ""
	bestScore := 0
	for _, target := range targets {
		score := target.scoreTextMatch(normalized)
		if score > bestScore {
			bestScore = score
			bestSkill = target.Slug
		}
	}
	return bestSkill
}

func resolveSkillFromRegressionCase(name string, targets []skillQualityTarget) string {
	normalized := normalizeSkillQualityText(name)
	bestSkill := ""
	bestScore := 0
	for _, target := range targets {
		score := target.scoreRegressionCase(normalized)
		if score > bestScore {
			bestScore = score
			bestSkill = target.Slug
		}
	}
	return bestSkill
}

func normalizeSkillQualityText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func splitSkillQualityPhrases(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == '|' || r == '、'
	})
	if len(parts) == 0 {
		return []string{normalizeSkillQualityText(value)}
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if normalized := normalizeSkillQualityText(part); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
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
