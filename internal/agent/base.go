// Package agent implements the core AI agents for the writing pipeline.
package agent

import (
	"context"
	"fmt"
	"strings"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// BaseAgent provides shared Chat/Stream/ChatWithTools helpers for all agents.
type BaseAgent struct {
	Role    string
	Runtime llm.AgentRuntime
	Model   string
	Caps    llm.Capabilities
}

// NewBaseAgent creates a BaseAgent.
func NewBaseAgent(role string, runtime llm.AgentRuntime, model string) *BaseAgent {
	return &BaseAgent{
		Role:    role,
		Runtime: runtime,
		Model:   model,
		Caps:    llm.CapabilitiesForRuntime(runtime),
	}
}

// Chat performs a non-streaming chat completion and returns the response text.
func (a *BaseAgent) Chat(ctx context.Context, system, user string) (string, *llm.TokenUsage, error) {
	req := a.newRequest(system, user)
	resp, err := a.Runtime.Chat(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("agent %s chat: %w", a.Role, err)
	}
	return resp.Content, &resp.Usage, nil
}

// Stream performs a streaming chat completion, calling cb for each token.
func (a *BaseAgent) Stream(ctx context.Context, system, user string, cb llm.StreamCallback) (string, *llm.TokenUsage, error) {
	if !a.Caps.SupportsStreaming {
		content, usage, err := a.Chat(ctx, system, user)
		if err != nil {
			return "", nil, err
		}
		if cb != nil && content != "" {
			if err := cb(content); err != nil {
				return "", nil, fmt.Errorf("agent %s stream callback: %w", a.Role, err)
			}
		}
		return content, usage, nil
	}
	req := a.newRequest(system, user)
	req.Stream = true
	resp, err := a.Runtime.Stream(ctx, req, cb)
	if err != nil {
		return "", nil, fmt.Errorf("agent %s stream: %w", a.Role, err)
	}
	return resp.Content, &resp.Usage, nil
}

// StreamWithMaxTokens performs a streaming chat completion with an explicit
// output token budget.
func (a *BaseAgent) StreamWithMaxTokens(ctx context.Context, system, user string, maxTokens int, cb llm.StreamCallback) (string, *llm.TokenUsage, error) {
	if !a.Caps.SupportsStreaming {
		req := a.newRequest(system, user)
		req.MaxTokens = maxTokens
		resp, err := a.Runtime.Chat(ctx, req)
		if err != nil {
			return "", nil, fmt.Errorf("agent %s chat: %w", a.Role, err)
		}
		if cb != nil && resp.Content != "" {
			if err := cb(resp.Content); err != nil {
				return "", nil, fmt.Errorf("agent %s stream callback: %w", a.Role, err)
			}
		}
		return resp.Content, &resp.Usage, nil
	}
	req := a.newRequest(system, user)
	req.Stream = true
	req.MaxTokens = maxTokens
	resp, err := a.Runtime.Stream(ctx, req, cb)
	if err != nil {
		return "", nil, fmt.Errorf("agent %s stream: %w", a.Role, err)
	}
	return resp.Content, &resp.Usage, nil
}

// ChatWithTools performs a tool-calling chat completion.
func (a *BaseAgent) ChatWithTools(ctx context.Context, system, user string, tools []llm.Tool) (*llm.ToolResponse, error) {
	if !a.Caps.SupportsToolCalls {
		return nil, fmt.Errorf("agent %s model %s does not support tool calling", a.Role, a.Model)
	}
	req := llm.ChatRequest{
		Model:    a.Model,
		Messages: a.messages(system, user),
	}
	resp, err := a.Runtime.ChatWithTools(ctx, req, tools)
	if err != nil {
		return nil, fmt.Errorf("agent %s tool-call: %w", a.Role, err)
	}
	return resp, nil
}

func (a *BaseAgent) newRequest(system, user string) llm.ChatRequest {
	return llm.ChatRequest{
		Model:    a.Model,
		Messages: a.messages(system, user),
	}
}

func (a *BaseAgent) messages(system, user string) []llm.Message {
	messages := make([]llm.Message, 0, 2)
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)
	if system != "" && a.Caps.SupportsSystemPrompt {
		messages = append(messages, llm.Message{Role: "system", Content: system})
	} else if system != "" {
		user = strings.TrimSpace("System instructions:\n" + system + "\n\nUser request:\n" + user)
	}
	messages = append(messages, llm.Message{Role: "user", Content: user})
	return messages
}

// ToModelUsage converts llm.TokenUsage to model.TokenUsage.
func ToModelUsage(u *llm.TokenUsage) *model.TokenUsage {
	if u == nil {
		return nil
	}
	return &model.TokenUsage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
	}
}

// ExtractJSON attempts to extract a JSON object or array from a response that
// may contain markdown code fences or surrounding prose.
func ExtractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	}

	s = strings.TrimSpace(s)

	// Find first { or [
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return s
	}
	return s[start:]
}
