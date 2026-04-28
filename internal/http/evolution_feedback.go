package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type evolutionFeedbackValue struct {
	FeedbackType     string `json:"feedback_type"`
	MessageRef       string `json:"message_ref"`
	MessageContent   string `json:"message_content,omitempty"`
	Correction       string `json:"correction,omitempty"`
	UserID           string `json:"user_id,omitempty"`
	RequiresApproval bool   `json:"requires_approval"`
	Status           string `json:"status"`
}

type evolutionFeedbackResponse struct {
	ID         uuid.UUID              `json:"id"`
	TenantID   uuid.UUID              `json:"tenant_id"`
	AgentID    uuid.UUID              `json:"agent_id"`
	SessionKey string                 `json:"session_key"`
	MetricType store.MetricType       `json:"metric_type"`
	MetricKey  string                 `json:"metric_key"`
	Value      evolutionFeedbackValue `json:"value"`
	CreatedAt  time.Time              `json:"created_at"`
}

func (h *EvolutionHandler) handleCreateFeedback(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	agentID, err := uuid.Parse(r.PathValue("agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	var body struct {
		FeedbackType   string `json:"feedback_type"`
		SessionKey     string `json:"session_key"`
		MessageRef     string `json:"message_ref"`
		MessageContent string `json:"message_content"`
		Correction     string `json:"correction"`
	}
	if !bindJSON(w, r, locale, &body) {
		return
	}

	body.FeedbackType = strings.TrimSpace(body.FeedbackType)
	if body.FeedbackType != "useful" && body.FeedbackType != "not_useful" && body.FeedbackType != "correction" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feedback_type must be useful, not_useful, or correction"})
		return
	}
	body.SessionKey = strings.TrimSpace(body.SessionKey)
	body.MessageRef = strings.TrimSpace(body.MessageRef)
	body.MessageContent = strings.TrimSpace(body.MessageContent)
	body.Correction = strings.TrimSpace(body.Correction)
	if body.SessionKey == "" || body.MessageRef == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_key and message_ref are required"})
		return
	}
	if body.FeedbackType == "correction" && body.Correction == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "correction is required for correction feedback"})
		return
	}

	userID := store.UserIDFromContext(r.Context())
	value, _ := json.Marshal(evolutionFeedbackValue{
		FeedbackType:     body.FeedbackType,
		MessageRef:       body.MessageRef,
		MessageContent:   truncateString(body.MessageContent, 4000),
		Correction:       truncateString(body.Correction, 4000),
		UserID:           userID,
		RequiresApproval: body.FeedbackType != "useful",
		Status:           "pending_review",
	})

	metricID := uuid.New()
	metric := store.EvolutionMetric{
		ID:         metricID,
		AgentID:    agentID,
		SessionKey: body.SessionKey,
		MetricType: store.MetricFeedback,
		MetricKey:  body.FeedbackType,
		Value:      value,
	}
	if err := h.metrics.RecordMetric(r.Context(), metric); err != nil {
		slog.Warn("evolution.feedback.record_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if body.FeedbackType != "useful" {
		if err := h.createFeedbackSuggestion(r.Context(), agentID, metricID, body.FeedbackType, body.SessionKey, body.MessageRef, body.MessageContent, body.Correction, userID); err != nil {
			slog.Warn("evolution.feedback.suggestion_failed", "error", err)
		}
	}
	h.recordAuditEvent(r, agentID, "feedback_received", "", body.FeedbackType, "ok", "chat feedback recorded")

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":      "ok",
		"feedback_id": metricID,
	})
}

func (h *EvolutionHandler) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(r.PathValue("agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent ID"})
		return
	}

	since := time.Now().AddDate(0, 0, -30)
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	metrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricFeedback, since, limit)
	if err != nil {
		slog.Warn("evolution.feedback.list_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	items := make([]evolutionFeedbackResponse, 0, len(metrics))
	for _, metric := range metrics {
		var value evolutionFeedbackValue
		if err := json.Unmarshal(metric.Value, &value); err != nil {
			value = evolutionFeedbackValue{FeedbackType: metric.MetricKey, Status: "unreadable"}
		}
		items = append(items, evolutionFeedbackResponse{
			ID:         metric.ID,
			TenantID:   metric.TenantID,
			AgentID:    metric.AgentID,
			SessionKey: metric.SessionKey,
			MetricType: metric.MetricType,
			MetricKey:  metric.MetricKey,
			Value:      value,
			CreatedAt:  metric.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *EvolutionHandler) createFeedbackSuggestion(ctx context.Context, agentID, feedbackID uuid.UUID, feedbackType, sessionKey, messageRef, messageContent, correction, userID string) error {
	if h.suggestions == nil {
		return nil
	}
	params, _ := json.Marshal(map[string]any{
		"feedback_id":     feedbackID.String(),
		"feedback_type":   feedbackType,
		"session_key":     sessionKey,
		"message_ref":     messageRef,
		"message_content": truncateString(messageContent, 1200),
		"correction":      truncateString(correction, 1200),
		"user_id":         userID,
		"approval_policy": "admin_required",
	})
	label := "User feedback requires review"
	if feedbackType == "correction" {
		label = "User correction requires review"
	}
	rationale := "Created from chat feedback. This suggestion only enters the review queue and does not automatically modify core skills, models, tools, or source code."
	if correction != "" {
		rationale = fmt.Sprintf("%s User correction: %s", rationale, truncateString(correction, 300))
	}
	return h.suggestions.CreateSuggestion(ctx, store.EvolutionSuggestion{
		ID:             uuid.New(),
		AgentID:        agentID,
		SuggestionType: store.SuggestFeedbackCorrection,
		Suggestion:     label,
		Rationale:      rationale,
		Parameters:     params,
		Status:         "pending",
	})
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
