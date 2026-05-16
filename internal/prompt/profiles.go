package prompt

// PromptProfile maps an agent role to an ordered list of sections and an output schema.
type PromptProfile struct {
	ID           string      // e.g. "writer/zh"
	Role         AgentRole
	Language     string
	SectionOrder []string // ordered section IDs to compose
	OutputSchema string   // JSON schema or format description for the output
}

// WriterSectionOrder defines the 12-section composition order for the Writer prompt.
var WriterSectionOrder = []string{
	"writer/genre_intro",
	"writer/core_rules",
	"writer/input_contract",
	"writer/word_count",
	"writer/anti_ai",
	"writer/psychology",
	"writer/genre_rules",
	"writer/protagonist_rules",
	"writer/book_rules",
	"writer/style_guide",
	"writer/fanfic_canon",
	"writer/output_format",
}

// DefaultProfiles returns the built-in profiles for all roles and languages.
func DefaultProfiles() []*PromptProfile {
	return []*PromptProfile{
		{
			ID:           "writer/zh",
			Role:         RoleWriter,
			Language:     "zh",
			SectionOrder: WriterSectionOrder,
			OutputSchema: writerOutputSchema,
		},
		{
			ID:           "writer/en",
			Role:         RoleWriter,
			Language:     "en",
			SectionOrder: WriterSectionOrder,
			OutputSchema: writerOutputSchema,
		},
		{
			ID:           "observer/zh",
			Role:         RoleObserver,
			Language:     "zh",
			SectionOrder: []string{"observer/task", "observer/output_format"},
			OutputSchema: observerOutputSchema,
		},
		{
			ID:           "observer/en",
			Role:         RoleObserver,
			Language:     "en",
			SectionOrder: []string{"observer/task", "observer/output_format"},
			OutputSchema: observerOutputSchema,
		},
		{
			ID:           "reflector/zh",
			Role:         RoleReflector,
			Language:     "zh",
			SectionOrder: []string{"reflector/task", "reflector/output_format"},
			OutputSchema: reflectorOutputSchema,
		},
		{
			ID:           "reflector/en",
			Role:         RoleReflector,
			Language:     "en",
			SectionOrder: []string{"reflector/task", "reflector/output_format"},
			OutputSchema: reflectorOutputSchema,
		},
		{
			ID:           "auditor/zh",
			Role:         RoleAuditor,
			Language:     "zh",
			SectionOrder: []string{"auditor/preamble", "auditor/output_format"},
			OutputSchema: auditorOutputSchema,
		},
		{
			ID:           "auditor/en",
			Role:         RoleAuditor,
			Language:     "en",
			SectionOrder: []string{"auditor/preamble", "auditor/output_format"},
			OutputSchema: auditorOutputSchema,
		},
	}
}

const writerOutputSchema = `{
  "type": "object",
  "properties": {
    "title": {"type": "string"},
    "body": {"type": "string"}
  },
  "required": ["title", "body"]
}`

const observerOutputSchema = `{
  "type": "object",
  "properties": {
    "facts": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "kind": {"type": "string"},
          "subject": {"type": "string"},
          "content": {"type": "string"}
        }
      }
    }
  }
}`

const reflectorOutputSchema = `{
  "type": "object",
  "description": "RuntimeStateDelta JSON"
}`

const auditorOutputSchema = `{
  "type": "object",
  "properties": {
    "passed": {"type": "boolean"},
    "issues": {"type": "array"},
    "dimensions": {"type": "array"}
  }
}`
