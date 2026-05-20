package agent

import (
	"strings"
	"testing"
)

func TestCompressToolResultForNextTurn_CompressesExecOutput(t *testing.T) {
	content := strings.Repeat("a", immediateToolResultCompressThreshold+1000)
	got := compressToolResultForNextTurn("exec", content)
	if got == content {
		t.Fatal("expected exec output to be compressed")
	}
	if !strings.Contains(got, "Tool result compressed for next LLM turn") {
		t.Fatalf("missing compression marker: %q", testTruncate(got, 200))
	}
	if len(got) >= len(content) {
		t.Fatalf("compressed output should be shorter, got %d >= %d", len(got), len(content))
	}
}

func TestCompressToolResultForNextTurn_SkipsSmallOutput(t *testing.T) {
	content := strings.Repeat("a", 1000)
	got := compressToolResultForNextTurn("exec", content)
	if got != content {
		t.Fatal("small output should not be compressed")
	}
}

func TestCompressToolResultForNextTurn_SkipsNonNoisyTools(t *testing.T) {
	content := strings.Repeat("a", immediateToolResultCompressThreshold+1000)
	got := compressToolResultForNextTurn("send_file", content)
	if got != content {
		t.Fatal("send_file output should not be compressed")
	}
}

func TestCompressToolResultForNextTurn_PreservesImportantTail(t *testing.T) {
	content := strings.Repeat("h", 26000) + "\nERROR: failed summary"
	got := compressToolResultForNextTurn("bash", content)
	if !strings.Contains(got, "ERROR: failed summary") {
		t.Fatalf("important tail should be preserved: %q", testTruncate(got, 200))
	}
	if !strings.Contains(got, "kept first 6000 and last 6000 chars") {
		t.Fatalf("expected important-tail budget marker, got: %q", testTruncate(got, 300))
	}
}
