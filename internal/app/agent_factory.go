package app

import (
	"storyforge/internal/agent"
	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// newValidatedBaseAgent constructs an agent base only after the routed model
// satisfies the role's required capabilities. This stays backend-internal so
// capability details do not leak into API contracts.
func newValidatedBaseAgent(cfg model.ProjectConfig, router *llm.Router, agentName string) (*agent.BaseAgent, string, error) {
	resolved := model.ResolveAgentLLMConfig(cfg, agentName)
	runtime := router.ForAgent(agentName)
	if err := agent.ValidateRuntimeForRole(agentName, resolved.Model, runtime); err != nil {
		return nil, "", err
	}
	return agent.NewBaseAgent(agentName, runtime, resolved.Model), profileNameForAgent(cfg, agentName), nil
}
