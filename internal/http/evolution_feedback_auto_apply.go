package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type feedbackSuggestionParams struct {
	FeedbackID     string `json:"feedback_id"`
	FeedbackType   string `json:"feedback_type"`
	SessionKey     string `json:"session_key"`
	MessageRef     string `json:"message_ref"`
	MessageContent string `json:"message_content"`
	Correction     string `json:"correction"`
	UserID         string `json:"user_id"`
}

func (h *EvolutionHandler) autoApplyFeedbackCorrections(r *http.Request, agentID uuid.UUID) error {
	if h.skillStore == nil || h.skillLoader == nil || h.agentStore == nil || h.suggestions == nil || h.metrics == nil {
		return fmt.Errorf("feedback auto-apply is not fully configured")
	}

	pending, err := h.suggestions.ListSuggestions(r.Context(), agentID, "pending", 200)
	if err != nil {
		return err
	}
	var correctionSuggestions []store.EvolutionSuggestion
	for _, sg := range pending {
		if sg.SuggestionType == store.SuggestFeedbackCorrection {
			correctionSuggestions = append(correctionSuggestions, sg)
		}
	}
	if len(correctionSuggestions) == 0 {
		return nil
	}

	preflight := h.executeRegressionRun(r, agentID, "business_workflow_smoke", "")
	if err := h.recordRegressionRun(r, agentID, preflight); err != nil {
		return fmt.Errorf("record feedback auto-apply preflight: %w", err)
	}
	if preflight.Status != "passed" {
		h.recordAuditEvent(r, agentID, "feedback_auto_apply_blocked", "", "blocked", "failed", "business_workflow_smoke preflight failed")
		return nil
	}

	draft, slug, err := h.buildFeedbackMemorySkillDraft(r, agentID)
	if err != nil {
		return err
	}
	if draft == "" {
		return nil
	}

	violations, safe := skills.GuardSkillContent(draft)
	if !safe {
		return fmt.Errorf("feedback memory skill failed security scan: %s", skills.FormatGuardViolations(violations))
	}

	anchor := correctionSuggestions[0]
	params := anchor.Parameters
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	p["skill_draft"] = draft
	p["auto_apply"] = true
	p["auto_skill_slug"] = slug
	updatedParams, _ := json.Marshal(p)
	_ = h.suggestions.UpdateSuggestionParameters(r.Context(), anchor.ID, updatedParams)
	anchor.Parameters = updatedParams

	result, err := h.applySkillDraft(r.Context(), anchor, draft, "auto-feedback")
	if err != nil {
		h.recordAuditEvent(r, agentID, "feedback_auto_apply_failed", anchor.ID.String(), "pending", "failed", err.Error())
		return err
	}
	if action, err := h.postflightSkillAddApplication(r, agentID, anchor, result, "auto-feedback"); err != nil {
		h.recordAuditEvent(r, agentID, "feedback_auto_apply_failed", anchor.ID.String(), "pending", "failed", err.Error())
		return fmt.Errorf("%s: %w", action, err)
	}

	applied := 0
	for _, sg := range correctionSuggestions {
		if sg.ID == anchor.ID {
			applied++
			continue
		}
		if err := h.suggestions.UpdateSuggestionStatus(r.Context(), sg.ID, "applied", "auto-feedback"); err != nil {
			slog.Warn("evolution.feedback.auto_apply_status_failed", "suggestion", sg.ID, "error", err)
			continue
		}
		applied++
	}
	h.recordAuditEvent(r, agentID, "feedback_auto_applied", anchor.ID.String(), "applied", "ok",
		fmt.Sprintf("%d feedback corrections consolidated into %s", applied, slug))
	return nil
}

func (h *EvolutionHandler) buildFeedbackMemorySkillDraft(r *http.Request, agentID uuid.UUID) (string, string, error) {
	ag, err := h.agentStore.GetByID(r.Context(), agentID)
	if err != nil {
		return "", "", fmt.Errorf("load agent for feedback skill: %w", err)
	}
	agentKey := strings.TrimSpace(ag.AgentKey)
	if agentKey == "" {
		agentKey = agentID.String()
	}
	slug := "auto-feedback-" + skills.Slugify(agentKey)

	since := time.Now().AddDate(0, 0, -90)
	metrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricFeedback, since, 200)
	if err != nil {
		return "", "", err
	}

	items := make([]agent.FeedbackInsight, 0, len(metrics))
	for _, metric := range metrics {
		if metric.MetricKey == "useful" {
			continue
		}
		var value evolutionFeedbackValue
		if err := json.Unmarshal(metric.Value, &value); err != nil {
			continue
		}
		items = append(items, agent.FeedbackInsight{
			CreatedAt:      metric.CreatedAt,
			FeedbackType:   value.FeedbackType,
			SessionKey:     metric.SessionKey,
			MessageRef:     value.MessageRef,
			MessageContent: value.MessageContent,
			Correction:     value.Correction,
			UserID:         value.UserID,
		})
	}
	if len(items) == 0 {
		return "", slug, nil
	}

	return agent.GenerateFeedbackSkillDraft(agentKey, ag.DisplayName, slug, items), slug, nil
}
