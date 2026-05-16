package agent

import (
	"fmt"
	"strings"

	"storyforge/internal/llm"
)

// CapabilityRequirements describes the model features an agent role needs.
type CapabilityRequirements struct {
	Role                   string
	RequiresChat           bool
	RequiresToolCalling    bool
	PrefersStreaming       bool
	PrefersSystemPrompt    bool
	RequiresThinkingBudget bool
}

// RequirementsForRole returns the capability contract for an agent role.
func RequirementsForRole(role string) CapabilityRequirements {
	req := CapabilityRequirements{
		Role:                strings.TrimSpace(role),
		RequiresChat:        true,
		PrefersSystemPrompt: true,
	}
	switch req.Role {
	case "architect":
		req.RequiresToolCalling = true
	case "writer":
		req.PrefersStreaming = true
	}
	return req
}

// ValidateRuntimeForRole checks whether a runtime satisfies an agent role.
func ValidateRuntimeForRole(role, modelName string, runtime llm.AgentRuntime) error {
	req := RequirementsForRole(role)
	caps := runtime.Capabilities()

	missing := make([]string, 0, 3)
	if req.RequiresChat && !caps.SupportsChat {
		missing = append(missing, "chat")
	}
	if req.RequiresToolCalling && !caps.SupportsToolCalls {
		missing = append(missing, "tool calling")
	}
	if req.RequiresThinkingBudget && !caps.SupportsThinkingBudget {
		missing = append(missing, "thinking budget")
	}
	if len(missing) == 0 {
		return nil
	}

	role = strings.TrimSpace(role)
	if role == "" {
		role = "unknown"
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = caps.Model
	}
	return fmt.Errorf("agent %s model %s is missing required capabilities: %s", role, strings.TrimSpace(modelName), strings.Join(missing, ", "))
}

// ValidateProviderForRole is kept as a compatibility wrapper for call sites
// that still speak in terms of providers.
func ValidateProviderForRole(role, modelName string, provider llm.Provider) error {
	return ValidateRuntimeForRole(role, modelName, provider)
}

// MissingPreferredCapabilities reports downgraded-but-supported execution modes.
func MissingPreferredCapabilities(role string, runtime llm.AgentRuntime) []string {
	req := RequirementsForRole(role)
	caps := runtime.Capabilities()
	missing := make([]string, 0, 2)
	if req.PrefersStreaming && !caps.SupportsStreaming {
		missing = append(missing, "streaming")
	}
	if req.PrefersSystemPrompt && !caps.SupportsSystemPrompt {
		missing = append(missing, "native system prompt")
	}
	return missing
}
