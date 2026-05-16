package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"storyforge/internal/llm"
)

// --- OpenAI mock server ---

func openAIMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func openAIChatHandler(content string, inputTokens, outputTokens int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": content}},
			},
			"usage": map[string]any{
				"prompt_tokens":     inputTokens,
				"completion_tokens": outputTokens,
				"total_tokens":      inputTokens + outputTokens,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func openAIStreamHandler(tokens []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, tok := range tokens {
			chunk := map[string]any{
				"choices": []map[string]any{
					{"delta": map[string]any{"content": tok}},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func openAIResponsesHandler(content string, inputTokens, outputTokens int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t := "unexpected path"
			http.Error(w, t, http.StatusNotFound)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": content},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  inputTokens + outputTokens,
			},
		})
	}
}

func openAIResponsesStreamHandler(tokens []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, tok := range tokens {
			chunk := map[string]any{
				"type":  "response.output_text.delta",
				"delta": tok,
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "event: response.output_text.delta\n")
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		done := map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"usage": map[string]any{
					"input_tokens":  9,
					"output_tokens": len(tokens),
					"total_tokens":  9 + len(tokens),
				},
			},
		}
		data, _ := json.Marshal(done)
		fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func openAIToolHandler(toolName string, args map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		argsJSON, _ := json.Marshal(args)
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id": "call-1",
								"function": map[string]any{
									"name":      toolName,
									"arguments": string(argsJSON),
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TestOpenAI_Chat(t *testing.T) {
	srv := openAIMockServer(t, openAIChatHandler("Hello, world!", 10, 5))
	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-4o", WireAPI: "chat"})

	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("content mismatch: %q", resp.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage mismatch: %+v", resp.Usage)
	}
}

func TestOpenAI_ChatRetriesWithoutMaxTokensOnUnsupportedParameter(t *testing.T) {
	var attempts int
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			if _, ok := body["max_tokens"]; !ok {
				t.Fatalf("expected first request to include max_tokens")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      nil,
				"model":   nil,
				"object":  nil,
				"choices": nil,
				"error": map[string]any{
					"code":    "unsupported_parameter",
					"param":   "max_tokens",
					"type":    "invalid_request_error",
					"message": "max_tokens is not supported",
				},
			})
			return
		}

		if _, ok := body["max_tokens"]; ok {
			t.Fatalf("expected retry request to omit max_tokens")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "fallback ok"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     7,
				"completion_tokens": 3,
				"total_tokens":      10,
			},
		})
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-4o", WireAPI: "chat"})
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages:  []llm.Message{{Role: "user", Content: "Hi"}},
		MaxTokens: 5,
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "fallback ok" {
		t.Fatalf("expected fallback content, got %q", resp.Content)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestOpenAI_ChatUsesMaxCompletionTokensForGPT5Models(t *testing.T) {
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := body["max_completion_tokens"]; !ok {
			t.Fatalf("expected gpt-5 request to use max_completion_tokens")
		}
		if _, ok := body["max_tokens"]; ok {
			t.Fatalf("expected gpt-5 request to omit max_tokens")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     5,
				"completion_tokens": 3,
				"total_tokens":      8,
			},
		})
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.3-chat", WireAPI: "chat"})
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages:  []llm.Message{{Role: "user", Content: "Hi"}},
		MaxTokens: 8000,
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("expected ok content, got %q", resp.Content)
	}
}

func TestOpenAI_ResponsesWireAPIChat(t *testing.T) {
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["instructions"] != "system" {
			t.Fatalf("expected instructions field, got %#v", body["instructions"])
		}
		if body["input"] != "user" {
			t.Fatalf("expected string input, got %#v", body["input"])
		}
		if _, ok := body["max_output_tokens"]; !ok {
			t.Fatalf("expected max_output_tokens in responses request")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "OK"}}},
			},
			"usage": map[string]any{"input_tokens": 4, "output_tokens": 1, "total_tokens": 5},
		})
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.4", WireAPI: "responses"})
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages:  []llm.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "user"}},
		MaxTokens: 32,
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "OK" {
		t.Fatalf("expected OK content, got %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("expected usage total 5, got %+v", resp.Usage)
	}
}

func TestOpenAI_RetriesTransientResponsesRequestError(t *testing.T) {
	attempts := 0
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("expected response writer to support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatalf("hijack connection: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "OK"}}},
			},
			"usage": map[string]any{"input_tokens": 4, "output_tokens": 1, "total_tokens": 5},
		})
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.4", WireAPI: "responses"})
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "retry"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "OK" {
		t.Fatalf("expected OK content, got %q", resp.Content)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestOpenAI_DoesNotRetryForbidden(t *testing.T) {
	attempts := 0
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exhausted"}}`))
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.4", WireAPI: "responses"})
	_, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "no retry"}},
	})
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	if attempts != 1 {
		t.Fatalf("expected forbidden response not to retry, got %d attempts", attempts)
	}
	if !strings.Contains(err.Error(), "llm provider quota or permission error") {
		t.Fatalf("expected actionable quota/permission error, got %v", err)
	}
	if !strings.Contains(err.Error(), "quota exhausted") {
		t.Fatalf("expected provider message in error, got %v", err)
	}
}

func TestOpenAI_ForbiddenCreditErrorIsActionable(t *testing.T) {
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":null,"message":"尊享积分用量不足，请先购买尊享积分或开通套餐","details":null,"data":{},"validationErrors":null}}`))
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.4", WireAPI: "responses"})
	_, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "no credits"}},
	})
	if err == nil {
		t.Fatal("expected forbidden credit error")
	}
	for _, want := range []string{
		"llm provider quota or permission error",
		"HTTP 403",
		"尊享积分用量不足",
		"补充额度",
		"切换可用 profile",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestOpenAI_ResponsesWireAPIStream(t *testing.T) {
	srv := openAIMockServer(t, openAIResponsesStreamHandler([]string{"He", "llo"}))
	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.4", WireAPI: "responses"})

	var received []string
	resp, err := p.Stream(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		Stream:   true,
	}, func(token string) error {
		received = append(received, token)
		return nil
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Fatalf("expected Hello content, got %q", resp.Content)
	}
	if strings.Join(received, "") != "Hello" {
		t.Fatalf("expected streamed tokens to join to Hello, got %q", strings.Join(received, ""))
	}
	if resp.Usage.TotalTokens != 11 {
		t.Fatalf("expected usage total 11, got %+v", resp.Usage)
	}
}

func TestOpenAI_ChatWireAPIAliasUsesChatCompletions(t *testing.T) {
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "chat-ok"}},
			},
			"usage": map[string]any{
				"prompt_tokens":     3,
				"completion_tokens": 2,
				"total_tokens":      5,
			},
		})
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.3-chat", WireAPI: "chat"})
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "chat-ok" {
		t.Fatalf("expected chat-ok content, got %q", resp.Content)
	}
}

func TestOpenAI_ResponsesWireAPIChatWithToolsUsesResponsesAPI(t *testing.T) {
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path for tool call, got %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		tools, ok := body["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("expected one responses tool payload, got %#v", body["tools"])
		}
		tool, ok := tools[0].(map[string]any)
		if !ok {
			t.Fatalf("expected tool object, got %#v", tools[0])
		}
		if tool["type"] != "function" {
			t.Fatalf("expected responses tool type=function, got %#v", tool["type"])
		}
		if tool["name"] != "get_weather" {
			t.Fatalf("expected tool name get_weather, got %#v", tool["name"])
		}
		if body["tool_choice"] != "auto" {
			t.Fatalf("expected tool_choice=auto, got %#v", body["tool_choice"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"id":        "fc_123",
					"type":      "function_call",
					"status":    "completed",
					"call_id":   "call_123",
					"name":      "get_weather",
					"arguments": "{\"city\":\"Beijing\"}",
				},
			},
			"usage": map[string]any{"input_tokens": 3, "output_tokens": 2, "total_tokens": 5},
		})
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-5.4", WireAPI: "responses"})
	resp, err := p.ChatWithTools(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Weather?"}},
	}, []llm.Tool{{
		Name:        "get_weather",
		Description: "Get weather for a city",
		Parameters:  llm.ObjectSchema(map[string]llm.PropertyDef{"city": {Type: "string"}}, []string{"city"}),
	}})
	if err != nil {
		t.Fatalf("tool call error: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("expected responses tool call to return get_weather, got %+v", resp.ToolCalls)
	}
	if resp.ToolCalls[0].ID != "call_123" {
		t.Fatalf("expected call_id to be used as tool call id, got %+v", resp.ToolCalls[0])
	}
}

func TestOpenAI_Stream(t *testing.T) {
	tokens := []string{"Hello", ", ", "world", "!"}
	srv := openAIMockServer(t, openAIStreamHandler(tokens))
	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-4o", WireAPI: "chat"})

	var received []string
	resp, err := p.Stream(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hi"}},
		Stream:   true,
	}, func(token string) error {
		received = append(received, token)
		return nil
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("content mismatch: %q", resp.Content)
	}
	if len(received) != 4 {
		t.Errorf("expected 4 tokens, got %d", len(received))
	}
}

func TestOpenAI_ChatWithTools(t *testing.T) {
	srv := openAIMockServer(t, openAIToolHandler("get_weather", map[string]any{"city": "Beijing"}))
	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-4o", WireAPI: "chat"})

	tools := []llm.Tool{{
		Name:        "get_weather",
		Description: "Get weather for a city",
		Parameters:  llm.ObjectSchema(map[string]llm.PropertyDef{"city": {Type: "string"}}, []string{"city"}),
	}}
	resp, err := p.ChatWithTools(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Weather in Beijing?"}},
	}, tools)
	if err != nil {
		t.Fatalf("tool call error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("tool name mismatch: %q", resp.ToolCalls[0].Name)
	}
}

// --- Anthropic mock server ---

func anthropicChatHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": content},
			},
			"usage": map[string]any{"input_tokens": 8, "output_tokens": 4},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func anthropicStreamHandler(tokens []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// message_start
		start := map[string]any{
			"type":    "message_start",
			"message": map[string]any{"usage": map[string]any{"input_tokens": 8}},
		}
		data, _ := json.Marshal(start)
		fmt.Fprintf(w, "data: %s\n\n", data)

		for _, tok := range tokens {
			chunk := map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]any{"type": "text_delta", "text": tok},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		done := map[string]any{
			"type":  "message_delta",
			"usage": map[string]any{"output_tokens": len(tokens)},
		}
		data, _ = json.Marshal(done)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func TestAnthropic_Chat(t *testing.T) {
	srv := httptest.NewServer(anthropicChatHandler("Hi there!"))
	t.Cleanup(srv.Close)
	p := llm.NewAnthropic(llm.AnthropicConfig{APIKey: "test", BaseURL: srv.URL, Model: "claude-sonnet-4-6"})

	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("chat error: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("content mismatch: %q", resp.Content)
	}
}

func TestAnthropic_Chat_SkipTLSVerify(t *testing.T) {
	srv := httptest.NewTLSServer(anthropicChatHandler("Hi there!"))
	t.Cleanup(srv.Close)

	strict := llm.NewAnthropic(llm.AnthropicConfig{
		APIKey:  "test",
		BaseURL: srv.URL,
		Model:   "claude-sonnet-4-6",
	})
	if _, err := strict.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hello"}},
	}); err == nil {
		t.Fatal("expected strict tls anthropic client to fail against self-signed server")
	}

	insecure := llm.NewAnthropic(llm.AnthropicConfig{
		APIKey:             "test",
		BaseURL:            srv.URL,
		Model:              "claude-sonnet-4-6",
		InsecureSkipVerify: true,
	})
	resp, err := insecure.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("chat with insecure tls disabled should succeed: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("content mismatch: %q", resp.Content)
	}
}

func TestAnthropic_Stream(t *testing.T) {
	tokens := []string{"Hi", " there", "!"}
	srv := httptest.NewServer(anthropicStreamHandler(tokens))
	t.Cleanup(srv.Close)
	p := llm.NewAnthropic(llm.AnthropicConfig{APIKey: "test", BaseURL: srv.URL, Model: "claude-sonnet-4-6"})

	resp, err := p.Stream(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Hello"}},
	}, nil)
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("content mismatch: %q", resp.Content)
	}
}

// --- Router ---

func TestRouter_FallbackToGlobal(t *testing.T) {
	srv := openAIMockServer(t, openAIChatHandler("global response", 5, 3))
	global := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-4o", WireAPI: "chat"})
	router := llm.NewRouter(global)

	p := router.For("unknown-agent")
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "global response" {
		t.Errorf("expected global response, got %q", resp.Content)
	}
}

func TestRouter_PerAgentOverride(t *testing.T) {
	globalSrv := openAIMockServer(t, openAIChatHandler("global", 5, 3))
	agentSrv := openAIMockServer(t, openAIChatHandler("agent-specific", 5, 3))

	global := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: globalSrv.URL, Model: "gpt-4o", WireAPI: "chat"})
	agentP := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: agentSrv.URL, Model: "gpt-4o-mini", WireAPI: "chat"})

	router := llm.NewRouter(global)
	router.Register("writer", agentP)

	resp, err := router.For("writer").Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "write"}},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "agent-specific" {
		t.Errorf("expected agent-specific, got %q", resp.Content)
	}
}

func TestRouter_StreamFallback(t *testing.T) {
	// Simulate a stream that returns < 500 chars then errors
	// The provider should fall back to sync Chat
	callCount := 0
	srv := openAIMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if stream, _ := body["stream"].(bool); stream {
			// Return a broken stream with only a few chars
			w.Header().Set("Content-Type", "text/event-stream")
			chunk := map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{"content": "hi"}}},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			// Abruptly close without [DONE]
			return
		}
		// Sync fallback
		openAIChatHandler("fallback content", 5, 3)(w, r)
	})

	p := llm.NewOpenAI(llm.OpenAIConfig{APIKey: "test", BaseURL: srv.URL, Model: "gpt-4o", WireAPI: "chat"})
	resp, err := p.Stream(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Stream:   true,
	}, nil)
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}
	// Either partial content or fallback content is acceptable
	if resp.Content == "" {
		t.Error("expected non-empty content")
	}
	_ = strings.Contains(resp.Content, "hi") // partial or fallback
}
