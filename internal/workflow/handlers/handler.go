// Package handlers defines the node-handler contract and the per-node-type
// implementations for the workflow DAG executor.
//
// The contract mirrors sim's executor BlockHandler: a handler decides whether
// it can handle a block, then executes it given already-resolved inputs and
// returns a normalized Output. Control-flow signaling (which outgoing edge to
// take) is carried on the Output via SelectedOption (condition) and
// SelectedRoute (router/loop/parallel), matching sim's edge-activation model.
package handlers

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
)

// Output is the normalized result of executing a node.
type Output struct {
	// Value is the node's output object, exposed to downstream nodes as the
	// block's output (referenced via <blockName.field>).
	Value map[string]any

	// SelectedOption is set by condition nodes to the matched option id; the
	// executor activates the outgoing edge whose handle is "condition-{id}".
	SelectedOption string

	// SelectedRoute is set by router nodes (route id) and by loop/parallel
	// sentinels (loop_continue/loop_exit/…); the executor activates the edge
	// whose handle equals this route.
	SelectedRoute string

	// Err marks a node-level error. When set and an outgoing "error" edge
	// exists, the executor takes the error edge instead of failing the run.
	Err error

	// Pause, when non-nil, suspends the run for human input (see PauseInfo).
	Pause *PauseInfo

	// FinalResponse marks this output as the workflow's final response
	// (response node), surfaced as the run's output.
	FinalResponse bool
}

// PauseInfo describes a human-in-the-loop / wait suspension point.
type PauseInfo struct {
	NodeID  string
	Reason  string // "human" | "wait"
	Missing []string
	Prompt  string
}

// Handler executes a single node type.
type Handler interface {
	// CanHandle reports whether this handler serves the given block's type.
	CanHandle(b graph.Block) bool
	// Execute runs the node with already-resolved inputs.
	Execute(ctx context.Context, b graph.Block, inputs map[string]any) (Output, error)
}

// Registry maps node types to handlers (first CanHandle match wins, with a
// fallback generic handler).
type Registry struct {
	handlers []Handler
	fallback Handler
}

// NewRegistry builds a handler registry from an ordered list, plus an optional
// fallback used when no handler matches.
func NewRegistry(fallback Handler, hs ...Handler) *Registry {
	return &Registry{handlers: hs, fallback: fallback}
}

// Find returns the handler for a block, or the fallback (possibly nil).
func (r *Registry) Find(b graph.Block) Handler {
	for _, h := range r.handlers {
		if h.CanHandle(b) {
			return h
		}
	}
	return r.fallback
}
