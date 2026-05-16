package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// ObserverInput is the input to the Observer agent.
type ObserverInput struct {
	Book        model.BookConfig
	Chapter     int
	ChapterText string
	State       model.RuntimeState
}

// ObserverOutput is the output of the Observer agent.
type ObserverOutput struct {
	Facts []model.ObservedFact
	Usage *model.TokenUsage
}

// Observer extracts 9 categories of facts from chapter prose.
type Observer struct {
	*BaseAgent
}

const observerMaxTokens = 900

// NewObserver creates an Observer agent.
func NewObserver(base *BaseAgent) *Observer {
	return &Observer{BaseAgent: base}
}

// Observe extracts facts from the chapter text.
func (o *Observer) Observe(ctx context.Context, input ObserverInput) (*ObserverOutput, error) {
	system := fmt.Sprintf(
		"You are a meticulous story analyst. Language: %s. "+
			"Extract the highest-value observable facts from the chapter text. "+
			"Return a JSON array of facts with fields: kind, subject, content, chapter.\n"+
			"Fact kinds: character, location, event, hook, resource, relation, knowledge, emotion, subplot.\n"+
			"Prefer concise, state-changing facts. Skip low-value repetition and minor atmospheric details.\n"+
			"Cap the output to roughly 40-60 facts and keep each content field brief.",
		input.Book.Language,
	)
	user := fmt.Sprintf(
		"Chapter %d text:\n\n%s\n\n"+
			"Extract all facts. Return JSON array: [{\"kind\":\"...\",\"subject\":\"...\",\"content\":\"...\",\"chapter\":%d}]",
		input.Chapter, compactObserverChapterText(input.ChapterText), input.Chapter,
	)

	resp, err := o.Runtime.Chat(ctx, llm.ChatRequest{
		Model:     o.Model,
		Messages:  o.messages(system, user),
		MaxTokens: observerMaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("observer: %w", err)
	}

	var facts []model.ObservedFact
	if err := json.Unmarshal([]byte(ExtractJSON(resp.Content)), &facts); err != nil {
		// Return empty facts rather than failing hard
		return &ObserverOutput{Usage: ToModelUsage(&resp.Usage)}, nil
	}

	// Ensure chapter is set on all facts
	for i := range facts {
		if facts[i].Chapter == 0 {
			facts[i].Chapter = input.Chapter
		}
	}

	return &ObserverOutput{Facts: facts, Usage: ToModelUsage(&resp.Usage)}, nil
}

func compactObserverChapterText(text string) string {
	return compactAgentText(text, 6000)
}

func compactAgentText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	head := maxRunes * 2 / 3
	tail := maxRunes - head
	return string(runes[:head]) + "\n...\n" + string(runes[len(runes)-tail:])
}
