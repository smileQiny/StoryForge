package llm_test

import (
	"testing"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

func TestOpenAIProvider_Capabilities(t *testing.T) {
	provider := llm.NewOpenAI(llm.OpenAIConfig{
		Model:   "gpt-5.4",
		WireAPI: model.WireAPIResponses,
	})

	caps := provider.Capabilities()
	if !caps.SupportsStreaming {
		t.Fatalf("expected openai provider to support streaming")
	}
	if !caps.SupportsToolCalls {
		t.Fatalf("expected openai provider to support tool calls")
	}
	if !caps.SupportsSystemPrompt {
		t.Fatalf("expected openai provider to support system prompts")
	}
	if caps.SupportsThinkingBudget {
		t.Fatalf("expected openai provider to not expose thinking budget support")
	}
	if caps.ConfiguredWireAPI != model.WireAPIResponses {
		t.Fatalf("expected configured wire api responses, got %q", caps.ConfiguredWireAPI)
	}
	if !caps.SupportsWireAPI(model.WireAPIChat) || !caps.SupportsWireAPI(model.WireAPIResponses) {
		t.Fatalf("expected openai provider to advertise chat and responses wire api support, got %+v", caps)
	}
}

func TestAnthropicProvider_Capabilities(t *testing.T) {
	provider := llm.NewAnthropic(llm.AnthropicConfig{
		Model:          "claude-sonnet-4-6",
		ThinkingBudget: 2048,
	})

	caps := provider.Capabilities()
	if !caps.SupportsStreaming {
		t.Fatalf("expected anthropic provider to support streaming")
	}
	if !caps.SupportsToolCalls {
		t.Fatalf("expected anthropic provider to support tool calls")
	}
	if !caps.SupportsSystemPrompt {
		t.Fatalf("expected anthropic provider to support system prompts")
	}
	if !caps.SupportsThinkingBudget {
		t.Fatalf("expected anthropic provider to expose thinking budget support")
	}
	if caps.ConfiguredThinkingBudget != 2048 {
		t.Fatalf("expected configured thinking budget 2048, got %d", caps.ConfiguredThinkingBudget)
	}
}
