package http

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TeamAttachmentsHandler serves team task attachment files for download.
type TeamAttachmentsHandler struct {
	teamStore store.TeamStore
	dataDir   string
}

// NewTeamAttachmentsHandler creates a new handler for serving team attachments.
func NewTeamAttachmentsHandler(teamStore store.TeamStore, dataDir string) *TeamAttachmentsHandler {
	return &TeamAttachmentsHandler{teamStore: teamStore, dataDir: dataDir}
}

// RegisterRoutes registers the attachment download endpoint.
func (h *TeamAttachmentsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/teams/{teamId}/attachments/{attachmentId}/download", h.authMiddleware(h.handleDownload))
}

func (h *TeamAttachmentsHandler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Attachments are delivered as scoped files. Require an authenticated
		// request so the same download URL cannot be reused across users.
		provided := extractBearerToken(r)
		authedReq, ok := requireAuthBearer("", provided, w, r)
		if !ok {
			return
		}
		next(w, authedReq)
	}
}

func (h *TeamAttachmentsHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)

	if h.teamStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgTeamsNotConfigured)})
		return
	}

	teamID, err := uuid.Parse(r.PathValue("teamId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid team id"})
		return
	}

	attachmentID, err := uuid.Parse(r.PathValue("attachmentId"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid attachment id"})
		return
	}

	att, err := h.teamStore.GetAttachment(r.Context(), attachmentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// IDOR check: attachment must belong to the requested team.
	if att.TeamID != teamID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "attachment does not belong to this team"})
		return
	}

	task, err := h.teamStore.GetTask(r.Context(), att.TaskID)
	if err != nil || task == nil || task.TeamID != teamID {
		http.NotFound(w, r)
		return
	}

	ft := r.URL.Query().Get("ft")
	if ft == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing file token"})
		return
	}
	claims, ok := VerifyScopedFileToken(ft, r.URL.Path, FileSigningKey())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired file token"})
		return
	}
	if claims.TeamID != teamID.String() || claims.TaskID != task.ID.String() || claims.TenantID != task.TenantID.String() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "file token scope mismatch"})
		return
	}

	userID := store.UserIDFromContext(r.Context())
	role := permissions.Role(store.RoleFromContext(r.Context()))
	if claims.UserID != "" && claims.UserID != userID && !permissions.HasMinRole(role, permissions.RoleAdmin) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "attachment is not available to this user"})
		return
	}
	if task.UserID != "" && task.UserID != userID && !permissions.HasMinRole(role, permissions.RoleAdmin) {
		if hasAccess, err := h.teamStore.HasTeamAccess(r.Context(), teamID, userID); err != nil || !hasAccess {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "attachment is not available to this user"})
			return
		}
	}

	// Resolve disk path. Absolute paths (new) are used directly;
	// relative paths (legacy) are joined with the team workspace directory.
	var cleanPath string
	if filepath.IsAbs(att.Path) {
		cleanPath = filepath.Clean(att.Path)
	} else {
		// Legacy: {workspace}/teams/{teamID}/{chatID}/{relPath}
		cleanPath = filepath.Clean(filepath.Join(h.dataDir, "teams", att.TeamID.String(), att.ChatID, att.Path))
	}
	// Security: file must be within the workspace root to prevent path traversal.
	wsRoot := filepath.Clean(h.dataDir) + string(filepath.Separator)
	if !strings.HasPrefix(cleanPath, wsRoot) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid attachment path"})
		return
	}

	// Set download headers with sanitized filename (prevent header injection).
	filename := filepath.Base(att.Path)
	safeName := strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' {
			return '_'
		}
		return r
	}, filename)
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeName+`"`)
	if att.MimeType != "" {
		w.Header().Set("Content-Type", att.MimeType)
	}

	http.ServeFile(w, r, cleanPath)
}
