package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type evolutionRegressionCase struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type evolutionRegressionRun struct {
	RunID        string                    `json:"run_id"`
	Status       string                    `json:"status"`
	Scope        string                    `json:"scope"`
	SuggestionID string                    `json:"suggestion_id,omitempty"`
	Total        int                       `json:"total"`
	Passed       int                       `json:"passed"`
	Failed       int                       `json:"failed"`
	StartedAt    time.Time                 `json:"started_at"`
	CompletedAt  time.Time                 `json:"completed_at"`
	Cases        []evolutionRegressionCase `json:"cases"`
}

type evolutionAuditEvent struct {
	Action       string    `json:"action"`
	Actor        string    `json:"actor,omitempty"`
	SuggestionID string    `json:"suggestion_id,omitempty"`
	Status       string    `json:"status,omitempty"`
	Result       string    `json:"result"`
	Message      string    `json:"message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (h *EvolutionHandler) handleListRegressionRuns(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.resolveAgentID(w, r)
	if !ok {
		return
	}
	limit := boundedLimit(r.URL.Query().Get("limit"), 20, 100)
	since := time.Now().AddDate(0, 0, -30)
	metrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricRegression, since, limit)
	if err != nil {
		slog.Warn("evolution.regression.list_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	runs := make([]evolutionRegressionRun, 0, len(metrics))
	for _, metric := range metrics {
		var run evolutionRegressionRun
		if err := json.Unmarshal(metric.Value, &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *EvolutionHandler) handleRunRegression(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	agentID, ok := h.resolveAgentID(w, r)
	if !ok {
		return
	}
	var body struct {
		Scope        string `json:"scope"`
		SuggestionID string `json:"suggestion_id"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !bindJSON(w, r, locale, &body) {
			return
		}
	}
	if body.Scope == "" {
		body.Scope = "agent_safety"
	}

	run := h.executeRegressionRun(r, agentID, body.Scope, body.SuggestionID)
	value, _ := json.Marshal(run)
	if err := h.metrics.RecordMetric(r.Context(), store.EvolutionMetric{
		ID:         uuid.New(),
		AgentID:    agentID,
		SessionKey: "evolution-center",
		MetricType: store.MetricRegression,
		MetricKey:  run.Status,
		Value:      value,
	}); err != nil {
		slog.Warn("evolution.regression.record_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.recordAuditEvent(r, agentID, "regression_run", body.SuggestionID, run.Status, "ok",
		fmt.Sprintf("%s: %d/%d passed", run.Scope, run.Passed, run.Total))
	writeJSON(w, http.StatusCreated, run)
}

func (h *EvolutionHandler) executeRegressionRun(r *http.Request, agentID uuid.UUID, scope, suggestionID string) evolutionRegressionRun {
	started := time.Now().UTC()
	run := evolutionRegressionRun{
		RunID:        uuid.New().String(),
		Scope:        scope,
		SuggestionID: suggestionID,
		StartedAt:    started,
		Cases:        []evolutionRegressionCase{},
	}
	addCase := func(name, status, message string) {
		run.Cases = append(run.Cases, evolutionRegressionCase{Name: name, Status: status, Message: message})
		if status == "passed" {
			run.Passed++
		} else if status == "failed" {
			run.Failed++
		}
	}

	if h.agentStore == nil {
		addCase("agent_load", "failed", "agent store is not configured")
	} else if ag, err := h.agentStore.GetByID(r.Context(), agentID); err != nil {
		addCase("agent_load", "failed", err.Error())
	} else {
		addCase("agent_load", "passed", fmt.Sprintf("agent %s is readable", ag.AgentKey))
	}

	if items, err := h.suggestions.ListSuggestions(r.Context(), agentID, "", 100); err != nil {
		addCase("suggestions_read", "failed", err.Error())
	} else {
		addCase("suggestions_read", "passed", fmt.Sprintf("%d suggestions readable", len(items)))
		appliedThresholdsWithoutBaseline := 0
		for _, item := range items {
			if item.Status != "applied" || item.SuggestionType != store.SuggestThreshold {
				continue
			}
			var params map[string]any
			_ = json.Unmarshal(item.Parameters, &params)
			if _, ok := params["_baseline"].(map[string]any); !ok {
				appliedThresholdsWithoutBaseline++
			}
		}
		if appliedThresholdsWithoutBaseline > 0 {
			addCase("rollback_baseline", "failed", fmt.Sprintf("%d applied threshold suggestions have no rollback baseline", appliedThresholdsWithoutBaseline))
		} else {
			addCase("rollback_baseline", "passed", "applied threshold suggestions have rollback baselines")
		}
	}

	since := time.Now().AddDate(0, 0, -30)
	if _, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricFeedback, since, 10); err != nil {
		addCase("feedback_metrics_read", "failed", err.Error())
	} else {
		addCase("feedback_metrics_read", "passed", "feedback metrics query is available")
	}
	if _, err := h.metrics.AggregateToolMetrics(r.Context(), agentID, since); err != nil {
		addCase("tool_metrics_aggregate", "failed", err.Error())
	} else {
		addCase("tool_metrics_aggregate", "passed", "tool metrics aggregation is available")
	}
	if _, err := h.metrics.AggregateRetrievalMetrics(r.Context(), agentID, since); err != nil {
		addCase("retrieval_metrics_aggregate", "failed", err.Error())
	} else {
		addCase("retrieval_metrics_aggregate", "passed", "retrieval metrics aggregation is available")
	}

	run.Total = len(run.Cases)
	run.Status = "passed"
	if run.Failed > 0 {
		run.Status = "failed"
	}
	run.CompletedAt = time.Now().UTC()
	return run
}

func (h *EvolutionHandler) handleListAudit(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.resolveAgentID(w, r)
	if !ok {
		return
	}
	limit := boundedLimit(r.URL.Query().Get("limit"), 50, 200)
	since := time.Now().AddDate(0, 0, -90)
	metrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricAudit, since, limit)
	if err != nil {
		slog.Warn("evolution.audit.list_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	events := make([]evolutionAuditEvent, 0, len(metrics))
	for _, metric := range metrics {
		var event evolutionAuditEvent
		if err := json.Unmarshal(metric.Value, &event); err != nil {
			continue
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = metric.CreatedAt
		}
		events = append(events, event)
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *EvolutionHandler) recordAuditEvent(r *http.Request, agentID uuid.UUID, action, suggestionID, status, result, message string) {
	if h == nil || h.metrics == nil {
		return
	}
	actor := store.ActorIDFromContext(r.Context())
	value, _ := json.Marshal(evolutionAuditEvent{
		Action:       action,
		Actor:        actor,
		SuggestionID: suggestionID,
		Status:       status,
		Result:       result,
		Message:      message,
		CreatedAt:    time.Now().UTC(),
	})
	if err := h.metrics.RecordMetric(r.Context(), store.EvolutionMetric{
		ID:         uuid.New(),
		AgentID:    agentID,
		SessionKey: "evolution-center",
		MetricType: store.MetricAudit,
		MetricKey:  action,
		Value:      value,
	}); err != nil {
		slog.Warn("evolution.audit.record_failed", "action", action, "error", err)
	}
}

func (h *EvolutionHandler) rollbackSuggestion(r *http.Request, sg store.EvolutionSuggestion, reviewedBy string) error {
	if sg.Status == "applied" && sg.SuggestionType == store.SuggestThreshold {
		if h.agentStore == nil {
			return fmt.Errorf("threshold rollback is not available")
		}
		if err := agent.RollbackSuggestion(r.Context(), h.agentStore, h.suggestions, sg); err != nil {
			return err
		}
		if reviewedBy != "" && reviewedBy != "auto-adapt" {
			return h.suggestions.UpdateSuggestionStatus(r.Context(), sg.ID, "rolled_back", reviewedBy)
		}
		return nil
	}
	return h.suggestions.UpdateSuggestionStatus(r.Context(), sg.ID, "rolled_back", reviewedBy)
}

func boundedLimit(raw string, fallback, max int) int {
	limit, _ := strconv.Atoi(raw)
	if limit <= 0 {
		limit = fallback
	}
	if limit > max {
		limit = max
	}
	return limit
}
