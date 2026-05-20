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

type toolResultCompressionInfo struct {
	OriginalChars   int
	CompressedChars int
	Compressed      bool
}

// compressToolResultForNextTurn caps high-noise tool outputs before they enter
// the next LLM call. Historical context pruning still handles old messages; this
// protects the immediate next iteration from large shell/read outputs.
func compressToolResultForNextTurn(toolName, content string) string {
	compressed, _ := compressToolResultForNextTurnWithInfo(toolName, content)
	return compressed
}

func compressToolResultForNextTurnWithInfo(toolName, content string) (string, toolResultCompressionInfo) {
	info := toolResultCompressionInfo{
		OriginalChars: utf8.RuneCountInString(content),
	}
	if content == "" {
		info.CompressedChars = 0
		return content, info
	}
	if !immediateCompressToolNames[strings.ToLower(toolName)] {
		info.CompressedChars = info.OriginalChars
		return content, info
	}
	totalChars := info.OriginalChars
	if totalChars <= immediateToolResultCompressThreshold {
		info.CompressedChars = totalChars
		return content, info
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
	compressed := fmt.Sprintf(
		"%s\n\n[... middle tool output omitted for speed ...]\n\n%s\n\n[Tool result compressed for next LLM turn: original %d chars, kept first %d and last %d chars. If exact omitted lines are required, rerun a narrower command/query or read the specific file range.]",
		head, tail, totalChars, headChars, tailChars,
	)
	info.Compressed = true
	info.CompressedChars = utf8.RuneCountInString(compressed)
	return compressed, info
}
