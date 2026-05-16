package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"storyforge/internal/model"
)

// AnthropicConfig holds configuration for the Anthropic provider.
type AnthropicConfig struct {
	APIKey             string
	BaseURL            string // defaults to https://api.anthropic.com
	Model              string
	Timeout            time.Duration
	ThinkingBudget     int // extended thinking token budget (0 = disabled)
	InsecureSkipVerify bool
}

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	cfg    AnthropicConfig
	client *http.Client
}

var _ Provider = (*AnthropicProvider)(nil)
var _ AgentRuntime = (*AnthropicProvider)(nil)

// NewAnthropic creates an Anthropic provider.
func NewAnthropic(cfg AnthropicConfig) *AnthropicProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &AnthropicProvider{
		cfg:    cfg,
		client: newHTTPClient(timeout, cfg.InsecureSkipVerify),
	}
}

// Capabilities describes the configured Anthropic provider features.
func (p *AnthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		Provider:                 "anthropic",
		Model:                    p.cfg.Model,
		ConfiguredWireAPI:        model.WireAPIChat,
		SupportedWireAPIs:        []string{model.WireAPIChat},
		SupportsChat:             true,
		SupportsStreaming:        true,
		SupportsToolCalls:        true,
		SupportsSystemPrompt:     true,
		SupportsThinkingBudget:   true,
		ConfiguredThinkingBudget: p.cfg.ThinkingBudget,
	}
}

// Chat performs a non-streaming chat completion.
func (p *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := p.buildRequestBody(req, false, nil)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	content := extractAnthropicText(result.Content)
	return &ChatResponse{
		Content: content,
		Usage: TokenUsage{
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
			TotalTokens:  result.Usage.InputTokens + result.Usage.OutputTokens,
		},
	}, nil
}

// ChatWithTools performs a tool-calling chat completion.
func (p *AnthropicProvider) ChatWithTools(ctx context.Context, req ChatRequest, tools []Tool) (*ToolResponse, error) {
	body := p.buildRequestBody(req, false, tools)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	tr := &ToolResponse{
		Content: extractAnthropicText(result.Content),
		Usage: TokenUsage{
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
			TotalTokens:  result.Usage.InputTokens + result.Usage.OutputTokens,
		},
	}
	for _, block := range result.Content {
		if block.Type == "tool_use" {
			tr.ToolCalls = append(tr.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}
	return tr, nil
}

// Stream performs a streaming chat completion.
func (p *AnthropicProvider) Stream(ctx context.Context, req ChatRequest, cb StreamCallback) (*ChatResponse, error) {
	body := p.buildRequestBody(req, true, nil)
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sb strings.Builder
	var inputTokens, outputTokens int

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			switch event.Type {
			case "content_block_delta":
				if event.Delta.Type == "text_delta" {
					token := event.Delta.Text
					sb.WriteString(token)
					if cb != nil {
						if err := cb(token); err != nil {
							break
						}
					}
				}
			case "message_delta":
				outputTokens = event.Usage.OutputTokens
			case "message_start":
				inputTokens = event.Message.Usage.InputTokens
			}
		}
	}

	content := sb.String()
	if err := scanner.Err(); err != nil && len(content) < 500 {
		req.Stream = false
		return p.Chat(ctx, req)
	}

	return &ChatResponse{
		Content: content,
		Usage: TokenUsage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) buildRequestBody(req ChatRequest, stream bool, tools []Tool) map[string]any {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	// Separate system message from user/assistant messages
	var systemPrompt string
	var msgs []map[string]any
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
	}

	body := map[string]any{
		"model":      model,
		"messages":   msgs,
		"max_tokens": 4096,
		"stream":     stream,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	if p.cfg.ThinkingBudget > 0 {
		body["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": p.cfg.ThinkingBudget,
		}
	}
	if len(tools) > 0 {
		anthropicTools := make([]map[string]any, len(tools))
		for i, t := range tools {
			anthropicTools[i] = map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": t.Parameters,
			}
		}
		body["tools"] = anthropicTools
	}
	return body
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body map[string]any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

// --- internal response types ---

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"` // text/tool_use
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta,omitempty"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
}

func extractAnthropicText(blocks []anthropicContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}
