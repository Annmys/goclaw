-- Migration 000060: deterministic workflow run and message history.
CREATE TABLE IF NOT EXISTS workflow_runs (
    id          TEXT PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT '',
    run_json    JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_runs_tenant_user
    ON workflow_runs (tenant_id, user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS workflow_messages (
    id          TEXT PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content     TEXT NOT NULL DEFAULT '',
    files       JSONB NOT NULL DEFAULT '[]',
    run_id      TEXT REFERENCES workflow_runs(id) ON DELETE SET NULL,
    kind        TEXT NOT NULL DEFAULT 'chat' CHECK (kind IN ('chat', 'workflow')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_messages_tenant_user
    ON workflow_messages (tenant_id, user_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_workflow_messages_run
    ON workflow_messages (run_id);
