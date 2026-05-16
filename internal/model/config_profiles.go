package model

import "strings"

const DefaultLLMProfileName = "default"

// NormalizeLLMProfiles backfills profile-based routing from legacy config fields.
func NormalizeLLMProfiles(cfg *ProjectConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "StoryForge"
	}
	if strings.TrimSpace(cfg.Language) == "" {
		cfg.Language = "zh"
	}

	cfg.LLM = normalizeLLMConfig(cfg.LLM)

	if len(cfg.LLMProfiles) == 0 {
		cfg.LLMProfiles = []LLMProfile{profileFromConfig(firstNonEmpty(cfg.DefaultLLMProfile, DefaultLLMProfileName), cfg.Language, cfg.LLM)}
	}

	names := make(map[string]struct{}, len(cfg.LLMProfiles))
	normalizedProfiles := make([]LLMProfile, 0, len(cfg.LLMProfiles))
	for idx, profile := range cfg.LLMProfiles {
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.Name == "" {
			if idx == 0 {
				profile.Name = DefaultLLMProfileName
			} else {
				profile.Name = DefaultLLMProfileName + "-" + strings.TrimSpace(strings.ToLower(profile.Model))
			}
		}
		if _, exists := names[profile.Name]; exists {
			continue
		}
		profile.Language = firstNonEmpty(profile.Language, cfg.Language)
		profile.Provider = firstNonEmpty(profile.Provider, cfg.LLM.Provider)
		profile.Model = firstNonEmpty(profile.Model, cfg.LLM.Model)
		profile.WireAPI = NormalizeWireAPI(profile.WireAPI)
		if strings.TrimSpace(profile.BaseURL) == "" {
			profile.BaseURL = cfg.LLM.BaseURL
		}
		if strings.TrimSpace(profile.APIKey) == "" {
			profile.APIKey = cfg.LLM.APIKey
		}
		if !profile.SkipTLSVerify {
			profile.SkipTLSVerify = cfg.LLM.SkipTLSVerify
		}
		if strings.TrimSpace(profile.WireAPI) == "" {
			profile.WireAPI = cfg.LLM.WireAPI
		}
		if profile.ThinkingBudget == 0 {
			profile.ThinkingBudget = cfg.LLM.ThinkingBudget
		}
		normalizedProfiles = append(normalizedProfiles, profile)
		names[profile.Name] = struct{}{}
	}
	cfg.LLMProfiles = normalizedProfiles

	if strings.TrimSpace(cfg.DefaultLLMProfile) == "" {
		cfg.DefaultLLMProfile = cfg.LLMProfiles[0].Name
	}
	if _, ok := names[cfg.DefaultLLMProfile]; !ok {
		cfg.DefaultLLMProfile = cfg.LLMProfiles[0].Name
	}

	bindings := make([]AgentLLMBinding, 0, len(cfg.AgentLLMBindings)+len(cfg.LLM.AgentOverrides))
	seenAgents := map[string]struct{}{}
	for _, binding := range cfg.AgentLLMBindings {
		agent := strings.TrimSpace(binding.Agent)
		if agent == "" {
			continue
		}
		profile := strings.TrimSpace(binding.Profile)
		if profile == "" {
			profile = cfg.DefaultLLMProfile
		}
		if _, ok := names[profile]; !ok {
			profile = cfg.DefaultLLMProfile
		}
		if _, exists := seenAgents[agent]; exists {
			continue
		}
		bindings = append(bindings, AgentLLMBinding{Agent: agent, Profile: profile})
		seenAgents[agent] = struct{}{}
	}

	for _, override := range cfg.LLM.AgentOverrides {
		agent := strings.TrimSpace(override.Agent)
		if agent == "" {
			continue
		}
		profileName := agent + "-profile"
		if _, exists := names[profileName]; !exists {
			base := cfg.LLM
			if strings.TrimSpace(override.Provider) != "" {
				base.Provider = override.Provider
			}
			if strings.TrimSpace(override.Model) != "" {
				base.Model = override.Model
			}
			if strings.TrimSpace(override.BaseURL) != "" {
				base.BaseURL = override.BaseURL
			}
			if strings.TrimSpace(override.APIKey) != "" {
				base.APIKey = override.APIKey
			}
			if override.SkipTLSVerify {
				base.SkipTLSVerify = true
			}
			if strings.TrimSpace(override.WireAPI) != "" {
				base.WireAPI = override.WireAPI
			}
			if override.ThinkingBudget > 0 {
				base.ThinkingBudget = override.ThinkingBudget
			}
			cfg.LLMProfiles = append(cfg.LLMProfiles, profileFromConfig(profileName, cfg.Language, base))
			names[profileName] = struct{}{}
		}
		if _, exists := seenAgents[agent]; exists {
			continue
		}
		bindings = append(bindings, AgentLLMBinding{Agent: agent, Profile: profileName})
		seenAgents[agent] = struct{}{}
	}

	cfg.AgentLLMBindings = bindings
	cfg.LLM.AgentOverrides = nil
	cfg.LLM = ResolveDefaultLLMConfig(*cfg)
}

// ResolveDefaultLLMConfig returns the active default config.
func ResolveDefaultLLMConfig(cfg ProjectConfig) LLMConfig {
	if profile, ok := FindLLMProfile(cfg, cfg.DefaultLLMProfile); ok {
		return configFromProfile(profile)
	}
	return normalizeLLMConfig(cfg.LLM)
}

// ResolveAgentLLMConfig returns the effective config for an agent.
func ResolveAgentLLMConfig(cfg ProjectConfig, agent string) LLMConfig {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return ResolveDefaultLLMConfig(cfg)
	}
	for _, binding := range cfg.AgentLLMBindings {
		if strings.TrimSpace(binding.Agent) != agent {
			continue
		}
		if profile, ok := FindLLMProfile(cfg, binding.Profile); ok {
			return configFromProfile(profile)
		}
		break
	}
	for _, override := range cfg.LLM.AgentOverrides {
		if strings.TrimSpace(override.Agent) != agent {
			continue
		}
		resolved := ResolveDefaultLLMConfig(cfg)
		if strings.TrimSpace(override.Provider) != "" {
			resolved.Provider = override.Provider
		}
		if strings.TrimSpace(override.Model) != "" {
			resolved.Model = override.Model
		}
		if strings.TrimSpace(override.BaseURL) != "" {
			resolved.BaseURL = override.BaseURL
		}
		if strings.TrimSpace(override.APIKey) != "" {
			resolved.APIKey = override.APIKey
		}
		if override.SkipTLSVerify {
			resolved.SkipTLSVerify = true
		}
		if strings.TrimSpace(override.WireAPI) != "" {
			resolved.WireAPI = override.WireAPI
		}
		if override.ThinkingBudget > 0 {
			resolved.ThinkingBudget = override.ThinkingBudget
		}
		return resolved
	}
	return ResolveDefaultLLMConfig(cfg)
}

// FindLLMProfile looks up a profile by name.
func FindLLMProfile(cfg ProjectConfig, name string) (LLMProfile, bool) {
	needle := strings.TrimSpace(name)
	if needle == "" {
		return LLMProfile{}, false
	}
	for _, profile := range cfg.LLMProfiles {
		if strings.TrimSpace(profile.Name) == needle {
			return profile, true
		}
	}
	return LLMProfile{}, false
}

// ConfiguredLLMAgents returns every agent with an explicit profile binding or legacy override.
func ConfiguredLLMAgents(cfg ProjectConfig) []string {
	seen := map[string]struct{}{}
	agents := make([]string, 0, len(cfg.AgentLLMBindings)+len(cfg.LLM.AgentOverrides))
	for _, binding := range cfg.AgentLLMBindings {
		agent := strings.TrimSpace(binding.Agent)
		if agent == "" {
			continue
		}
		if _, ok := seen[agent]; ok {
			continue
		}
		agents = append(agents, agent)
		seen[agent] = struct{}{}
	}
	for _, override := range cfg.LLM.AgentOverrides {
		agent := strings.TrimSpace(override.Agent)
		if agent == "" {
			continue
		}
		if _, ok := seen[agent]; ok {
			continue
		}
		agents = append(agents, agent)
		seen[agent] = struct{}{}
	}
	return agents
}

func profileFromConfig(name, language string, cfg LLMConfig) LLMProfile {
	cfg = normalizeLLMConfig(cfg)
	return LLMProfile{
		Name:           strings.TrimSpace(name),
		Language:       firstNonEmpty(language, "zh"),
		Provider:       cfg.Provider,
		Model:          cfg.Model,
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		SkipTLSVerify:  cfg.SkipTLSVerify,
		WireAPI:        cfg.WireAPI,
		Stream:         cfg.Stream,
		Temperature:    cfg.Temperature,
		MaxTokens:      cfg.MaxTokens,
		ThinkingBudget: cfg.ThinkingBudget,
	}
}

func configFromProfile(profile LLMProfile) LLMConfig {
	return normalizeLLMConfig(LLMConfig{
		Provider:       profile.Provider,
		Model:          profile.Model,
		BaseURL:        profile.BaseURL,
		APIKey:         profile.APIKey,
		SkipTLSVerify:  profile.SkipTLSVerify,
		WireAPI:        profile.WireAPI,
		Stream:         profile.Stream,
		Temperature:    profile.Temperature,
		MaxTokens:      profile.MaxTokens,
		ThinkingBudget: profile.ThinkingBudget,
	})
}

func normalizeLLMConfig(cfg LLMConfig) LLMConfig {
	cfg.Provider = firstNonEmpty(cfg.Provider, "openai")
	cfg.Model = firstNonEmpty(cfg.Model, "gpt-5.4-mini")
	cfg.WireAPI = NormalizeWireAPI(cfg.WireAPI)
	return cfg
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
