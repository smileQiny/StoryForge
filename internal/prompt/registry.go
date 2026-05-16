package prompt

// AgentRole identifies the role of an agent in the pipeline.
type AgentRole string

const (
	RoleWriter    AgentRole = "writer"
	RoleObserver  AgentRole = "observer"
	RoleReflector AgentRole = "reflector"
	RoleAuditor   AgentRole = "auditor"
	RoleReviser   AgentRole = "reviser"
	RolePlanner   AgentRole = "planner"
	RoleComposer  AgentRole = "composer"
	RoleArchitect AgentRole = "architect"
	RoleReviewer  AgentRole = "foundation_reviewer"
	RoleNormalizer AgentRole = "normalizer"
	RoleRadar     AgentRole = "radar"
)

// PromptSection is a named, reusable prompt fragment.
type PromptSection struct {
	ID        string      // unique identifier, e.g. "writer/genre_intro"
	Kind      string      // "system" or "user"
	Languages []string    // supported languages; empty = all
	Roles     []AgentRole // applicable roles; empty = all
	Template  string      // Go text/template source
}

// Registry holds all registered prompt sections and profiles.
type Registry struct {
	sections map[string]*PromptSection
	profiles map[string]*PromptProfile
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		sections: make(map[string]*PromptSection),
		profiles: make(map[string]*PromptProfile),
	}
}

// RegisterSection adds or replaces a section.
func (r *Registry) RegisterSection(s *PromptSection) {
	r.sections[s.ID] = s
}

// GetSection returns a section by ID.
func (r *Registry) GetSection(id string) (*PromptSection, bool) {
	s, ok := r.sections[id]
	return s, ok
}

// RegisterProfile adds or replaces a profile.
func (r *Registry) RegisterProfile(p *PromptProfile) {
	r.profiles[p.ID] = p
}

// GetProfile returns a profile by ID.
func (r *Registry) GetProfile(id string) (*PromptProfile, bool) {
	p, ok := r.profiles[id]
	return p, ok
}

// ProfileForRole returns the profile for a given role and language.
func (r *Registry) ProfileForRole(role AgentRole, language string) (*PromptProfile, bool) {
	id := string(role) + "/" + language
	if p, ok := r.profiles[id]; ok {
		return p, true
	}
	// Fallback to role-only profile
	if p, ok := r.profiles[string(role)]; ok {
		return p, true
	}
	return nil, false
}
