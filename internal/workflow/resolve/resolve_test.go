package resolve

import (
	"reflect"
	"testing"
)

func baseScope() *Scope {
	return &Scope{
		BlockOutputs: map[string]map[string]any{
			"blk1": {"content": "hello", "count": float64(3), "nested": map[string]any{"k": "v"}},
			"blk2": {"items": []any{"a", "b", "c"}},
		},
		BlockNameToID: map[string]string{"Agent 1": "blk1", "Parser": "blk2"},
		Variables:     map[string]any{"threshold": float64(10)},
		Env:           map[string]string{"API_KEY": "secret"},
	}
}

func TestResolveString(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"whole-ref typed string", "<blk1.content>", "hello"},
		{"whole-ref typed number", "<blk1.count>", float64(3)},
		{"whole-ref by name", "<Agent 1.content>", "hello"},
		{"nested path", "<blk1.nested.k>", "v"},
		{"slice index", "<blk2.items.1>", "b"},
		{"embedded ref stringified", "msg: <blk1.content>!", "msg: hello!"},
		{"embedded number", "n=<blk1.count>", "n=3"},
		{"env ref", "key={{API_KEY}}", "key=secret"},
		{"variable ref", "<variable.threshold>", float64(10)},
		{"unresolved left intact", "<nope.field>", "<nope.field>"},
		{"unresolved env intact", "{{MISSING}}", "{{MISSING}}"},
		{"non-string passthrough", float64(42), float64(42)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := baseScope().Value(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Value(%v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveLoopScope(t *testing.T) {
	s := baseScope()
	s.Loop = &LoopScope{Index: 2, Item: map[string]any{"name": "x"}}
	if got := s.Value("<loop.index>"); got != 2 {
		t.Fatalf("loop.index = %#v, want 2", got)
	}
	if got := s.Value("<loop.item.name>"); got != "x" {
		t.Fatalf("loop.item.name = %#v, want x", got)
	}
	// loop ref with no active scope stays intact
	s2 := baseScope()
	if got := s2.Value("<loop.index>"); got != "<loop.index>" {
		t.Fatalf("loop.index without scope = %#v, want intact", got)
	}
}

func TestResolveParallelScope(t *testing.T) {
	s := baseScope()
	s.Parallel = &ParallelScope{Index: 1, CurrentItem: "branch-item"}
	if got := s.Value("<parallel.index>"); got != 1 {
		t.Fatalf("parallel.index = %#v, want 1", got)
	}
	if got := s.Value("<parallel.currentItem>"); got != "branch-item" {
		t.Fatalf("parallel.currentItem = %#v, want branch-item", got)
	}
}

func TestParamsRecursive(t *testing.T) {
	s := baseScope()
	in := map[string]any{
		"top":    "<blk1.content>",
		"nested": map[string]any{"inner": "<blk1.count>"},
		"list":   []any{"<blk2.items.0>", "literal"},
	}
	got := s.Params(in)
	want := map[string]any{
		"top":    "hello",
		"nested": map[string]any{"inner": float64(3)},
		"list":   []any{"a", "literal"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Params = %#v, want %#v", got, want)
	}
	// input must be untouched
	if in["top"] != "<blk1.content>" {
		t.Fatalf("Params mutated input")
	}
}
