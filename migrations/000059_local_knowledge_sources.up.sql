-- Migration 000059: local knowledge source registry.
--
-- This table only records local source configuration and sync status. Actual
-- file scanning/conversion remains owned by cron jobs and future sync workers.
CREATE TABLE IF NOT EXISTS local_knowledge_sources (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_key     VARCHAR(100) NOT NULL,
    name           VARCHAR(255) NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    path_windows   TEXT NOT NULL DEFAULT '',
    path_container TEXT NOT NULL DEFAULT '',
    tenant_scope   VARCHAR(20) NOT NULL DEFAULT 'tenant'
                   CHECK (tenant_scope IN ('system', 'tenant', 'shared')),
    sync_mode      VARCHAR(20) NOT NULL DEFAULT 'manual'
                   CHECK (sync_mode IN ('manual', 'scheduled', 'watch')),
    index_target   VARCHAR(20) NOT NULL DEFAULT 'registry'
                   CHECK (index_target IN ('registry', 'vault', 'memory', 'knowledge_graph', 'tool_cache')),
    enabled        BOOL NOT NULL DEFAULT TRUE,
    last_sync_at   TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_error     TEXT,
    file_count     BIGINT NOT NULL DEFAULT 0,
    record_count   BIGINT NOT NULL DEFAULT 0,
    content_hash   VARCHAR(128) NOT NULL DEFAULT '',
    metadata       JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, source_key)
);

CREATE INDEX IF NOT EXISTS idx_local_knowledge_sources_tenant
    ON local_knowledge_sources (tenant_id, enabled);
CREATE INDEX IF NOT EXISTS idx_local_knowledge_sources_target
    ON local_knowledge_sources (tenant_id, index_target);

INSERT INTO local_knowledge_sources (
    tenant_id, source_key, name, description, path_windows, path_container,
    tenant_scope, sync_mode, index_target, enabled, metadata
) VALUES
    (
        '0193a5b0-7000-7000-8000-000000000001',
        'flow-orders',
        '包装流转单',
        '包装流转单源文件和转换后的订单索引数据。',
        'D:\数据\包装流转单',
        '/mnt/target/flow-orders',
        'system',
        'scheduled',
        'tool_cache',
        TRUE,
        '{"seed":"000059","owner":"goclaw"}'
    ),
    (
        '0193a5b0-7000-7000-8000-000000000001',
        'package-weights',
        '产品包装重量表',
        '产品包装重量表 Excel 与 SQLite 查询缓存。',
        'D:\数据\产品包装重量表',
        '/mnt/source/product-package-weights',
        'system',
        'scheduled',
        'tool_cache',
        TRUE,
        '{"seed":"000059","owner":"goclaw"}'
    ),
    (
        '0193a5b0-7000-7000-8000-000000000001',
        'packaging-data',
        '包装资料',
        '包装计算使用的新包装资料数据源。',
        'D:\数据\包装资料',
        '/mnt/package-materials',
        'system',
        'scheduled',
        'tool_cache',
        TRUE,
        '{"seed":"000059","owner":"goclaw"}'
    ),
    (
        '0193a5b0-7000-7000-8000-000000000001',
        'label-templates',
        '标签模板',
        'BarTender 标签模板目录。',
        'D:\数据\标签模板',
        '/mnt/label-templates',
        'system',
        'manual',
        'registry',
        TRUE,
        '{"seed":"000059","owner":"goclaw"}'
    ),
    (
        '0193a5b0-7000-7000-8000-000000000001',
        'operation-log',
        'GoClaw 操作记录',
        'GoClaw 项目续接记录、规则和本地备份资料。',
        'D:\goclaw操作记录',
        '',
        'system',
        'manual',
        'registry',
        TRUE,
        '{"seed":"000059","owner":"goclaw"}'
    )
ON CONFLICT (tenant_id, source_key) DO NOTHING;

