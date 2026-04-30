package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/permissions"
)

// LabelPrintHandler proxies controlled label print requests to a Windows-side
// helper service. BarTender runs on the Windows host, not inside the Linux
// GoClaw container.
type LabelPrintHandler struct {
	workspace string
	helperURL string
	client    *http.Client
}

func NewLabelPrintHandler(workspace string) *LabelPrintHandler {
	helperURL := strings.TrimRight(os.Getenv("GOCLAW_LABEL_HELPER_URL"), "/")
	if helperURL == "" {
		helperURL = "http://host.docker.internal:18791"
	}
	return &LabelPrintHandler{
		workspace: filepath.Clean(workspace),
		helperURL: helperURL,
		client:    &http.Client{Timeout: 2 * time.Minute},
	}
}

func (h *LabelPrintHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/labels/helper/status", requireAuth("", h.handleStatus))
	mux.HandleFunc("POST /v1/labels/print", requireAuth(permissions.RoleOperator, h.handlePrint))
}

func (h *LabelPrintHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.helperURL+"/health", nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := h.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":         false,
			"helper_url": h.helperURL,
			"error":      err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         resp.StatusCode >= 200 && resp.StatusCode < 300,
		"helper_url": h.helperURL,
		"status":     resp.StatusCode,
		"body":       string(body),
	})
}

func (h *LabelPrintHandler) handlePrint(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PreviewPath string `json:"preview_path"`
		Copies      int    `json:"copies"`
		PrinterName string `json:"printer_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if in.Copies < 1 {
		in.Copies = 1
	}
	if in.Copies > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "copies must be between 1 and 100"})
		return
	}

	previewPath, err := h.resolvePreviewPath(in.PreviewPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	jobDir := filepath.Dir(previewPath)
	runScript := filepath.Join(jobDir, "run_bartender.ps1")
	if _, err := os.Stat(runScript); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run_bartender.ps1 not found for this label preview"})
		return
	}

	payload := map[string]any{
		"job_dir":      jobDir,
		"preview_path": previewPath,
		"copies":       in.Copies,
		"printer_name": in.PrinterName,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.helperURL+"/print", bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":         false,
			"helper_url": h.helperURL,
			"error":      "label helper is not reachable: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":         false,
			"helper_url": h.helperURL,
			"status":     resp.StatusCode,
			"error":      string(respBody),
		})
		return
	}

	var out map[string]any
	if err := json.Unmarshal(respBody, &out); err != nil {
		out = map[string]any{"ok": true, "body": string(respBody)}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *LabelPrintHandler) resolvePreviewPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("preview_path is required")
	}
	pathPart := strings.TrimSpace(raw)
	if u, err := url.Parse(pathPart); err == nil && u.Path != "" {
		pathPart = u.Path
	}
	if strings.HasPrefix(pathPart, "/v1/files/") {
		pathPart = strings.TrimPrefix(pathPart, "/v1/files/")
		if decoded, err := url.PathUnescape(pathPart); err == nil {
			pathPart = decoded
		}
		if len(pathPart) >= 2 && pathPart[1] == ':' {
			// Windows absolute path, keep as-is.
		} else {
			pathPart = "/" + strings.TrimPrefix(pathPart, "/")
		}
	}
	cleaned := filepath.Clean(pathPart)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("preview_path must resolve to an absolute workspace path")
	}
	workspace := h.workspace
	sep := string(filepath.Separator)
	if workspace == "" || (cleaned != workspace && !strings.HasPrefix(cleaned, workspace+sep)) {
		return "", fmt.Errorf("preview_path is outside workspace")
	}
	base := strings.ToLower(filepath.Base(cleaned))
	if base != "preview.png" && base != "label_preview.png" {
		return "", fmt.Errorf("only label preview images can be printed")
	}
	if _, err := os.Stat(cleaned); err != nil {
		return "", fmt.Errorf("preview image not found")
	}
	return cleaned, nil
}
