package handlers

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/internal/vault"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// Runner is the subset of workflow.NodeRunner that node handlers depend on.
// It is declared here (rather than imported) so the handlers package does not
// import its parent; the engine adapts workflow.NodeRunner to this interface.
type Runner interface {
	HasAgent() bool
	HasTool() bool
	HasKnowledge() bool
	AgentRun(ctx context.Context, agentID string, req agent.RunRequest) (*agent.RunResult, error)
	ToolExec(ctx context.Context, name string, args map[string]any) *tools.Result
	KnowledgeSearch(ctx context.Context, opts vault.UnifiedSearchOptions) ([]vault.UnifiedSearchResult, error)
}

// AgentHandler runs an agent turn via the injected runner. The agent id comes
// from inputs["agent"] (or block config tool), the message from inputs["prompt"]
// / inputs["message"].
type AgentHandler struct{ R Runner }

func (AgentHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeAgent }

func (h AgentHandler) Execute(ctx context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	if h.R == nil || !h.R.HasAgent() {
		return Output{}, fmt.Errorf("agent node %q: agent runner not configured", b.ID)
	}
	agentID := firstString(inputs["agent"], inputs["agentId"], b.Config.Tool)
	if agentID == "" {
		return Output{}, fmt.Errorf("agent node %q: no agent id", b.ID)
	}
	msg := firstString(inputs["prompt"], inputs["message"], inputs["input"])
	req := agent.RunRequest{
		Message: msg,
		RunKind: "delegation",
	}
	if sp := firstString(inputs["systemPrompt"], inputs["system"]); sp != "" {
		req.ExtraSystemPrompt = sp
	}
	res, err := h.R.AgentRun(ctx, agentID, req)
	if err != nil {
		return Output{Err: err, Value: map[string]any{"error": err.Error()}}, nil
	}
	out := map[string]any{"content": res.Content, "iterations": res.Iterations}
	if res.Thinking != "" {
		out["thinking"] = res.Thinking
	}
	if res.Usage != nil {
		out["tokens"] = map[string]any{
			"prompt":     res.Usage.PromptTokens,
			"completion": res.Usage.CompletionTokens,
			"total":      res.Usage.TotalTokens,
		}
	}
	return Output{Value: out}, nil
}

// ToolHandler executes a registered goclaw tool via the runner. The tool name
// comes from block config tool (or inputs["tool"]); the remaining inputs are
// passed as args.
type ToolHandler struct{ R Runner }

func (ToolHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeTool }

func (h ToolHandler) Execute(ctx context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	if h.R == nil || !h.R.HasTool() {
		return Output{}, fmt.Errorf("tool node %q: tool runner not configured", b.ID)
	}
	name := b.Config.Tool
	if name == "" {
		name = firstString(inputs["tool"], inputs["name"])
	}
	if name == "" {
		return Output{}, fmt.Errorf("tool node %q: no tool name", b.ID)
	}
	args := make(map[string]any, len(inputs))
	for k, v := range inputs {
		if k == "tool" || k == "name" {
			continue
		}
		args[k] = v
	}
	res := h.R.ToolExec(ctx, name, args)
	if res == nil {
		return Output{}, fmt.Errorf("tool node %q: nil result", b.ID)
	}
	out := map[string]any{"content": res.ForLLM, "isError": res.IsError}
	if res.IsError {
		return Output{Value: out, Err: fmt.Errorf("tool %q error: %s", name, res.ForLLM)}, nil
	}
	return Output{Value: out}, nil
}

// KnowledgeHandler performs semantic retrieval via the vault search service.
// inputs["query"] is the search text; inputs["maxResults"] optionally caps hits.
type KnowledgeHandler struct {
	R        Runner
	AgentID  string
	UserID   string
	TenantID string
}

func (KnowledgeHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeKnowledge }

func (h KnowledgeHandler) Execute(ctx context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	if h.R == nil || !h.R.HasKnowledge() {
		return Output{}, fmt.Errorf("knowledge node %q: knowledge runner not configured", b.ID)
	}
	query := firstString(inputs["query"], inputs["q"], inputs["input"])
	if query == "" {
		return Output{}, fmt.Errorf("knowledge node %q: empty query", b.ID)
	}
	opts := vault.UnifiedSearchOptions{
		Query:    query,
		AgentID:  h.AgentID,
		UserID:   h.UserID,
		TenantID: h.TenantID,
	}
	if n, ok := toInt(inputs["maxResults"]); ok {
		opts.MaxResults = n
	}
	results, err := h.R.KnowledgeSearch(ctx, opts)
	if err != nil {
		return Output{Err: err, Value: map[string]any{"error": err.Error()}}, nil
	}
	hits := make([]any, 0, len(results))
	for _, r := range results {
		hits = append(hits, map[string]any{
			"id": r.ID, "title": r.Title, "path": r.Path,
			"source": r.Source, "score": r.Score, "snippet": r.Snippet,
		})
	}
	return Output{Value: map[string]any{"results": hits, "count": len(hits)}}, nil
}

func firstString(vals ...any) string {
	for _, v := range vals {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	default:
		return 0, false
	}
}
