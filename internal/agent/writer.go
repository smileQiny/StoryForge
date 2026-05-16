package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"storyforge/internal/llm"
	"storyforge/internal/model"
	"storyforge/internal/prompt"
)

// LengthSpec defines the word count target for a chapter.
type LengthSpec struct {
	Min    int
	Target int
	Max    int
}

// WriterInput is the input to the Writer agent.
type WriterInput struct {
	Book       model.BookConfig
	Chapter    int
	Intent     model.ChapterIntent
	Context    model.ContextPackage
	RuleStack  model.RuleStack
	LengthSpec LengthSpec
	Builder    *prompt.Builder
}

// WriterOutput is the output of the Writer agent.
type WriterOutput struct {
	Content   string
	WordCount int
	Usage     *model.TokenUsage
	// NeedsNormalize is true if the word count is outside the soft range.
	NeedsNormalize bool
}

// Writer generates chapter prose using the LLM.
type Writer struct {
	*BaseAgent
}

// NewWriter creates a Writer agent.
func NewWriter(base *BaseAgent) *Writer {
	return &Writer{BaseAgent: base}
}

// Write generates chapter content via streaming LLM call.
func (w *Writer) Write(ctx context.Context, input WriterInput, tokenCb llm.StreamCallback) (*WriterOutput, error) {
	system, user, err := buildWriterPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("writer: build prompt: %w", err)
	}

	var sb strings.Builder
	cb := func(token string) error {
		sb.WriteString(token)
		if tokenCb != nil {
			return tokenCb(token)
		}
		return nil
	}

	content, usage, err := w.StreamWithMaxTokens(ctx, system, user, RecommendedWriterMaxTokens(input.LengthSpec), cb)
	if err != nil {
		// If we got partial content from the stream, use it
		if sb.Len() >= 500 {
			content = sb.String()
		} else {
			return nil, fmt.Errorf("writer: stream: %w", err)
		}
	}
	if content == "" {
		content = sb.String()
	}

	wc := countRunes(content)
	needsNorm := wc < input.LengthSpec.Min || wc > input.LengthSpec.Max

	return &WriterOutput{
		Content:        content,
		WordCount:      wc,
		Usage:          ToModelUsage(usage),
		NeedsNormalize: needsNorm,
	}, nil
}

func buildWriterPrompt(input WriterInput) (system, user string, err error) {
	if input.Builder != nil {
		ctx := prompt.PromptContext{
			Language:      string(input.Book.Language),
			Genre:         input.Book.Genre,
			FanficMode:    string(input.Book.FanficMode),
			BookTitle:     input.Book.Title,
			ChapterNumber: input.Chapter,
			WordCountMin:  input.LengthSpec.Min,
			WordCountMax:  input.LengthSpec.Max,
			Goal:          input.Intent.Goal,
			MustKeep:      input.Intent.MustKeep,
			MustAvoid:     input.Intent.MustAvoid,
			ContextBundle: renderContextBundle(input.Context),
			RuleStack:     renderRuleStack(input.RuleStack),
			HookAgenda:    renderHookAgenda(input.Intent.HookAgenda),
		}
		system, user, err = input.Builder.Build(prompt.RoleWriter, string(input.Book.Language), ctx)
		if err == nil {
			return system, user, nil
		}
		// Fall through to default prompt on error
	}

	// Default prompt when no builder is configured
	system = fmt.Sprintf(
		"You are a professional novelist writing in %s. Genre: %s. "+
			"Write vivid, engaging prose. Follow all rules strictly.",
		input.Book.Language, input.Book.Genre,
	)
	user = fmt.Sprintf(
		"Write Chapter %d of \"%s\".\n\nGoal: %s\n\nTarget length: %d-%d words.\n\n"+
			"Context:\n%s\n\nRules:\n%s",
		input.Chapter, input.Book.Title, input.Intent.Goal,
		input.LengthSpec.Min, input.LengthSpec.Max,
		renderContextBundle(input.Context),
		renderRuleStack(input.RuleStack),
	)
	return system, user, nil
}

func renderContextBundle(pkg model.ContextPackage) string {
	var sb strings.Builder
	for _, src := range pkg.SelectedContext {
		fmt.Fprintf(&sb, "[%s] %s\n%s\n\n", src.Kind, src.Label, src.Content)
	}
	return strings.TrimSpace(sb.String())
}

func renderRuleStack(stack model.RuleStack) string {
	var sb strings.Builder
	if len(stack.Sections.Hard) > 0 {
		sb.WriteString("HARD RULES:\n")
		for _, r := range stack.Sections.Hard {
			fmt.Fprintf(&sb, "- %s\n", r)
		}
	}
	if len(stack.Sections.Soft) > 0 {
		sb.WriteString("SOFT RULES:\n")
		for _, r := range stack.Sections.Soft {
			fmt.Fprintf(&sb, "- %s\n", r)
		}
	}
	return strings.TrimSpace(sb.String())
}

func renderHookAgenda(agenda model.HookAgenda) string {
	if len(agenda.MustAdvance) == 0 && len(agenda.EligibleResolve) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(agenda.MustAdvance) > 0 {
		fmt.Fprintf(&sb, "Must advance hooks: %s\n", strings.Join(agenda.MustAdvance, ", "))
	}
	if len(agenda.EligibleResolve) > 0 {
		fmt.Fprintf(&sb, "Eligible to resolve: %s\n", strings.Join(agenda.EligibleResolve, ", "))
	}
	return strings.TrimSpace(sb.String())
}

// countRunes counts characters (runes) — appropriate for CJK text.
func countRunes(s string) int {
	return utf8.RuneCountInString(s)
}
