package llm

import (
	"fmt"
	"strings"
	"sync"

	"storyforge/internal/model"
)

// ProviderFactoryConfig is the normalized input used to construct a provider.
type ProviderFactoryConfig struct {
	Provider       string
	Model          string
	BaseURL        string
	APIKey         string
	SkipTLSVerify  bool
	WireAPI        string
	ThinkingBudget int
}

// ProviderFactory creates a provider for a given normalized config.
type ProviderFactory func(cfg ProviderFactoryConfig) (Provider, error)

type providerRegistry struct {
	mu        sync.RWMutex
	factories map[string]ProviderFactory
}

var defaultProviderRegistry = newProviderRegistry()

func newProviderRegistry() *providerRegistry {
	registry := &providerRegistry{
		factories: make(map[string]ProviderFactory),
	}
	registry.mustRegister("openai", func(cfg ProviderFactoryConfig) (Provider, error) {
		return NewOpenAI(OpenAIConfig{
			APIKey:             cfg.APIKey,
			BaseURL:            cfg.BaseURL,
			Model:              cfg.Model,
			WireAPI:            cfg.WireAPI,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}), nil
	})
	registry.mustRegister("custom", func(cfg ProviderFactoryConfig) (Provider, error) {
		return NewOpenAI(OpenAIConfig{
			APIKey:             cfg.APIKey,
			BaseURL:            cfg.BaseURL,
			Model:              cfg.Model,
			WireAPI:            cfg.WireAPI,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}), nil
	})
	registry.mustRegister("claude", func(cfg ProviderFactoryConfig) (Provider, error) {
		return NewAnthropic(AnthropicConfig{
			APIKey:             cfg.APIKey,
			BaseURL:            cfg.BaseURL,
			Model:              cfg.Model,
			ThinkingBudget:     cfg.ThinkingBudget,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}), nil
	})
	registry.mustRegister("anthropic", func(cfg ProviderFactoryConfig) (Provider, error) {
		return NewAnthropic(AnthropicConfig{
			APIKey:             cfg.APIKey,
			BaseURL:            cfg.BaseURL,
			Model:              cfg.Model,
			ThinkingBudget:     cfg.ThinkingBudget,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}), nil
	})
	return registry
}

func (r *providerRegistry) mustRegister(name string, factory ProviderFactory) {
	if err := r.register(name, factory); err != nil {
		panic(err)
	}
}

func (r *providerRegistry) register(name string, factory ProviderFactory) error {
	name = normalizeProviderName(name)
	if name == "" {
		return fmt.Errorf("provider name is required")
	}
	if factory == nil {
		return fmt.Errorf("provider factory %q is nil", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("provider factory %q already registered", name)
	}
	r.factories[name] = factory
	return nil
}

func (r *providerRegistry) build(cfg ProviderFactoryConfig) (Provider, error) {
	name := normalizeProviderName(cfg.Provider)

	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", strings.TrimSpace(cfg.Provider))
	}

	cfg.Provider = name
	cfg.WireAPI = model.NormalizeWireAPI(cfg.WireAPI)
	return factory(cfg)
}

// RegisterProviderFactory adds a provider adapter factory to the shared LLM registry.
// This keeps router assembly stable as new model families are added.
func RegisterProviderFactory(name string, factory ProviderFactory) error {
	return defaultProviderRegistry.register(name, factory)
}

// BuildProvider constructs a provider from a project LLM config through the registry.
func BuildProvider(cfg model.LLMConfig) (Provider, error) {
	return defaultProviderRegistry.build(ProviderFactoryConfig{
		Provider:       cfg.Provider,
		Model:          cfg.Model,
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		SkipTLSVerify:  cfg.SkipTLSVerify,
		WireAPI:        cfg.WireAPI,
		ThinkingBudget: cfg.ThinkingBudget,
	})
}

func normalizeProviderName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "openai"
	}
	return name
}
