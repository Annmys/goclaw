package http

import (
	"errors"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/workflow"
)

type WorkflowHandler struct {
	engine *workflow.Engine
}

func NewWorkflowHandler(engine *workflow.Engine) *WorkflowHandler {
	return &WorkflowHandler{engine: engine}
}

func (h *WorkflowHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/workflows", requireAuth("", h.handleList))
	mux.HandleFunc("POST /v1/workflows/match", requireAuth(permissions.RoleOperator, h.handleMatch))
	mux.HandleFunc("GET /v1/workflow-runs", requireAuth("", h.handleListRuns))
	mux.HandleFunc("GET /v1/workflow-messages", requireAuth("", h.handleListMessages))
	mux.HandleFunc("POST /v1/workflow-messages", requireAuth(permissions.RoleOperator, h.handleAppendMessage))
	mux.HandleFunc("POST /v1/workflow-runs", requireAuth(permissions.RoleOperator, h.handleStartRun))
	mux.HandleFunc("GET /v1/workflow-runs/{id}", requireAuth("", h.handleGetRun))
	mux.HandleFunc("POST /v1/workflow-runs/{id}/resume", requireAuth(permissions.RoleOperator, h.handleResumeRun))
	mux.HandleFunc("POST /v1/workflow-feedback", requireAuth(permissions.RoleOperator, h.handleFeedback))

	// sim-style graph definitions (CRUD) + graph run trigger. Additive: the
	// legacy routes above are unchanged.
	mux.HandleFunc("GET /v1/workflow-definitions", requireAuth("", h.handleListDefinitions))
	mux.HandleFunc("POST /v1/workflow-definitions", requireAuth(permissions.RoleOperator, h.handleSaveDefinition))
	mux.HandleFunc("GET /v1/workflow-definitions/{id}", requireAuth("", h.handleGetDefinition))
	mux.HandleFunc("PUT /v1/workflow-definitions/{id}", requireAuth(permissions.RoleOperator, h.handleSaveDefinition))
	mux.HandleFunc("DELETE /v1/workflow-definitions/{id}", requireAuth(permissions.RoleOperator, h.handleDeleteDefinition))
	mux.HandleFunc("POST /v1/workflow-definitions/{id}/run", requireAuth(permissions.RoleOperator, h.handleRunDefinition))
	mux.HandleFunc("POST /v1/workflow-definitions/{id}/run/stream", requireAuth(permissions.RoleOperator, h.handleRunDefinitionSSE))
	mux.HandleFunc("POST /v1/workflow-generate", requireAuth(permissions.RoleOperator, h.handleGenerate))
	mux.HandleFunc("POST /v1/workflow-templates/epl", requireAuth(permissions.RoleOperator, h.handleSeedEPL))
}

func (h *WorkflowHandler) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"workflows": h.engine.Definitions(r.Context()),
	})
}

func (h *WorkflowHandler) handleMatch(w http.ResponseWriter, r *http.Request) {
	var req workflow.MatchRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	writeJSON(w, http.StatusOK, h.engine.Match(r.Context(), req))
}

func (h *WorkflowHandler) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var req workflow.StartRunRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	req.TenantID = store.TenantIDFromContext(r.Context()).String()
	req.UserID = store.UserIDFromContext(r.Context())

	run, err := h.engine.StartRun(r.Context(), req)
	if err != nil {
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			http.Error(w, `{"error":"workflow not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"start workflow run failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"run": run})
}

func (h *WorkflowHandler) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.engine.GetRun(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			http.Error(w, `{"error":"workflow run not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"get workflow run failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (h *WorkflowHandler) handleListRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"runs": h.engine.ListRuns(r.Context()),
	})
}

func (h *WorkflowHandler) handleListMessages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"messages": h.engine.ListMessages(r.Context()),
	})
}

func (h *WorkflowHandler) handleAppendMessage(w http.ResponseWriter, r *http.Request) {
	var req workflow.AppendMessageRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	if req.Content == "" {
		http.Error(w, `{"error":"workflow message requires content"}`, http.StatusBadRequest)
		return
	}
	msg := h.engine.AppendMessage(r.Context(), req)
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg})
}

func (h *WorkflowHandler) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	var req workflow.ResumeRunRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}

	run, err := h.engine.ResumeRun(r.Context(), r.PathValue("id"), req)
	if err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			http.Error(w, `{"error":"workflow run not found"}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, workflow.ErrWorkflowNotFound) {
			http.Error(w, `{"error":"workflow not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"resume workflow run failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}

func (h *WorkflowHandler) handleFeedback(w http.ResponseWriter, r *http.Request) {
	var req workflow.FeedbackRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	if req.RunID == "" || req.Message == "" {
		http.Error(w, `{"error":"workflow feedback requires run_id and message"}`, http.StatusBadRequest)
		return
	}
	if err := h.engine.SubmitFeedback(r.Context(), req); err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			http.Error(w, `{"error":"workflow run not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"submit workflow feedback failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
