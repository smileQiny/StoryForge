package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"storyforge/internal/model"
	"storyforge/internal/state"
)

// ComposerInput is the input to the Composer agent.
type ComposerInput struct {
	Book        model.BookConfig
	Chapter     int
	Intent      model.ChapterIntent
	State       model.RuntimeState
	Memory      MemoryRecaller // optional; may be nil
	TokenBudget int            // max tokens for context (default 4000)
}

// MemoryRecaller is an interface for retrieving relevant memory entries.
type MemoryRecaller interface {
	Recall(ctx context.Context, bookID string, chapter int, terms []string, limit int) ([]MemoryEntry, error)
}

// MemoryEntry is a single recalled memory item.
type MemoryEntry struct {
	Kind    string
	Subject string
	Content string
	Chapter int
}

// Composer deterministically compiles the ContextPackage and RuleStack.
type Composer struct {
	*BaseAgent
}

// NewComposer creates a Composer agent.
func NewComposer(base *BaseAgent) *Composer {
	return &Composer{BaseAgent: base}
}

// Compose builds the ContextPackage and RuleStack for a chapter.
// This is deterministic — no LLM call is made.
func (c *Composer) Compose(_ context.Context, input ComposerInput) (*model.ContextPackage, *model.RuleStack, *model.ChapterTrace, error) {
	budget := input.TokenBudget
	if budget <= 0 {
		budget = 4000
	}

	pkg, trace := buildContextPackage(input, budget)
	stack := buildRuleStack(input)

	// Validate state before returning
	if err := state.ValidateRuntimeState(input.State); err != nil {
		return nil, nil, nil, fmt.Errorf("composer: invalid state: %w", err)
	}

	return pkg, stack, trace, nil
}

func buildContextPackage(input ComposerInput, budget int) (*model.ContextPackage, *model.ChapterTrace) {
	pkg := &model.ContextPackage{Chapter: input.Chapter}
	trace := &model.ChapterTrace{Chapter: input.Chapter, TokenBudget: budget}

	used := 0
	addSource := func(kind, label, content string, relevance float64, ch int) {
		est := len(content) / 4 // rough token estimate
		if used+est > budget {
			pkg.ExcludedSources = append(pkg.ExcludedSources, label)
			trace.ExcludedSources = append(trace.ExcludedSources, label)
			return
		}
		pkg.SelectedContext = append(pkg.SelectedContext, model.ContextSource{
			Kind: kind, Label: label, Content: content,
			Relevance: relevance, Chapter: ch,
		})
		trace.InjectedSources = append(trace.InjectedSources, label)
		used += est
	}

	// 1. Recent chapter summaries (last 5)
	summaries := input.State.ChapterSummaries
	start := len(summaries) - 5
	if start < 0 {
		start = 0
	}
	for _, row := range summaries[start:] {
		addSource("summary", fmt.Sprintf("chapter-%d-summary", row.Chapter),
			row.Summary, 0.9, row.Chapter)
	}

	// 2. Active hooks
	for _, h := range input.State.PendingHooks {
		if h.Status == model.HookStatusResolved {
			continue
		}
		content := fmt.Sprintf("[%s] %s → %s", h.Status, h.Type, h.ExpectedPayoff)
		addSource("hook", "hook-"+h.HookID, content, 0.8, h.StartChapter)
	}

	// 3. Current state snapshot (characters, locations)
	if cs := input.State.CurrentState; len(cs) > 0 {
		b := &strings.Builder{}
		for k, v := range cs {
			fmt.Fprintf(b, "%s: %v\n", k, v)
		}
		addSource("state", "current-state", b.String(), 0.7, 0)
	}

	// 4. Subplot board
	for _, sp := range input.State.SubplotBoard {
		if sp.Status == "closed" {
			continue
		}
		addSource("subplot", "subplot-"+sp.ID, sp.Title+" ("+sp.Status+")", 0.6, 0)
	}

	pkg.TokenBudgetUsed = used
	trace.TokenUsed = used
	return pkg, trace
}

func buildRuleStack(input ComposerInput) *model.RuleStack {
	stack := &model.RuleStack{}

	// Layer 1: base rules (hard)
	baseRules := []model.Rule{
		{ID: "base-continuity", Text: "Maintain narrative continuity with previous chapters.", Severity: "hard", Source: "base"},
		{ID: "base-pov", Text: "Maintain consistent point of view.", Severity: "hard", Source: "base"},
		{ID: "base-no-recap", Text: "Do not recap previous chapters verbatim.", Severity: "hard", Source: "base"},
	}
	stack.Layers = append(stack.Layers, model.RuleLayer{
		Name: "base", Priority: 100, Rules: baseRules,
	})

	// Layer 2: intent rules (from must-keep / must-avoid)
	var intentRules []model.Rule
	for i, mk := range input.Intent.MustKeep {
		intentRules = append(intentRules, model.Rule{
			ID: fmt.Sprintf("intent-keep-%d", i), Text: "Must keep: " + mk,
			Severity: "hard", Source: "intent",
		})
	}
	for i, ma := range input.Intent.MustAvoid {
		intentRules = append(intentRules, model.Rule{
			ID: fmt.Sprintf("intent-avoid-%d", i), Text: "Must avoid: " + ma,
			Severity: "hard", Source: "intent",
		})
	}
	if len(intentRules) > 0 {
		stack.Layers = append(stack.Layers, model.RuleLayer{
			Name: "intent", Priority: 90, Rules: intentRules,
		})
	}

	// Sort layers by priority descending
	sort.Slice(stack.Layers, func(i, j int) bool {
		return stack.Layers[i].Priority > stack.Layers[j].Priority
	})

	// Compile sections
	for _, layer := range stack.Layers {
		for _, rule := range layer.Rules {
			switch rule.Severity {
			case "hard":
				stack.Sections.Hard = append(stack.Sections.Hard, rule.Text)
			case "soft":
				stack.Sections.Soft = append(stack.Sections.Soft, rule.Text)
			default:
				stack.Sections.Diagnostic = append(stack.Sections.Diagnostic, rule.Text)
			}
		}
	}

	return stack
}
