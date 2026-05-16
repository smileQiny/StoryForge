package llm

import (
	"context"
	"fmt"

	"storyforge/internal/model"
)

// Router dispatches LLM calls to per-agent providers, falling back to a global provider.
type Router struct {
	global Provider
	agents map[string]Provider
}

// NewRouter creates a Router with a global fallback provider.
func NewRouter(global Provider) *Router {
	return &Router{
		global: global,
		agents: make(map[string]Provider),
	}
}

// Register sets a per-agent provider override.
func (r *Router) Register(agentName string, p Provider) {
	r.agents[agentName] = p
}

// For returns the provider for the given agent name.
// Falls back to the global provider if no override is registered.
func (r *Router) For(agentName string) Provider {
	if p, ok := r.agents[agentName]; ok {
		return p
	}
	return r.global
}

// BuildFromConfig constructs a Router from a ProjectConfig.
func BuildFromConfig(cfg model.ProjectConfig) (*Router, error) {
	globalCfg := model.ResolveDefaultLLMConfig(cfg)
	global, err := BuildProvider(globalCfg)
	if err != nil {
		return nil, fmt.Errorf("global provider: %w", err)
	}
	router := NewRouter(global)
	for _, agentName := range model.ConfiguredLLMAgents(cfg) {
		agentCfg := model.ResolveAgentLLMConfig(cfg, agentName)
		p, err := BuildProvider(agentCfg)
		if err != nil {
			return nil, fmt.Errorf("agent %q provider: %w", agentName, err)
		}
		router.Register(agentName, p)
	}
	return router, nil
}

// routerProvider wraps a Router to implement Provider for a specific agent.
type routerProvider struct {
	router    *Router
	agentName string
}

var _ Provider = (*routerProvider)(nil)
var _ AgentRuntime = (*routerProvider)(nil)

// ForAgent returns a Provider that always uses the routing for the given agent.
func (r *Router) ForAgent(agentName string) Provider {
	return &routerProvider{router: r, agentName: agentName}
}

func (rp *routerProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return rp.router.For(rp.agentName).Chat(ctx, req)
}

func (rp *routerProvider) ChatWithTools(ctx context.Context, req ChatRequest, tools []Tool) (*ToolResponse, error) {
	return rp.router.For(rp.agentName).ChatWithTools(ctx, req, tools)
}

func (rp *routerProvider) Stream(ctx context.Context, req ChatRequest, cb StreamCallback) (*ChatResponse, error) {
	return rp.router.For(rp.agentName).Stream(ctx, req, cb)
}

func (rp *routerProvider) Capabilities() Capabilities {
	return CapabilitiesForProvider(rp.router.For(rp.agentName))
}
