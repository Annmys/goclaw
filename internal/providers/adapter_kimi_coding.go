package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// KimiCodingAdapter wraps OpenAI transport with headers expected by Kimi Coding.
type KimiCodingAdapter struct {
	provider *KimiCodingProvider
}

func NewKimiCodingAdapter(cfg ProviderConfig) (ProviderAdapter, error) {
	p := NewKimiCodingProvider(cfg.Name, cfg.APIKey, cfg.BaseURL, cfg.Model)
	return &KimiCodingAdapter{provider: p}, nil
}

func (a *KimiCodingAdapter) Name() string { return "kimi_coding" }

func (a *KimiCodingAdapter) Capabilities() ProviderCapabilities {
	return a.provider.Capabilities()
}

func (a *KimiCodingAdapter) ToRequest(req ChatRequest) ([]byte, http.Header, error) {
	stream := true
	if v, ok := req.Options["stream"].(bool); ok {
		stream = v
	}

	model := a.provider.resolveModel(req.Model)
	body := a.provider.buildRequestBody(model, req, stream)

	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("kimi_coding adapter: marshal: %w", err)
	}

	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "Bearer "+a.provider.apiKey)
	h.Set("anthropic-version", kimiCodingAPIVersion)
	h.Set("User-Agent", kimiCodingUserAgent)
	h.Set("Accept", "application/json")
	if _, hasThinking := body["thinking"]; hasThinking {
		h.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	}

	return data, h, nil
}

func (a *KimiCodingAdapter) FromResponse(data []byte) (*ChatResponse, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("kimi_coding adapter: decode: %w", err)
	}
	return a.provider.parseResponse(&resp), nil
}

func (a *KimiCodingAdapter) FromStreamChunk(data []byte) (*StreamChunk, error) {
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, nil
	}

	switch envelope.Type {
	case "content_block_delta":
		var ev anthropicContentBlockDeltaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return nil, nil
		}
		switch ev.Delta.Type {
		case "text_delta":
			return &StreamChunk{Content: ev.Delta.Text}, nil
		case "thinking_delta":
			return &StreamChunk{Thinking: ev.Delta.Thinking}, nil
		}
	case "message_stop":
		return &StreamChunk{Done: true}, nil
	case "error":
		var ev anthropicErrorEvent
		if err := json.Unmarshal(data, &ev); err == nil {
			return nil, fmt.Errorf("kimi-code stream: %s: %s", ev.Error.Type, ev.Error.Message)
		}
	}

	return nil, nil
}
