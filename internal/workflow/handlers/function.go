package handlers

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// FunctionHandler runs a user-provided JavaScript body and returns its result
// as the node output. The merged, reference-resolved inputs are exposed to the
// script as the global `context`. Mirrors sim's function-handler (which runs
// the code in an isolated VM via the function_execute tool).
//
// inputs["code"] holds the script body. The script should `return` a value;
// objects become the node's output map, scalars are wrapped as {"result": v}.
type FunctionHandler struct{}

func (FunctionHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeFunction }

func (FunctionHandler) Execute(ctx context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	code, _ := inputs["code"].(string)
	if code == "" {
		return Output{}, fmt.Errorf("function node %q: empty code", b.ID)
	}
	// Wrap so a top-level `return` is legal and the value is captured.
	body := "(function(){\n" + code + "\n})()"
	v, err := evalJS(ctx, body, inputs, nil)
	if err != nil {
		return Output{Err: err, Value: map[string]any{"error": err.Error()}}, nil
	}
	switch t := v.(type) {
	case map[string]any:
		return Output{Value: t}, nil
	default:
		return Output{Value: map[string]any{"result": v}}, nil
	}
}
