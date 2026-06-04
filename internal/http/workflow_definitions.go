package http

import (
	"errors"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/workflow"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// runDefinitionRequest is the body for triggering a graph run.
type runDefinitionRequest struct {
	Input map[string]any `json:"input"`
}

func (h *WorkflowHandler) handleListDefinitions(w http.ResponseWriter, r *http.Request) {
	defs, err := h.engine.ListDefinitions(r.Context())
	if err != nil {
		http.Error(w, `{"error":"list workflow definitions failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"definitions": defs})
}

func (h *WorkflowHandler) handleGetDefinition(w http.ResponseWriter, r *http.Request) {
	def, err := h.engine.GetDefinition(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, workflow.ErrDefinitionNotFound) {
			http.Error(w, `{"error":"workflow definition not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"get workflow definition failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"definition": def})
}

// handleSaveDefinition handles both POST (create) and PUT (update). On PUT the
// path id is applied to the definition so the upsert targets the right row.
func (h *WorkflowHandler) handleSaveDefinition(w http.ResponseWriter, r *http.Request) {
	var def workflow.GraphDefinition
	if !bindJSON(w, r, extractLocale(r), &def) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		def.ID = id
	}
	if err := h.engine.SaveDefinition(r.Context(), &def); err != nil {
		// Graph validation errors are client errors.
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"definition": def})
}

func (h *WorkflowHandler) handleDeleteDefinition(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.DeleteDefinition(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, workflow.ErrDefinitionNotFound) {
			http.Error(w, `{"error":"workflow definition not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"delete workflow definition failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *WorkflowHandler) handleRunDefinition(w http.ResponseWriter, r *http.Request) {
	var req runDefinitionRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	run, err := h.engine.StartGraphRun(r.Context(), r.PathValue("id"), req.Input)
	if err != nil {
		if errors.Is(err, workflow.ErrDefinitionNotFound) {
			http.Error(w, `{"error":"workflow definition not found"}`, http.StatusNotFound)
			return
		}
		// The run record is still returned (with failed status) alongside 200
		// so the client can show the failure detail; execution errors are not
		// transport errors.
		writeJSON(w, http.StatusOK, map[string]any{"run": run, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"run": run})
}

// generateRequest is the body for AI workflow generation.
type generateRequest struct {
	Prompt  string        `json:"prompt"`
	AgentID string        `json:"agent_id,omitempty"`
	Current *graphPayload `json:"current,omitempty"`
}

// graphPayload mirrors the engine's graph.Graph for request binding; we decode
// into the engine type via re-marshal to avoid importing the graph package here.
type graphPayload = map[string]any

// handleGenerate is the AI "copilot": natural language → workflow graph, via
// goclaw's own agent runtime (no external dependency).
func (h *WorkflowHandler) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}
	var current map[string]any
	if req.Current != nil {
		current = *req.Current
	}
	res, err := h.engine.GenerateGraphJSON(r.Context(), req.Prompt, req.AgentID, current)
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSeedEPL creates (or returns) the ready-to-run EPL packing-list workflow
// template for the caller, so it can be tested immediately from 流程库.
func (h *WorkflowHandler) handleSeedEPL(w http.ResponseWriter, r *http.Request) {
	def, err := h.engine.SeedEPLTemplate(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"definition": def})
}
