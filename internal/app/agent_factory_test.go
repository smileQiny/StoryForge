package app

import (
	"context"
	"strings"
	"testing"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

type capabilityTestProvider struct {
	caps llm.Capabilities
}

func (p *capabilityTestProvider) Capabilities() llm.Capabilities {
	return p.caps
}

func (p *capabilityTestProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func (p *capabilityTestProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	return &llm.ToolResponse{}, nil
}

func (p *capabilityTestProvider) Stream(_ context.Context, _ llm.ChatRequest, _ llm.StreamCallback) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func testProjectConfigForAgent(agentName string) model.ProjectConfig {
	return model.ProjectConfig{
		LLM:               model.LLMConfig{Provider: "openai", Model: "global-model"},
		DefaultLLMProfile: "default",
		LLMProfiles: []model.LLMProfile{
			{Name: "default", Provider: "openai", Model: "global-model"},
			{Name: agentName + "-profile", Provider: "openai", Model: agentName + "-model"},
		},
		AgentLLMBindings: []model.AgentLLMBinding{
			{Agent: agentName, Profile: agentName + "-profile"},
		},
	}
}

func TestNewValidatedBaseAgent_RejectsMissingRequiredCapabilities(t *testing.T) {
	cfg := testProjectConfigForAgent("architect")
	router := llm.NewRouter(&capabilityTestProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    true,
			SupportsToolCalls:    true,
			SupportsSystemPrompt: true,
		},
	})
	router.Register("architect", &capabilityTestProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    true,
			SupportsToolCalls:    false,
			SupportsSystemPrompt: true,
		},
	})

	_, _, err := newValidatedBaseAgent(cfg, router, "architect")
	if err == nil {
		t.Fatal("expected architect base agent construction to fail")
	}
	if !strings.Contains(err.Error(), "tool calling") {
		t.Fatalf("expected missing tool calling in error, got %v", err)
	}
}

func TestNewValidatedBaseAgent_AllowsPreferredCapabilityDowngrades(t *testing.T) {
	cfg := testProjectConfigForAgent("writer")
	router := llm.NewRouter(&capabilityTestProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    true,
			SupportsToolCalls:    true,
			SupportsSystemPrompt: true,
		},
	})
	router.Register("writer", &capabilityTestProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    false,
			SupportsToolCalls:    true,
			SupportsSystemPrompt: true,
		},
	})

	base, profile, err := newValidatedBaseAgent(cfg, router, "writer")
	if err != nil {
		t.Fatalf("expected writer base agent construction to succeed, got %v", err)
	}
	if profile != "writer-profile" {
		t.Fatalf("expected writer profile to be selected, got %q", profile)
	}
	if base == nil {
		t.Fatal("expected base agent to be returned")
	}
	if base.Caps.SupportsStreaming {
		t.Fatal("expected writer base agent to preserve downgraded streaming capability")
	}
}
