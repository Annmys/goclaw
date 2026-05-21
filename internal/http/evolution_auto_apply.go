package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func (h *EvolutionHandler) autoApplySuggestion(r *http.Request, agentID uuid.UUID, sg store.EvolutionSuggestion) (bool, string, error) {
	switch sg.SuggestionType {
	case store.SuggestFeedbackCorrection:
		if err := h.autoApplyFeedbackCorrections(r, agentID); err != nil {
			return false, "feedback_auto_apply_failed", err
		}
		pending, err := h.suggestions.ListSuggestions(r.Context(), agentID, "pending", 200)
		if err != nil {
			return false, "feedback_auto_apply_verify_failed", err
		}
		for _, item := range pending {
			if item.ID == sg.ID {
				return false, "feedback_auto_apply_skipped", nil
			}
		}
		return true, "feedback_auto_applied", nil

	case store.SuggestSkillAdd:
		preflight := h.executeRegressionRun(r, agentID, "core_skill_smoke", sg.ID.String())
		if err := h.recordRegressionRun(r, agentID, preflight); err != nil {
			return false, "skill_add_preflight_record_failed", err
		}
		if preflight.Status != "passed" {
			return false, "skill_add_preflight_failed", nil
		}
		if err := h.applySkillDraft(r.Context(), sg, "", "auto-evolution"); err != nil {
			return false, "skill_add_apply_failed", err
		}
		return true, "skill_add_applied", nil

	case store.SuggestThreshold:
		if h.agentStore == nil {
			return false, "threshold_agent_store_missing", nil
		}
		since := time.Now().AddDate(0, 0, -7)
		recentMetrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricRetrieval, since, 500)
		if err != nil {
			return false, "threshold_metrics_failed", err
		}
		guardrails := agent.DefaultGuardrails()
		if err := agent.CheckGuardrails(guardrails, sg, len(recentMetrics)); err != nil {
			return false, "threshold_guardrail_blocked: " + err.Error(), nil
		}
		if err := agent.ApplySuggestion(r.Context(), h.agentStore, h.suggestions, sg, guardrails); err != nil {
			return false, "threshold_apply_failed", err
		}
		return true, "threshold_applied", nil

	case store.SuggestToolOrder:
		// Tool disabling can remove capabilities and break business workflows.
		// Keep it as a reviewed suggestion even in automatic mode.
		return false, "tool_order_requires_review", nil
	case store.SuggestSkillRepair:
		// Repairing a business skill must produce a versioned patch and human review.
		return false, "skill_repair_requires_review", nil
	default:
		return false, "unsupported_suggestion_type", nil
	}
}

func (h *EvolutionHandler) autoApplyPendingSuggestions(r *http.Request, agentID uuid.UUID, limit int) {
	if h == nil || h.suggestions == nil {
		return
	}
	if limit <= 0 {
		limit = 50
	}
	items, err := h.suggestions.ListSuggestions(r.Context(), agentID, "pending", limit)
	if err != nil {
		h.recordAuditEvent(r, agentID, "auto_evolution_scan_failed", "", "failed", "failed", err.Error())
		return
	}
	for _, sg := range items {
		applied, action, err := h.autoApplySuggestion(r, agentID, sg)
		if err != nil {
			h.recordAuditEvent(r, agentID, "auto_"+action, sg.ID.String(), sg.Status, "failed", err.Error())
			continue
		}
		result := "skipped"
		status := sg.Status
		if applied {
			result = "ok"
			status = "applied"
		}
		h.recordAuditEvent(r, agentID, "auto_"+action, sg.ID.String(), status, result, "automatic evolution pipeline")
	}
}
