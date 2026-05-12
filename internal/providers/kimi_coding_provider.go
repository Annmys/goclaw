package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const (
	kimiCodingAPIBase    = "https://api.kimi.com/coding/v1"
	kimiCodingUserAgent  = "claude-code/0.1.0"
	kimiCodingAPIVersion = "2023-06-01"
)

// KimiCodingProvider is a dedicated transport for Kimi For Coding.
// Unlike the regular Moonshot chat API, the coding endpoint is most stable on
// the Anthropic-style /messages contract with Bearer auth.
type KimiCodingProvider struct {
	*AnthropicProvider
	apiKey string
}

func NewKimiCodingProvider(name, apiKey, baseURL, defaultModel string) *KimiCodingProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = kimiCodingAPIBase
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1/messages") {
		baseURL = strings.TrimSuffix(baseURL, "/messages")
	} else if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	if defaultModel == "" {
		defaultModel = "kimi-for-coding"
	}
	ap := NewAnthropicProvider(
		apiKey,
		WithAnthropicName(name),
		WithAnthropicBaseURL(baseURL),
		WithAnthropicModel(defaultModel),
	)
	return &KimiCodingProvider{
		AnthropicProvider: ap,
		apiKey:            apiKey,
	}
}

func (p *KimiCodingProvider) SupportsThinking() bool { return true }

func (p *KimiCodingProvider) resolveModel(model string) string {
	return resolveAnthropicModel(model, p.defaultModel, p.registry)
}

func (p *KimiCodingProvider) middlewareConfig(model string, req ChatRequest) MiddlewareConfig {
	return MiddlewareConfig{
		Provider: "kimi_coding",
		Model:    model,
		Caps:     p.Capabilities(),
		AuthType: "api_key",
		APIBase:  p.baseURL,
		Options:  req.Options,
	}
}

func (p *KimiCodingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := resolveAnthropicModel(req.Model, p.defaultModel, p.registry)
	body := p.buildRequestBody(model, req, false)
	body = ApplyMiddlewares(body, p.middlewares, p.middlewareConfig(model, req))

	resp, err := RetryDo(ctx, p.retryConfig, func() (*ChatResponse, error) {
		respBody, err := p.doRequest(ctx, body)
		if err != nil {
			return nil, err
		}
		defer respBody.Close()

		var parsed anthropicResponse
		if err := json.NewDecoder(respBody).Decode(&parsed); err != nil {
			return nil, fmt.Errorf("kimi-code: decode response: %w", err)
		}

		return p.parseResponse(&parsed), nil
	})
	if resp != nil {
		if strip, _ := req.Options[OptStripThinking].(bool); strip {
			resp.Thinking = ""
		}
	}
	return resp, err
}

func (p *KimiCodingProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	model := resolveAnthropicModel(req.Model, p.defaultModel, p.registry)
	stripThinking, _ := req.Options[OptStripThinking].(bool)

	body := p.buildRequestBody(model, req, true)
	body = ApplyMiddlewares(body, p.middlewares, p.middlewareConfig(model, req))

	respBody, err := RetryDo(ctx, p.retryConfig, func() (io.ReadCloser, error) {
		return p.doRequest(ctx, body)
	})
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	result := &ChatResponse{FinishReason: "stop"}
	toolCallJSON := make(map[int]string)
	var rawContentBlocks []json.RawMessage
	var currentBlockType string
	thinkingChars := 0
	var thinkingSignature strings.Builder

	sse := NewSSEScanner(respBody)
	for sse.Next() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		data := sse.Data()

		switch sse.EventType() {
		case "message_start":
			var ev anthropicMessageStartEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if result.Usage == nil {
					result.Usage = &Usage{}
				}
				if ev.Message.Usage.InputTokens > 0 {
					result.Usage.PromptTokens = ev.Message.Usage.InputTokens
				}
				result.Usage.CacheCreationTokens = ev.Message.Usage.CacheCreationInputTokens
				result.Usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			}
		case "content_block_start":
			var ev anthropicContentBlockStartEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				currentBlockType = ev.ContentBlock.Type
				if ev.ContentBlock.Type == "tool_use" {
					result.ToolCalls = append(result.ToolCalls, ToolCall{
						ID:        ev.ContentBlock.ID,
						Name:      strings.TrimSpace(ev.ContentBlock.Name),
						Arguments: make(map[string]any),
					})
				}
				rawContentBlocks = append(rawContentBlocks, json.RawMessage(fmt.Sprintf(`{"type":"%s"}`, ev.ContentBlock.Type)))
			}
		case "content_block_delta":
			var ev anthropicContentBlockDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				switch ev.Delta.Type {
				case "text_delta":
					result.Content += ev.Delta.Text
					if onChunk != nil {
						onChunk(StreamChunk{Content: ev.Delta.Text})
					}
				case "thinking_delta":
					thinkingChars += len(ev.Delta.Thinking)
					if !stripThinking {
						result.Thinking += ev.Delta.Thinking
						if onChunk != nil {
							onChunk(StreamChunk{Thinking: ev.Delta.Thinking})
						}
					}
				case "input_json_delta":
					if len(result.ToolCalls) > 0 {
						idx := len(result.ToolCalls) - 1
						toolCallJSON[idx] += ev.Delta.PartialJSON
					}
				case "signature_delta":
					thinkingSignature.WriteString(ev.Delta.Signature)
				}
			}
		case "content_block_stop":
			if len(rawContentBlocks) > 0 {
				idx := len(rawContentBlocks) - 1
				block := p.buildRawBlock(currentBlockType, result, toolCallJSON, idx)
				if block != nil {
					rawContentBlocks[idx] = block
				}
			}
			currentBlockType = ""
		case "message_delta":
			var ev anthropicMessageDeltaEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				if ev.Delta.StopReason != "" {
					switch ev.Delta.StopReason {
					case "tool_use":
						result.FinishReason = "tool_calls"
					case "max_tokens":
						result.FinishReason = "length"
					default:
						result.FinishReason = "stop"
					}
				}
				if ev.Usage.OutputTokens > 0 {
					if result.Usage == nil {
						result.Usage = &Usage{}
					}
					result.Usage.CompletionTokens = ev.Usage.OutputTokens
				}
			}
		case "error":
			var ev anthropicErrorEvent
			if err := json.Unmarshal([]byte(data), &ev); err == nil {
				return nil, fmt.Errorf("kimi-code stream error: %s: %s", ev.Error.Type, ev.Error.Message)
			}
		case "message_stop":
		}
	}

	if err := sse.Err(); err != nil {
		return nil, fmt.Errorf("kimi-code: stream read error: %w", err)
	}

	for i, rawJSON := range toolCallJSON {
		if rawJSON != "" && i < len(result.ToolCalls) {
			args := make(map[string]any)
			if err := json.Unmarshal([]byte(rawJSON), &args); err != nil {
				slog.Warn("kimi-code: failed to parse tool call arguments", "tool", result.ToolCalls[i].Name, "raw_len", len(rawJSON), "error", err)
				result.ToolCalls[i].ParseError = fmt.Sprintf("malformed JSON (%d chars): %v", len(rawJSON), err)
			}
			result.ToolCalls[i].Arguments = args
		}
	}

	if result.Usage != nil {
		result.Usage.TotalTokens = result.Usage.PromptTokens + result.Usage.CompletionTokens
		if thinkingChars > 0 {
			result.Usage.ThinkingTokens = thinkingChars / 4
		}
	}

	if len(rawContentBlocks) > 0 && len(result.ToolCalls) > 0 {
		if b, err := json.Marshal(rawContentBlocks); err == nil {
			result.RawAssistantContent = b
		}
	}
	result.ThinkingSignature = thinkingSignature.String()

	if onChunk != nil {
		onChunk(StreamChunk{Done: true})
	}
	return result, nil
}

func (p *KimiCodingProvider) doRequest(ctx context.Context, body any) (io.ReadCloser, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kimi-code: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("kimi-code: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("anthropic-version", kimiCodingAPIVersion)
	httpReq.Header.Set("User-Agent", kimiCodingUserAgent)
	httpReq.Header.Set("Accept", "application/json")

	if bodyMap, ok := body.(map[string]any); ok {
		if _, hasThinking := bodyMap["thinking"]; hasThinking {
			httpReq.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
		}
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kimi-code: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		retryAfter := ParseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &HTTPError{
			Status:     resp.StatusCode,
			Body:       fmt.Sprintf("kimi-code: %s", string(respBody)),
			RetryAfter: retryAfter,
		}
	}

	return resp.Body, nil
}
