// Serialized workflow graph types — mirror of the Go side
// (internal/workflow/graph/types.go). Field names match so the canvas JSON and
// the backend Graph struct are the same shape.

export interface GraphPosition {
  x: number;
  y: number;
}

export interface GraphBlockMetadata {
  id: string; // node type: agent | tool | condition | router | function | api | knowledge | response | trigger | human-in-the-loop | wait
  name?: string;
  description?: string;
  category?: string;
  icon?: string;
  color?: string;
}

export interface GraphBlockConfig {
  tool?: string;
  params?: Record<string, unknown>;
}

export interface GraphBlock {
  id: string;
  position: GraphPosition;
  config: GraphBlockConfig;
  inputs?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  metadata?: GraphBlockMetadata;
  enabled: boolean;
  canonicalModes?: Record<string, string>;
}

export interface GraphEdgeCondition {
  type: "if" | "else if" | "else";
  expression?: string;
}

export interface GraphConnection {
  source: string;
  target: string;
  sourceHandle?: string;
  targetHandle?: string;
  condition?: GraphEdgeCondition;
}

export interface GraphLoop {
  id: string;
  nodes: string[];
  iterations?: number;
  loopType?: "for" | "forEach" | "while" | "doWhile";
  forEachItems?: unknown;
  whileCondition?: string;
  doWhileCondition?: string;
}

export interface GraphParallel {
  id: string;
  nodes: string[];
  parallelType?: "count" | "collection";
  count?: number;
  distribution?: unknown;
  batchSize?: number;
}

export interface WorkflowGraph {
  version: string;
  blocks: GraphBlock[];
  connections: GraphConnection[];
  loops?: Record<string, GraphLoop>;
  parallels?: Record<string, GraphParallel>;
}

export interface GraphDefinition {
  id: string;
  tenant_id?: string;
  user_id?: string;
  name: string;
  description?: string;
  graph: WorkflowGraph;
  version?: number;
  active?: boolean;
  created_at?: string;
  updated_at?: string;
}

export const GRAPH_VERSION = "1.0";

// Node type catalog for the canvas palette.
export const NODE_TYPES: { type: string; label: string; category: string }[] = [
  { type: "trigger", label: "触发", category: "control" },
  { type: "agent", label: "智能体", category: "core" },
  { type: "tool", label: "工具", category: "core" },
  { type: "function", label: "函数", category: "core" },
  { type: "api", label: "API", category: "core" },
  { type: "condition", label: "条件", category: "control" },
  { type: "router", label: "路由", category: "control" },
  { type: "knowledge", label: "知识检索", category: "core" },
  { type: "response", label: "响应", category: "control" },
  { type: "human-in-the-loop", label: "人工介入", category: "control" },
];
