package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/internal/vault"
	"github.com/nextlevelbuilder/goclaw/internal/workflow"
)

// buildWorkflowNodeRunner wires the workflow engine's graph nodes to goclaw's
// existing subsystems. Each callback is nil-guarded by the dependency it needs,
// so a node type degrades gracefully ("not configured") when its subsystem is
// absent rather than panicking.
//
// This mirrors the delegate-tool wiring pattern (gateway_managed.go): the
// workflow package never imports gateway wiring; the live instances are passed
// in as callbacks here.
func (d *gatewayDeps) buildWorkflowNodeRunner() *workflow.NodeRunner {
	r := &workflow.NodeRunner{}

	if d.agentRouter != nil {
		r.AgentRun = func(ctx context.Context, agentID string, req agent.RunRequest) (*agent.RunResult, error) {
			loop, err := d.agentRouter.Get(ctx, agentID)
			if err != nil {
				return nil, fmt.Errorf("workflow agent node: agent %q not found: %w", agentID, err)
			}
			if req.SessionKey == "" {
				req.SessionKey = fmt.Sprintf("workflow:%s", agentID)
			}
			return loop.Run(ctx, req)
		}
		r.ListAgents = func(ctx context.Context) []string {
			// Prefer the agent store for complete, bare agent keys. The Router
			// cache only holds already-instantiated agents and stores them under
			// tenant-prefixed keys, which Get() would then double-prefix.
			if d.pgStores != nil && d.pgStores.Agents != nil {
				agents, err := d.pgStores.Agents.List(ctx, "")
				if err == nil {
					keys := make([]string, 0, len(agents))
					for i := range agents {
						if agents[i].AgentKey != "" {
							keys = append(keys, agents[i].AgentKey)
						}
					}
					if len(keys) > 0 {
						return keys
					}
				}
			}
			// Fallback: strip tenant prefix from router cache keys ("tid:key" → "key").
			raw := d.agentRouter.List()
			out := make([]string, 0, len(raw))
			for _, k := range raw {
				if i := strings.LastIndex(k, ":"); i >= 0 {
					out = append(out, k[i+1:])
				} else {
					out = append(out, k)
				}
			}
			return out
		}
	}

	if d.toolsReg != nil {
		r.ToolExec = func(ctx context.Context, name string, args map[string]any) *tools.Result {
			return d.toolsReg.Execute(ctx, name, args)
		}
	}

	if d.pgStores != nil {
		searchSvc := vault.NewVaultSearchService(d.pgStores.Vault, d.pgStores.Episodic, d.pgStores.KnowledgeGraph)
		r.KnowledgeSearch = func(ctx context.Context, opts vault.UnifiedSearchOptions) ([]vault.UnifiedSearchResult, error) {
			return searchSvc.Search(ctx, opts)
		}
	}

	return r
}
