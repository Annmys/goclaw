export interface LocalKnowledgeSource {
  id: string;
  tenant_id: string;
  source_key: string;
  name: string;
  description: string;
  path_windows: string;
  path_container: string;
  tenant_scope: "system" | "tenant" | "shared" | string;
  sync_mode: "manual" | "scheduled" | "watch" | string;
  index_target: "registry" | "vault" | "memory" | "knowledge_graph" | "tool_cache" | string;
  enabled: boolean;
  last_sync_at?: string | null;
  last_success_at?: string | null;
  last_error?: string | null;
  file_count: number;
  record_count: number;
  content_hash: string;
  created_at: string;
  updated_at: string;
}

export interface LocalKnowledgeSourcesResponse {
  sources: LocalKnowledgeSource[];
}

