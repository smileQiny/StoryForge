package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storyforge/internal/model"
)

// AuditorInput is the input to the Auditor agent.
type AuditorInput struct {
	Book                 model.BookConfig
	Chapter              int
	ChapterText          string
	PreviousSummary      string
	State                model.RuntimeState
	ActiveDimensions     []string // dimension keys to check; nil = use defaults
	PreviousChapterText  string
	CurrentStateText     string
	ParticleLedgerText   string
	HooksText            string
	ChapterSummariesText string
	SubplotBoardText     string
	EmotionalArcsText    string
	CharacterMatrixText  string
	StyleGuideText       string
	StoryBibleText       string
	VolumeOutlineText    string
	ParentCanonText      string
	FanficCanonText      string
}

// Auditor checks chapter content across multiple quality dimensions.
type Auditor struct {
	*BaseAgent
}

// NewAuditor creates an Auditor agent.
func NewAuditor(base *BaseAgent) *Auditor {
	return &Auditor{BaseAgent: base}
}

// Audit runs the continuity audit and returns an AuditReport.
func (a *Auditor) Audit(ctx context.Context, input AuditorInput) (*model.AuditReport, *model.TokenUsage, error) {
	dims := resolveActiveDimensions(input)
	dimList := strings.Join(dims, ", ")

	system := fmt.Sprintf(
		"You are a professional novel editor performing a continuity audit. Language: %s.\n"+
			"Check the chapter against these dimensions: %s.\n\n"+
			"Return JSON:\n"+
			"{\n"+
			"  \"chapter\": <int>,\n"+
			"  \"passed\": <bool>,\n"+
			"  \"issues\": [{\"dimension\":\"...\",\"severity\":\"critical|warning|info\",\"summary\":\"...\",\"evidence\":\"...\",\"suggestion\":\"...\"}],\n"+
			"  \"dimensions\": [{\"key\":\"...\",\"passed\":<bool>,\"score\":<0-100>,\"notes\":\"...\"}]\n"+
			"}",
		input.Book.Language, dimList,
	)

	user := fmt.Sprintf(
		"Chapter %d under review:\n\n%s\n\n%s\nAudit all %d dimensions and return the JSON report.",
		input.Chapter, input.ChapterText,
		buildAuditorContextBlock(input),
		len(dims),
	)

	resp, usage, err := a.Chat(ctx, system, user)
	if err != nil {
		return nil, nil, fmt.Errorf("auditor: %w", err)
	}

	var report model.AuditReport
	if err := json.Unmarshal([]byte(ExtractJSON(resp)), &report); err != nil {
		return nil, ToModelUsage(usage), fmt.Errorf("auditor: parse report: %w", err)
	}
	if report.Chapter == 0 {
		report.Chapter = input.Chapter
	}

	return &report, ToModelUsage(usage), nil
}

func buildAuditorContextBlock(input AuditorInput) string {
	blocks := []string{
		auditorSection("Current state", firstNonEmpty(input.CurrentStateText, mustJSON(input.State.CurrentState))),
		auditorSection("Particle ledger", firstNonEmpty(input.ParticleLedgerText, mustJSON(input.State.ParticleLedger))),
		auditorSection("Pending hooks", firstNonEmpty(input.HooksText, renderActiveHooks(input.State))),
		auditorSection("Chapter summaries", input.ChapterSummariesText),
		auditorSection("Subplot board", input.SubplotBoardText),
		auditorSection("Emotional arcs", input.EmotionalArcsText),
		auditorSection("Character matrix", input.CharacterMatrixText),
		auditorSection("Previous chapter summary", input.PreviousSummary),
		auditorSection("Previous chapter full text", input.PreviousChapterText),
		auditorSection("Style guide", input.StyleGuideText),
		auditorSection("Story bible", input.StoryBibleText),
		auditorSection("Volume outline", input.VolumeOutlineText),
		auditorSection("Parent canon", input.ParentCanonText),
		auditorSection("Fanfic canon", input.FanficCanonText),
	}

	var filtered []string
	for _, block := range blocks {
		if strings.TrimSpace(block) != "" {
			filtered = append(filtered, block)
		}
	}
	return strings.Join(filtered, "\n\n")
}

func auditorSection(label, content string) string {
	content = strings.TrimSpace(content)
	if content == "" || content == "null" || content == "{}" || content == "[]" {
		return ""
	}
	return fmt.Sprintf("%s:\n%s", label, content)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mustJSON(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

// resolveActiveDimensions returns the list of dimension keys to audit.
func resolveActiveDimensions(input AuditorInput) []string {
	if len(input.ActiveDimensions) > 0 {
		return input.ActiveDimensions
	}

	disabled := make(map[string]bool, len(input.Book.DisabledAuditDimensions))
	for _, d := range input.Book.DisabledAuditDimensions {
		disabled[d] = true
	}

	var dims []string

	// Always include core dimensions
	for _, d := range model.CoreAuditDimensions {
		if !disabled[d.Key] {
			dims = append(dims, d.Key)
		}
	}

	// Genre dimensions — activate based on genre flags
	for _, d := range model.GenreAuditDimensions {
		if disabled[d.Key] {
			continue
		}
		for _, req := range d.GenreReq {
			if req == input.Book.Genre {
				dims = append(dims, d.Key)
				break
			}
		}
	}

	// Fanfic dimensions
	if input.Book.FanficMode != model.FanficModeNone {
		for _, d := range model.FanficAuditDimensions {
			if !disabled[d.Key] {
				dims = append(dims, d.Key)
			}
		}
	}

	return dims
}
