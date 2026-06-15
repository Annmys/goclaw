package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/workflow"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// handleRunDefinitionSSE is the streaming version of handleRunDefinition.
// It sends Server-Sent Events as each node starts/completes, so the frontend
// can show real-time progress instead of waiting for the entire run to finish.
func (h *WorkflowHandler) handleRunDefinitionSSE(w http.ResponseWriter, r *http.Request) {
	var req runDefinitionRequest
	if !bindJSON(w, r, extractLocale(r), &req) {
		return
	}

	defID := r.PathValue("id")
	if defID == "" {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, "missing definition id")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send an SSE event
	send := func(eventType string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
		flusher.Flush()
	}

	// Run the workflow with an observer that streams events
	observer := &sseObserver{send: send}
	run, err := h.engine.StartGraphRunWithObserver(r.Context(), defID, req.Input, observer)

	if err != nil {
		if errors.Is(err, workflow.ErrDefinitionNotFound) {
			send("error", map[string]string{"error": "workflow definition not found"})
		} else {
			send("error", map[string]string{"error": err.Error()})
		}
		send("done", map[string]any{"run": run})
		return
	}

	send("done", map[string]any{"run": run})
}

// sseObserver implements dag.StepObserver, streaming events to the client.
type sseObserver struct {
	send func(eventType string, data any)
}

func (o *sseObserver) OnNodeStart(nodeID, nodeType string, inputs map[string]any) {
	o.send("node_start", map[string]string{"node_id": nodeID, "node_type": nodeType})
}

func (o *sseObserver) OnNodeComplete(nodeID, nodeType string, output map[string]any) {
	// Truncate output for SSE (avoid huge payloads)
	summary := ""
	if output != nil {
		b, _ := json.Marshal(output)
		summary = string(b)
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
	}
	o.send("node_complete", map[string]any{"node_id": nodeID, "node_type": nodeType, "output": summary})
}

func (o *sseObserver) OnNodeError(nodeID, nodeType string, err error) {
	o.send("node_error", map[string]any{"node_id": nodeID, "node_type": nodeType, "error": err.Error()})
}
