package llm_test

import (
	"context"
	"strings"
	"testing"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

type registryStubProvider struct{}

func (p *registryStubProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (p *registryStubProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func (p *registryStubProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	return &llm.ToolResponse{}, nil
}

func (p *registryStubProvider) Stream(_ context.Context, _ llm.ChatRequest, _ llm.StreamCallback) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

func TestBuildProvider_UsesRegisteredFactory(t *testing.T) {
	name := "registry-test-provider"
	var captured llm.ProviderFactoryConfig

	err := llm.RegisterProviderFactory(name, func(cfg llm.ProviderFactoryConfig) (llm.Provider, error) {
		captured = cfg
		return &registryStubProvider{}, nil
	})
	if err != nil {
		t.Fatalf("register provider factory: %v", err)
	}

	provider, err := llm.BuildProvider(model.LLMConfig{
		Provider:       name,
		Model:          "test-model",
		BaseURL:        "https://example.invalid/v1",
		APIKey:         "test-key",
		WireAPI:        "responses",
		ThinkingBudget: 128,
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider to be returned")
	}
	if captured.Provider != name {
		t.Fatalf("expected provider name %q, got %q", name, captured.Provider)
	}
	if captured.Model != "test-model" {
		t.Fatalf("expected model to flow into factory, got %q", captured.Model)
	}
	if captured.WireAPI != model.WireAPIResponses {
		t.Fatalf("expected normalized wire api %q, got %q", model.WireAPIResponses, captured.WireAPI)
	}
	if captured.ThinkingBudget != 128 {
		t.Fatalf("expected thinking budget 128, got %d", captured.ThinkingBudget)
	}
}

func TestBuildProvider_RejectsUnknownProvider(t *testing.T) {
	_, err := llm.BuildProvider(model.LLMConfig{Provider: "definitely-not-registered"})
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown provider error, got %v", err)
	}
}

func TestRegisterProviderFactory_RejectsDuplicateName(t *testing.T) {
	name := "registry-duplicate-provider"
	factory := func(cfg llm.ProviderFactoryConfig) (llm.Provider, error) {
		return &registryStubProvider{}, nil
	}

	if err := llm.RegisterProviderFactory(name, factory); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := llm.RegisterProviderFactory(name, factory); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
}

func TestBuildProvider_ClaudeAliasUsesAnthropicRuntime(t *testing.T) {
	provider, err := llm.BuildProvider(model.LLMConfig{
		Provider:       "claude",
		Model:          "claude-sonnet-4-6",
		BaseURL:        "https://example.invalid/claude",
		APIKey:         "test-key",
		ThinkingBudget: 2048,
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}

	caps := provider.Capabilities()
	if !caps.SupportsThinkingBudget {
		t.Fatal("expected claude alias to use anthropic runtime capabilities")
	}
	if caps.ConfiguredWireAPI != model.WireAPIChat {
		t.Fatalf("expected claude alias to use chat wire api, got %q", caps.ConfiguredWireAPI)
	}
	if caps.ConfiguredThinkingBudget != 2048 {
		t.Fatalf("expected thinking budget 2048, got %d", caps.ConfiguredThinkingBudget)
	}
}
