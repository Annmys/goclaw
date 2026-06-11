// Package dag executes a serialized workflow graph using a ready-frontier
// scheduler, mirroring sim's executor/execution/engine.ts.
//
// Scheduling model:
//   - A node becomes ready when all of its *active* incoming edges have been
//     satisfied (their source completed and the edge was activated).
//   - When a node completes, its outgoing edges are evaluated: each edge either
//     activates (its downstream gains a satisfied predecessor) or is pruned. A
//     pruned edge can leave its target unreachable; such targets are skipped.
//   - Ready nodes execute concurrently (goroutines); state mutations are
//     serialized under a single lock, matching sim's queue-lock model.
//
// This file covers acyclic control flow (trigger/agent/tool/condition/router/
// function/api/knowledge/response). Loop and parallel sentinels are layered on
// top in loops.go.
package dag

import (
	"context"
	"fmt"
	"sync"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/handlers"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/resolve"
)

// StepObserver is notified as nodes start and finish, so the engine can emit
// RunEvents / persist per-step state. All methods may be nil-safe no-ops.
type StepObserver interface {
	OnNodeStart(nodeID, nodeType string, inputs map[string]any)
	OnNodeComplete(nodeID, nodeType string, output map[string]any)
	OnNodeError(nodeID, nodeType string, err error)
}

// Result is the outcome of executing a graph.
type Result struct {
	// Output is the final workflow output (from a response node, or the last
	// completed node's output as a fallback).
	Output map[string]any
	// BlockOutputs is every node's output, keyed by block id.
	BlockOutputs map[string]map[string]any
	// Paused is set when the run suspended for human input.
	Paused *handlers.PauseInfo
	// CompletedOrder lists node ids in completion order (for audit/replay).
	CompletedOrder []string
}

// Executor runs one graph.
type Executor struct {
	g         *graph.Graph
	registry  *handlers.Registry
	observer  StepObserver
	variables map[string]any
	env       map[string]string

	// derived
	byID     map[string]*graph.Block
	nameToID map[string]string
	outgoing map[string][]graph.Connection
	indegree map[string]int // count of incoming edges per node

	// subflow indexing (loop/parallel membership)
	subflows    []*subflow
	nodeSubflow map[string]*subflow

	// mutable run state (guarded by mu)
	mu        sync.Mutex
	outputs   map[string]handlers.Output
	satisfied map[string]int    // satisfied incoming edges per node
	done      map[string]bool   // node completed
	skipped   map[string]bool   // node pruned (unreachable)
	completed []string          // completion order
	paused    *handlers.PauseInfo
	final     map[string]any
	runErr    error
}

// NewExecutor builds an executor for a validated graph.
func NewExecutor(g *graph.Graph, reg *handlers.Registry, opts ...Option) *Executor {
	e := &Executor{
		g:         g,
		registry:  reg,
		byID:      make(map[string]*graph.Block, len(g.Blocks)),
		nameToID:  make(map[string]string),
		outgoing:  make(map[string][]graph.Connection),
		indegree:  make(map[string]int),
		outputs:   make(map[string]handlers.Output),
		satisfied: make(map[string]int),
		done:      make(map[string]bool),
		skipped:   make(map[string]bool),
		final:     make(map[string]any),
	}
	for i := range g.Blocks {
		b := &g.Blocks[i]
		e.byID[b.ID] = b
		if b.Metadata != nil && b.Metadata.Name != "" {
			e.nameToID[b.Metadata.Name] = b.ID
		}
	}
	for _, c := range g.Connections {
		e.outgoing[c.Source] = append(e.outgoing[c.Source], c)
		e.indegree[c.Target]++
	}
	e.buildSubflows()
	for _, o := range opts {
		o(e)
	}
	return e
}

// Option configures an Executor.
type Option func(*Executor)

// WithObserver attaches a step observer.
func WithObserver(o StepObserver) Option { return func(e *Executor) { e.observer = o } }

// WithVariables sets workflow-level variables for reference resolution.
func WithVariables(v map[string]any) Option { return func(e *Executor) { e.variables = v } }

// WithEnv sets environment variables for {{ENV}} resolution.
func WithEnv(env map[string]string) Option { return func(e *Executor) { e.env = env } }

// Run executes the graph starting from all source nodes (indegree 0), or from
// the given trigger node if non-empty. It returns when the frontier drains, an
// error occurs, or the run pauses.
func (e *Executor) Run(ctx context.Context, triggerNodeID string) (*Result, error) {
	frontier := e.initialFrontier(triggerNodeID)
	if len(frontier) == 0 {
		return nil, fmt.Errorf("dag: no start node (trigger=%q)", triggerNodeID)
	}

	for len(frontier) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Split the frontier: subflow entries are driven as whole subflows; the
		// rest execute as ordinary concurrent nodes. A subflow is triggered by
		// its first entry node reaching the frontier; the entry is consumed by
		// the subflow driver.
		var plain []string
		var subRuns []*subflow
		seenSub := make(map[*subflow]bool)
		for _, id := range frontier {
			if sf := e.nodeSubflow[id]; sf != nil {
				if !seenSub[sf] {
					seenSub[sf] = true
					subRuns = append(subRuns, sf)
				}
				continue
			}
			plain = append(plain, id)
		}

		var next []string

		// Drive subflows (sequentially; each internally parallelizes branches).
		for _, sf := range subRuns {
			ready, err := e.runSubflow(ctx, sf)
			if err != nil {
				return nil, err
			}
			next = append(next, ready...)
		}
		e.mu.Lock()
		paused0 := e.paused
		e.mu.Unlock()
		if paused0 != nil {
			return e.buildResult(paused0), nil
		}

		// Execute the current frontier concurrently.
		type res struct {
			id  string
			out handlers.Output
			err error
		}
		results := make([]res, len(plain))
		var wg sync.WaitGroup
		for i, id := range plain {
			wg.Add(1)
			go func(i int, id string) {
				defer wg.Done()
				out, err := e.executeNode(ctx, id)
				results[i] = res{id, out, err}
			}(i, id)
		}
		wg.Wait()

		// Apply results serially; collect the next frontier.
		e.mu.Lock()
		for _, r := range results {
			if r.err != nil {
				e.runErr = fmt.Errorf("node %q: %w", r.id, r.err)
				e.mu.Unlock()
				return nil, e.runErr
			}
			e.outputs[r.id] = r.out
			e.done[r.id] = true
			e.completed = append(e.completed, r.id)
			if r.out.FinalResponse && r.out.Value != nil {
				e.final = r.out.Value
			}
			if r.out.Pause != nil {
				e.paused = r.out.Pause
			}
			ready := e.activateEdges(r.id, r.out)
			next = append(next, ready...)
		}
		paused := e.paused
		e.mu.Unlock()

		if paused != nil {
			return e.buildResult(paused), nil
		}
		frontier = dedupe(next)
	}

	return e.buildResult(nil), nil
}

// initialFrontier returns the set of nodes to start with.
func (e *Executor) initialFrontier(triggerNodeID string) []string {
	if triggerNodeID != "" {
		if _, ok := e.byID[triggerNodeID]; ok {
			return []string{triggerNodeID}
		}
		return nil
	}
	var starts []string
	for i := range e.g.Blocks {
		id := e.g.Blocks[i].ID
		if e.indegree[id] == 0 {
			starts = append(starts, id)
		}
	}
	return starts
}

// executeNode resolves inputs and runs the node's handler.
func (e *Executor) executeNode(ctx context.Context, id string) (handlers.Output, error) {
	b := e.byID[id]
	scope := &resolve.Scope{
		BlockOutputs:  e.collectOutputs(),
		BlockNameToID: e.nameToID,
		Variables:     e.variables,
		Env:           e.env,
	}
	inputs := scope.Params(b.Config.Params)

	// Trigger nodes with empty params should expose the run's input variables
	// as their output (so downstream nodes can reference <trigger.file_path> etc.)
	if b.Type() == "trigger" && len(inputs) == 0 && len(e.variables) > 0 {
		inputs = make(map[string]any, len(e.variables))
		for k, v := range e.variables {
			inputs[k] = v
		}
	}

	if e.observer != nil {
		e.observer.OnNodeStart(id, b.Type(), inputs)
	}

	h := e.registry.Find(*b)
	if h == nil {
		err := fmt.Errorf("no handler for node type %q", b.Type())
		if e.observer != nil {
			e.observer.OnNodeError(id, b.Type(), err)
		}
		return handlers.Output{}, err
	}

	out, err := h.Execute(ctx, *b, inputs)
	if err != nil {
		if e.observer != nil {
			e.observer.OnNodeError(id, b.Type(), err)
		}
		return handlers.Output{}, err
	}
	if e.observer != nil {
		e.observer.OnNodeComplete(id, b.Type(), out.Value)
	}
	return out, nil
}

// collectOutputs snapshots completed node outputs for reference resolution.
// Caller need not hold the lock during executeNode because reads happen before
// concurrent writes for that node's predecessors are all complete.
func (e *Executor) collectOutputs() map[string]map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]map[string]any, len(e.outputs))
	for id, o := range e.outputs {
		if o.Value != nil {
			out[id] = o.Value
		}
	}
	return out
}

// activateEdges evaluates a completed node's outgoing edges and returns nodes
// that became ready. Must be called with e.mu held.
func (e *Executor) activateEdges(srcID string, out handlers.Output) []string {
	var ready []string
	for _, c := range e.outgoing[srcID] {
		if !e.edgeActivates(c, out) {
			continue
		}
		e.satisfied[c.Target]++
		if e.satisfied[c.Target] >= e.indegree[c.Target] && !e.done[c.Target] && !e.skipped[c.Target] {
			ready = append(ready, c.Target)
		}
	}
	return ready
}

// edgeActivates decides whether a single outgoing edge fires, based on the
// source node's output and the edge's handle. Mirrors sim's
// EdgeManager.shouldActivateEdge.
func (e *Executor) edgeActivates(c graph.Connection, out handlers.Output) bool {
	switch {
	case c.SourceHandle == graph.HandleError:
		return out.Err != nil
	case c.SourceHandle == "" || c.SourceHandle == graph.HandleSource:
		// default success edge: fires unless the node errored
		return out.Err == nil
	}
	if id, ok := graph.ConditionID(c.SourceHandle); ok {
		return out.SelectedOption == id
	}
	if id, ok := graph.RouterID(c.SourceHandle); ok {
		return out.SelectedRoute == id
	}
	// loop/parallel control handles: fire when route matches
	switch c.SourceHandle {
	case graph.HandleLoopContinue, graph.HandleLoopExit,
		graph.HandleParallelContinue, graph.HandleParallelExit:
		return out.SelectedRoute == c.SourceHandle
	}
	return false
}

func (e *Executor) buildResult(paused *handlers.PauseInfo) *Result {
	e.mu.Lock()
	defer e.mu.Unlock()
	blockOutputs := make(map[string]map[string]any, len(e.outputs))
	for id, o := range e.outputs {
		if o.Value != nil {
			blockOutputs[id] = o.Value
		}
	}
	final := e.final
	if len(final) == 0 && len(e.completed) > 0 {
		// fallback: last completed node's output
		last := e.completed[len(e.completed)-1]
		final = blockOutputs[last]
	}
	return &Result{
		Output:         final,
		BlockOutputs:   blockOutputs,
		Paused:         paused,
		CompletedOrder: append([]string(nil), e.completed...),
	}
}

func dedupe(ids []string) []string {
	if len(ids) <= 1 {
		return ids
	}
	seen := make(map[string]struct{}, len(ids))
	out := ids[:0]
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
