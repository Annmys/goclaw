package handlers

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// TriggerHandler is the entry node. It passes its inputs through as the run's
// initial payload.
type TriggerHandler struct{}

func (TriggerHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeTrigger }
func (TriggerHandler) Execute(_ context.Context, _ graph.Block, inputs map[string]any) (Output, error) {
	return Output{Value: passthrough(inputs)}, nil
}

// ResponseHandler marks the workflow's final response. Its resolved inputs
// become the run output.
type ResponseHandler struct{}

func (ResponseHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeResponse }
func (ResponseHandler) Execute(_ context.Context, _ graph.Block, inputs map[string]any) (Output, error) {
	return Output{Value: passthrough(inputs), FinalResponse: true}, nil
}

// HumanHandler suspends the run for human input. inputs["prompt"] is shown to
// the user; inputs["fields"] (slice of strings) lists required fields.
type HumanHandler struct{}

func (HumanHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeHumanInTheLoop }
func (HumanHandler) Execute(_ context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	var missing []string
	if arr, ok := inputs["fields"].([]any); ok {
		for _, f := range arr {
			if s, ok := f.(string); ok {
				missing = append(missing, s)
			}
		}
	}
	prompt, _ := inputs["prompt"].(string)
	return Output{Pause: &PauseInfo{NodeID: b.ID, Reason: "human", Missing: missing, Prompt: prompt}}, nil
}

// PassthroughHandler is the fallback: it returns its inputs as output. Used for
// node types without a dedicated handler yet (variables, evaluator, …) so a
// graph still runs end-to-end during incremental rollout.
type PassthroughHandler struct{}

func (PassthroughHandler) CanHandle(graph.Block) bool { return true }
func (PassthroughHandler) Execute(_ context.Context, _ graph.Block, inputs map[string]any) (Output, error) {
	return Output{Value: passthrough(inputs)}, nil
}

func passthrough(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}
	return out
}

// BuildRegistry assembles the standard handler set. The runner backs the
// agent/tool/knowledge nodes; scope ids thread tenant/user/agent into knowledge
// retrieval. A PassthroughHandler is the fallback for not-yet-specialized types.
func BuildRegistry(r Runner, agentID, userID, tenantID string) *Registry {
	return NewRegistry(
		PassthroughHandler{}, // fallback
		TriggerHandler{},
		AgentHandler{R: r},
		ToolHandler{R: r},
		ConditionHandler{},
		RouterHandler{},
		FunctionHandler{},
		KnowledgeHandler{R: r, AgentID: agentID, UserID: userID, TenantID: tenantID},
		ResponseHandler{},
		HumanHandler{},
	)
}
