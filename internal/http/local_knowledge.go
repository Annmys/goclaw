package http

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	_ "modernc.org/sqlite"
)

// LocalKnowledgeHandler exposes the local knowledge source registry.
type LocalKnowledgeHandler struct {
	store  store.LocalKnowledgeSourceStore
	syncer *LocalKnowledgeSyncer
}

func NewLocalKnowledgeHandler(s store.LocalKnowledgeSourceStore) *LocalKnowledgeHandler {
	return NewLocalKnowledgeHandlerWithSyncer(s, NewLocalKnowledgeSyncer(s))
}

func NewLocalKnowledgeHandlerWithSyncer(s store.LocalKnowledgeSourceStore, syncer *LocalKnowledgeSyncer) *LocalKnowledgeHandler {
	if syncer == nil {
		syncer = NewLocalKnowledgeSyncer(s)
	}
	return &LocalKnowledgeHandler{store: s, syncer: syncer}
}

func (h *LocalKnowledgeHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/local-knowledge/sources", requireAuth("", h.handleList))
	mux.HandleFunc("POST /v1/local-knowledge/sources/sync", requireAuth(permissions.RoleAdmin, h.handleSyncAll))
	mux.HandleFunc("GET /v1/local-knowledge/sources/{sourceKey}", requireAuth("", h.handleGet))
	mux.HandleFunc("POST /v1/local-knowledge/sources/{sourceKey}/sync", requireAuth(permissions.RoleAdmin, h.handleSyncOne))
}

func (h *LocalKnowledgeHandler) handleList(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store.ListSources(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": toLocalKnowledgeSourceResponses(sources)})
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
	writeJSON(w, http.StatusOK, toLocalKnowledgeSourceResponse(*source))
}

type localKnowledgeSourceResponse struct {
	ID            string  `json:"id"`
	TenantID      string  `json:"tenant_id"`
	SourceKey     string  `json:"source_key"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	PathWindows   string  `json:"path_windows"`
	PathContainer string  `json:"path_container"`
	TenantScope   string  `json:"tenant_scope"`
	SyncMode      string  `json:"sync_mode"`
	IndexTarget   string  `json:"index_target"`
	Enabled       bool    `json:"enabled"`
	LastSyncAt    *string `json:"last_sync_at"`
	LastSuccessAt *string `json:"last_success_at"`
	LastError     *string `json:"last_error"`
	FileCount     int64   `json:"file_count"`
	RecordCount   int64   `json:"record_count"`
	ContentHash   string  `json:"content_hash"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func toLocalKnowledgeSourceResponses(sources []store.LocalKnowledgeSourceData) []localKnowledgeSourceResponse {
	out := make([]localKnowledgeSourceResponse, len(sources))
	for i := range sources {
		out[i] = toLocalKnowledgeSourceResponse(sources[i])
	}
	return out
}

func toLocalKnowledgeSourceResponse(source store.LocalKnowledgeSourceData) localKnowledgeSourceResponse {
	return localKnowledgeSourceResponse{
		ID:            source.ID.String(),
		TenantID:      source.TenantID.String(),
		SourceKey:     source.SourceKey,
		Name:          source.Name,
		Description:   source.Description,
		PathWindows:   source.PathWindows,
		PathContainer: source.PathContainer,
		TenantScope:   source.TenantScope,
		SyncMode:      source.SyncMode,
		IndexTarget:   source.IndexTarget,
		Enabled:       source.Enabled,
		LastSyncAt:    nullTimeRFC3339(source.LastSyncAt),
		LastSuccessAt: nullTimeRFC3339(source.LastSuccessAt),
		LastError:     nullStringPtr(source.LastError),
		FileCount:     source.FileCount,
		RecordCount:   source.RecordCount,
		ContentHash:   source.ContentHash,
		CreatedAt:     source.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:     source.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func nullTimeRFC3339(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func (h *LocalKnowledgeHandler) handleSyncOne(w http.ResponseWriter, r *http.Request) {
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

	result := h.syncer.SyncSource(r.Context(), source)
	writeJSON(w, statusForSyncResults([]LocalKnowledgeSyncResult{result}), result)
}

func (h *LocalKnowledgeHandler) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	results, err := h.syncer.SyncAll(r.Context(), func(source store.LocalKnowledgeSourceData) bool {
		return source.Enabled
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, statusForSyncResults(results), map[string]any{"results": results})
}

// LocalKnowledgeSyncResult is returned by manual sync APIs and used by the
// background sync loop for structured logging.
type LocalKnowledgeSyncResult struct {
	SourceKey     string `json:"source_key"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	FileCount     int64  `json:"file_count"`
	RecordCount   int64  `json:"record_count"`
	ContentHash   string `json:"content_hash"`
	LastSyncAt    string `json:"last_sync_at"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	Error         string `json:"error,omitempty"`
}

// LocalKnowledgeSyncer scans local knowledge source paths and writes runtime
// status back to the registry. It deliberately does not import content into
// Vault, Memory, or Knowledge Graph.
type LocalKnowledgeSyncer struct {
	store store.LocalKnowledgeSourceStore
}

func NewLocalKnowledgeSyncer(s store.LocalKnowledgeSourceStore) *LocalKnowledgeSyncer {
	return &LocalKnowledgeSyncer{store: s}
}

func (s *LocalKnowledgeSyncer) SyncByKey(ctx context.Context, sourceKey string) (LocalKnowledgeSyncResult, error) {
	source, err := s.store.GetSource(ctx, sourceKey)
	if err != nil {
		return LocalKnowledgeSyncResult{}, err
	}
	return s.SyncSource(ctx, source), nil
}

func (s *LocalKnowledgeSyncer) SyncScheduled(ctx context.Context) ([]LocalKnowledgeSyncResult, error) {
	return s.SyncAll(ctx, func(source store.LocalKnowledgeSourceData) bool {
		return source.Enabled && source.SyncMode == "scheduled"
	})
}

func (s *LocalKnowledgeSyncer) SyncAll(ctx context.Context, filter func(store.LocalKnowledgeSourceData) bool) ([]LocalKnowledgeSyncResult, error) {
	sources, err := s.store.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]LocalKnowledgeSyncResult, 0, len(sources))
	for i := range sources {
		if filter != nil && !filter(sources[i]) {
			continue
		}
		src := sources[i]
		results = append(results, s.SyncSource(ctx, &src))
	}
	return results, nil
}

type localKnowledgeScanStats struct {
	Path        string
	FileCount   int64
	RecordCount int64
	ContentHash string
}

func (s *LocalKnowledgeSyncer) SyncSource(ctx context.Context, source *store.LocalKnowledgeSourceData) LocalKnowledgeSyncResult {
	now := time.Now().UTC()
	result := LocalKnowledgeSyncResult{
		SourceKey:  source.SourceKey,
		Name:       source.Name,
		LastSyncAt: now.Format(time.RFC3339Nano),
	}

	stats, scanErr := scanLocalKnowledgeSource(source)
	status := store.LocalKnowledgeSourceStatus{
		LastSyncAt: sql.NullTime{Time: now, Valid: true},
		// Preserve the previous successful snapshot when the current scan fails.
		LastSuccessAt: source.LastSuccessAt,
		FileCount:     source.FileCount,
		RecordCount:   source.RecordCount,
		ContentHash:   source.ContentHash,
	}
	if scanErr != nil {
		result.Error = scanErr.Error()
		status.LastError = sql.NullString{String: result.Error, Valid: true}
	} else {
		result.Path = stats.Path
		result.FileCount = stats.FileCount
		result.RecordCount = stats.RecordCount
		result.ContentHash = stats.ContentHash
		result.LastSuccessAt = now.Format(time.RFC3339Nano)
		status.LastSuccessAt = sql.NullTime{Time: now, Valid: true}
		status.FileCount = stats.FileCount
		status.RecordCount = stats.RecordCount
		status.ContentHash = stats.ContentHash
	}

	if err := s.store.UpdateSourceStatus(ctx, source.SourceKey, status); err != nil {
		if result.Error != "" {
			result.Error = result.Error + "; status update failed: " + err.Error()
		} else {
			result.Error = "status update failed: " + err.Error()
		}
	}
	return result
}

func statusForSyncResults(results []LocalKnowledgeSyncResult) int {
	for _, r := range results {
		if r.Error != "" {
			return http.StatusMultiStatus
		}
	}
	return http.StatusOK
}

func scanLocalKnowledgeSource(source *store.LocalKnowledgeSourceData) (localKnowledgeScanStats, error) {
	root, err := resolveLocalKnowledgePath(source)
	if err != nil {
		return localKnowledgeScanStats{}, err
	}

	var stats localKnowledgeScanStats
	stats.Path = root
	hasher := sha256.New()
	_, _ = io.WriteString(hasher, root+"\n")

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "__pycache__" {
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		stats.FileCount++
		_, _ = fmt.Fprintf(hasher, "%s|%d|%d\n", rel, info.Size(), info.ModTime().UnixNano())
		stats.RecordCount += estimateLocalKnowledgeRecords(path)
		return nil
	})
	if err != nil {
		return localKnowledgeScanStats{}, err
	}
	stats.ContentHash = hex.EncodeToString(hasher.Sum(nil))
	return stats, nil
}

func resolveLocalKnowledgePath(source *store.LocalKnowledgeSourceData) (string, error) {
	candidates := make([]string, 0, 2)
	if strings.TrimSpace(source.PathContainer) != "" {
		candidates = append(candidates, source.PathContainer)
	}
	if strings.TrimSpace(source.PathWindows) != "" {
		candidates = append(candidates, source.PathWindows)
	}
	var tried []string
	for _, p := range candidates {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		tried = append(tried, p)
		info, err := os.Stat(p)
		if err == nil && info.IsDir() {
			return p, nil
		}
		if err == nil {
			return p, nil
		}
	}
	if len(tried) == 0 {
		return "", fmt.Errorf("source path is not configured")
	}
	return "", fmt.Errorf("source path is not accessible: %s", strings.Join(tried, ", "))
}

func estimateLocalKnowledgeRecords(path string) int64 {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sqlite", ".sqlite3", ".db":
		if n, err := countSQLiteRows(path); err == nil && n > 0 {
			return n
		}
	case ".xlsx":
		if n, err := countXLSXRows(path); err == nil && n > 0 {
			return n
		}
	case ".csv", ".tsv", ".txt", ".md", ".jsonl":
		if n, err := countTextLines(path); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

func countSQLiteRows(path string) (int64, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var total int64
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return total, err
		}
		var count int64
		if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteSQLiteIdentifier(tableName))).Scan(&count); err != nil {
			return total, err
		}
		total += count
	}
	return total, rows.Err()
}

func quoteSQLiteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func countXLSXRows(path string) (int64, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()

	var total int64
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if !strings.HasPrefix(name, "xl/worksheets/") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return total, err
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return total, err
		}
		total += int64(bytes.Count(content, []byte("<row")))
	}
	return total, nil
}

func countTextLines(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	var lines int64
	for scanner.Scan() {
		lines++
	}
	return lines, scanner.Err()
}
