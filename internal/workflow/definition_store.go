package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// ErrDefinitionNotFound is returned when a workflow definition cannot be found
// (within the caller's tenant/user scope).
var ErrDefinitionNotFound = errors.New("workflow definition not found")

// GraphDefinition is a user-authored, DB-backed workflow built on the sim-style
// serialized graph. It is distinct from the legacy in-code Definition (which
// drives the deterministic business runners); both coexist during rollout.
type GraphDefinition struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	UserID      string      `json:"user_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Graph       graph.Graph `json:"graph"`
	Version     int         `json:"version"`
	Active      bool        `json:"active"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// scopeIDs resolves the tenant (defaulting to master) and user from context.
func scopeIDs(ctx context.Context) (uuid.UUID, string) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	return tenantID, store.UserIDFromContext(ctx)
}

// SaveDefinition upserts a graph definition within the caller's tenant/user
// scope. The graph is validated before persistence. ID is assigned when empty.
func (e *Engine) SaveDefinition(ctx context.Context, def *GraphDefinition) error {
	if e.db == nil {
		return errors.New("workflow: no database configured")
	}
	if err := def.Graph.Validate(); err != nil {
		return err
	}
	tenantID, userID := scopeIDs(ctx)
	now := time.Now().UTC()
	if def.ID == "" {
		def.ID = uuid.NewString()
		def.CreatedAt = now
	}
	if def.Version == 0 {
		def.Version = 1
	}
	def.TenantID = tenantID.String()
	def.UserID = userID
	def.UpdatedAt = now

	graphJSON, err := json.Marshal(def.Graph)
	if err != nil {
		return err
	}
	_, err = e.db.ExecContext(ctx, `
		INSERT INTO workflow_definitions
			(id, tenant_id, user_id, name, description, graph_json, version, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name, description=EXCLUDED.description, graph_json=EXCLUDED.graph_json,
			version=EXCLUDED.version, active=EXCLUDED.active, updated_at=EXCLUDED.updated_at
	`, def.ID, tenantID, userID, def.Name, def.Description, graphJSON, def.Version, def.Active, def.CreatedAt, def.UpdatedAt)
	return err
}

// GetDefinition loads a graph definition by id within the caller's scope.
func (e *Engine) GetDefinition(ctx context.Context, id string) (*GraphDefinition, error) {
	if e.db == nil {
		return nil, errors.New("workflow: no database configured")
	}
	tenantID, _ := scopeIDs(ctx)
	def, err := scanDefinition(e.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, user_id, name, description, graph_json, version, active, created_at, updated_at
		FROM workflow_definitions
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDefinitionNotFound
	}
	return def, err
}

// ListDefinitions returns all graph definitions in the caller's tenant (shared).
func (e *Engine) ListDefinitions(ctx context.Context) ([]GraphDefinition, error) {
	if e.db == nil {
		return nil, errors.New("workflow: no database configured")
	}
	tenantID, _ := scopeIDs(ctx)
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, tenant_id, user_id, name, description, graph_json, version, active, created_at, updated_at
		FROM workflow_definitions
		WHERE tenant_id = $1
		ORDER BY updated_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GraphDefinition, 0)
	for rows.Next() {
		def, err := scanDefinition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *def)
	}
	return out, rows.Err()
}

// DeleteDefinition removes a graph definition within the caller's scope.
func (e *Engine) DeleteDefinition(ctx context.Context, id string) error {
	if e.db == nil {
		return errors.New("workflow: no database configured")
	}
	tenantID, userID := scopeIDs(ctx)
	res, err := e.db.ExecContext(ctx, `
		DELETE FROM workflow_definitions
		WHERE id = $1 AND tenant_id = $2 AND user_id = $3
	`, id, tenantID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDefinitionNotFound
	}
	return nil
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanDefinition(s rowScanner) (*GraphDefinition, error) {
	var (
		def       GraphDefinition
		graphJSON []byte
	)
	if err := s.Scan(&def.ID, &def.TenantID, &def.UserID, &def.Name, &def.Description,
		&graphJSON, &def.Version, &def.Active, &def.CreatedAt, &def.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(graphJSON, &def.Graph); err != nil {
		return nil, err
	}
	return &def, nil
}
