package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"storyforge/internal/model"
)

// ReviserInput is the input to the Reviser agent.
type ReviserInput struct {
	Book        model.BookConfig
	Chapter     int
	ChapterText string
	Report      model.AuditReport
	Mode        string // spot-fix / polish / rewrite / rework / anti-detect
	AntiDetect  bool   // enable anti-AI-detection mode
}

// ReviserOutput is the output of the Reviser agent.
type ReviserOutput struct {
	Content      string
	FixedIssues  []string
	MarkedIssues []string // non-critical issues marked but not fixed
	Usage        *model.TokenUsage
}

// Reviser auto-fixes critical audit issues and marks non-critical ones.
type Reviser struct {
	*BaseAgent
}

// NewReviser creates a Reviser agent.
func NewReviser(base *BaseAgent) *Reviser {
	return &Reviser{BaseAgent: base}
}

// Revise applies fixes for critical issues and marks non-critical ones.
func (r *Reviser) Revise(ctx context.Context, input ReviserInput) (*ReviserOutput, error) {
	critical, nonCritical := partitionIssues(input.Report.Issues)
	mode := normalizeReviserMode(input.Mode)

	if len(critical) == 0 && !requiresRewriteWithoutCritical(mode) {
		// Nothing to fix
		return &ReviserOutput{
			Content:      input.ChapterText,
			MarkedIssues: issueKeys(nonCritical),
		}, nil
	}

	issuesJSON, _ := json.Marshal(critical)

	antiDetectNote := ""
	if input.AntiDetect {
		antiDetectNote = "\nAdditionally, vary sentence structure and rhythm to reduce AI-detection patterns."
	}
	modeNote := reviserModeNote(mode)
	issueInstruction := "Fix EVERY listed critical issue. Treat each issue's summary, evidence, and suggestion as an acceptance checklist. Make the smallest necessary edits, but do not omit any required scene beat, external evidence, rule clarification, or continuity repair."
	if len(critical) == 0 && requiresRewriteWithoutCritical(mode) {
		issueInstruction = "No critical issues were found. Apply only the requested revision mode while preserving plot facts, canon constraints, and scene intent."
	}

	system := fmt.Sprintf(
		"You are a professional novel editor. Language: %s.%s\n"+
			"%s\n%s\n"+
			"Before finalizing, silently verify that every critical issue is resolved in the revised text.\n"+
			"Return ONLY the revised chapter text.",
		input.Book.Language, antiDetectNote, issueInstruction, modeNote,
	)
	user := fmt.Sprintf(
		"Chapter %d — revision mode: %s\nCritical issues:\n%s\n\nChapter text:\n\n%s",
		input.Chapter, mode, string(issuesJSON), input.ChapterText,
	)

	content, usage, err := r.Chat(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("reviser: %w", err)
	}

	content = strings.TrimSpace(content)

	return &ReviserOutput{
		Content:      content,
		FixedIssues:  issueKeys(critical),
		MarkedIssues: issueKeys(nonCritical),
		Usage:        ToModelUsage(usage),
	}, nil
}

func partitionIssues(issues []model.AuditIssue) (critical, other []model.AuditIssue) {
	for _, iss := range issues {
		if iss.Severity == "critical" {
			critical = append(critical, iss)
		} else {
			other = append(other, iss)
		}
	}
	return
}

func issueKeys(issues []model.AuditIssue) []string {
	keys := make([]string, len(issues))
	for i, iss := range issues {
		keys[i] = iss.Dimension + ": " + iss.Summary
	}
	return keys
}

func normalizeReviserMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "", "spot-fix", "polish", "rewrite", "rework", "anti-detect":
		return mode
	default:
		return "spot-fix"
	}
}

func requiresRewriteWithoutCritical(mode string) bool {
	switch mode {
	case "polish", "rewrite", "rework", "anti-detect":
		return true
	default:
		return false
	}
}

func reviserModeNote(mode string) string {
	switch mode {
	case "polish":
		return "Polish prose for clarity, flow, and tone consistency, but keep scene events unchanged."
	case "rewrite":
		return "Permit a stronger sentence-level rewrite where needed, while preserving canon facts and chapter outcomes."
	case "rework":
		return "Rework weak passages more aggressively for impact and readability, but do not alter established story facts."
	case "anti-detect":
		return "Prioritize reducing repetitive rhythm, over-regular phrasing, and obvious AI-style patterns while keeping meaning intact."
	default:
		return "Keep edits tightly scoped and faithful to the existing chapter."
	}
}
