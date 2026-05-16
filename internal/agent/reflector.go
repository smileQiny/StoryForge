package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// ReflectorInput is the input to the Reflector agent.
type ReflectorInput struct {
	Book                 model.BookConfig
	Chapter              int
	ChapterText          string
	Facts                []model.ObservedFact
	State                model.RuntimeState
	CurrentStateText     string
	HooksText            string
	ChapterSummariesText string
	SubplotBoardText     string
	EmotionalArcsText    string
	CharacterMatrixText  string
	PreviousSummary      string
}

// ReflectorOutput is the output of the Reflector agent.
type ReflectorOutput struct {
	Delta model.RuntimeStateDelta
	Usage *model.TokenUsage
}

// Reflector converts observed facts into a RuntimeStateDelta.
type Reflector struct {
	*BaseAgent
}

const reflectorMaxTokens = 1000

// NewReflector creates a Reflector agent.
func NewReflector(base *BaseAgent) *Reflector {
	return &Reflector{BaseAgent: base}
}

// Reflect produces a RuntimeStateDelta from observed facts.
func (r *Reflector) Reflect(ctx context.Context, input ReflectorInput) (*ReflectorOutput, error) {
	factsJSON, _ := json.Marshal(input.Facts)

	system := fmt.Sprintf(
		"You are a story state manager. Language: %s. "+
			"Given observed facts from a chapter, produce a minimal RuntimeStateDelta "+
			"that updates the story state. Only include changes — omit unchanged fields.\n\n"+
			"RuntimeStateDelta schema:\n"+
			"{\n"+
			"  \"chapter\": <int>,\n"+
			"  \"currentStatePatch\": {\"characterUpdates\":[...],\"locationUpdates\":[...],...},\n"+
			"  \"hookOps\": {\"advance\":[{\"hookId\":\"...\",\"chapter\":<int>,\"note\":\"...\"}],\"resolve\":[...],\"defer\":[...]},\n"+
			"  \"newHookCandidates\": [{\"type\":\"...\",\"description\":\"...\",\"expectedPayoff\":\"...\"}],\n"+
			"  \"chapterSummary\": {\"chapter\":<int>,\"title\":\"...\",\"summary\":\"...\",\"hookUpdates\":\"...\"}\n"+
			"}",
		input.Book.Language,
	)
	user := fmt.Sprintf(
		"Chapter %d draft:\n%s\n\n"+
			"Observed facts:\n%s\n\n%s\n\n"+
			"Produce the RuntimeStateDelta JSON.",
		input.Chapter, compactReflectorChapterText(input.ChapterText), string(factsJSON), buildReflectorContextBlock(input),
	)

	resp, err := r.Runtime.Chat(ctx, llm.ChatRequest{
		Model:     r.Model,
		Messages:  r.messages(system, user),
		MaxTokens: reflectorMaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("reflector: %w", err)
	}

	var delta model.RuntimeStateDelta
	if err := json.Unmarshal([]byte(ExtractJSON(resp.Content)), &delta); err != nil {
		return nil, fmt.Errorf("reflector: parse delta: %w", err)
	}
	if delta.Chapter == 0 {
		delta.Chapter = input.Chapter
	}

	return &ReflectorOutput{Delta: delta, Usage: ToModelUsage(&resp.Usage)}, nil
}

func buildReflectorContextBlock(input ReflectorInput) string {
	blocks := []string{
		reflectorSection("Current state", firstReflectorNonEmpty(input.CurrentStateText, mustPrettyJSON(input.State.CurrentState))),
		reflectorSection("Active hooks", firstReflectorNonEmpty(input.HooksText, renderActiveHooks(input.State))),
		reflectorSection("Chapter summaries", input.ChapterSummariesText),
		reflectorSection("Subplot board", input.SubplotBoardText),
		reflectorSection("Emotional arcs", input.EmotionalArcsText),
		reflectorSection("Character matrix", input.CharacterMatrixText),
		reflectorSection("Previous chapter summary", input.PreviousSummary),
	}
	var filtered []string
	for _, block := range blocks {
		if strings.TrimSpace(block) != "" {
			filtered = append(filtered, block)
		}
	}
	return strings.Join(filtered, "\n\n")
}

func reflectorSection(label, content string) string {
	content = strings.TrimSpace(content)
	if content == "" || content == "null" || content == "{}" || content == "[]" {
		return ""
	}
	content = compactAgentText(content, 1800)
	return fmt.Sprintf("%s:\n%s", label, content)
}

func firstReflectorNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mustPrettyJSON(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

func renderActiveHooks(state model.RuntimeState) string {
	var result []map[string]any
	for _, h := range state.PendingHooks {
		if h.Status == model.HookStatusResolved {
			continue
		}
		result = append(result, map[string]any{
			"hookId": h.HookID,
			"type":   h.Type,
			"status": h.Status,
			"payoff": h.ExpectedPayoff,
		})
	}
	b, _ := json.Marshal(result)
	return string(b)
}

func compactReflectorChapterText(text string) string {
	return compactAgentText(text, 2500)
}
