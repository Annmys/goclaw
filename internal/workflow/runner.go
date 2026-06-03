package workflow

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/internal/vault"
)

// NodeRunner is the set of callbacks a workflow node uses to reach goclaw's
// existing subsystems (agent loop, tool registry, knowledge retrieval).
//
// The workflow package may import the agent/tools/vault TYPES freely (verified
// acyclic: no goclaw package imports internal/workflow). But the concrete
// execution instances — the live agent Router, the shared tools.Registry, the
// VaultSearchService — are owned and wired by the gateway. To avoid pulling
// gateway wiring into the engine, those instances are injected as callbacks at
// startup (cmd/gateway_http_wiring.go), mirroring the existing DelegateRunFunc
// pattern (internal/tools/delegate_tool.go).
//
// All callbacks are optional. A nil callback means that node type is
// unavailable; handlers must treat a nil runner field as "not configured" and
// fail the node gracefully rather than panic.
type NodeRunner struct {
	// AgentRun executes an agent turn. Backed by Router.Get(agentID).Run, or by
	// Scheduler.Schedule(LaneTeam, …) when concurrency control is wanted.
	// The agent block sets req fields (Message, agent id via SessionKey, etc.).
	AgentRun func(ctx context.Context, agentID string, req agent.RunRequest) (*agent.RunResult, error)

	// ToolExec runs a registered tool. Backed by tools.Registry.Execute.
	ToolExec func(ctx context.Context, name string, args map[string]any) *tools.Result

	// KnowledgeSearch performs semantic retrieval. Backed by
	// vault.VaultSearchService.Search.
	KnowledgeSearch func(ctx context.Context, opts vault.UnifiedSearchOptions) ([]vault.UnifiedSearchResult, error)

	// ListAgents returns the available agent ids. Backed by Router.List. Used by
	// the AI generator to fall back to an available agent when no specific one
	// is requested.
	ListAgents func() []string
}

// HasAgent reports whether agent nodes can run.
func (r *NodeRunner) HasAgent() bool { return r != nil && r.AgentRun != nil }

// HasTool reports whether tool nodes can run.
func (r *NodeRunner) HasTool() bool { return r != nil && r.ToolExec != nil }

// HasKnowledge reports whether knowledge nodes can run.
func (r *NodeRunner) HasKnowledge() bool { return r != nil && r.KnowledgeSearch != nil }
