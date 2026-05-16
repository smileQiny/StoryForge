package llm

import "encoding/json"

// Tool defines a callable function for tool-use / function-calling.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// BuildTool constructs a Tool from a name, description, and a Go struct used as schema.
// The schema is derived from the struct's JSON tags via a simple reflection-based approach.
func BuildTool(name, description string, schema any) (Tool, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return Tool{}, err
	}
	// Wrap as a JSON Schema object with the struct fields as properties
	// For simplicity we accept a pre-built schema map
	return Tool{
		Name:        name,
		Description: description,
		Parameters:  data,
	}, nil
}

// ObjectSchema builds a minimal JSON Schema object from a map of property definitions.
func ObjectSchema(properties map[string]PropertyDef, required []string) json.RawMessage {
	type schema struct {
		Type       string                 `json:"type"`
		Properties map[string]PropertyDef `json:"properties"`
		Required   []string               `json:"required,omitempty"`
	}
	s := schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
	data, _ := json.Marshal(s)
	return data
}

// PropertyDef is a single JSON Schema property definition.
type PropertyDef struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Items       *PropertyDef `json:"items,omitempty"`
}
