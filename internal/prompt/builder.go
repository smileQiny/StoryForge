package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// PromptContext holds all variables available for template rendering.
type PromptContext struct {
	Language      string
	Genre         string
	FanficMode    string
	BookTitle     string
	BookRules     string
	StyleGuide    string
	GenreRules    string
	ProtagonistRules string
	FanficCanon   string
	ChapterNumber int
	WordCountMin  int
	WordCountMax  int
	Goal          string
	MustKeep      []string
	MustAvoid     []string
	HookAgenda    string // pre-rendered hook agenda text
	ContextBundle string // pre-rendered context bundle
	RuleStack     string // pre-rendered rule stack
	// Observer/Reflector/Auditor specific
	DraftBody     string
	TruthSnapshot string
	AuditDimensions []string
}

// Builder composes prompts from a Registry and PromptContext.
type Builder struct {
	registry *Registry
	loader   Loader
}

// NewBuilder creates a Builder.
func NewBuilder(registry *Registry, loader Loader) *Builder {
	return &Builder{registry: registry, loader: loader}
}

// BuildSystemPrompt builds the system prompt for a given profile.
func (b *Builder) BuildSystemPrompt(profile *PromptProfile, ctx PromptContext) (string, error) {
	return b.buildSections(profile, ctx, "system")
}

// BuildUserPrompt builds the user prompt for a given profile.
func (b *Builder) BuildUserPrompt(profile *PromptProfile, ctx PromptContext) (string, error) {
	return b.buildSections(profile, ctx, "user")
}

// Build returns both system and user prompts for a role+language.
func (b *Builder) Build(role AgentRole, language string, ctx PromptContext) (system, user string, err error) {
	profile, ok := b.registry.ProfileForRole(role, language)
	if !ok {
		return "", "", fmt.Errorf("no profile for role %q language %q", role, language)
	}
	system, err = b.BuildSystemPrompt(profile, ctx)
	if err != nil {
		return "", "", fmt.Errorf("system prompt: %w", err)
	}
	user, err = b.BuildUserPrompt(profile, ctx)
	if err != nil {
		return "", "", fmt.Errorf("user prompt: %w", err)
	}
	return system, user, nil
}

func (b *Builder) buildSections(profile *PromptProfile, ctx PromptContext, kind string) (string, error) {
	var parts []string
	for _, sectionID := range profile.SectionOrder {
		section, ok := b.registry.GetSection(sectionID)
		if !ok {
			// Try loading from loader
			tmplText, err := b.loader.Load(sectionID, ctx.Language)
			if err != nil {
				continue // section not found; skip
			}
			section = &PromptSection{ID: sectionID, Template: tmplText}
		}
		if section.Kind != "" && section.Kind != kind {
			continue
		}
		rendered, err := renderTemplate(section.Template, ctx)
		if err != nil {
			return "", fmt.Errorf("render section %q: %w", sectionID, err)
		}
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func renderTemplate(tmplText string, ctx PromptContext) (string, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(tmplText)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}
