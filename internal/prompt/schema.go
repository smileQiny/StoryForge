package prompt

import "encoding/json"

// OutputSchema holds the registered output schemas for each agent role.
type OutputSchema struct {
	Role   AgentRole
	Schema json.RawMessage
}

// SchemaRegistry holds output schemas keyed by role.
type SchemaRegistry struct {
	schemas map[AgentRole]json.RawMessage
}

// NewSchemaRegistry creates a SchemaRegistry pre-populated with default schemas.
func NewSchemaRegistry() *SchemaRegistry {
	sr := &SchemaRegistry{schemas: make(map[AgentRole]json.RawMessage)}
	for _, p := range DefaultProfiles() {
		if p.OutputSchema != "" {
			sr.schemas[p.Role] = json.RawMessage(p.OutputSchema)
		}
	}
	return sr
}

// Get returns the output schema for a role.
func (sr *SchemaRegistry) Get(role AgentRole) (json.RawMessage, bool) {
	s, ok := sr.schemas[role]
	return s, ok
}

// Register adds or replaces a schema.
func (sr *SchemaRegistry) Register(role AgentRole, schema json.RawMessage) {
	sr.schemas[role] = schema
}
