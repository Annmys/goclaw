export type WorkflowNodeType =
  | "trigger"
  | "detect"
  | "parse"
  | "fill"
  | "validate"
  | "choice"
  | "action"
  | "transform"
  | "output"
  | "persist"
  | "feedback";

export type WorkflowRunStatus = "draft" | "running" | "paused" | "waiting_user_input" | "failed" | "completed";
export type WorkflowStepStatus = "pending" | "running" | "waiting_user_input" | "failed" | "completed";

export interface WorkflowMissingField {
  key: string;
  label: string;
  kind: string;
  required: boolean;
  options?: string[];
  description?: string;
  details?: Array<{
    value: string;
    title?: string;
    description?: string;
    highlights?: string[];
    metadata?: Record<string, string>;
  }>;
}

export interface WorkflowNode {
  id: string;
  type: WorkflowNodeType;
  type_label: string;
  instance_no: number;
  name: string;
  description?: string;
  depends_on?: string[];
  input_schema?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
  missing_fields?: WorkflowMissingField[];
}

export interface WorkflowOutput {
  adapter: string;
  output_schema?: Record<string, unknown>;
  result_template?: Record<string, unknown>;
}

export interface WorkflowDefinition {
  id: string;
  version: number;
  name: string;
  description: string;
  domain: string;
  active: boolean;
  match_rules: Array<{
    key: string;
    label: string;
    file_types?: string[];
    keywords?: string[];
    description?: string;
  }>;
  nodes: WorkflowNode[];
  output: WorkflowOutput;
  metadata?: Record<string, string>;
}

export interface WorkflowMatchCandidate {
  workflow_id: string;
  workflow_version: number;
  name: string;
  score: number;
  reasons: string[];
}

export interface WorkflowMatchResult {
  matched: boolean;
  needs_choice: boolean;
  candidates: WorkflowMatchCandidate[];
  message: string;
}

export interface WorkflowRunEvent {
  id: string;
  type: string;
  node_id?: string;
  message: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

export interface WorkflowStepRun {
  id: string;
  node_id: string;
  node_type: WorkflowNodeType;
  node_label: string;
  node_name: string;
  instance_no: number;
  status: WorkflowStepStatus;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  missing?: WorkflowMissingField[];
  error?: string;
  started_at?: string;
  completed_at?: string;
}

export interface WorkflowRun {
  id: string;
  workflow_id: string;
  workflow_name: string;
  workflow_version: number;
  tenant_id: string;
  user_id: string;
  status: WorkflowRunStatus;
  input: Record<string, unknown>;
  artifacts?: Array<{
    path: string;
    filename: string;
    mime_type: string;
  }>;
  output?: Record<string, unknown>;
  steps: WorkflowStepRun[];
  events?: WorkflowRunEvent[];
  created_at: string;
  updated_at: string;
}

export interface WorkflowFeedbackRequest {
  run_id: string;
  step_id?: string;
  message: string;
}

export interface WorkflowMessage {
  id: string;
  tenant_id: string;
  user_id: string;
  role: "user" | "assistant";
  content: string;
  files?: string[];
  run_id?: string;
  kind: "chat" | "workflow";
  created_at: string;
}
