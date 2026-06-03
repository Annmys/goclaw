package handlers

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// RouterHandler selects one of several named routes. The selected route id is
// surfaced on Output.SelectedRoute so the executor activates the "router-{id}"
// edge. Mirrors sim's router (v2, port-based).
//
// For deterministic routing, inputs["route"] may directly name the chosen route
// id. LLM-driven routing (sim's default) is layered later via the agent runner;
// for now an explicit route or the first route is taken.
type RouterHandler struct{}

func (RouterHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeRouter }

func (RouterHandler) Execute(_ context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	if r := firstString(inputs["route"], inputs["selectedRoute"]); r != "" {
		return Output{Value: map[string]any{"selectedRoute": r}, SelectedRoute: r}, nil
	}
	routes := asObjectSlice(inputs["routes"])
	if len(routes) > 0 {
		if id, _ := routes[0]["id"].(string); id != "" {
			return Output{Value: map[string]any{"selectedRoute": id}, SelectedRoute: id}, nil
		}
	}
	return Output{}, fmt.Errorf("router node %q: no route selected", b.ID)
}
