package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// StartAutoApplyLoop continuously scans active agents for pending evolution
// suggestions and runs the existing guarded auto-apply pipeline.
func (h *EvolutionHandler) StartAutoApplyLoop(ctx context.Context, interval time.Duration, limit int) {
	if h == nil || h.agentStore == nil || h.suggestions == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	if limit <= 0 {
		limit = 100
	}

	go func() {
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				h.autoApplyPendingSuggestionsForActiveAgents(ctx, limit)
				timer.Reset(interval)
			}
		}
	}()
}

func (h *EvolutionHandler) autoApplyPendingSuggestionsForActiveAgents(ctx context.Context, limit int) {
	agents, err := h.agentStore.List(ctx, "")
	if err != nil {
		return
	}
	for _, ag := range agents {
		if ag.Status != store.AgentStatusActive {
			continue
		}
		flags := ag.ParseV3Flags()
		if !flags.EvolutionMetrics || !flags.EvolutionSuggest {
			continue
		}
		agentCtx := store.WithTenantID(ctx, ag.TenantID)
		agentCtx = store.WithUserID(agentCtx, "auto-evolution")
		req, err := http.NewRequestWithContext(agentCtx, http.MethodPost, "/internal/evolution/auto-apply", nil)
		if err != nil {
			continue
		}
		h.autoApplyPendingSuggestions(req, ag.ID, limit)
	}
}

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
		preflight := h.executeRegressionRun(r, agentID, "business_workflow_smoke", sg.ID.String())
		if err := h.recordRegressionRun(r, agentID, preflight); err != nil {
			return false, "skill_repair_preflight_record_failed", err
		}
		if preflight.Status != "passed" {
			return false, "skill_repair_preflight_failed", nil
		}
		if err := h.applySkillRepair(r.Context(), sg, "auto-evolution"); err != nil {
			return false, "skill_repair_apply_failed", err
		}
		postflight := h.executeRegressionRun(r, agentID, "business_workflow_smoke", sg.ID.String())
		if err := h.recordRegressionRun(r, agentID, postflight); err != nil {
			return false, "skill_repair_postflight_record_failed", err
		}
		if postflight.Status != "passed" {
			if rollbackErr := h.rollbackSkillRepair(r.Context(), sg, "auto-evolution"); rollbackErr != nil {
				return false, "skill_repair_postflight_failed_rollback_failed", rollbackErr
			}
			return false, "skill_repair_postflight_failed_rolled_back", nil
		}
		return true, "skill_repair_applied", nil
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
