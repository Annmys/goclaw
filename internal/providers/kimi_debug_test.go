package providers

import (
	"encoding/json"
	"testing"
)

func TestKimiCodingBuildRequestUsesAnthropicMessagesFormat(t *testing.T) {
	p := NewKimiCodingProvider("kimi-code", "key", "https://api.kimi.com/coding/v1", "kimi-for-coding")

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are an assistant"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "call_1", Name: "bash", Arguments: map[string]any{"command": "ls"}}}},
			{Role: "tool", Content: "file.txt", ToolCallID: "call_1"},
			{Role: "user", Content: "next"},
		},
		Tools: []ToolDefinition{
			{
				Type: "function",
				Function: &ToolFunctionSchema{
					Name:        "bash",
					Description: "Run shell command",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"command": map[string]any{"type": "string"},
						},
						"required": []string{"command"},
					},
				},
			},
		},
	}

	body := p.buildRequestBody("kimi-for-coding", req, false)
	if _, has := body[OptReasoningEffort]; has {
		t.Fatalf("kimi coding messages request must not emit reasoning_effort, body=%v", body)
	}
	if _, has := body["thinking"]; has {
		t.Fatalf("kimi coding messages request must not emit thinking when off, body=%v", body)
	}

	msgs, ok := body["messages"].([]map[string]any)
	if !ok || len(msgs) == 0 {
		t.Fatalf("messages = %#v, want anthropic messages", body["messages"])
	}
	foundToolUse := false
	foundToolResult := false
	for _, msg := range msgs {
		role, _ := msg["role"].(string)
		content := msg["content"]
		blocks, ok := content.([]map[string]any)
		if !ok {
			continue
		}
		if role == "assistant" {
			for _, block := range blocks {
				if block["type"] == "tool_use" {
					foundToolUse = true
				}
			}
		}
		if role == "user" {
			for _, block := range blocks {
				if block["type"] == "tool_result" {
					foundToolResult = true
				}
			}
		}
	}
	if !foundToolUse {
		t.Fatal("expected anthropic tool_use block for assistant replay")
	}
	if !foundToolResult {
		t.Fatal("expected anthropic tool_result block for tool replay")
	}

	tools, ok := body["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want anthropic tools", body["tools"])
	}
	if _, has := tools[0]["input_schema"]; !has {
		t.Fatalf("tool missing input_schema: %#v", tools[0])
	}
}

func TestKimiCodingThinkingOffOmitsThinkingBlock(t *testing.T) {
	p := NewKimiCodingProvider("kimi-code", "key", "https://api.kimi.com/coding/v1", "kimi-for-coding")

	body := p.buildRequestBody("kimi-for-coding", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Options: map[string]any{
			OptTemperature: 0.2,
		},
	}, false)

	if _, has := body["thinking"]; has {
		t.Fatalf("thinking must be omitted when off, body=%v", body)
	}
	if got, has := body["temperature"]; !has || got != 0.2 {
		t.Fatalf("temperature = %v (present=%v), want 0.2 preserved", got, has)
	}
}

func TestKimiCodingExplicitThinkingUsesAnthropicThinkingConfig(t *testing.T) {
	p := NewKimiCodingProvider("kimi-code", "key", "https://api.kimi.com/coding/v1", "kimi-for-coding")

	body := p.buildRequestBody("kimi-for-coding", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Options: map[string]any{
			OptThinkingLevel: "medium",
			OptTemperature:   0.2,
		},
	}, false)

	thinking, ok := body["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want config", body["thinking"])
	}
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking.type = %v, want enabled", thinking["type"])
	}
	if _, has := body["temperature"]; has {
		t.Fatalf("temperature must be removed when thinking enabled, body=%v", body)
	}
}

func TestKimiCodingAdapterParsesAnthropicResponse(t *testing.T) {
	adapter, err := NewKimiCodingAdapter(ProviderConfig{
		Name:   "kimi-code",
		APIKey: "sk-test",
		BaseURL: "https://api.kimi.com/coding/v1",
		Model:  "kimi-for-coding",
	})
	if err != nil {
		t.Fatal(err)
	}

	respJSON := `{
		"content": [
			{"type": "thinking", "thinking": "internal"},
			{"type": "text", "text": "ok"},
			{"type": "tool_use", "id": "tool_1", "name": "echo", "input": {"text":"hi"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	resp, err := adapter.FromResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Content)
	}
	if resp.Thinking != "internal" {
		t.Fatalf("thinking = %q, want internal", resp.Thinking)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", resp.FinishReason)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool_calls = %#v, want echo", resp.ToolCalls)
	}
}

func TestKimiCodingAdapterHeadersUseBearerAndAnthropicVersion(t *testing.T) {
	adapter, err := NewKimiCodingAdapter(ProviderConfig{
		Name:   "kimi-code",
		APIKey: "sk-test",
		BaseURL: "https://api.kimi.com/coding/v1",
		Model:  "kimi-for-coding",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, headers, err := adapter.ToRequest(ChatRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := headers.Get("Authorization"); got != "Bearer sk-test" {
		t.Fatalf("Authorization = %q, want Bearer sk-test", got)
	}
	if got := headers.Get("anthropic-version"); got != kimiCodingAPIVersion {
		t.Fatalf("anthropic-version = %q, want %q", got, kimiCodingAPIVersion)
	}
	if got := headers.Get("User-Agent"); got != kimiCodingUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, kimiCodingUserAgent)
	}

	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if _, has := body["messages"]; !has {
		t.Fatalf("body missing messages: %#v", body)
	}
}
