package llm

import "storyforge/internal/model"

// Capabilities describes what an LLM provider/model pairing can do for agents.
type Capabilities struct {
	Provider                 string   `json:"provider,omitempty"`
	Model                    string   `json:"model,omitempty"`
	ConfiguredWireAPI        string   `json:"configuredWireApi,omitempty"`
	SupportedWireAPIs        []string `json:"supportedWireApis,omitempty"`
	SupportsChat             bool     `json:"supportsChat"`
	SupportsStreaming        bool     `json:"supportsStreaming"`
	SupportsToolCalls        bool     `json:"supportsToolCalls"`
	SupportsSystemPrompt     bool     `json:"supportsSystemPrompt"`
	SupportsThinkingBudget   bool     `json:"supportsThinkingBudget"`
	ConfiguredThinkingBudget int      `json:"configuredThinkingBudget,omitempty"`
}

// CapabilitiesForRuntime returns the runtime's declared capabilities.
func CapabilitiesForRuntime(runtime CapabilityReporter) Capabilities {
	return runtime.Capabilities()
}

// CapabilitiesForProvider returns the provider's declared capabilities.
func CapabilitiesForProvider(p Provider) Capabilities {
	return CapabilitiesForRuntime(p)
}

// SupportsWireAPI reports whether the provider declares support for a wire API.
func (c Capabilities) SupportsWireAPI(wireAPI string) bool {
	wireAPI = model.NormalizeWireAPI(wireAPI)
	for _, supported := range c.SupportedWireAPIs {
		if model.NormalizeWireAPI(supported) == wireAPI {
			return true
		}
	}
	return false
}
