package model

const (
	WireAPIChat      = "chat"
	WireAPIResponses = "responses"
)

// ProjectConfig holds top-level project configuration.
type ProjectConfig struct {
	Name               string            `json:"name,omitempty"`
	Language           string            `json:"language,omitempty"`
	DataDir            string            `json:"dataDir"`
	LLM                LLMConfig         `json:"llm"`
	LLMProfiles        []LLMProfile      `json:"llmProfiles,omitempty"`
	DefaultLLMProfile  string            `json:"defaultLlmProfile,omitempty"`
	AgentLLMBindings   []AgentLLMBinding `json:"agentLlmBindings,omitempty"`
	MaxConcurrentBooks int               `json:"maxConcurrentBooks"`
	Webhooks           []WebhookConfig   `json:"webhooks,omitempty"`
}

// LLMConfig holds global LLM provider settings.
type LLMConfig struct {
	Provider       string               `json:"provider"` // openai / anthropic
	Model          string               `json:"model"`
	BaseURL        string               `json:"baseUrl,omitempty"`
	APIKey         string               `json:"apiKey,omitempty"`
	SkipTLSVerify  bool                 `json:"skipTlsVerify,omitempty"`
	WireAPI        string               `json:"wireApi,omitempty"`
	Stream         bool                 `json:"stream,omitempty"`
	Temperature    float64              `json:"temperature,omitempty"`
	MaxTokens      int                  `json:"maxTokens,omitempty"`
	ThinkingBudget int                  `json:"thinkingBudget,omitempty"`
	AgentOverrides []AgentModelOverride `json:"agentOverrides,omitempty"`
}

// LLMProfile is a named reusable LLM configuration.
type LLMProfile struct {
	Name           string  `json:"name"`
	Language       string  `json:"language,omitempty"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	BaseURL        string  `json:"baseUrl,omitempty"`
	APIKey         string  `json:"apiKey,omitempty"`
	SkipTLSVerify  bool    `json:"skipTlsVerify,omitempty"`
	WireAPI        string  `json:"wireApi,omitempty"`
	Stream         bool    `json:"stream,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	MaxTokens      int     `json:"maxTokens,omitempty"`
	ThinkingBudget int     `json:"thinkingBudget,omitempty"`
}

// AgentLLMBinding maps an agent to a named LLM profile.
type AgentLLMBinding struct {
	Agent   string `json:"agent"`
	Profile string `json:"profile"`
}

// AgentModelOverride allows per-agent model configuration.
type AgentModelOverride struct {
	Agent          string `json:"agent"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model"`
	BaseURL        string `json:"baseUrl,omitempty"`
	APIKey         string `json:"apiKey,omitempty"`
	SkipTLSVerify  bool   `json:"skipTlsVerify,omitempty"`
	WireAPI        string `json:"wireApi,omitempty"`
	ThinkingBudget int    `json:"thinkingBudget,omitempty"`
}

// WebhookConfig configures a signed outbound webhook subscription.
type WebhookConfig struct {
	Enabled      bool              `json:"enabled"`
	URL          string            `json:"url"`
	Secret       string            `json:"secret"`
	EventFilters []string          `json:"eventFilters,omitempty"`
	TimeoutMS    int               `json:"timeoutMs,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
}
