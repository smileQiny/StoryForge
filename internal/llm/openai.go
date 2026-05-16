package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"storyforge/internal/model"
)

// OpenAIConfig holds configuration for the OpenAI provider.
type OpenAIConfig struct {
	APIKey             string
	BaseURL            string // defaults to https://api.openai.com/v1
	Model              string
	WireAPI            string
	Timeout            time.Duration
	InsecureSkipVerify bool
}

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	cfg    OpenAIConfig
	client *http.Client
}

var _ Provider = (*OpenAIProvider)(nil)
var _ AgentRuntime = (*OpenAIProvider)(nil)

const openAIRequestMaxAttempts = 3

// NewOpenAI creates an OpenAI provider.
func NewOpenAI(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
		if model.NormalizeWireAPI(cfg.WireAPI) == model.WireAPIResponses {
			timeout = 5 * time.Minute
		}
	}
	return &OpenAIProvider{
		cfg:    cfg,
		client: newHTTPClient(timeout, cfg.InsecureSkipVerify),
	}
}

// Capabilities describes the configured OpenAI-compatible provider features.
func (p *OpenAIProvider) Capabilities() Capabilities {
	return Capabilities{
		Provider:             "openai",
		Model:                p.cfg.Model,
		ConfiguredWireAPI:    model.NormalizeWireAPI(p.cfg.WireAPI),
		SupportedWireAPIs:    []string{model.WireAPIChat, model.WireAPIResponses},
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

// Chat performs a non-streaming chat completion.
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if p.usesResponsesAPI() {
		return p.chatResponses(ctx, req)
	}
	body := p.buildRequestBody(req, false, nil)
	resp, err := p.doRequest(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	return &ChatResponse{
		Content: result.Choices[0].Message.Content,
		Usage: TokenUsage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
	}, nil
}

// ChatWithTools performs a tool-calling chat completion.
func (p *OpenAIProvider) ChatWithTools(ctx context.Context, req ChatRequest, tools []Tool) (*ToolResponse, error) {
	if p.usesResponsesAPI() {
		return p.chatWithToolsResponses(ctx, req, tools)
	}
	body := p.buildRequestBody(req, false, tools)
	resp, err := p.doRequest(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := result.Choices[0]
	tr := &ToolResponse{
		Content: choice.Message.Content,
		Usage: TokenUsage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
	}
	for _, tc := range choice.Message.ToolCalls {
		tr.ToolCalls = append(tr.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	return tr, nil
}

func (p *OpenAIProvider) chatWithToolsResponses(ctx context.Context, req ChatRequest, tools []Tool) (*ToolResponse, error) {
	body := p.buildResponsesRequestBody(req, false, tools)
	resp, err := p.doRequest(ctx, "/responses", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openAIResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	toolCalls := extractResponsesToolCalls(result.Output)
	content := extractResponsesText(result.Output)
	if content == "" && len(toolCalls) == 0 && len(result.Output) == 0 {
		return nil, fmt.Errorf("no output in response")
	}

	return &ToolResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Usage:     result.Usage.toTokenUsage(),
	}, nil
}

// Stream performs a streaming chat completion with automatic fallback on interruption.
func (p *OpenAIProvider) Stream(ctx context.Context, req ChatRequest, cb StreamCallback) (*ChatResponse, error) {
	if p.usesResponsesAPI() {
		return p.streamResponses(ctx, req, cb)
	}
	body := p.buildRequestBody(req, true, nil)
	resp, err := p.doRequest(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		token := chunk.Choices[0].Delta.Content
		if token == "" {
			continue
		}
		sb.WriteString(token)
		if cb != nil {
			if err := cb(token); err != nil {
				break
			}
		}
	}

	content := sb.String()
	// Partial content recovery: if >= 500 chars, treat as success
	if err := scanner.Err(); err != nil && len(content) < 500 {
		// Fall back to sync
		req.Stream = false
		return p.Chat(ctx, req)
	}

	return &ChatResponse{Content: content}, nil
}

func (p *OpenAIProvider) chatResponses(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := p.buildResponsesRequestBody(req, false, nil)
	resp, err := p.doRequest(ctx, "/responses", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result openAIResponsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	content := extractResponsesText(result.Output)
	if content == "" && len(result.Output) == 0 {
		return nil, fmt.Errorf("no output in response")
	}
	return &ChatResponse{
		Content: content,
		Usage:   result.Usage.toTokenUsage(),
	}, nil
}

func (p *OpenAIProvider) streamResponses(ctx context.Context, req ChatRequest, cb StreamCallback) (*ChatResponse, error) {
	body := p.buildResponsesRequestBody(req, true, nil)
	resp, err := p.doRequest(ctx, "/responses", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var (
		sb    strings.Builder
		usage TokenUsage
	)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var event openAIResponsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta == "" {
				continue
			}
			sb.WriteString(event.Delta)
			if cb != nil {
				if err := cb(event.Delta); err != nil {
					break
				}
			}
		case "response.completed":
			usage = event.Response.Usage.toTokenUsage()
		}
	}

	content := sb.String()
	if err := scanner.Err(); err != nil && len(content) < 500 {
		req.Stream = false
		return p.Chat(ctx, req)
	}
	return &ChatResponse{Content: content, Usage: usage}, nil
}

func (p *OpenAIProvider) buildRequestBody(req ChatRequest, stream bool, tools []Tool) map[string]any {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	msgs := make([]map[string]any, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = map[string]any{"role": m.Role, "content": m.Content}
	}
	body := map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   stream,
	}
	if req.MaxTokens > 0 {
		body[maxTokensField(model)] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(tools) > 0 {
		oaiTools := make([]map[string]any, len(tools))
		for i, t := range tools {
			oaiTools[i] = map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			}
		}
		body["tools"] = oaiTools
	}
	return body
}

func (p *OpenAIProvider) buildResponsesRequestBody(req ChatRequest, stream bool, tools []Tool) map[string]any {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	body := map[string]any{
		"model":  model,
		"stream": stream,
		"input":  buildResponsesInput(req.Messages),
	}
	if instructions := buildResponsesInstructions(req.Messages); instructions != "" {
		body["instructions"] = instructions
	}
	if req.MaxTokens > 0 {
		body["max_output_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}
	if len(tools) > 0 {
		responseTools := make([]map[string]any, len(tools))
		for i, t := range tools {
			responseTools[i] = map[string]any{
				"type":        "function",
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			}
		}
		body["tools"] = responseTools
		body["tool_choice"] = "auto"
	}
	return body
}

func (p *OpenAIProvider) doRequest(ctx context.Context, path string, body map[string]any) (*http.Response, error) {
	resp, rawBody, err := p.doRequestWithTransientRetries(ctx, path, body)
	if err != nil {
		return nil, err
	}
	if shouldRetryWithoutMaxTokens(rawBody, body) {
		retryBody := cloneRequestBody(body)
		delete(retryBody, "max_tokens")
		delete(retryBody, "max_completion_tokens")
		delete(retryBody, "max_output_tokens")
		_ = resp.Body.Close()
		resp, rawBody, err = p.doRequestWithTransientRetries(ctx, path, retryBody)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode < 400 {
		return resp, nil
	}
	defer resp.Body.Close()
	return nil, formatOpenAIHTTPError(resp.StatusCode, rawBody)
}

func (p *OpenAIProvider) doRequestWithTransientRetries(ctx context.Context, path string, body map[string]any) (*http.Response, []byte, error) {
	var lastErr error
	for attempt := 1; attempt <= openAIRequestMaxAttempts; attempt++ {
		resp, rawBody, err := p.doRequestOnce(ctx, path, body)
		if err != nil {
			lastErr = err
			if attempt == openAIRequestMaxAttempts || !isRetryableRequestError(err) || !sleepBeforeRetry(ctx, attempt) {
				return nil, nil, err
			}
			continue
		}
		if isRetryableStatus(resp.StatusCode) && attempt < openAIRequestMaxAttempts {
			_ = resp.Body.Close()
			if !sleepBeforeRetry(ctx, attempt) {
				return resp, rawBody, nil
			}
			continue
		}
		return resp, rawBody, nil
	}
	return nil, nil, lastErr
}

func (p *OpenAIProvider) doRequestOnce(ctx context.Context, path string, body map[string]any) (*http.Response, []byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode >= 400 || isJSONContentType(resp.Header.Get("Content-Type")) {
		rawBody, _ := io.ReadAll(resp.Body)
		resp.Body = io.NopCloser(bytes.NewReader(rawBody))
		return resp, rawBody, nil
	}
	return resp, nil, nil
}

func isJSONContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "application/json")
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func formatOpenAIHTTPError(status int, rawBody []byte) error {
	providerMessage := extractProviderErrorMessage(rawBody)
	category, action := classifyOpenAIHTTPError(status)
	if providerMessage != "" {
		return fmt.Errorf("%s (HTTP %d): %s. 操作提示：%s", category, status, providerMessage, action)
	}
	return fmt.Errorf("%s (HTTP %d). 操作提示：%s", category, status, action)
}

func extractProviderErrorMessage(rawBody []byte) string {
	raw := strings.TrimSpace(string(rawBody))
	if raw == "" {
		return ""
	}
	var payload struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(rawBody, &payload); err == nil && payload.Error != nil {
		switch e := payload.Error.(type) {
		case string:
			return strings.TrimSpace(e)
		case map[string]any:
			if msg, ok := e["message"].(string); ok && strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
			if msg, ok := e["error"].(string); ok && strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
		}
	}
	var fallback map[string]any
	if err := json.Unmarshal(rawBody, &fallback); err == nil {
		for _, key := range []string{"message", "error_description", "detail"} {
			if msg, ok := fallback[key].(string); ok && strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
		}
	}
	return raw
}

func classifyOpenAIHTTPError(status int) (category, action string) {
	switch status {
	case http.StatusUnauthorized:
		return "llm provider authentication error", "检查 API Key 是否正确、是否过期，以及当前 profile 是否绑定了正确密钥"
	case http.StatusPaymentRequired, http.StatusForbidden:
		return "llm provider quota or permission error", "外部模型服务拒绝请求，通常是额度/套餐/尊享积分不足，或模型/API Key 无权限；请补充额度、开通套餐或切换可用 profile 后重试"
	case http.StatusTooManyRequests:
		return "llm provider rate limit error", "模型供应商限流或额度窗口耗尽；请稍后重试、降低并发，或切换可用 profile"
	default:
		return "llm provider request error", "检查模型供应商返回、baseUrl、wireApi、模型名和网络状态"
	}
}

func isRetryableRequestError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "server closed idle connection") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.HasSuffix(msg, ": eof") ||
		msg == "eof"
}

func sleepBeforeRetry(ctx context.Context, attempt int) bool {
	delay := time.Duration(attempt*attempt) * 250 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func shouldRetryWithoutMaxTokens(rawBody []byte, body map[string]any) bool {
	if _, hasLegacy := body["max_tokens"]; !hasLegacy {
		if _, hasCompletion := body["max_completion_tokens"]; !hasCompletion {
			return false
		}
	}

	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Param   string `json:"param"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return false
	}

	if resp.Error.Param == "max_tokens" || resp.Error.Param == "max_completion_tokens" {
		return true
	}
	if resp.Error.Code == "unsupported_parameter" && mentionsOutputTokenParam(string(rawBody), resp.Error.Message) {
		return true
	}
	if resp.Error.Type == "invalid_request_error" && mentionsOutputTokenParam(string(rawBody), resp.Error.Message) {
		return true
	}
	return false
}

func buildResponsesInstructions(messages []Message) string {
	var instructions []string
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			instructions = append(instructions, message.Content)
		}
	}
	return strings.TrimSpace(strings.Join(instructions, "\n\n"))
}

func buildResponsesInput(messages []Message) any {
	inputMessages := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role == "" || role == "system" {
			continue
		}
		inputMessages = append(inputMessages, map[string]any{
			"role": role,
			"content": []map[string]any{{
				"type": "input_text",
				"text": message.Content,
			}},
		})
	}
	if len(inputMessages) == 1 {
		role, _ := inputMessages[0]["role"].(string)
		if role == "user" {
			content, _ := inputMessages[0]["content"].([]map[string]any)
			if len(content) == 1 {
				if text, _ := content[0]["text"].(string); text != "" {
					return text
				}
			}
		}
	}
	return inputMessages
}

func maxTokensField(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "gpt-5") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

func mentionsOutputTokenParam(values ...string) bool {
	for _, value := range values {
		if strings.Contains(value, "max_tokens") || strings.Contains(value, "max_completion_tokens") || strings.Contains(value, "max_output_tokens") {
			return true
		}
	}
	return false
}

func cloneRequestBody(body map[string]any) map[string]any {
	cloned := make(map[string]any, len(body))
	for key, value := range body {
		cloned[key] = value
	}
	return cloned
}

// --- internal response types ---

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type openAIResponsesResponse struct {
	Output []openAIResponsesOutputItem `json:"output"`
	Usage  openAIResponsesUsage        `json:"usage"`
}

type openAIResponsesOutputItem struct {
	ID        string                       `json:"id,omitempty"`
	Type      string                       `json:"type"`
	Status    string                       `json:"status,omitempty"`
	Name      string                       `json:"name,omitempty"`
	CallID    string                       `json:"call_id,omitempty"`
	Arguments string                       `json:"arguments,omitempty"`
	Content   []openAIResponsesContentItem `json:"content,omitempty"`
}

type openAIResponsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (u openAIResponsesUsage) toTokenUsage() TokenUsage {
	return TokenUsage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
	}
}

type openAIResponsesStreamEvent struct {
	Type     string                `json:"type"`
	Delta    string                `json:"delta"`
	Response openAIResponsesStream `json:"response"`
}

type openAIResponsesStream struct {
	Usage openAIResponsesUsage `json:"usage"`
}

func extractResponsesText(output []openAIResponsesOutputItem) string {
	var sb strings.Builder
	for _, item := range output {
		for _, content := range item.Content {
			if content.Type == "output_text" {
				sb.WriteString(content.Text)
			}
		}
	}
	return sb.String()
}

func extractResponsesToolCalls(output []openAIResponsesOutputItem) []ToolCall {
	toolCalls := make([]ToolCall, 0)
	for _, item := range output {
		if item.Type != "function_call" {
			continue
		}
		args := strings.TrimSpace(item.Arguments)
		if args == "" {
			args = "{}"
		}
		id := strings.TrimSpace(item.CallID)
		if id == "" {
			id = strings.TrimSpace(item.ID)
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        id,
			Name:      item.Name,
			Arguments: json.RawMessage(args),
		})
	}
	return toolCalls
}

func (p *OpenAIProvider) usesResponsesAPI() bool {
	return model.NormalizeWireAPI(p.cfg.WireAPI) == model.WireAPIResponses
}
