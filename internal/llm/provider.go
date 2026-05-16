package llm

import "context"

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"` // system/user/assistant/tool
	Content string `json:"content"`
	Name    string `json:"name,omitempty"` // for tool results
}

// ChatRequest is the input to a chat completion call.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"maxTokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse is the output of a non-streaming chat call.
type ChatResponse struct {
	Content string     `json:"content"`
	Usage   TokenUsage `json:"usage"`
}

// TokenUsage records token consumption.
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// StreamCallback is called for each token chunk during streaming.
// Return a non-nil error to abort the stream.
type StreamCallback func(token string) error

// ToolResponse is the output of a tool-calling chat completion.
type ToolResponse struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	Usage     TokenUsage `json:"usage"`
}

// CapabilityReporter exposes the feature set available to agents.
type CapabilityReporter interface {
	// Capabilities reports the feature set exposed to agents.
	Capabilities() Capabilities
}

// ChatProvider supports non-streaming chat completions.
type ChatProvider interface {
	CapabilityReporter

	// Chat performs a single non-streaming chat completion.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

// ToolCallingProvider supports chat completions with tool calling.
type ToolCallingProvider interface {
	CapabilityReporter

	// ChatWithTools performs a chat completion with tool-calling support.
	ChatWithTools(ctx context.Context, req ChatRequest, tools []Tool) (*ToolResponse, error)
}

// StreamingProvider supports streaming chat completions.
type StreamingProvider interface {
	CapabilityReporter

	// Stream performs a streaming chat completion, calling cb for each token.
	// If the stream is interrupted after >= 500 chars, the partial content is returned.
	Stream(ctx context.Context, req ChatRequest, cb StreamCallback) (*ChatResponse, error)
}

// AgentRuntime is the minimal contract the agent layer needs from an LLM.
// Different provider adapters can satisfy this contract without the agent
// package needing to know any provider-specific details.
type AgentRuntime interface {
	ChatProvider
	ToolCallingProvider
	StreamingProvider
}

// Provider is the unified backend adapter interface for LLM implementations.
// Providers satisfy the narrower AgentRuntime contract used by agents.
type Provider interface {
	AgentRuntime
}
