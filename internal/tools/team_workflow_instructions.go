package tools

import "encoding/json"

// ParseWorkflowInstructions extracts team-specific workflow guidance from
// settings. It is prompt-only policy used to specialize a team without adding
// hard-coded team names to the core engine.
func ParseWorkflowInstructions(settings json.RawMessage) string {
	if len(settings) == 0 {
		return ""
	}
	var s struct {
		WorkflowInstructions string `json:"workflow_instructions"`
	}
	if json.Unmarshal(settings, &s) != nil {
		return ""
	}
	return s.WorkflowInstructions
}
