package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// RadarSignal is a single signal detected by the Radar agent.
type RadarSignal struct {
	Kind       string  `json:"kind"`    // ai_trace/fatigue_word/cliche/repetition
	Text       string  `json:"text"`    // the offending text
	Position   int     `json:"position"` // approximate char offset
	Confidence float64 `json:"confidence"`
	Suggestion string  `json:"suggestion,omitempty"`
}

// RadarOutput is the output of the Radar agent.
type RadarOutput struct {
	Signals []RadarSignal
	Skipped bool
}

// RadarInput is the input to the Radar agent.
type RadarInput struct {
	ChapterText  string
	Language     string
	FatigueWords []string // from genre config
	Skip         bool     // if true, Radar is bypassed
}

// Radar is a pluggable, skippable signal detector for AI traces and clichés.
type Radar struct {
	*BaseAgent
}

// NewRadar creates a Radar agent.
func NewRadar(base *BaseAgent) *Radar {
	return &Radar{BaseAgent: base}
}

// Scan runs the radar scan. Returns immediately if Skip is true.
func (r *Radar) Scan(ctx context.Context, input RadarInput) (*RadarOutput, error) {
	if input.Skip {
		return &RadarOutput{Skipped: true}, nil
	}

	fatigueList := ""
	if len(input.FatigueWords) > 0 {
		b, _ := json.Marshal(input.FatigueWords)
		fatigueList = string(b)
	}

	system := fmt.Sprintf(
		"You are an AI-trace and cliché detector for %s fiction.\n"+
			"Detect: ai_trace (robotic phrasing), fatigue_word (overused words), cliche (stock phrases), repetition (repeated sentence patterns).\n"+
			"Fatigue words to flag: %s\n"+
			"Return JSON array: [{\"kind\":\"...\",\"text\":\"...\",\"position\":<int>,\"confidence\":<0-1>,\"suggestion\":\"...\"}]",
		input.Language, fatigueList,
	)
	user := fmt.Sprintf("Scan this chapter text:\n\n%s", input.ChapterText)

	resp, _, err := r.Chat(ctx, system, user)
	if err != nil {
		// Radar is non-critical — return empty on failure
		return &RadarOutput{}, nil
	}

	var signals []RadarSignal
	if err := json.Unmarshal([]byte(ExtractJSON(resp)), &signals); err != nil {
		return &RadarOutput{}, nil
	}

	return &RadarOutput{Signals: signals}, nil
}
