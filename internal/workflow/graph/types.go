// Package graph defines the serialized workflow graph format.
//
// The schema mirrors sim's (sim.ai) serializer/types.ts closed schema so that
// graphs authored in the goclaw canvas and graphs exported from sim are
// structurally identical. Field names and the set of well-known handle/type
// strings are kept aligned with sim on purpose — see constants.go for the
// strings that enumerate them.
package graph

import "encoding/json"

// Version is the current graph schema version emitted by the canvas.
const Version = "1.0"

// Graph is the top-level serialized workflow. It is the on-the-wire and
// at-rest representation (stored as graph_json JSONB in workflow_definitions).
type Graph struct {
	Version     string              `json:"version"`
	Blocks      []Block             `json:"blocks"`
	Connections []Connection        `json:"connections"`
	Loops       map[string]Loop     `json:"loops,omitempty"`
	Parallels   map[string]Parallel `json:"parallels,omitempty"`
}

// Block is a single node on the canvas.
//
// NOTE: Block.ID is the unique instance id of this node in the graph.
// The node TYPE (agent/function/condition/...) lives in Metadata.ID, matching
// sim's convention where metadata.id carries the block type.
type Block struct {
	ID       string                     `json:"id"`
	Position Position                   `json:"position"`
	Config   BlockConfig                `json:"config"`
	Inputs   map[string]json.RawMessage `json:"inputs,omitempty"`
	Outputs  map[string]json.RawMessage `json:"outputs,omitempty"`
	Metadata *BlockMetadata             `json:"metadata,omitempty"`
	Enabled  bool                       `json:"enabled"`
	// CanonicalModes records per-field UI mode ("basic"|"advanced"); opaque to
	// the engine, preserved for round-tripping with the canvas.
	CanonicalModes map[string]string `json:"canonicalModes,omitempty"`
}

// Type returns the node type (agent/function/condition/...), read from
// Metadata.ID. Returns "" when metadata is absent.
func (b Block) Type() string {
	if b.Metadata == nil {
		return ""
	}
	return b.Metadata.ID
}

// BlockConfig holds the tool selection and user-provided params for a block.
// Params values may contain <ref> reference strings resolved at execution time.
type BlockConfig struct {
	Tool   string         `json:"tool,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// BlockMetadata carries display + type metadata. Metadata.ID is the block type.
type BlockMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
}

// Position is the canvas coordinate of a block (engine-irrelevant, preserved
// for the canvas).
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Connection is a directed edge between two blocks.
//
// SourceHandle encodes edge semantics (see constants.go):
//   - "source"            default success edge
//   - "error"             error edge (taken when the source block errors)
//   - "condition-{id}"    condition-block branch, taken when output.SelectedOption == id
//   - "router-{id}"       router-block branch, taken when output.SelectedRoute == id
//   - "loop_continue" / "loop_exit"         loop control flow
//   - "parallel_continue" / "parallel_exit" parallel control flow
type Connection struct {
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	SourceHandle string         `json:"sourceHandle,omitempty"`
	TargetHandle string         `json:"targetHandle,omitempty"`
	Condition    *EdgeCondition `json:"condition,omitempty"`
}

// EdgeCondition is an optional JS-expression guard on a connection
// (condition-block edges). Type is one of "if" | "else if" | "else".
type EdgeCondition struct {
	Type       string `json:"type"`
	Expression string `json:"expression,omitempty"`
}

// Loop describes a loop subflow. Nodes are the base block IDs inside the loop.
type Loop struct {
	ID               string   `json:"id"`
	Nodes            []string `json:"nodes"`
	Iterations       int      `json:"iterations,omitempty"`
	LoopType         string   `json:"loopType,omitempty"` // for | forEach | while | doWhile
	ForEachItems     any      `json:"forEachItems,omitempty"`
	WhileCondition   string   `json:"whileCondition,omitempty"`
	DoWhileCondition string   `json:"doWhileCondition,omitempty"`
}

// Parallel describes a parallel subflow. Nodes are the base block IDs inside.
type Parallel struct {
	ID           string   `json:"id"`
	Nodes        []string `json:"nodes"`
	ParallelType string   `json:"parallelType,omitempty"` // count | collection
	Count        int      `json:"count,omitempty"`
	Distribution any      `json:"distribution,omitempty"`
	BatchSize    int      `json:"batchSize,omitempty"`
}
