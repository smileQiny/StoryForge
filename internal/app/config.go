package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"storyforge/internal/model"
)

// ConfigService handles project-level configuration management.
type ConfigService struct {
	dataDir string
	mu      sync.Mutex
}

// ModelRoutes is the API view for global and per-agent model routing.
type ModelRoutes struct {
	Global         model.LLMConfig         `json:"global"`
	DefaultProfile string                  `json:"defaultProfile"`
	Profiles       []model.LLMProfile      `json:"profiles"`
	Agents         []model.AgentLLMBinding `json:"agents"`
}

// NewConfigService creates a ConfigService rooted at dataDir.
func NewConfigService(dataDir string) *ConfigService {
	return &ConfigService{dataDir: dataDir}
}

func (s *ConfigService) path() string {
	return filepath.Join(s.dataDir, "config.json")
}

// Path returns the config file path for compatibility handlers.
func (s *ConfigService) Path() string {
	return s.path()
}

// Get returns project config from disk, or default config if not found.
func (s *ConfigService) Get() (*model.ProjectConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path())
	if os.IsNotExist(err) {
		cfg := defaultProjectConfig(s.dataDir)
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg model.ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}
	normalizeProjectConfig(&cfg, s.dataDir)
	return &cfg, nil
}

// Update validates and persists project config.
func (s *ConfigService) Update(cfg model.ProjectConfig) (*model.ProjectConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeProjectConfig(&cfg, s.dataDir)
	if cfg.MaxConcurrentBooks <= 0 {
		return nil, fmt.Errorf("maxConcurrentBooks must be positive")
	}
	if cfg.LLM.Provider == "" {
		return nil, fmt.Errorf("llm.provider is required")
	}
	if cfg.LLM.Model == "" {
		return nil, fmt.Errorf("llm.model is required")
	}
	if cfg.LLM.ThinkingBudget < 0 {
		return nil, fmt.Errorf("llm.thinkingBudget must be >= 0")
	}
	if len(cfg.LLMProfiles) == 0 {
		return nil, fmt.Errorf("at least one llm profile is required")
	}
	if !model.IsSupportedWireAPI(cfg.LLM.WireAPI) {
		return nil, fmt.Errorf("llm.wireApi must be one of: chat, responses")
	}
	for i, profile := range cfg.LLMProfiles {
		if strings.TrimSpace(profile.Name) == "" {
			return nil, fmt.Errorf("llmProfiles[%d].name is required", i)
		}
		if strings.TrimSpace(profile.Provider) == "" {
			return nil, fmt.Errorf("llmProfiles[%d].provider is required", i)
		}
		if strings.TrimSpace(profile.Model) == "" {
			return nil, fmt.Errorf("llmProfiles[%d].model is required", i)
		}
		if profile.ThinkingBudget < 0 {
			return nil, fmt.Errorf("llmProfiles[%d].thinkingBudget must be >= 0", i)
		}
		if !model.IsSupportedWireAPI(profile.WireAPI) {
			return nil, fmt.Errorf("llmProfiles[%d].wireApi must be one of: chat, responses", i)
		}
	}
	for i, hook := range cfg.Webhooks {
		if !hook.Enabled {
			continue
		}
		if strings.TrimSpace(hook.URL) == "" {
			return nil, fmt.Errorf("webhooks[%d].url is required", i)
		}
		if strings.TrimSpace(hook.Secret) == "" {
			return nil, fmt.Errorf("webhooks[%d].secret is required", i)
		}
		if hook.TimeoutMS < 0 {
			return nil, fmt.Errorf("webhooks[%d].timeoutMs must be >= 0", i)
		}
	}

	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.path(), data, 0o644); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetModelRoutes returns the current global model route plus per-agent overrides.
func (s *ConfigService) GetModelRoutes() (*ModelRoutes, error) {
	cfg, err := s.Get()
	if err != nil {
		return nil, err
	}
	return &ModelRoutes{
		Global:         model.ResolveDefaultLLMConfig(*cfg),
		DefaultProfile: cfg.DefaultLLMProfile,
		Profiles:       append([]model.LLMProfile(nil), cfg.LLMProfiles...),
		Agents:         append([]model.AgentLLMBinding(nil), cfg.AgentLLMBindings...),
	}, nil
}

// SetAgentModel upserts a per-agent LLM profile binding.
func (s *ConfigService) SetAgentModel(agent string, binding model.AgentLLMBinding) (*ModelRoutes, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return nil, fmt.Errorf("agent is required")
	}
	binding.Profile = strings.TrimSpace(binding.Profile)
	if binding.Profile == "" {
		return nil, fmt.Errorf("profile is required")
	}

	cfg, err := s.Get()
	if err != nil {
		return nil, err
	}
	if _, ok := model.FindLLMProfile(*cfg, binding.Profile); !ok {
		return nil, fmt.Errorf("profile %q not found", binding.Profile)
	}
	binding.Agent = agent
	found := false
	for i := range cfg.AgentLLMBindings {
		if cfg.AgentLLMBindings[i].Agent == agent {
			cfg.AgentLLMBindings[i] = binding
			found = true
			break
		}
	}
	if !found {
		cfg.AgentLLMBindings = append(cfg.AgentLLMBindings, binding)
	}
	updated, err := s.Update(*cfg)
	if err != nil {
		return nil, err
	}
	return &ModelRoutes{
		Global:         model.ResolveDefaultLLMConfig(*updated),
		DefaultProfile: updated.DefaultLLMProfile,
		Profiles:       append([]model.LLMProfile(nil), updated.LLMProfiles...),
		Agents:         append([]model.AgentLLMBinding(nil), updated.AgentLLMBindings...),
	}, nil
}

func defaultProjectConfig(dataDir string) model.ProjectConfig {
	cfg := model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            dataDir,
		MaxConcurrentBooks: 1,
		LLM:                model.LLMConfig{Provider: "openai", Model: "gpt-5.4-mini"},
	}
	normalizeProjectConfig(&cfg, dataDir)
	return cfg
}

func normalizeProjectConfig(cfg *model.ProjectConfig, dataDir string) {
	if cfg.DataDir == "" {
		cfg.DataDir = dataDir
	}
	if cfg.MaxConcurrentBooks == 0 {
		cfg.MaxConcurrentBooks = 1
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = "openai"
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "gpt-5.4-mini"
	}
	model.NormalizeLLMProfiles(cfg)
	for i := range cfg.Webhooks {
		if cfg.Webhooks[i].TimeoutMS == 0 {
			cfg.Webhooks[i].TimeoutMS = 5000
		}
	}
}
