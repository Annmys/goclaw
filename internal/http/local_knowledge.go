package http

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// LocalKnowledgeHandler exposes the local knowledge source registry.
type LocalKnowledgeHandler struct {
	store store.LocalKnowledgeSourceStore
}

func NewLocalKnowledgeHandler(s store.LocalKnowledgeSourceStore) *LocalKnowledgeHandler {
	return &LocalKnowledgeHandler{store: s}
}

func (h *LocalKnowledgeHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/local-knowledge/sources", requireAuth("", h.handleList))
	mux.HandleFunc("GET /v1/local-knowledge/sources/{sourceKey}", requireAuth("", h.handleGet))
}

func (h *LocalKnowledgeHandler) handleList(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.ListSources(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}

func (h *LocalKnowledgeHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	sourceKey := r.PathValue("sourceKey")
	source, err := h.store.GetSource(r.Context(), sourceKey)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "local knowledge source not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, source)
}
