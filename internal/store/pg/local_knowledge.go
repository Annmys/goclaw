package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGLocalKnowledgeSourceStore implements store.LocalKnowledgeSourceStore.
type PGLocalKnowledgeSourceStore struct {
	db *sql.DB
}

func NewPGLocalKnowledgeSourceStore(db *sql.DB) *PGLocalKnowledgeSourceStore {
	return &PGLocalKnowledgeSourceStore{db: db}
}

func (s *PGLocalKnowledgeSourceStore) ListSources(ctx context.Context) ([]store.LocalKnowledgeSourceData, error) {
	tid := resolveTenantID(ctx)
	if tid == uuid.Nil {
		return nil, fmt.Errorf("local knowledge list: tenant_id required")
	}

	var result []store.LocalKnowledgeSourceData
	err := pkgSqlxDB.SelectContext(ctx, &result,
		`SELECT id, tenant_id, source_key, name, description, path_windows, path_container,
		        tenant_scope, sync_mode, index_target, enabled, last_sync_at, last_success_at,
		        last_error, file_count, record_count, content_hash, metadata, created_at, updated_at
		   FROM local_knowledge_sources
		  WHERE tenant_id = $1
		  ORDER BY source_key`, tid)
	return result, err
}

func (s *PGLocalKnowledgeSourceStore) GetSource(ctx context.Context, sourceKey string) (*store.LocalKnowledgeSourceData, error) {
	tid := resolveTenantID(ctx)
	if tid == uuid.Nil {
		return nil, fmt.Errorf("local knowledge get: tenant_id required")
	}

	var result store.LocalKnowledgeSourceData
	err := pkgSqlxDB.GetContext(ctx, &result,
		`SELECT id, tenant_id, source_key, name, description, path_windows, path_container,
		        tenant_scope, sync_mode, index_target, enabled, last_sync_at, last_success_at,
		        last_error, file_count, record_count, content_hash, metadata, created_at, updated_at
		   FROM local_knowledge_sources
		  WHERE tenant_id = $1 AND source_key = $2`, tid, sourceKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *PGLocalKnowledgeSourceStore) UpsertSource(ctx context.Context, source store.LocalKnowledgeSourceSeed) (*store.LocalKnowledgeSourceData, error) {
	tid := resolveTenantID(ctx)
	if tid == uuid.Nil {
		return nil, fmt.Errorf("local knowledge upsert: tenant_id required")
	}
	if len(source.Metadata) == 0 {
		source.Metadata = []byte(`{}`)
	}

	var result store.LocalKnowledgeSourceData
	err := pkgSqlxDB.GetContext(ctx, &result,
		`INSERT INTO local_knowledge_sources (
		     id, tenant_id, source_key, name, description, path_windows, path_container,
		     tenant_scope, sync_mode, index_target, enabled, metadata, created_at, updated_at
		 ) VALUES (
		     $1, $2, $3, $4, $5, $6, $7,
		     $8, $9, $10, $11, $12, $13, $13
		 )
		 ON CONFLICT (tenant_id, source_key) DO UPDATE SET
		     name = EXCLUDED.name,
		     description = EXCLUDED.description,
		     path_windows = EXCLUDED.path_windows,
		     path_container = EXCLUDED.path_container,
		     tenant_scope = EXCLUDED.tenant_scope,
		     sync_mode = EXCLUDED.sync_mode,
		     index_target = EXCLUDED.index_target,
		     enabled = EXCLUDED.enabled,
		     metadata = EXCLUDED.metadata,
		     updated_at = EXCLUDED.updated_at
		 RETURNING id, tenant_id, source_key, name, description, path_windows, path_container,
		           tenant_scope, sync_mode, index_target, enabled, last_sync_at, last_success_at,
		           last_error, file_count, record_count, content_hash, metadata, created_at, updated_at`,
		store.GenNewID(), tid, source.SourceKey, source.Name, source.Description,
		source.PathWindows, source.PathContainer, source.TenantScope, source.SyncMode,
		source.IndexTarget, source.Enabled, source.Metadata, time.Now(),
	)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *PGLocalKnowledgeSourceStore) UpdateSourceStatus(ctx context.Context, sourceKey string, status store.LocalKnowledgeSourceStatus) error {
	tid := resolveTenantID(ctx)
	if tid == uuid.Nil {
		return fmt.Errorf("local knowledge update status: tenant_id required")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE local_knowledge_sources
		    SET last_sync_at = $1,
		        last_success_at = $2,
		        last_error = $3,
		        file_count = $4,
		        record_count = $5,
		        content_hash = $6,
		        updated_at = $7
		  WHERE tenant_id = $8 AND source_key = $9`,
		status.LastSyncAt, status.LastSuccessAt, status.LastError, status.FileCount,
		status.RecordCount, status.ContentHash, time.Now(), tid, sourceKey,
	)
	return err
}

var _ store.LocalKnowledgeSourceStore = (*PGLocalKnowledgeSourceStore)(nil)
