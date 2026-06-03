package dag

import (
	"context"
	"reflect"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/handlers"
)

// stubHandler returns a preset Output for any block. Used to exercise
// scheduling/edge logic in isolation from real node implementations.
type stubHandler struct {
	fn func(b graph.Block, inputs map[string]any) handlers.Output
}

func (s stubHandler) CanHandle(graph.Block) bool { return true }
func (s stubHandler) Execute(_ context.Context, b graph.Block, inputs map[string]any) (handlers.Output, error) {
	return s.fn(b, inputs), nil
}

func blk(id, typ, name string) graph.Block {
	return graph.Block{ID: id, Enabled: true, Metadata: &graph.BlockMetadata{ID: typ, Name: name}}
}

func TestLinearFlow(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks:  []graph.Block{blk("t", "trigger", "T"), blk("a", "agent", "A"), blk("b", "response", "B")},
		Connections: []graph.Connection{
			{Source: "t", Target: "a", SourceHandle: graph.HandleSource},
			{Source: "a", Target: "b", SourceHandle: graph.HandleSource},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, _ map[string]any) handlers.Output {
		out := handlers.Output{Value: map[string]any{"from": b.ID}}
		if b.ID == "b" {
			out.FinalResponse = true
		}
		return out
	}})
	res, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.CompletedOrder, []string{"t", "a", "b"}) {
		t.Fatalf("order = %v, want [t a b]", res.CompletedOrder)
	}
	if res.Output["from"] != "b" {
		t.Fatalf("final = %v, want from=b", res.Output)
	}
}

func TestConditionBranch(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks: []graph.Block{
			blk("c", "condition", "C"), blk("yes", "agent", "Yes"), blk("no", "agent", "No"),
		},
		Connections: []graph.Connection{
			{Source: "c", Target: "yes", SourceHandle: "condition-opt1"},
			{Source: "c", Target: "no", SourceHandle: "condition-opt2"},
		},
	}
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, _ map[string]any) handlers.Output {
		if b.ID == "c" {
			return handlers.Output{Value: map[string]any{}, SelectedOption: "opt1"}
		}
		return handlers.Output{Value: map[string]any{"ran": b.ID}}
	}})
	res, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.BlockOutputs["yes"]; !ok {
		t.Fatalf("expected 'yes' branch to run; got %v", res.CompletedOrder)
	}
	if _, ok := res.BlockOutputs["no"]; ok {
		t.Fatalf("'no' branch should NOT run; got %v", res.CompletedOrder)
	}
}

func TestRouterBranch(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks:  []graph.Block{blk("r", "router", "R"), blk("p1", "agent", "P1"), blk("p2", "agent", "P2")},
		Connections: []graph.Connection{
			{Source: "r", Target: "p1", SourceHandle: "router-r1"},
			{Source: "r", Target: "p2", SourceHandle: "router-r2"},
		},
	}
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, _ map[string]any) handlers.Output {
		if b.ID == "r" {
			return handlers.Output{Value: map[string]any{}, SelectedRoute: "r2"}
		}
		return handlers.Output{Value: map[string]any{"ran": b.ID}}
	}})
	res, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.BlockOutputs["p2"]; !ok {
		t.Fatalf("expected p2 to run; got %v", res.CompletedOrder)
	}
	if _, ok := res.BlockOutputs["p1"]; ok {
		t.Fatalf("p1 should not run; got %v", res.CompletedOrder)
	}
}

func TestErrorEdge(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks:  []graph.Block{blk("a", "tool", "A"), blk("ok", "agent", "OK"), blk("handler", "agent", "H")},
		Connections: []graph.Connection{
			{Source: "a", Target: "ok", SourceHandle: graph.HandleSource},
			{Source: "a", Target: "handler", SourceHandle: graph.HandleError},
		},
	}
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, _ map[string]any) handlers.Output {
		if b.ID == "a" {
			return handlers.Output{Value: map[string]any{}, Err: context.Canceled}
		}
		return handlers.Output{Value: map[string]any{"ran": b.ID}}
	}})
	res, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.BlockOutputs["handler"]; !ok {
		t.Fatalf("error edge should route to handler; got %v", res.CompletedOrder)
	}
	if _, ok := res.BlockOutputs["ok"]; ok {
		t.Fatalf("success edge should NOT fire on error; got %v", res.CompletedOrder)
	}
}

func TestDiamondJoin(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks: []graph.Block{
			blk("t", "trigger", "T"), blk("a", "agent", "A"), blk("b", "agent", "B"), blk("j", "agent", "J"),
		},
		Connections: []graph.Connection{
			{Source: "t", Target: "a", SourceHandle: graph.HandleSource},
			{Source: "t", Target: "b", SourceHandle: graph.HandleSource},
			{Source: "a", Target: "j", SourceHandle: graph.HandleSource},
			{Source: "b", Target: "j", SourceHandle: graph.HandleSource},
		},
	}
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, _ map[string]any) handlers.Output {
		return handlers.Output{Value: map[string]any{"ran": b.ID}}
	}})
	res, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.CompletedOrder[len(res.CompletedOrder)-1] != "j" {
		t.Fatalf("j should complete last; order = %v", res.CompletedOrder)
	}
	if len(res.CompletedOrder) != 4 {
		t.Fatalf("all 4 nodes should run once; order = %v", res.CompletedOrder)
	}
}

func TestPause(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks:  []graph.Block{blk("t", "trigger", "T"), blk("h", "human-in-the-loop", "H"), blk("after", "agent", "After")},
		Connections: []graph.Connection{
			{Source: "t", Target: "h", SourceHandle: graph.HandleSource},
			{Source: "h", Target: "after", SourceHandle: graph.HandleSource},
		},
	}
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, _ map[string]any) handlers.Output {
		if b.ID == "h" {
			return handlers.Output{Pause: &handlers.PauseInfo{NodeID: "h", Reason: "human", Missing: []string{"approval"}}}
		}
		return handlers.Output{Value: map[string]any{"ran": b.ID}}
	}})
	res, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Paused == nil || res.Paused.NodeID != "h" {
		t.Fatalf("expected pause at h; got %+v", res.Paused)
	}
	if _, ok := res.BlockOutputs["after"]; ok {
		t.Fatalf("'after' must not run while paused; got %v", res.CompletedOrder)
	}
}
