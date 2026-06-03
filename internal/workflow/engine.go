package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

var ErrWorkflowNotFound = errors.New("workflow not found")
var ErrRunNotFound = errors.New("workflow run not found")

type workflowExecutionState struct {
	EstimateOrder *EstimateOrder
	EstimateRows  []EstimateResultRow
}

type Engine struct {
	registry *Registry
	db       *sql.DB
	mu       sync.RWMutex
	runs     map[string]Run
	messages map[string]Message

	// runner backs sim-style graph nodes (agent/tool/knowledge). Injected at
	// wiring time; nil means those node types are unavailable. The legacy
	// in-code business runners do not use it.
	runner *NodeRunner
}

// SetNodeRunner injects the subsystem callbacks used by graph node handlers.
// Safe to call once at startup before serving requests.
func (e *Engine) SetNodeRunner(r *NodeRunner) { e.runner = r }

func NewEngine(registry *Registry) *Engine {
	if registry == nil {
		registry = NewDefaultRegistry()
	}
	return &Engine{
		registry: registry,
		runs:     make(map[string]Run),
		messages: make(map[string]Message),
	}
}

func NewEngineWithDB(registry *Registry, db *sql.DB) *Engine {
	engine := NewEngine(registry)
	engine.db = db
	return engine
}

func (e *Engine) Definitions(_ context.Context) []Definition {
	return e.registry.List()
}

func (e *Engine) Match(_ context.Context, req MatchRequest) MatchResult {
	return e.registry.Match(req)
}

func (e *Engine) ListRuns(ctx context.Context) []Run {
	if e.db != nil {
		runs, err := e.listRunsFromDB(ctx)
		if err == nil {
			return runs
		}
		slog.Warn("workflow: list runs from db failed", "error", err)
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	runs := make([]Run, 0, len(e.runs))
	for _, run := range e.runs {
		if tenantID != uuid.Nil && run.TenantID != "" && run.TenantID != tenantID.String() {
			continue
		}
		if run.UserID != userID {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs
}

func (e *Engine) ListMessages(ctx context.Context) []Message {
	if e.db != nil {
		messages, err := e.listMessagesFromDB(ctx)
		if err == nil {
			return messages
		}
		slog.Warn("workflow: list messages from db failed", "error", err)
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	messages := make([]Message, 0, len(e.messages))
	for _, msg := range e.messages {
		if tenantID != uuid.Nil && msg.TenantID != "" && msg.TenantID != tenantID.String() {
			continue
		}
		if msg.UserID != userID {
			continue
		}
		messages = append(messages, msg)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
	return messages
}

func (e *Engine) AppendMessage(ctx context.Context, req AppendMessageRequest) Message {
	msg := Message{
		ID:        uuid.NewString(),
		TenantID:  store.TenantIDFromContext(ctx).String(),
		UserID:    store.UserIDFromContext(ctx),
		Role:      req.Role,
		Content:   strings.TrimSpace(req.Content),
		Files:     append([]string(nil), req.Files...),
		RunID:     req.RunID,
		Kind:      req.Kind,
		CreatedAt: time.Now().UTC(),
	}
	if msg.Kind == "" {
		msg.Kind = MessageKindChat
	}
	e.mu.Lock()
	e.messages[msg.ID] = msg
	e.mu.Unlock()
	if e.db != nil {
		if err := e.saveMessage(ctx, msg); err != nil {
			slog.Warn("workflow: save message failed", "message_id", msg.ID, "error", err)
		}
	}
	return msg
}

type StartRunRequest struct {
	WorkflowID string         `json:"workflow_id"`
	Intent     string         `json:"intent"`
	FileName   string         `json:"file_name"`
	FileType   string         `json:"file_type"`
	Input      map[string]any `json:"input"`
	TenantID   string         `json:"tenant_id"`
	UserID     string         `json:"user_id"`
}

func (e *Engine) StartRun(ctx context.Context, req StartRunRequest) (Run, error) {
	def, ok := e.registry.Get(req.WorkflowID)
	if !ok {
		return Run{}, ErrWorkflowNotFound
	}

	input := req.Input
	if input == nil {
		input = map[string]any{}
	}
	input["intent"] = req.Intent
	input["file_name"] = req.FileName
	input["file_type"] = req.FileType

	artifacts := parseArtifacts(input)
	state, err := buildExecutionState(def, input, artifacts)
	if err != nil {
		return Run{}, err
	}

	now := time.Now().UTC()
	steps := make([]StepRun, 0, len(def.Nodes))
	for _, node := range def.Nodes {
		steps = append(steps, StepRun{
			ID:         uuid.NewString(),
			NodeID:     node.ID,
			NodeType:   node.Type,
			NodeLabel:  node.TypeLabel,
			NodeName:   node.Name,
			InstanceNo: node.InstanceNo,
			Status:     StepPending,
		})
	}

	run := Run{
		ID:              uuid.NewString(),
		WorkflowID:      def.ID,
		WorkflowName:    def.Name,
		WorkflowVersion: def.Version,
		TenantID:        req.TenantID,
		UserID:          req.UserID,
		Status:          RunRunning,
		Input:           input,
		Artifacts:       artifacts,
		Steps:           steps,
		Events: []RunEvent{{
			ID:        uuid.NewString(),
			Type:      "run_created",
			Message:   "workflow run created",
			CreatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}

	run = e.advanceRun(def, run, nil, state)

	e.mu.Lock()
	e.runs[run.ID] = run
	e.mu.Unlock()
	if e.db != nil {
		if err := e.saveRun(ctx, run); err != nil {
			slog.Warn("workflow: save run failed", "run_id", run.ID, "error", err)
		}
	}
	return run, nil
}

func (e *Engine) GetRun(ctx context.Context, id string) (Run, error) {
	e.mu.RLock()
	run, ok := e.runs[id]
	e.mu.RUnlock()
	if !ok {
		if e.db == nil {
			return Run{}, ErrRunNotFound
		}
		dbRun, err := e.getRunFromDB(ctx, id)
		if err != nil {
			return Run{}, err
		}
		e.mu.Lock()
		e.runs[dbRun.ID] = dbRun
		e.mu.Unlock()
		run = dbRun
	}
	if !runMatchesContext(ctx, run) {
		return Run{}, ErrRunNotFound
	}
	return run, nil
}

func (e *Engine) ResumeRun(ctx context.Context, id string, req ResumeRunRequest) (Run, error) {
	run, err := e.GetRun(ctx, id)
	if err != nil {
		return Run{}, err
	}
	def, ok := e.registry.Get(run.WorkflowID)
	if !ok {
		return Run{}, ErrWorkflowNotFound
	}
	if run.Input == nil {
		run.Input = map[string]any{}
	}
	for k, v := range req.Fields {
		run.Input[k] = v
	}
	state, err := buildExecutionState(def, run.Input, run.Artifacts)
	if err != nil {
		run.Status = RunFailed
		run.Events = append(run.Events, RunEvent{
			ID:        uuid.NewString(),
			Type:      "resume_failed",
			Message:   err.Error(),
			CreatedAt: time.Now().UTC(),
		})
		run.UpdatedAt = time.Now().UTC()
		e.runs[id] = run
		return run, err
	}
	run.Events = append(run.Events, RunEvent{
		ID:        uuid.NewString(),
		Type:      "user_input",
		Message:   "user supplied missing workflow fields",
		Payload:   map[string]any{"fields": req.Fields},
		CreatedAt: time.Now().UTC(),
	})
	run.Status = RunRunning
	run = e.advanceRun(def, run, req.Fields, state)
	run.UpdatedAt = time.Now().UTC()
	e.mu.Lock()
	e.runs[id] = run
	e.mu.Unlock()
	if e.db != nil {
		if err := e.saveRun(ctx, run); err != nil {
			slog.Warn("workflow: save resumed run failed", "run_id", run.ID, "error", err)
		}
	}
	return run, nil
}

func (e *Engine) SubmitFeedback(ctx context.Context, req FeedbackRequest) error {
	run, err := e.GetRun(ctx, req.RunID)
	if err != nil {
		return err
	}
	event := RunEvent{
		ID:        uuid.NewString(),
		Type:      "user_feedback",
		Message:   strings.TrimSpace(req.Message),
		Payload:   map[string]any{"step_id": req.StepID, "message": req.Message},
		CreatedAt: time.Now().UTC(),
	}
	run.Events = append(run.Events, event)
	run.UpdatedAt = event.CreatedAt
	e.mu.Lock()
	e.runs[req.RunID] = run
	e.mu.Unlock()
	if e.db != nil {
		if err := e.saveRun(ctx, run); err != nil {
			slog.Warn("workflow: save feedback run failed", "run_id", run.ID, "error", err)
		}
	}
	return nil
}

func (e *Engine) getRunFromDB(ctx context.Context, id string) (Run, error) {
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	var raw []byte
	err := e.db.QueryRowContext(ctx, `
		SELECT run_json
		FROM workflow_runs
		WHERE id = $1 AND tenant_id = $2 AND user_id = $3
	`, id, tenantID, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(raw, &run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (e *Engine) listRunsFromDB(ctx context.Context) ([]Run, error) {
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	rows, err := e.db.QueryContext(ctx, `
		SELECT run_json
		FROM workflow_runs
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at DESC
	`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal(raw, &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (e *Engine) listMessagesFromDB(ctx context.Context) ([]Message, error) {
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, tenant_id::text, user_id, role, content, files, COALESCE(run_id, ''), kind, created_at
		FROM workflow_messages
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY created_at ASC
	`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]Message, 0)
	for rows.Next() {
		var msg Message
		var filesRaw []byte
		if err := rows.Scan(&msg.ID, &msg.TenantID, &msg.UserID, &msg.Role, &msg.Content, &filesRaw, &msg.RunID, &msg.Kind, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if len(filesRaw) > 0 {
			if err := json.Unmarshal(filesRaw, &msg.Files); err != nil {
				return nil, err
			}
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (e *Engine) saveRun(ctx context.Context, run Run) error {
	tenantID, err := uuid.Parse(run.TenantID)
	if err != nil || tenantID == uuid.Nil {
		tenantID = store.TenantIDFromContext(ctx)
	}
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal run: %w", err)
	}
	_, err = e.db.ExecContext(ctx, `
		INSERT INTO workflow_runs (id, tenant_id, user_id, status, run_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			run_json = EXCLUDED.run_json,
			updated_at = EXCLUDED.updated_at
	`, run.ID, tenantID, run.UserID, string(run.Status), raw, run.CreatedAt, run.UpdatedAt)
	return err
}

func (e *Engine) saveMessage(ctx context.Context, msg Message) error {
	tenantID, err := uuid.Parse(msg.TenantID)
	if err != nil || tenantID == uuid.Nil {
		tenantID = store.TenantIDFromContext(ctx)
	}
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}
	filesRaw, err := json.Marshal(msg.Files)
	if err != nil {
		return fmt.Errorf("marshal files: %w", err)
	}
	var runID any
	if msg.RunID != "" {
		runID = msg.RunID
	}
	_, err = e.db.ExecContext(ctx, `
		INSERT INTO workflow_messages (id, tenant_id, user_id, role, content, files, run_id, kind, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`, msg.ID, tenantID, msg.UserID, string(msg.Role), msg.Content, filesRaw, runID, string(msg.Kind), msg.CreatedAt)
	return err
}

func runMatchesContext(ctx context.Context, run Run) bool {
	tenantID := store.TenantIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)
	if tenantID != uuid.Nil && run.TenantID != "" && run.TenantID != tenantID.String() {
		return false
	}
	if run.UserID != userID {
		return false
	}
	return true
}

func (e *Engine) advanceRun(def Definition, run Run, resumeFields map[string]any, state workflowExecutionState) Run {
	for i := range run.Steps {
		if run.Steps[i].Status == StepCompleted {
			continue
		}
		now := time.Now().UTC()
		node, ok := nodeByID(def, run.Steps[i].NodeID)
		if !ok {
			run.Steps[i].Status = StepFailed
			run.Steps[i].Error = "workflow definition node not found"
			run.Status = RunFailed
			run.Events = append(run.Events, eventForStep("step_failed", run.Steps[i], "workflow definition node not found", nil))
			return run
		}

		run.Steps[i].Status = StepRunning
		run.Steps[i].StartedAt = &now
		run.Steps[i].Input = stepInput(run.Input, node, resumeFields, state)

		missing := requiredMissingFields(node, run.Input)
		if len(missing) > 0 {
			run.Steps[i].Status = StepWaitingUserInput
			run.Steps[i].Missing = missing
			run.Status = RunWaitingUserInput
			run.UpdatedAt = now
			run.Events = append(run.Events, eventForStep("waiting_user_input", run.Steps[i], "workflow paused for missing fields", map[string]any{"missing": missing}))
			return run
		}

		output := executeNode(def, node, run.Input, state)
		completedAt := time.Now().UTC()
		run.Steps[i].Status = StepCompleted
		run.Steps[i].Output = output
		run.Steps[i].CompletedAt = &completedAt
		run.Steps[i].DurationMS = completedAt.Sub(now).Milliseconds()
		run.Steps[i].Missing = nil
		run.Events = append(run.Events, eventForStep("step_completed", run.Steps[i], "workflow step completed", output))
	}

	now := time.Now().UTC()
	run.Status = RunCompleted
	run.Output = finalOutput(def, run, state)
	run.UpdatedAt = now
	run.DurationMS = now.Sub(run.CreatedAt).Milliseconds()
	run.Events = append(run.Events, RunEvent{
		ID:        uuid.NewString(),
		Type:      "run_completed",
		Message:   "workflow run completed",
		Payload:   run.Output,
		CreatedAt: now,
	})
	return run
}

func nodeByID(def Definition, id string) (NodeDefinition, bool) {
	for _, node := range def.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return NodeDefinition{}, false
}

func requiredMissingFields(node NodeDefinition, input map[string]any) []MissingField {
	if len(node.MissingFields) == 0 {
		return nil
	}
	missing := make([]MissingField, 0)
	for _, field := range node.MissingFields {
		if !field.Required {
			continue
		}
		value, ok := input[field.Key]
		if !ok || value == nil || value == "" {
			missing = append(missing, field)
		}
	}
	return missing
}

func stepInput(input map[string]any, node NodeDefinition, resumeFields map[string]any, state workflowExecutionState) map[string]any {
	out := map[string]any{
		"node_id":       node.ID,
		"node_type":     node.Type,
		"node_label":    node.TypeLabel,
		"workflow_data": input,
	}
	if state.EstimateOrder != nil {
		out["estimate_order"] = map[string]any{
			"order_no": state.EstimateOrder.OrderNo,
			"summary":  state.EstimateOrder.Summary,
			"source":   state.EstimateOrder.Source,
		}
	}
	if len(resumeFields) > 0 {
		out["resume_fields"] = resumeFields
	}
	return out
}

func executeNode(def Definition, node NodeDefinition, input map[string]any, state workflowExecutionState) map[string]any {
	if def.Output.Adapter == "packing_list" && state.EstimateOrder != nil {
		return executeEstimateNode(def, node, input, state)
	}
	switch node.Type {
	case NodeTrigger:
		return map[string]any{
			"accepted":  true,
			"file_name": input["file_name"],
			"file_type": input["file_type"],
		}
	case NodeDetect:
		return map[string]any{
			"workflow_id":   def.ID,
			"workflow_name": def.Name,
			"domain":        def.Domain,
		}
	case NodeParse:
		return map[string]any{
			"parsed":    true,
			"source":    input["file_name"],
			"parser":    "deterministic_stub",
			"next_step": "missing_data_check",
		}
	case NodeChoice:
		return map[string]any{"choice_resolved": true}
	case NodeFill:
		return map[string]any{
			"filled": true,
			"fields": fieldsForNode(node, input),
			"agent":  "workflow_shujuchuli",
		}
	case NodeValidate:
		return map[string]any{"valid": true}
	case NodeAction:
		return map[string]any{
			"action": "business_workflow_execute",
			"status": "prepared",
		}
	case NodeTransform:
		return map[string]any{
			"transformed": true,
			"adapter":     def.Output.Adapter,
		}
	case NodeOutput:
		return finalOutput(def, Run{WorkflowID: def.ID, WorkflowName: def.Name, WorkflowVersion: def.Version, Input: input}, state)
	case NodePersist:
		return map[string]any{"persisted": true, "audit": "in_memory"}
	case NodeFeedback:
		return map[string]any{"feedback_route": "workflow_liuchengweihu"}
	default:
		return map[string]any{"status": "completed"}
	}
}

func executeEstimateNode(def Definition, node NodeDefinition, input map[string]any, state workflowExecutionState) map[string]any {
	order := state.EstimateOrder
	switch node.Type {
	case NodeTrigger:
		return map[string]any{
			"accepted": true,
			"source":   order.Source,
			"intent":   input["intent"],
		}
	case NodeDetect:
		return map[string]any{
			"workflow_id":   def.ID,
			"workflow_name": def.Name,
			"domain":        def.Domain,
			"order_no":      order.OrderNo,
			"customer_code": order.CustomerCode,
		}
	case NodeParse:
		return map[string]any{
			"parsed":       true,
			"source":       order.Source,
			"column_map":   order.ColumnMap,
			"summary":      order.Summary,
			"warnings":     order.Warnings,
			"missing_cols": order.MissingColumns,
		}
	case NodeChoice:
		return map[string]any{
			"choice_resolved": true,
			"carton_rule":     input["carton_rule"],
			"weight_source":   input["weight_source"],
		}
	case NodeFill:
		return map[string]any{
			"filled":        true,
			"agent":         "workflow_shujuchuli",
			"data_scope":    "仅处理预估箱单流程所需字段识别、汇总和填入",
			"order_summary": order.Summary,
			"rows":          len(order.Rows),
		}
	case NodeValidate:
		issues := estimateValidationIssues(*order, state.EstimateRows)
		return map[string]any{
			"valid":  len(issues) == 0,
			"issues": issues,
		}
	case NodeAction:
		return map[string]any{
			"action":         "estimate_packing_list",
			"order_no":       order.OrderNo,
			"result_rows":    len(state.EstimateRows),
			"total_quantity": order.Summary.TotalQuantity,
		}
	case NodeTransform:
		return map[string]any{
			"transformed": true,
			"adapter":     def.Output.Adapter,
			"schema": []string{
				"item_code",
				"item_name",
				"item_type",
				"spec_model",
				"quantity",
				"unit",
				"confidence",
				"reason",
			},
		}
	case NodeOutput:
		return finalOutput(def, Run{WorkflowID: def.ID, WorkflowName: def.Name, WorkflowVersion: def.Version, Input: input}, state)
	case NodePersist:
		return map[string]any{
			"persisted": true,
			"audit":     "in_memory",
			"note":      "当前版本记录保存在 workflow run 内存态；数据库持久化待接入",
		}
	case NodeFeedback:
		return map[string]any{
			"feedback_route": "workflow_liuchengweihu",
			"repair_scope":   []string{"数据识别错误", "包装资料匹配规则", "输出格式问题", "缺字段补齐规则"},
		}
	default:
		return map[string]any{"status": "completed"}
	}
}

func fieldsForNode(node NodeDefinition, input map[string]any) map[string]any {
	out := make(map[string]any)
	for _, field := range node.MissingFields {
		if value, ok := input[field.Key]; ok {
			out[field.Key] = value
		}
	}
	return out
}

func finalOutput(def Definition, run Run, state workflowExecutionState) map[string]any {
	base := map[string]any{
		"workflow_id":      def.ID,
		"workflow_name":    def.Name,
		"workflow_version": def.Version,
		"adapter":          def.Output.Adapter,
		"status":           "completed",
	}
	switch def.Output.Adapter {
	case "packing_list":
		if state.EstimateOrder != nil {
			base["order"] = state.EstimateOrder
			base["rows"] = state.EstimateRows
			base["warnings"] = state.EstimateOrder.Warnings
			base["result_file"] = "输出结果/" + state.EstimateOrder.OrderNo + "/包装材料需求流转单.xlsx"
			base["quality"] = map[string]any{
				"confidence_floor":  minEstimateConfidence(state.EstimateRows),
				"validation_issues": estimateValidationIssues(*state.EstimateOrder, state.EstimateRows),
			}
		} else {
			base["result_file"] = "输出结果/" + stringValue(run.Input["file_name"], "workflow-result") + "/包装材料需求流转单.xlsx"
			base["rows"] = []map[string]any{}
			base["warnings"] = []string{"未解析到预估箱单订单数据"}
		}
	case "label_generation":
		base["preview"] = []map[string]any{{"label": stringValue(run.Input["label_template"], "标签预览")}}
		base["print_payload"] = map[string]any{"mode": stringValue(run.Input["print_mode"], "预览")}
		base["result_file"] = "输出结果/" + stringValue(run.Input["file_name"], "workflow-result") + "/标签结果.xlsx"
	default:
		base["result"] = map[string]any{}
	}
	return base
}

func buildExecutionState(def Definition, input map[string]any, artifacts []FileArtifact) (workflowExecutionState, error) {
	state := workflowExecutionState{}
	if def.Output.Adapter != "packing_list" {
		return state, nil
	}
	file, ok := primaryUploadedFile(input, artifacts)
	if !ok {
		return state, nil
	}
	order, err := parseEstimateOrder(file)
	if err != nil {
		return state, err
	}
	rows := buildEstimateResultRows(order)
	state.EstimateOrder = &order
	state.EstimateRows = rows
	return state, nil
}

func parseArtifacts(input map[string]any) []FileArtifact {
	raw, ok := input["media"].([]any)
	if !ok {
		return nil
	}
	out := make([]FileArtifact, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, FileArtifact{
			Path:     stringValue(m["path"], ""),
			Filename: stringValue(m["filename"], ""),
			MIMEType: stringValue(m["mime_type"], ""),
		})
	}
	return out
}

func primaryUploadedFile(input map[string]any, artifacts []FileArtifact) (UploadedFileInput, bool) {
	if len(artifacts) > 0 {
		a := artifacts[0]
		if a.Path != "" {
			return UploadedFileInput{Path: a.Path, Filename: a.Filename, MIMEType: a.MIMEType}, true
		}
	}
	path := stringValue(input["file_path"], "")
	if path == "" {
		return UploadedFileInput{}, false
	}
	return UploadedFileInput{
		Path:     path,
		Filename: stringValue(input["file_name"], ""),
		MIMEType: stringValue(input["file_type"], ""),
	}, true
}

func estimateValidationIssues(order EstimateOrder, rows []EstimateResultRow) []string {
	issues := make([]string, 0)
	if order.OrderNo == "" {
		issues = append(issues, "缺少订单号")
	}
	if order.Summary.LineCount == 0 {
		issues = append(issues, "没有可处理订单明细")
	}
	if len(order.MissingColumns) > 0 {
		issues = append(issues, "缺少关键列: "+strings.Join(order.MissingColumns, ", "))
	}
	if len(rows) == 0 {
		issues = append(issues, "未生成任何预估包材行")
	}
	if order.Summary.UnknownRows > 0 {
		issues = append(issues, "存在未识别物料编码行")
	}
	return issues
}

func minEstimateConfidence(rows []EstimateResultRow) float64 {
	if len(rows) == 0 {
		return 0
	}
	min := rows[0].Confidence
	for _, row := range rows[1:] {
		if row.Confidence < min {
			min = row.Confidence
		}
	}
	return min
}

func eventForStep(eventType string, step StepRun, message string, payload map[string]any) RunEvent {
	return RunEvent{
		ID:        uuid.NewString(),
		Type:      eventType,
		NodeID:    step.NodeID,
		Message:   message,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
}

func stringValue(value any, fallback string) string {
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return fallback
}

