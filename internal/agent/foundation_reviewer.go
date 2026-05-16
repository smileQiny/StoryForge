package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// FoundationScore is the result of reviewing a single foundation dimension.
type FoundationScore struct {
	Dimension string `json:"dimension"`
	Score     int    `json:"score"` // 0-100
	Feedback  string `json:"feedback"`
}

// FoundationReviewResult is the output of the Foundation Reviewer.
type FoundationReviewResult struct {
	TotalScore      int               `json:"totalScore"`
	Passed          bool              `json:"passed"` // score >= 80
	Scores          []FoundationScore `json:"scores"`
	OverallFeedback string            `json:"overallFeedback"`
	// FanficDivergencePoint is required when fanficMode is set.
	FanficDivergencePoint string `json:"fanficDivergencePoint,omitempty"`
	Usage                 *model.TokenUsage
}

// FoundationReviewerInput is the input to the Foundation Reviewer.
type FoundationReviewerInput struct {
	Book      model.BookConfig
	Architect *ArchitectOutput
}

// FoundationReviewer scores the architect's output across 5 dimensions.
// If score < 80, it rejects and the caller should retry.
type FoundationReviewer struct {
	*BaseAgent
}

var foundationScoreDimensions = []string{
	"world_coherence",
	"character_depth",
	"plot_structure",
	"style_clarity",
	"bible_completeness",
}

// NewFoundationReviewer creates a FoundationReviewer agent.
func NewFoundationReviewer(base *BaseAgent) *FoundationReviewer {
	return &FoundationReviewer{BaseAgent: base}
}

// Review scores the foundation files and returns a pass/fail result.
func (fr *FoundationReviewer) Review(ctx context.Context, input FoundationReviewerInput) (*FoundationReviewResult, error) {
	fanficNote := ""
	if input.Book.FanficMode != model.FanficModeNone {
		fanficNote = fmt.Sprintf(
			"\nIMPORTANT: This is a fanfic (mode: %s). The foundation MUST include a clear original "+
				"divergence point from the source material. Reject if missing.",
			input.Book.FanficMode,
		)
	}

	system := fmt.Sprintf(
		"You are a senior story editor reviewing foundation files for a novel. Language: %s. Genre: %s.%s\n\n"+
			"Score across 5 dimensions (0-100 each):\n"+
			"1. world_coherence: Internal consistency of the world\n"+
			"2. character_depth: Character complexity and motivation clarity\n"+
			"3. plot_structure: Story arc completeness and pacing\n"+
			"4. style_clarity: Style guide specificity and actionability\n"+
			"5. bible_completeness: Writing bible coverage of rules and constraints\n\n"+
			"Return JSON: {\"totalScore\":<avg>,\"passed\":<score>=80>,\"scores\":[...],\"overallFeedback\":\"...\",\"fanficDivergencePoint\":\"...\"}",
		input.Book.Language, input.Book.Genre, fanficNote,
	)

	archJSON, _ := json.Marshal(compactArchitectReviewPayload(input.Architect))
	user := fmt.Sprintf(
		"Book: \"%s\"\n\nFoundation files:\n%s\n\nScore and return the review JSON.",
		input.Book.Title, string(archJSON),
	)

	resp, err := fr.Runtime.Chat(ctx, llm.ChatRequest{
		Model:     fr.Model,
		Messages:  fr.messages(system, user),
		MaxTokens: 800,
	})
	if err != nil {
		return nil, fmt.Errorf("foundation_reviewer: %w", err)
	}

	result, err := parseFoundationReviewResult(ExtractJSON(resp.Content))
	if err != nil {
		return nil, fmt.Errorf("foundation_reviewer: parse result: %w", err)
	}
	result.Usage = ToModelUsage(&resp.Usage)

	// Enforce fanfic divergence point requirement
	if input.Book.FanficMode != model.FanficModeNone && result.FanficDivergencePoint == "" {
		result.Passed = false
		result.OverallFeedback = "Fanfic mode requires an original divergence point. " + result.OverallFeedback
	}

	return &result, nil
}

func compactArchitectReviewPayload(input *ArchitectOutput) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	return map[string]any{
		"worldBuilding": compactArchitectReviewSection(input.WorldBuilding),
		"characters":    compactArchitectReviewSection(input.Characters),
		"plotOutline":   compactArchitectReviewSection(input.PlotOutline),
		"styleGuide":    compactArchitectReviewSection(input.StyleGuide),
		"writingBible":  compactArchitectReviewSection(input.WritingBible),
	}
}

func compactArchitectReviewSection(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}

	if wrapped, ok := payload.(map[string]any); ok {
		if content, exists := wrapped["content"]; exists {
			payload = content
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	text := strings.TrimSpace(string(data))
	if len([]rune(text)) <= 1800 {
		return payload
	}
	return map[string]any{
		"summary": string([]rune(text)[:1800]),
	}
}

func parseFoundationReviewResult(raw string) (FoundationReviewResult, error) {
	var result FoundationReviewResult
	if err := json.Unmarshal([]byte(raw), &result); err == nil {
		return result, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return FoundationReviewResult{}, err
	}

	result = FoundationReviewResult{
		TotalScore:            intValue(payload["totalScore"]),
		OverallFeedback:       stringValue(payload["overallFeedback"]),
		FanficDivergencePoint: stringValue(payload["fanficDivergencePoint"]),
		Scores:                normalizeFoundationScores(payload["scores"]),
	}
	if passed, ok := boolValue(payload["passed"]); ok {
		result.Passed = passed
	} else if result.TotalScore > 0 {
		result.Passed = result.TotalScore >= 80
	}
	if result.TotalScore == 0 && len(result.Scores) > 0 {
		total := 0
		for _, score := range result.Scores {
			total += score.Score
		}
		result.TotalScore = total / len(result.Scores)
		if _, ok := boolValue(payload["passed"]); !ok {
			result.Passed = result.TotalScore >= 80
		}
	}
	return result, nil
}

func normalizeFoundationScores(raw any) []FoundationScore {
	switch typed := raw.(type) {
	case []any:
		rows := make([]FoundationScore, 0, len(typed))
		for index, item := range typed {
			switch entry := item.(type) {
			case map[string]any:
				row := FoundationScore{
					Dimension: stringValue(entry["dimension"]),
					Score:     intValue(entry["score"]),
					Feedback:  stringValue(entry["feedback"]),
				}
				if row.Dimension == "" {
					row.Dimension = foundationScoreDimension(index)
				}
				if row.Dimension != "" || row.Score != 0 || row.Feedback != "" {
					rows = append(rows, row)
				}
			default:
				score := intValue(entry)
				if score == 0 && strings.TrimSpace(fmt.Sprint(entry)) == "" {
					continue
				}
				rows = append(rows, FoundationScore{
					Dimension: foundationScoreDimension(index),
					Score:     score,
				})
			}
		}
		return rows
	case map[string]any:
		rows := make([]FoundationScore, 0, len(typed))
		for _, dimension := range foundationScoreDimensions {
			if value, ok := typed[dimension]; ok {
				rows = append(rows, FoundationScore{Dimension: dimension, Score: intValue(value)})
			}
		}
		for dimension, value := range typed {
			if containsFoundationScoreDimension(dimension) {
				continue
			}
			rows = append(rows, FoundationScore{Dimension: dimension, Score: intValue(value)})
		}
		return rows
	default:
		return nil
	}
}

func foundationScoreDimension(index int) string {
	if index < 0 || index >= len(foundationScoreDimensions) {
		return fmt.Sprintf("dimension_%d", index+1)
	}
	return foundationScoreDimensions[index]
}

func containsFoundationScoreDimension(target string) bool {
	for _, dimension := range foundationScoreDimensions {
		if dimension == target {
			return true
		}
	}
	return false
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		number, err := typed.Int64()
		if err == nil {
			return int(number)
		}
		floatNumber, err := typed.Float64()
		if err == nil {
			return int(floatNumber)
		}
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return number
		}
	}
	return 0
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}
