package dag

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/handlers"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/resolve"
)

// maxLoopIterations bounds unbounded while/doWhile loops as a safety net.
const maxLoopIterations = 10000

// runSubflowBody executes the member nodes of a subflow exactly once under the
// given loop or parallel scope. It is a nested sub-execution: member nodes form
// their own DAG (edges among members), started from the subflow entries, and
// run with the scope injected into reference resolution.
//
// Returns the merged output of the body's terminal nodes (the body's value for
// that iteration/branch).
func (e *Executor) runSubflowBody(ctx context.Context, sf *subflow, loop *resolve.LoopScope, par *resolve.ParallelScope) (map[string]any, error) {
	// Local per-iteration state, isolated from the parent execution so the same
	// member node can run again on the next iteration.
	localDone := make(map[string]bool)
	localSat := make(map[string]int)
	localOut := make(map[string]map[string]any)

	// in-subflow indegree for each member
	indeg := make(map[string]int)
	for _, c := range e.g.Connections {
		if sf.members[c.Source] && sf.members[c.Target] {
			indeg[c.Target]++
		}
	}

	frontier := append([]string(nil), sf.entries...)
	for len(frontier) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var next []string
		for _, id := range frontier {
			if localDone[id] {
				continue
			}
			b := e.byID[id]
			scope := &resolve.Scope{
				BlockOutputs:  e.mergedOutputs(localOut),
				BlockNameToID: e.nameToID,
				Variables:     e.variables,
				Env:           e.env,
				Loop:          loop,
				Parallel:      par,
			}
			inputs := scope.Params(b.Config.Params)
			if e.observer != nil {
				e.observer.OnNodeStart(id, b.Type(), inputs)
			}
			h := e.registry.Find(*b)
			if h == nil {
				return nil, fmt.Errorf("no handler for node type %q", b.Type())
			}
			out, err := h.Execute(ctx, *b, inputs)
			if err != nil {
				if e.observer != nil {
					e.observer.OnNodeError(id, b.Type(), err)
				}
				return nil, fmt.Errorf("node %q: %w", id, err)
			}
			if e.observer != nil {
				e.observer.OnNodeComplete(id, b.Type(), out.Value)
			}
			localDone[id] = true
			if out.Value != nil {
				localOut[id] = out.Value
			}
			e.recordCompletion(id, out)

			// activate in-subflow edges
			for _, c := range e.outgoing[id] {
				if !sf.members[c.Target] {
					continue
				}
				if !e.edgeActivates(c, out) {
					continue
				}
				localSat[c.Target]++
				if localSat[c.Target] >= indeg[c.Target] && !localDone[c.Target] {
					next = append(next, c.Target)
				}
			}
		}
		frontier = dedupe(next)
	}

	// Body value = merged outputs of member terminal nodes (no in-subflow successor).
	merged := map[string]any{}
	for id, ov := range localOut {
		if e.isSubflowTerminal(sf, id) {
			for k, v := range ov {
				merged[k] = v
			}
		}
	}
	return merged, nil
}

// recordCompletion appends to the global audit trail under the lock. Per-iteration
// node outputs are NOT stored globally (they would collide across iterations);
// only the subflow's aggregate result is stored on exit.
func (e *Executor) recordCompletion(id string, out handlers.Output) {
	e.mu.Lock()
	e.completed = append(e.completed, id)
	if out.FinalResponse && out.Value != nil {
		e.final = out.Value
	}
	e.mu.Unlock()
}

// mergedOutputs combines globally completed outputs with this iteration's local
// outputs so body nodes can reference both upstream (pre-loop) and sibling
// (in-iteration) blocks.
func (e *Executor) mergedOutputs(local map[string]map[string]any) map[string]map[string]any {
	e.mu.Lock()
	out := make(map[string]map[string]any, len(e.outputs)+len(local))
	for id, o := range e.outputs {
		if o.Value != nil {
			out[id] = o.Value
		}
	}
	e.mu.Unlock()
	for id, v := range local {
		out[id] = v
	}
	return out
}

// isSubflowTerminal reports whether a member has no successor inside the subflow.
func (e *Executor) isSubflowTerminal(sf *subflow, id string) bool {
	for _, c := range e.outgoing[id] {
		if sf.members[c.Target] {
			return false
		}
	}
	return true
}

// exitSubflow records the subflow's aggregate output under its representative id
// and returns downstream nodes made ready by its exit edges. The subflow's
// output is stored under each member's id so downstream <member.field> refs and
// loop/parallel exit edges resolve. The exit edges originate from member
// terminal nodes via loop_exit / parallel_exit handles.
func (e *Executor) exitSubflow(sf *subflow, aggregate map[string]any, exitHandle string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var ready []string
	for id := range sf.members {
		e.done[id] = true
		e.outputs[id] = handlers.Output{Value: aggregate, SelectedRoute: exitHandle}
		for _, c := range e.outgoing[id] {
			if sf.members[c.Target] {
				continue // internal edge, already handled
			}
			// Exit edges use the loop_exit/parallel_exit handle; default edges
			// from a terminal member also flow downstream.
			fire := c.SourceHandle == exitHandle ||
				c.SourceHandle == "" || c.SourceHandle == graph.HandleSource
			if !fire {
				continue
			}
			e.satisfied[c.Target]++
			if e.satisfied[c.Target] >= e.indegree[c.Target] && !e.done[c.Target] && !e.skipped[c.Target] {
				ready = append(ready, c.Target)
			}
		}
	}
	return ready
}

// resolveItems coerces a forEach/distribution value into a slice. It accepts a
// literal slice, or a reference string already resolved upstream, or a JSON
// array string.
func (e *Executor) resolveItems(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case string:
		// resolve references against current global outputs
		scope := &resolve.Scope{
			BlockOutputs:  e.mergedOutputs(nil),
			BlockNameToID: e.nameToID,
			Variables:     e.variables,
			Env:           e.env,
		}
		resolved := scope.Value(t)
		if arr, ok := resolved.([]any); ok {
			return arr
		}
		// try JSON array
		if s, ok := resolved.(string); ok {
			var arr []any
			if json.Unmarshal([]byte(s), &arr) == nil {
				return arr
			}
		}
		return nil
	default:
		return nil
	}
}

// evalLoopCondition evaluates a while/doWhile JS condition with the loop index
// and prior iteration results exposed under context.loop.
func (e *Executor) evalLoopCondition(ctx context.Context, expr string, index int, prior []any) (bool, error) {
	if expr == "" {
		return false, fmt.Errorf("empty loop condition")
	}
	contextObj := map[string]any{
		"loop": map[string]any{"index": index, "results": prior},
	}
	// merge global outputs so conditions can reference upstream blocks
	for id, o := range e.mergedOutputs(nil) {
		contextObj[id] = o
	}
	return handlers.EvalBoolPublic(ctx, expr, contextObj)
}
