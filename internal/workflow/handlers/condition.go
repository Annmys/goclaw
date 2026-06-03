package handlers

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// ConditionHandler evaluates an ordered list of conditions and selects the
// matching one. The selected option id is surfaced on Output.SelectedOption so
// the executor activates the "condition-{id}" edge. Mirrors sim's
// condition-handler.
//
// inputs["conditions"] is expected to be a slice of objects, each with:
//
//	{ "id": "<optionId>", "title": "if|else if|else", "value": "<jsExpr>" }
//
// The condition's JS expression is evaluated against the upstream block output
// (the merged inputs object is exposed as `context`). An "else" entry always
// matches.
type ConditionHandler struct{}

func (ConditionHandler) CanHandle(b graph.Block) bool { return b.Type() == graph.TypeCondition }

func (ConditionHandler) Execute(ctx context.Context, b graph.Block, inputs map[string]any) (Output, error) {
	conds := asObjectSlice(inputs["conditions"])
	for _, c := range conds {
		id, _ := c["id"].(string)
		title, _ := c["title"].(string)
		if title == "else" {
			return Output{Value: map[string]any{"selectedOption": id, "conditionResult": true}, SelectedOption: id}, nil
		}
		expr, _ := c["value"].(string)
		met, err := evalBool(ctx, expr, inputs)
		if err != nil {
			return Output{Err: err, Value: map[string]any{"error": err.Error()}}, nil
		}
		if met {
			return Output{Value: map[string]any{"selectedOption": id, "conditionResult": true}, SelectedOption: id}, nil
		}
	}
	// No condition matched: no edge selected (downstream stays unreached).
	return Output{Value: map[string]any{"conditionResult": false}}, nil
}

// asObjectSlice coerces a loosely-typed value into a slice of string-keyed maps.
func asObjectSlice(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
