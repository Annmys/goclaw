package workflow

import "testing"

func TestParseGeneratedGraph(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantBlocks  int
		wantExplan  string
		wantErr     bool
	}{
		{
			name: "fenced json block",
			content: "我设计了一个简单流程。\n```json\n{\"version\":\"1.0\",\"blocks\":[{\"id\":\"t\",\"metadata\":{\"id\":\"trigger\"},\"enabled\":true}],\"connections\":[]}\n```",
			wantBlocks: 1,
			wantExplan: "我设计了一个简单流程。",
		},
		{
			name:       "bare json object",
			content:    "{\"version\":\"1.0\",\"blocks\":[{\"id\":\"a\",\"metadata\":{\"id\":\"agent\"},\"enabled\":true},{\"id\":\"b\",\"metadata\":{\"id\":\"response\"},\"enabled\":true}],\"connections\":[]}",
			wantBlocks: 2,
		},
		{
			name:    "no json",
			content: "抱歉我无法生成。",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, explan, err := parseGeneratedGraph(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(g.Blocks) != tt.wantBlocks {
				t.Fatalf("blocks = %d, want %d", len(g.Blocks), tt.wantBlocks)
			}
			if tt.wantExplan != "" && explan != tt.wantExplan {
				t.Fatalf("explanation = %q, want %q", explan, tt.wantExplan)
			}
		})
	}
}
