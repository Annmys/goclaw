package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/internal/vault"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/dag"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/handlers"
)

// runnerAdapter bridges *NodeRunner (callbacks) to the handlers.Runner
// interface consumed by node handlers. A nil callback degrades to "not
// configured" so a missing subsystem fails the node gracefully.
type runnerAdapter struct{ r *NodeRunner }

func (a runnerAdapter) HasAgent() bool     { return a.r.HasAgent() }
func (a runnerAdapter) HasTool() bool      { return a.r.HasTool() }
func (a runnerAdapter) HasKnowledge() bool { return a.r.HasKnowledge() }

func (a runnerAdapter) AgentRun(ctx context.Context, agentID string, req agent.RunRequest) (*agent.RunResult, error) {
	return a.r.AgentRun(ctx, agentID, req)
}
func (a runnerAdapter) ToolExec(ctx context.Context, name string, args map[string]any) *tools.Result {
	return a.r.ToolExec(ctx, name, args)
}
func (a runnerAdapter) KnowledgeSearch(ctx context.Context, opts vault.UnifiedSearchOptions) ([]vault.UnifiedSearchResult, error) {
	return a.r.KnowledgeSearch(ctx, opts)
}

// eventCollector implements dag.StepObserver, recording RunEvents and step
// outputs as the graph executes.
type eventCollector struct {
	events []RunEvent
}

func (c *eventCollector) OnNodeStart(nodeID, nodeType string, _ map[string]any) {
	c.events = append(c.events, RunEvent{
		ID: uuid.NewString(), Type: "node_start", NodeID: nodeID,
		Message: fmt.Sprintf("node %s (%s) started", nodeID, nodeType), CreatedAt: time.Now().UTC(),
	})
}
func (c *eventCollector) OnNodeComplete(nodeID, nodeType string, output map[string]any) {
	c.events = append(c.events, RunEvent{
		ID: uuid.NewString(), Type: "node_complete", NodeID: nodeID,
		Message: fmt.Sprintf("node %s (%s) completed", nodeID, nodeType),
		Payload: output, CreatedAt: time.Now().UTC(),
	})
}
func (c *eventCollector) OnNodeError(nodeID, nodeType string, err error) {
	c.events = append(c.events, RunEvent{
		ID: uuid.NewString(), Type: "node_error", NodeID: nodeID,
		Message: fmt.Sprintf("node %s (%s) error: %v", nodeID, nodeType, err), CreatedAt: time.Now().UTC(),
	})
}

// StartGraphRun executes a stored graph definition and persists a Run.
//
// It is the sim-style execution path, parallel to the legacy StartRun. The
// input map seeds the trigger node; the DAG executor runs the graph using the
// injected NodeRunner for agent/tool/knowledge nodes. A paused run (human
// node) is persisted as RunWaitingUserInput.
func (e *Engine) StartGraphRun(ctx context.Context, defID string, input map[string]any) (Run, error) {
	def, err := e.GetDefinition(ctx, defID)
	if err != nil {
		return Run{}, err
	}

	tenantID, userID := scopeIDs(ctx)
	now := time.Now().UTC()
	run := Run{
		ID:              uuid.NewString(),
		WorkflowID:      def.ID,
		WorkflowName:    def.Name,
		WorkflowVersion: def.Version,
		TenantID:        tenantID.String(),
		UserID:          userID,
		Status:          RunRunning,
		Input:           input,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	var runner handlers.Runner
	if e.runner != nil {
		runner = runnerAdapter{r: e.runner}
	}
	reg := handlers.BuildRegistry(runner, "", userID, tenantID.String())

	collector := &eventCollector{}
	exec := dag.NewExecutor(&def.Graph, reg,
		dag.WithObserver(collector),
		dag.WithVariables(input),
	)

	start := time.Now()
	res, execErr := exec.Run(ctx, "")
	run.Events = collector.events
	run.DurationMS = time.Since(start).Milliseconds()
	run.UpdatedAt = time.Now().UTC()

	if execErr != nil {
		run.Status = RunFailed
		run.Output = map[string]any{"error": execErr.Error()}
	} else if res.Paused != nil {
		run.Status = RunWaitingUserInput
		run.Output = res.Output
	} else {
		run.Status = RunCompleted
		run.Output = res.Output
	}

	e.mu.Lock()
	e.runs[run.ID] = run
	e.mu.Unlock()
	if e.db != nil {
		if err := e.saveRun(ctx, run); err != nil {
			return run, err
		}
	}
	return run, execErr
}
