//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteLocalKnowledgeSourceStore implements store.LocalKnowledgeSourceStore.
type SQLiteLocalKnowledgeSourceStore struct {
	db *sql.DB
}

func NewSQLiteLocalKnowledgeSourceStore(db *sql.DB) *SQLiteLocalKnowledgeSourceStore {
	return &SQLiteLocalKnowledgeSourceStore{db: db}
}

func (s *SQLiteLocalKnowledgeSourceStore) ListSources(ctx context.Context) ([]store.LocalKnowledgeSourceData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("local knowledge list: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, source_key, name, description, path_windows, path_container,
		        tenant_scope, sync_mode, index_target, enabled, last_sync_at, last_success_at,
		        last_error, file_count, record_count, content_hash, metadata, created_at, updated_at
		   FROM local_knowledge_sources
		  WHERE tenant_id = ?
		  ORDER BY source_key`, tid.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []store.LocalKnowledgeSourceData
	for rows.Next() {
		source, err := scanLocalKnowledgeSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *source)
	}
	return result, rows.Err()
}

func (s *SQLiteLocalKnowledgeSourceStore) GetSource(ctx context.Context, sourceKey string) (*store.LocalKnowledgeSourceData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("local knowledge get: %w", err)
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, source_key, name, description, path_windows, path_container,
		        tenant_scope, sync_mode, index_target, enabled, last_sync_at, last_success_at,
		        last_error, file_count, record_count, content_hash, metadata, created_at, updated_at
		   FROM local_knowledge_sources
		  WHERE tenant_id = ? AND source_key = ?`, tid.String(), sourceKey)
	source, err := scanLocalKnowledgeSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	return source, err
}

func (s *SQLiteLocalKnowledgeSourceStore) UpsertSource(ctx context.Context, source store.LocalKnowledgeSourceSeed) (*store.LocalKnowledgeSourceData, error) {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("local knowledge upsert: %w", err)
	}
	if len(source.Metadata) == 0 {
		source.Metadata = []byte(`{}`)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := store.GenNewID()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO local_knowledge_sources (
		     id, tenant_id, source_key, name, description, path_windows, path_container,
		     tenant_scope, sync_mode, index_target, enabled, metadata, created_at, updated_at
		 ) VALUES (
		     ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		 )
		 ON CONFLICT (tenant_id, source_key) DO UPDATE SET
		     name = excluded.name,
		     description = excluded.description,
		     path_windows = excluded.path_windows,
		     path_container = excluded.path_container,
		     tenant_scope = excluded.tenant_scope,
		     sync_mode = excluded.sync_mode,
		     index_target = excluded.index_target,
		     enabled = excluded.enabled,
		     metadata = excluded.metadata,
		     updated_at = excluded.updated_at`,
		id.String(), tid.String(), source.SourceKey, source.Name, source.Description,
		source.PathWindows, source.PathContainer, source.TenantScope, source.SyncMode,
		source.IndexTarget, source.Enabled, string(source.Metadata), now, now,
	)
	if err != nil {
		return nil, err
	}
	return s.GetSource(ctx, source.SourceKey)
}

func (s *SQLiteLocalKnowledgeSourceStore) UpdateSourceStatus(ctx context.Context, sourceKey string, status store.LocalKnowledgeSourceStatus) error {
	tid, err := requireTenantID(ctx)
	if err != nil {
		return fmt.Errorf("local knowledge update status: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE local_knowledge_sources
		    SET last_sync_at = ?,
		        last_success_at = ?,
		        last_error = ?,
		        file_count = ?,
		        record_count = ?,
		        content_hash = ?,
		        updated_at = ?
		  WHERE tenant_id = ? AND source_key = ?`,
		nullTimeString(status.LastSyncAt), nullTimeString(status.LastSuccessAt),
		nullStringValue(status.LastError), status.FileCount, status.RecordCount,
		status.ContentHash, time.Now().UTC().Format(time.RFC3339Nano), tid.String(), sourceKey)
	return err
}

type localKnowledgeScanner interface {
	Scan(dest ...any) error
}

func scanLocalKnowledgeSource(scanner localKnowledgeScanner) (*store.LocalKnowledgeSourceData, error) {
	var d store.LocalKnowledgeSourceData
	var idStr, tenantIDStr string
	var metadata []byte
	var createdAt, updatedAt sqliteTime
	var lastSyncAt, lastSuccessAt nullSqliteTime
	if err := scanner.Scan(
		&idStr, &tenantIDStr, &d.SourceKey, &d.Name, &d.Description,
		&d.PathWindows, &d.PathContainer, &d.TenantScope, &d.SyncMode,
		&d.IndexTarget, &d.Enabled, &lastSyncAt, &lastSuccessAt, &d.LastError,
		&d.FileCount, &d.RecordCount, &d.ContentHash, &metadata, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	d.ID, _ = uuid.Parse(idStr)
	d.TenantID, _ = uuid.Parse(tenantIDStr)
	d.LastSyncAt = lastSyncAt.NullTime()
	d.LastSuccessAt = lastSuccessAt.NullTime()
	d.Metadata = metadata
	d.CreatedAt = createdAt.Time
	d.UpdatedAt = updatedAt.Time
	return &d, nil
}

func nullTimeString(v sql.NullTime) any {
	if !v.Valid {
		return nil
	}
	return v.Time.UTC().Format(time.RFC3339Nano)
}

func nullStringValue(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

var _ store.LocalKnowledgeSourceStore = (*SQLiteLocalKnowledgeSourceStore)(nil)
