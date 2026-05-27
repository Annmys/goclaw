package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type skillQualityStoreStub struct {
	skills []store.SkillInfo
}

func (s *skillQualityStoreStub) ListSkills(context.Context) []store.SkillInfo     { return s.skills }
func (s *skillQualityStoreStub) LoadSkill(context.Context, string) (string, bool) { return "", false }
func (s *skillQualityStoreStub) LoadForContext(context.Context, []string) string  { return "" }
func (s *skillQualityStoreStub) BuildSummary(context.Context, []string) string    { return "" }
func (s *skillQualityStoreStub) GetSkill(context.Context, string) (*store.SkillInfo, bool) {
	return nil, false
}
func (s *skillQualityStoreStub) FilterSkills(context.Context, []string) []store.SkillInfo { return nil }
func (s *skillQualityStoreStub) Version() int64                                           { return 1 }
func (s *skillQualityStoreStub) BumpVersion()                                             {}
func (s *skillQualityStoreStub) Dirs() []string                                           { return nil }

type skillQualityMetricsStub struct {
	feedback   []store.EvolutionMetric
	regression []store.EvolutionMetric
}

func (m *skillQualityMetricsStub) RecordMetric(context.Context, store.EvolutionMetric) error {
	return nil
}
func (m *skillQualityMetricsStub) QueryMetrics(_ context.Context, _ uuid.UUID, metricType store.MetricType, _ time.Time, _ int) ([]store.EvolutionMetric, error) {
	switch metricType {
	case store.MetricFeedback:
		return m.feedback, nil
	case store.MetricRegression:
		return m.regression, nil
	default:
		return nil, nil
	}
}
func (m *skillQualityMetricsStub) AggregateToolMetrics(context.Context, uuid.UUID, time.Time) ([]store.ToolAggregate, error) {
	return nil, nil
}
func (m *skillQualityMetricsStub) AggregateRetrievalMetrics(context.Context, uuid.UUID, time.Time) ([]store.RetrievalAggregate, error) {
	return nil, nil
}
func (m *skillQualityMetricsStub) Cleanup(context.Context, time.Time) (int64, error) { return 0, nil }

func TestBuildSkillQualityScores_UsesSkillMetadata(t *testing.T) {
	skillStore := &skillQualityStoreStub{
		skills: []store.SkillInfo{
			{
				Name:               "包装计算",
				Slug:               "package-calculation",
				DisplayName:        "包装计算",
				Family:             "package-calculation",
				Canonical:          true,
				Aliases:            []string{"包装资料"},
				RegressionPrefixes: []string{"package_materials_"},
			},
		},
	}
	metrics := &skillQualityMetricsStub{
		feedback: []store.EvolutionMetric{mustMetric(t, store.MetricFeedback, map[string]any{
			"feedback_type":   "correction",
			"message_content": "包装资料输出少了一个字段",
			"correction":      "应该更新包装计算 skill",
		})},
		regression: []store.EvolutionMetric{mustMetric(t, store.MetricRegression, map[string]any{
			"cases": []map[string]any{
				{"name": "package_materials_case_001", "status": "failed"},
			},
		})},
	}

	scores, err := BuildSkillQualityScores(context.Background(), metrics, skillStore, uuid.New(), time.Now().Add(-24*time.Hour), []store.ToolAggregate{
		{ToolName: "skill:package-calculation", CallCount: 10, SuccessRate: 0.9, AvgDurationMs: 50},
	})
	if err != nil {
		t.Fatalf("BuildSkillQualityScores returned error: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}
	if scores[0].SkillName != "package-calculation" {
		t.Fatalf("SkillName = %q, want package-calculation", scores[0].SkillName)
	}
	if scores[0].FeedbackCorrections != 1 {
		t.Fatalf("FeedbackCorrections = %d, want 1", scores[0].FeedbackCorrections)
	}
	if scores[0].RegressionFailures != 1 {
		t.Fatalf("RegressionFailures = %d, want 1", scores[0].RegressionFailures)
	}
}

func mustMetric(t *testing.T, metricType store.MetricType, value any) store.EvolutionMetric {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal metric value: %v", err)
	}
	return store.EvolutionMetric{
		MetricType: metricType,
		Value:      raw,
	}
}
