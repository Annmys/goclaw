package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	immediateToolResultCompressThreshold = 24000
	immediateToolResultHeadChars         = 8000
	immediateToolResultTailChars         = 4000
)

var immediateCompressToolNames = map[string]bool{
	"bash":       true,
	"exec":       true,
	"list_files": true,
	"read_file":  true,
	"read":       true,
	"web_fetch":  true,
}

// compressToolResultForNextTurn caps high-noise tool outputs before they enter
// the next LLM call. Historical context pruning still handles old messages; this
// protects the immediate next iteration from large shell/read outputs.
func compressToolResultForNextTurn(toolName, content string) string {
	if content == "" {
		return content
	}
	if !immediateCompressToolNames[strings.ToLower(toolName)] {
		return content
	}
	totalChars := utf8.RuneCountInString(content)
	if totalChars <= immediateToolResultCompressThreshold {
		return content
	}

	headChars := immediateToolResultHeadChars
	tailChars := immediateToolResultTailChars
	if hasImportantTail(content) {
		// Error/summary tails are often more valuable than middle rows.
		headChars = 6000
		tailChars = 6000
	}

	head := takeHead(content, headChars)
	tail := takeTail(content, tailChars)
	return fmt.Sprintf(
		"%s\n\n[... middle tool output omitted for speed ...]\n\n%s\n\n[Tool result compressed for next LLM turn: original %d chars, kept first %d and last %d chars. If exact omitted lines are required, rerun a narrower command/query or read the specific file range.]",
		head, tail, totalChars, headChars, tailChars,
	)
}
