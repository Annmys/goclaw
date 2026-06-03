package dag

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/workflow/graph"
	"github.com/nextlevelbuilder/goclaw/internal/workflow/handlers"
)

// countingHandler records how many times each block executed and accumulates a
// per-block call count into outputs.
func countingHandler(counts map[string]int) handlers.Handler {
	return stubHandler{func(b graph.Block, inputs map[string]any) handlers.Output {
		counts[b.ID]++
		return handlers.Output{Value: map[string]any{"ran": b.ID, "n": counts[b.ID]}}
	}}
}

// for-loop: body node "body" inside loop "L" with iterations=3 runs 3 times.
func TestForLoop(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks: []graph.Block{
			blk("t", "trigger", "T"), blk("body", "agent", "Body"), blk("after", "response", "After"),
		},
		Connections: []graph.Connection{
			{Source: "t", Target: "body", SourceHandle: graph.HandleSource},
			{Source: "body", Target: "after", SourceHandle: graph.HandleLoopExit},
		},
		Loops: map[string]graph.Loop{
			"L": {ID: "L", Nodes: []string{"body"}, LoopType: graph.LoopFor, Iterations: 3},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	reg := handlers.NewRegistry(countingHandler(counts))
	if _, err := NewExecutor(g, reg).Run(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if counts["body"] != 3 {
		t.Fatalf("body ran %d times, want 3", counts["body"])
	}
	if counts["after"] != 1 {
		t.Fatalf("after ran %d times, want 1", counts["after"])
	}
}

// forEach-loop: body runs once per item; <loop.item> resolves to the item.
func TestForEachLoop(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks: []graph.Block{
			blk("t", "trigger", "T"), blk("body", "agent", "Body"),
		},
		Connections: []graph.Connection{
			{Source: "t", Target: "body", SourceHandle: graph.HandleSource},
		},
		Loops: map[string]graph.Loop{
			"L": {ID: "L", Nodes: []string{"body"}, LoopType: graph.LoopForEach,
				ForEachItems: []any{"x", "y"}},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	var seenItems []any
	reg := handlers.NewRegistry(stubHandler{func(b graph.Block, inputs map[string]any) handlers.Output {
		if b.ID == "body" {
			seenItems = append(seenItems, inputs["item"])
		}
		return handlers.Output{Value: map[string]any{"ran": b.ID}}
	}})
	// body reads <loop.item> via a param
	g.Blocks[1].Config.Params = map[string]any{"item": "<loop.item>"}
	_, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(seenItems) != 2 || seenItems[0] != "x" || seenItems[1] != "y" {
		t.Fatalf("forEach items = %v, want [x y]", seenItems)
	}
}

// while-loop: runs while a JS condition over context holds. Condition counts
// down a counter exposed via loop index.
func TestWhileLoop(t *testing.T) {
	g := &graph.Graph{
		Version: graph.Version,
		Blocks: []graph.Block{
			blk("t", "trigger", "T"), blk("body", "agent", "Body"),
		},
		Connections: []graph.Connection{
			{Source: "t", Target: "body", SourceHandle: graph.HandleSource},
		},
		Loops: map[string]graph.Loop{
			// loop while index < 2  => runs at index 0 and 1 => 2 iterations
			"L": {ID: "L", Nodes: []string{"body"}, LoopType: graph.LoopWhile,
				WhileCondition: "context.loop.index < 2"},
		},
	}
	counts := map[string]int{}
	reg := handlers.NewRegistry(countingHandler(counts))
	_, err := NewExecutor(g, reg).Run(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if counts["body"] != 2 {
		t.Fatalf("while body ran %d times, want 2", counts["body"])
	}
}
