package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storyforge/internal/hook"
	"storyforge/internal/model"
)

// PlannerInput is the input to the Planner agent.
type PlannerInput struct {
	Book         model.BookConfig
	Chapter      int
	State        model.RuntimeState
	OutlineText  string // raw outline text (may be empty)
	AgendaConfig hook.AgendaConfig
}

// PlannerCore performs deterministic planning without any LLM call.
// It computes the hook agenda, finds the outline node, and assembles
// a ChapterIntent skeleton.
type PlannerCore struct{}

// Compute runs the deterministic planner.
func (PlannerCore) Compute(input PlannerInput) model.ChapterIntent {
	cfg := input.AgendaConfig
	agendaResult := hook.BuildHookAgenda(input.State, input.Chapter, cfg)

	intent := model.ChapterIntent{
		Chapter: input.Chapter,
	}

	// Populate hook agenda
	intent.HookAgenda = buildHookAgenda(agendaResult)

	// Find outline node
	if input.OutlineText != "" {
		intent.OutlineNode = findOutlineNode(input.OutlineText, input.Chapter)
	}

	return intent
}

// buildHookAgenda converts the agenda result into the model type.
func buildHookAgenda(r hook.HookAgendaResult) model.HookAgenda {
	agenda := model.HookAgenda{}

	for _, h := range r.MustAdvance {
		agenda.MustAdvance = append(agenda.MustAdvance, h.HookID)
	}
	for _, h := range r.EligibleResolve {
		agenda.EligibleResolve = append(agenda.EligibleResolve, h.HookID)
	}
	for _, h := range r.StaleDebt {
		agenda.StaleDebt = append(agenda.StaleDebt, h.HookID)
	}

	for hookID, pressure := range r.PressureMap {
		level := pressureLevel(pressure)
		agenda.PressureMap = append(agenda.PressureMap, model.HookPressure{
			HookID:   hookID,
			Pressure: level,
		})
	}

	return agenda
}

func pressureLevel(score int) string {
	switch {
	case score >= 100:
		return "critical"
	case score >= 60:
		return "high"
	case score >= 30:
		return "medium"
	default:
		return "low"
	}
}

// findOutlineNode searches the outline text for the entry matching chapterNum.
// Supports "第X章" (Chinese) and "Chapter X" (English) formats.
func findOutlineNode(outline string, chapterNum int) string {
	lines := strings.Split(outline, "\n")
	patterns := []string{
		fmt.Sprintf("第%d章", chapterNum),
		fmt.Sprintf("第 %d 章", chapterNum),
		fmt.Sprintf("Chapter %d", chapterNum),
		fmt.Sprintf("chapter %d", chapterNum),
		fmt.Sprintf("CHAPTER %d", chapterNum),
		fmt.Sprintf("Ch. %d", chapterNum),
		fmt.Sprintf("ch.%d", chapterNum),
	}

	for i, line := range lines {
		for _, pat := range patterns {
			if strings.Contains(line, pat) {
				// Collect this line and the next few lines as the node
				end := i + 4
				if end > len(lines) {
					end = len(lines)
				}
				return strings.TrimSpace(strings.Join(lines[i:end], "\n"))
			}
		}
	}
	return ""
}

// Planner is the full Planner agent (deterministic core + optional LLM enrichment).
type Planner struct {
	*BaseAgent
	core PlannerCore
}

// NewPlanner creates a Planner agent.
func NewPlanner(base *BaseAgent) *Planner {
	return &Planner{BaseAgent: base, core: PlannerCore{}}
}

// Plan computes a ChapterIntent. It always runs the deterministic core first.
// If the core produces an empty goal, it calls the LLM to enrich the intent.
func (p *Planner) Plan(ctx context.Context, input PlannerInput) (*model.ChapterIntent, *model.TokenUsage, error) {
	intent := p.core.Compute(input)

	// If we have an outline node, use it as the goal directly
	if intent.OutlineNode != "" {
		intent.Goal = intent.OutlineNode
		return &intent, nil, nil
	}

	// No outline — ask LLM to suggest a goal
	system := fmt.Sprintf(
		"You are a novel planning assistant. Language: %s. Genre: %s.",
		input.Book.Language, input.Book.Genre,
	)
	user := fmt.Sprintf(
		"Suggest a concise chapter goal for Chapter %d of \"%s\". "+
			"Return JSON: {\"goal\": \"...\", \"sceneDirective\": \"...\", \"moodDirective\": \"...\"}",
		input.Chapter, input.Book.Title,
	)

	resp, usage, err := p.Chat(ctx, system, user)
	if err != nil {
		// Non-fatal: return deterministic result
		return &intent, nil, nil
	}

	var patch struct {
		Goal           string `json:"goal"`
		SceneDirective string `json:"sceneDirective"`
		MoodDirective  string `json:"moodDirective"`
	}
	if err := json.Unmarshal([]byte(ExtractJSON(resp)), &patch); err == nil {
		intent.Goal = patch.Goal
		intent.SceneDirective = patch.SceneDirective
		intent.MoodDirective = patch.MoodDirective
	}

	return &intent, ToModelUsage(usage), nil
}
