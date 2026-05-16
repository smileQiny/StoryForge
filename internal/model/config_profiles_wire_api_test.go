package model_test

import (
	"testing"

	"storyforge/internal/model"
)

func TestResolveAgentLLMConfig_PreservesWireAPI(t *testing.T) {
	cfg := model.ProjectConfig{
		Language:          "zh",
		DefaultLLMProfile: "default",
		LLM: model.LLMConfig{
			Provider: "openai",
			Model:    "gpt-5.3-chat",
			WireAPI:  "chat",
		},
		LLMProfiles: []model.LLMProfile{
			{
				Name:     "default",
				Provider: "openai",
				Model:    "gpt-5.3-chat",
				WireAPI:  "chat",
			},
			{
				Name:     "chatgpt-54-responses",
				Provider: "openai",
				Model:    "gpt-5.4",
				WireAPI:  "responses",
			},
		},
		AgentLLMBindings: []model.AgentLLMBinding{
			{Agent: "writer", Profile: "chatgpt-54-responses"},
		},
	}

	resolved := model.ResolveAgentLLMConfig(cfg, "writer")
	if resolved.WireAPI != "responses" {
		t.Fatalf("expected writer wire api responses, got %q", resolved.WireAPI)
	}
}

func TestResolveAgentLLMConfig_PreservesThinkingBudget(t *testing.T) {
	cfg := model.ProjectConfig{
		Language:          "zh",
		DefaultLLMProfile: "default",
		LLM: model.LLMConfig{
			Provider:       "anthropic",
			Model:          "claude-sonnet-4-5",
			ThinkingBudget: 0,
		},
		LLMProfiles: []model.LLMProfile{
			{
				Name:           "default",
				Provider:       "anthropic",
				Model:          "claude-sonnet-4-5",
				ThinkingBudget: 0,
			},
			{
				Name:           "claude-deep",
				Provider:       "anthropic",
				Model:          "claude-sonnet-4-6",
				ThinkingBudget: 4096,
			},
		},
		AgentLLMBindings: []model.AgentLLMBinding{
			{Agent: "architect", Profile: "claude-deep"},
		},
	}

	resolved := model.ResolveAgentLLMConfig(cfg, "architect")
	if resolved.ThinkingBudget != 4096 {
		t.Fatalf("expected architect thinking budget 4096, got %d", resolved.ThinkingBudget)
	}
}

func TestNormalizeWireAPI_AcceptsChatAliases(t *testing.T) {
	cases := map[string]string{
		"":                 model.WireAPIResponses,
		"chat":             model.WireAPIChat,
		"CHAT":             model.WireAPIChat,
		"chat_completions": model.WireAPIChat,
		"chat-completions": model.WireAPIChat,
		"responses":        model.WireAPIResponses,
	}

	for input, want := range cases {
		if got := model.NormalizeWireAPI(input); got != want {
			t.Fatalf("NormalizeWireAPI(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveAgentLLMConfig_PreservesSkipTLSVerify(t *testing.T) {
	cfg := model.ProjectConfig{
		Language:          "zh",
		DefaultLLMProfile: "default",
		LLM: model.LLMConfig{
			Provider:      "claude",
			Model:         "claude-sonnet-4-5",
			SkipTLSVerify: false,
		},
		LLMProfiles: []model.LLMProfile{
			{
				Name:          "default",
				Provider:      "claude",
				Model:         "claude-sonnet-4-5",
				SkipTLSVerify: false,
			},
			{
				Name:          "claude-local",
				Provider:      "claude",
				Model:         "claude-sonnet-4-6",
				BaseURL:       "https://gaccode.com/claudecode",
				SkipTLSVerify: true,
			},
		},
		AgentLLMBindings: []model.AgentLLMBinding{
			{Agent: "architect", Profile: "claude-local"},
		},
	}

	resolved := model.ResolveAgentLLMConfig(cfg, "architect")
	if !resolved.SkipTLSVerify {
		t.Fatal("expected architect skip tls verify to be preserved")
	}
}
