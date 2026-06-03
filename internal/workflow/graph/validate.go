package graph

import (
	"fmt"
	"strings"
)

// Validate checks structural integrity of a graph: unique block ids, edge
// endpoints resolve to real blocks, loop/parallel node references resolve, and
// the non-loop edges contain no cycles (loop back-edges are excluded from the
// acyclicity check because they are intentional).
//
// It returns the first error found. A nil error means the graph is safe to
// hand to the executor.
func (g *Graph) Validate() error {
	if g == nil {
		return fmt.Errorf("graph: nil")
	}
	if len(g.Blocks) == 0 {
		return fmt.Errorf("graph: no blocks")
	}

	// Unique block ids + type presence.
	ids := make(map[string]struct{}, len(g.Blocks))
	for i := range g.Blocks {
		b := &g.Blocks[i]
		if b.ID == "" {
			return fmt.Errorf("graph: block at index %d has empty id", i)
		}
		if _, dup := ids[b.ID]; dup {
			return fmt.Errorf("graph: duplicate block id %q", b.ID)
		}
		ids[b.ID] = struct{}{}
		if b.Type() == "" {
			return fmt.Errorf("graph: block %q has no type (metadata.id)", b.ID)
		}
	}

	// Edge endpoints must resolve.
	for i, c := range g.Connections {
		if _, ok := ids[c.Source]; !ok {
			return fmt.Errorf("graph: connection %d source %q is not a block", i, c.Source)
		}
		if _, ok := ids[c.Target]; !ok {
			return fmt.Errorf("graph: connection %d target %q is not a block", i, c.Target)
		}
	}

	// Loop / parallel node references must resolve.
	for id, l := range g.Loops {
		for _, n := range l.Nodes {
			if _, ok := ids[n]; !ok {
				return fmt.Errorf("graph: loop %q references unknown block %q", id, n)
			}
		}
	}
	for id, p := range g.Parallels {
		for _, n := range p.Nodes {
			if _, ok := ids[n]; !ok {
				return fmt.Errorf("graph: parallel %q references unknown block %q", id, n)
			}
		}
	}

	// Acyclicity over forward edges only. Loop back-edges (loop_continue) and
	// parallel re-entry (parallel_continue) are intentional cycles, excluded.
	if err := g.checkAcyclic(); err != nil {
		return err
	}
	return nil
}

// isBackEdge reports whether a connection is an intentional control-flow
// back-edge that should be excluded from the acyclicity check.
func isBackEdge(c Connection) bool {
	switch c.SourceHandle {
	case HandleLoopContinue, HandleParallelContinue:
		return true
	default:
		return false
	}
}

// checkAcyclic runs a DFS three-color cycle detection over forward edges.
func (g *Graph) checkAcyclic() error {
	adj := make(map[string][]string, len(g.Blocks))
	for _, c := range g.Connections {
		if isBackEdge(c) {
			continue
		}
		adj[c.Source] = append(adj[c.Source], c.Target)
	}

	const (
		white = 0 // unvisited
		gray  = 1 // on current DFS stack
		black = 2 // fully explored
	)
	color := make(map[string]int, len(g.Blocks))

	var stack []string
	var visit func(node string) error
	visit = func(node string) error {
		color[node] = gray
		stack = append(stack, node)
		for _, next := range adj[node] {
			switch color[next] {
			case gray:
				return fmt.Errorf("graph: cycle detected through %q -> %q (%s)",
					node, next, strings.Join(append(append([]string{}, stack...), next), " -> "))
			case white:
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[node] = black
		return nil
	}

	for i := range g.Blocks {
		id := g.Blocks[i].ID
		if color[id] == white {
			if err := visit(id); err != nil {
				return err
			}
		}
	}
	return nil
}

// ConditionID returns the condition option id encoded in a "condition-{id}"
// handle, and whether the handle is a condition handle.
func ConditionID(handle string) (string, bool) {
	if strings.HasPrefix(handle, HandleConditionPrefix) {
		return strings.TrimPrefix(handle, HandleConditionPrefix), true
	}
	return "", false
}

// RouterID returns the route id encoded in a "router-{id}" handle, and whether
// the handle is a router handle.
func RouterID(handle string) (string, bool) {
	if strings.HasPrefix(handle, HandleRouterPrefix) {
		return strings.TrimPrefix(handle, HandleRouterPrefix), true
	}
	return "", false
}
