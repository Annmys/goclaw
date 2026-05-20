package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// LocalKnowledgeSourceData records a local folder/file source that GoClaw can
// mirror into memory, Vault, or the knowledge graph in later sync stages.
type LocalKnowledgeSourceData struct {
	ID            uuid.UUID      `json:"id" db:"id"`
	TenantID      uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	SourceKey     string         `json:"source_key" db:"source_key"`
	Name          string         `json:"name" db:"name"`
	Description   string         `json:"description" db:"description"`
	PathWindows   string         `json:"path_windows" db:"path_windows"`
	PathContainer string         `json:"path_container" db:"path_container"`
	TenantScope   string         `json:"tenant_scope" db:"tenant_scope"`
	SyncMode      string         `json:"sync_mode" db:"sync_mode"`
	IndexTarget   string         `json:"index_target" db:"index_target"`
	Enabled       bool           `json:"enabled" db:"enabled"`
	LastSyncAt    sql.NullTime   `json:"last_sync_at,omitempty" db:"last_sync_at"`
	LastSuccessAt sql.NullTime   `json:"last_success_at,omitempty" db:"last_success_at"`
	LastError     sql.NullString `json:"last_error,omitempty" db:"last_error"`
	FileCount     int64          `json:"file_count" db:"file_count"`
	RecordCount   int64          `json:"record_count" db:"record_count"`
	ContentHash   string         `json:"content_hash" db:"content_hash"`
	Metadata      []byte         `json:"metadata,omitempty" db:"metadata"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
}

// LocalKnowledgeSourceSeed is the idempotent input used by startup seeders.
type LocalKnowledgeSourceSeed struct {
	SourceKey     string
	Name          string
	Description   string
	PathWindows   string
	PathContainer string
	TenantScope   string
	SyncMode      string
	IndexTarget   string
	Enabled       bool
	Metadata      []byte
}

// LocalKnowledgeSourceStatus updates only runtime sync fields.
type LocalKnowledgeSourceStatus struct {
	LastSyncAt    sql.NullTime
	LastSuccessAt sql.NullTime
	LastError     sql.NullString
	FileCount     int64
	RecordCount   int64
	ContentHash   string
}

// LocalKnowledgeSourceStore manages the registry of local knowledge sources.
type LocalKnowledgeSourceStore interface {
	ListSources(ctx context.Context) ([]LocalKnowledgeSourceData, error)
	GetSource(ctx context.Context, sourceKey string) (*LocalKnowledgeSourceData, error)
	UpsertSource(ctx context.Context, source LocalKnowledgeSourceSeed) (*LocalKnowledgeSourceData, error)
	UpdateSourceStatus(ctx context.Context, sourceKey string, status LocalKnowledgeSourceStatus) error
}
