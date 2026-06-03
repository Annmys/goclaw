package workflow

import "time"

type NodeType string

const (
	NodeTrigger   NodeType = "trigger"
	NodeDetect    NodeType = "detect"
	NodeParse     NodeType = "parse"
	NodeFill      NodeType = "fill"
	NodeValidate  NodeType = "validate"
	NodeChoice    NodeType = "choice"
	NodeAction    NodeType = "action"
	NodeTransform NodeType = "transform"
	NodeOutput    NodeType = "output"
	NodePersist   NodeType = "persist"
	NodeFeedback  NodeType = "feedback"
)

type RunStatus string

const (
	RunDraft            RunStatus = "draft"
	RunRunning          RunStatus = "running"
	RunPaused           RunStatus = "paused"
	RunWaitingUserInput RunStatus = "waiting_user_input"
	RunFailed           RunStatus = "failed"
	RunCompleted        RunStatus = "completed"
)

type StepStatus string

const (
	StepPending          StepStatus = "pending"
	StepRunning          StepStatus = "running"
	StepWaitingUserInput StepStatus = "waiting_user_input"
	StepFailed           StepStatus = "failed"
	StepCompleted        StepStatus = "completed"
)

type NodeDefinition struct {
	ID            string         `json:"id"`
	Type          NodeType       `json:"type"`
	TypeLabel     string         `json:"type_label"`
	InstanceNo    int            `json:"instance_no"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	DependsOn     []string       `json:"depends_on,omitempty"`
	InputSchema   map[string]any `json:"input_schema,omitempty"`
	OutputSchema  map[string]any `json:"output_schema,omitempty"`
	MissingFields []MissingField `json:"missing_fields,omitempty"`
}

type MissingField struct {
	Key         string         `json:"key"`
	Label       string         `json:"label"`
	Kind        string         `json:"kind"`
	Required    bool           `json:"required"`
	Options     []string       `json:"options,omitempty"`
	Description string         `json:"description,omitempty"`
	Details     []OptionDetail `json:"details,omitempty"`
}

type OptionDetail struct {
	Value       string            `json:"value"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Highlights  []string          `json:"highlights,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type OutputDefinition struct {
	Adapter        string         `json:"adapter"`
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
	ResultTemplate map[string]any `json:"result_template,omitempty"`
}

type FileArtifact struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	MIMEType string `json:"mime_type"`
}

type MatchRule struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	FileTypes   []string `json:"file_types,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Description string   `json:"description,omitempty"`
}

type Definition struct {
	ID          string            `json:"id"`
	Version     int               `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Domain      string            `json:"domain"`
	Active      bool              `json:"active"`
	MatchRules  []MatchRule       `json:"match_rules"`
	Nodes       []NodeDefinition  `json:"nodes"`
	Output      OutputDefinition  `json:"output"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type StepRun struct {
	ID          string         `json:"id"`
	NodeID      string         `json:"node_id"`
	NodeType    NodeType       `json:"node_type"`
	NodeLabel   string         `json:"node_label"`
	NodeName    string         `json:"node_name"`
	InstanceNo  int            `json:"instance_no"`
	Status      StepStatus     `json:"status"`
	Input       map[string]any `json:"input,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Missing     []MissingField `json:"missing,omitempty"`
	Error       string         `json:"error,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	DurationMS  int64          `json:"duration_ms,omitempty"`
}

type Run struct {
	ID              string         `json:"id"`
	WorkflowID      string         `json:"workflow_id"`
	WorkflowName    string         `json:"workflow_name"`
	WorkflowVersion int            `json:"workflow_version"`
	TenantID        string         `json:"tenant_id"`
	UserID          string         `json:"user_id"`
	Status          RunStatus      `json:"status"`
	Input           map[string]any `json:"input"`
	Artifacts       []FileArtifact `json:"artifacts,omitempty"`
	Output          map[string]any `json:"output,omitempty"`
	Steps           []StepRun      `json:"steps"`
	Events          []RunEvent     `json:"events,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DurationMS      int64          `json:"duration_ms,omitempty"`
}

type RunEvent struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	NodeID    string         `json:"node_id,omitempty"`
	Message   string         `json:"message"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type MatchRequest struct {
	Intent     string `json:"intent"`
	FileName   string `json:"file_name"`
	FileType   string `json:"file_type"`
	ExplicitID string `json:"workflow_id"`
}

type MatchCandidate struct {
	WorkflowID      string   `json:"workflow_id"`
	WorkflowVersion int      `json:"workflow_version"`
	Name            string   `json:"name"`
	Score           int      `json:"score"`
	Reasons         []string `json:"reasons"`
}

type MatchResult struct {
	Matched     bool             `json:"matched"`
	NeedsChoice bool             `json:"needs_choice"`
	Candidates  []MatchCandidate `json:"candidates"`
	Message     string           `json:"message"`
}

type ResumeRunRequest struct {
	Fields map[string]any `json:"fields"`
}

type UploadedFileInput struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	MIMEType string `json:"mime_type"`
}

type FeedbackRequest struct {
	RunID   string `json:"run_id"`
	StepID  string `json:"step_id,omitempty"`
	Message string `json:"message"`
}

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type MessageKind string

const (
	MessageKindChat     MessageKind = "chat"
	MessageKindWorkflow MessageKind = "workflow"
)

type Message struct {
	ID        string      `json:"id"`
	TenantID  string      `json:"tenant_id"`
	UserID    string      `json:"user_id"`
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Files     []string    `json:"files,omitempty"`
	RunID     string      `json:"run_id,omitempty"`
	Kind      MessageKind `json:"kind"`
	CreatedAt time.Time   `json:"created_at"`
}

type AppendMessageRequest struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
	Files   []string    `json:"files,omitempty"`
	RunID   string      `json:"run_id,omitempty"`
	Kind    MessageKind `json:"kind,omitempty"`
}
