package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// ArchitectInput is the input to the Architect agent.
type ArchitectInput struct {
	Book           model.BookConfig
	Brief          string // user-provided creative brief
	ReviewFeedback string // feedback from previous foundation review round
}

// ArchitectOutput is the output of the Architect agent.
// It contains the 5 foundation truth files as raw JSON.
type ArchitectOutput struct {
	WorldBuilding json.RawMessage `json:"worldBuilding"`
	Characters    json.RawMessage `json:"characters"`
	PlotOutline   json.RawMessage `json:"plotOutline"`
	StyleGuide    json.RawMessage `json:"styleGuide"`
	WritingBible  json.RawMessage `json:"writingBible"`
	Usage         *model.TokenUsage
}

// Architect generates the 5 foundation truth files via tool-calling.
type Architect struct {
	*BaseAgent
}

// NewArchitect creates an Architect agent.
func NewArchitect(base *BaseAgent) *Architect {
	return &Architect{BaseAgent: base}
}

// Design generates the foundation truth files for a new book.
func (a *Architect) Design(ctx context.Context, input ArchitectInput) (*ArchitectOutput, error) {
	if a.prefersStructuredJSONDesign() {
		return a.designFromStructuredJSON(ctx, input)
	}

	tools := buildArchitectTools()

	system := fmt.Sprintf(
		"You are a master story architect. Language: %s. Genre: %s.\n"+
			"Given a creative brief, generate comprehensive foundation files for a novel.\n"+
			"Use the provided tools to submit each foundation file.",
		input.Book.Language, input.Book.Genre,
	)
	user := fmt.Sprintf(
		"Book: \"%s\"\nBrief: %s\n\n"+
			"Generate all 5 foundation files: world_building, characters, plot_outline, style_guide, writing_bible.",
		input.Book.Title, input.Brief,
	)
	if input.ReviewFeedback != "" {
		user += fmt.Sprintf("\n\nPrevious review feedback to fix in this generation:\n%s", input.ReviewFeedback)
	}

	resp, err := a.ChatWithTools(ctx, system, user, tools)
	if err != nil {
		return nil, fmt.Errorf("architect: %w", err)
	}

	output := &ArchitectOutput{Usage: ToModelUsage(&resp.Usage)}
	for _, tc := range resp.ToolCalls {
		raw := json.RawMessage(tc.Arguments)
		switch tc.Name {
		case "submit_world_building":
			output.WorldBuilding = raw
		case "submit_characters":
			output.Characters = raw
		case "submit_plot_outline":
			output.PlotOutline = raw
		case "submit_style_guide":
			output.StyleGuide = raw
		case "submit_writing_bible":
			output.WritingBible = raw
		}
	}
	fillArchitectOutputFromContent(resp.Content, output)
	if architectOutputComplete(output) || architectOutputFieldCount(output) > 0 {
		return output, nil
	}

	fallback, err := a.designFromStructuredJSON(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("architect fallback: %w", err)
	}
	mergeArchitectOutput(output, fallback)

	return output, nil
}

func (a *Architect) prefersStructuredJSONDesign() bool {
	return strings.EqualFold(strings.TrimSpace(a.Caps.Provider), "openai") &&
		strings.EqualFold(strings.TrimSpace(a.Caps.ConfiguredWireAPI), model.WireAPIResponses)
}

func (a *Architect) designFromStructuredJSON(ctx context.Context, input ArchitectInput) (*ArchitectOutput, error) {
	system := fmt.Sprintf(
		"You are a master story architect. Language: %s. Genre: %s.\n"+
			"Return only a single JSON object with the keys worldBuilding, characters, plotOutline, styleGuide, writingBible.\n"+
			"Every key must contain a populated JSON object. Do not leave any section empty.",
		input.Book.Language, input.Book.Genre,
	)
	user := fmt.Sprintf(
		"Book: \"%s\"\nBrief: %s\n\n"+
			"Build the five foundation files as structured JSON.\n"+
			"Keep the content concise, production-usable, and information-dense.\n"+
			"Prefer short paragraphs, bullet-like arrays, and compact field values instead of long narrative prose.\n"+
			"worldBuilding should cover world rules, setting anchors, threat mechanics, and clue systems.\n"+
			"characters should cover leads, antagonists, motivations, secrets, and relationships.\n"+
			"plotOutline should cover opening hooks, major reversals, midpoint, climax, and chapter progression for the planned arc.\n"+
			"styleGuide should cover tone, pacing, POV, suspense techniques, and chapter-ending hook rules.\n"+
			"writingBible should cover hard constraints, timeline, symbols, continuity rules, and forbidden contradictions.",
		input.Book.Title, input.Brief,
	)
	if input.ReviewFeedback != "" {
		user += fmt.Sprintf("\n\nPrevious review feedback to fix in this generation:\n%s", input.ReviewFeedback)
	}

	resp, err := a.Runtime.Chat(ctx, llm.ChatRequest{
		Model:     a.Model,
		Messages:  a.messages(system, user),
		MaxTokens: 2200,
	})
	if err != nil {
		return nil, err
	}

	output := &ArchitectOutput{Usage: ToModelUsage(&resp.Usage)}
	fillArchitectOutputFromContent(resp.Content, output)
	if !architectOutputComplete(output) {
		return nil, fmt.Errorf("fallback response did not contain complete architect JSON")
	}
	return output, nil
}

func fillArchitectOutputFromContent(content string, output *ArchitectOutput) {
	if output == nil {
		return
	}
	raw := json.RawMessage(ExtractJSON(content))
	if len(raw) == 0 {
		return
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}

	assignArchitectField(&output.WorldBuilding, payload, "worldBuilding", "world_building")
	assignArchitectField(&output.Characters, payload, "characters")
	assignArchitectField(&output.PlotOutline, payload, "plotOutline", "plot_outline")
	assignArchitectField(&output.StyleGuide, payload, "styleGuide", "style_guide")
	assignArchitectField(&output.WritingBible, payload, "writingBible", "writing_bible")
}

func mergeArchitectOutput(dst, src *ArchitectOutput) {
	if dst == nil || src == nil {
		return
	}
	if len(dst.WorldBuilding) == 0 {
		dst.WorldBuilding = src.WorldBuilding
	}
	if len(dst.Characters) == 0 {
		dst.Characters = src.Characters
	}
	if len(dst.PlotOutline) == 0 {
		dst.PlotOutline = src.PlotOutline
	}
	if len(dst.StyleGuide) == 0 {
		dst.StyleGuide = src.StyleGuide
	}
	if len(dst.WritingBible) == 0 {
		dst.WritingBible = src.WritingBible
	}
	if dst.Usage == nil {
		dst.Usage = src.Usage
	}
}

func assignArchitectField(dst *json.RawMessage, payload map[string]json.RawMessage, keys ...string) {
	if dst == nil || len(*dst) != 0 {
		return
	}
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || len(value) == 0 {
			continue
		}
		*dst = value
		return
	}
}

func architectOutputComplete(output *ArchitectOutput) bool {
	if output == nil {
		return false
	}
	return architectFieldPresent(output.WorldBuilding) &&
		architectFieldPresent(output.Characters) &&
		architectFieldPresent(output.PlotOutline) &&
		architectFieldPresent(output.StyleGuide) &&
		architectFieldPresent(output.WritingBible)
}

func architectOutputFieldCount(output *ArchitectOutput) int {
	if output == nil {
		return 0
	}
	count := 0
	for _, raw := range []json.RawMessage{
		output.WorldBuilding,
		output.Characters,
		output.PlotOutline,
		output.StyleGuide,
		output.WritingBible,
	} {
		if architectFieldPresent(raw) {
			count++
		}
	}
	return count
}

func architectFieldPresent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err == nil {
		if content, ok := payload["content"]; ok {
			return jsonObjectHasFields(content)
		}
		return len(payload) > 0
	}
	return false
}

func jsonObjectHasFields(raw json.RawMessage) bool {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return len(payload) > 0
}

func buildArchitectTools() []llm.Tool {
	fileSchema := llm.ObjectSchema(
		map[string]llm.PropertyDef{
			"content": {Type: "object", Description: "The foundation file content as a structured object"},
		},
		[]string{"content"},
	)

	names := []struct{ name, desc string }{
		{"submit_world_building", "Submit the world-building foundation file"},
		{"submit_characters", "Submit the characters foundation file"},
		{"submit_plot_outline", "Submit the plot outline foundation file"},
		{"submit_style_guide", "Submit the style guide foundation file"},
		{"submit_writing_bible", "Submit the writing bible foundation file"},
	}

	tools := make([]llm.Tool, 0, len(names))
	for _, n := range names {
		t, err := llm.BuildTool(n.name, n.desc, fileSchema)
		if err == nil {
			tools = append(tools, t)
		}
	}
	return tools
}
