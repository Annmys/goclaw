package dag

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/resolve"
)

// subflow groups loop/parallel membership info derived from the graph.
type subflow struct {
	loop     *graph.Loop
	parallel *graph.Parallel
	members  map[string]bool // member block ids
	entries  []string        // member nodes with no in-subflow predecessor
}

// buildSubflows indexes loop/parallel definitions into membership maps and
// computes each subflow's entry nodes (members whose incoming edges all come
// from outside the subflow).
func (e *Executor) buildSubflows() {
	e.subflows = nil
	e.nodeSubflow = make(map[string]*subflow)

	add := func(sf *subflow, nodes []string) {
		sf.members = make(map[string]bool, len(nodes))
		for _, n := range nodes {
			sf.members[n] = true
			e.nodeSubflow[n] = sf
		}
		for _, n := range nodes {
			hasInternalPred := false
			for _, c := range e.g.Connections {
				if c.Target == n && sf.members[c.Source] {
					hasInternalPred = true
					break
				}
			}
			if !hasInternalPred {
				sf.entries = append(sf.entries, n)
			}
		}
		e.subflows = append(e.subflows, sf)
	}

	for id := range e.g.Loops {
		l := e.g.Loops[id]
		add(&subflow{loop: &l}, l.Nodes)
	}
	for id := range e.g.Parallels {
		p := e.g.Parallels[id]
		add(&subflow{parallel: &p}, p.Nodes)
	}

	// Recompute global indegree for subflow members to count only EXTERNAL
	// incoming edges. Internal edges are driven inside the subflow body, so they
	// must not gate the subflow's global readiness.
	for _, sf := range e.subflows {
		for member := range sf.members {
			ext := 0
			for _, c := range e.g.Connections {
				if c.Target == member && !sf.members[c.Source] {
					ext++
				}
			}
			e.indegree[member] = ext
		}
	}
}

// runSubflow executes one loop or parallel subflow to completion, driving all
// iterations/branches, then returns the exit edges to activate downstream.
//
// It returns the aggregated output (exposed as the subflow's value) and the
// set of nodes that become ready after the subflow exits.
func (e *Executor) runSubflow(ctx context.Context, sf *subflow) ([]string, error) {
	if sf.loop != nil {
		return e.runLoop(ctx, sf)
	}
	return e.runParallel(ctx, sf)
}

// runLoop drives for/forEach/while/doWhile iteration over the subflow members.
func (e *Executor) runLoop(ctx context.Context, sf *subflow) ([]string, error) {
	l := sf.loop
	var iterResults []any

	iterate := func(index int, item any) error {
		scope := &resolve.LoopScope{Index: index, Item: item}
		out, err := e.runSubflowBody(ctx, sf, scope, nil)
		if err != nil {
			return err
		}
		iterResults = append(iterResults, out)
		return nil
	}

	switch l.LoopType {
	case graph.LoopForEach:
		items := e.resolveItems(l.ForEachItems)
		for i, it := range items {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := iterate(i, it); err != nil {
				return nil, err
			}
		}

	case graph.LoopWhile:
		for i := 0; ; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			ok, err := e.evalLoopCondition(ctx, l.WhileCondition, i, iterResults)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			if err := iterate(i, nil); err != nil {
				return nil, err
			}
			if i > maxLoopIterations {
				return nil, fmt.Errorf("loop %q exceeded %d iterations", l.ID, maxLoopIterations)
			}
		}

	case graph.LoopDoWhile:
		for i := 0; ; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := iterate(i, nil); err != nil {
				return nil, err
			}
			ok, err := e.evalLoopCondition(ctx, l.DoWhileCondition, i+1, iterResults)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			if i > maxLoopIterations {
				return nil, fmt.Errorf("loop %q exceeded %d iterations", l.ID, maxLoopIterations)
			}
		}

	default: // graph.LoopFor
		n := l.Iterations
		for i := 0; i < n; i++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := iterate(i, nil); err != nil {
				return nil, err
			}
		}
	}

	return e.exitSubflow(sf, map[string]any{"results": iterResults, "iterations": len(iterResults)}, graph.HandleLoopExit), nil
}

// runParallel drives count/collection parallel branches with optional batching.
func (e *Executor) runParallel(ctx context.Context, sf *subflow) ([]string, error) {
	p := sf.parallel
	var items []any
	if p.ParallelType == graph.ParallelCollection {
		items = e.resolveItems(p.Distribution)
	} else { // count
		n := p.Count
		items = make([]any, n)
	}

	batch := p.BatchSize
	if batch <= 0 {
		batch = len(items)
	}
	if batch <= 0 {
		batch = 1
	}

	branchResults := make([]any, len(items))
	for start := 0; start < len(items); start += batch {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := start + batch
		if end > len(items) {
			end = len(items)
		}
		// Run this batch's branches concurrently.
		type br struct {
			idx int
			out map[string]any
			err error
		}
		results := make([]br, end-start)
		errCh := make(chan error, end-start)
		done := make(chan struct{})
		var pending int
		for i := start; i < end; i++ {
			pending++
			go func(i int) {
				scope := &resolve.ParallelScope{Index: i, CurrentItem: items[i]}
				out, err := e.runSubflowBody(ctx, sf, nil, scope)
				results[i-start] = br{i, out, err}
				errCh <- err
			}(i)
		}
		go func() {
			for k := 0; k < pending; k++ {
				<-errCh
			}
			close(done)
		}()
		<-done
		for _, r := range results {
			if r.err != nil {
				return nil, r.err
			}
			branchResults[r.idx] = r.out
		}
	}

	return e.exitSubflow(sf, map[string]any{"results": branchResults, "branches": len(items)}, graph.HandleParallelExit), nil
}
