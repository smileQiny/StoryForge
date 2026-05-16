package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// NormalizerInput is the input to the Normalizer agent.
type NormalizerInput struct {
	ChapterText string
	TargetMin   int
	TargetMax   int
	Language    string
}

// NormalizerOutput is the output of the Normalizer agent.
type NormalizerOutput struct {
	Content   string
	WordCount int
	Action    string // "expanded", "compressed", "unchanged"
}

const normalizerMaxPasses = 3

// Normalizer performs a single-pass word count correction.
// It is a safety net — called only when Writer output is outside the soft range.
type Normalizer struct {
	*BaseAgent
}

// NewNormalizer creates a Normalizer agent.
func NewNormalizer(base *BaseAgent) *Normalizer {
	return &Normalizer{BaseAgent: base}
}

// Normalize adjusts the chapter text to fit within the target range.
func (n *Normalizer) Normalize(ctx context.Context, input NormalizerInput) (*NormalizerOutput, error) {
	currentContent := input.ChapterText
	currentWC := utf8.RuneCountInString(currentContent)
	if currentWC >= input.TargetMin && currentWC <= input.TargetMax {
		return &NormalizerOutput{
			Content:   currentContent,
			WordCount: currentWC,
			Action:    "unchanged",
		}, nil
	}

	action := "expanded"
	if currentWC > input.TargetMax {
		action = "compressed"
	}
	bestContent := currentContent
	bestWC := currentWC
	bestDistance := distanceToRange(currentWC, input.TargetMin, input.TargetMax)

	system := fmt.Sprintf(
		"You are a professional editor. Language: %s. "+
			"Return ONLY the revised chapter text, no commentary.",
		input.Language,
	)
	for pass := 0; pass < normalizerMaxPasses; pass++ {
		instruction := fmt.Sprintf(
			"The chapter is too short (%d chars). Expand it to %d-%d chars by adding vivid details, "+
				"dialogue, and scene description. Do NOT change the plot or characters.",
			currentWC, input.TargetMin, input.TargetMax,
		)
		if action == "compressed" {
			instruction = fmt.Sprintf(
				"The chapter is too long (%d chars). Compress it to %d-%d chars by removing redundant "+
					"descriptions and filler. Do NOT remove plot-critical content.",
				currentWC, input.TargetMin, input.TargetMax,
			)
		}

		user := fmt.Sprintf("%s\n\nChapter text:\n\n%s", instruction, currentContent)
		content, _, err := n.Chat(ctx, system, user)
		if err != nil {
			break
		}
		content = strings.TrimSpace(content)
		if content == "" {
			break
		}

		newWC := utf8.RuneCountInString(content)
		newDistance := distanceToRange(newWC, input.TargetMin, input.TargetMax)
		if newDistance < bestDistance {
			bestContent = content
			bestWC = newWC
			bestDistance = newDistance
		}
		if newWC >= input.TargetMin && newWC <= input.TargetMax {
			return &NormalizerOutput{
				Content:   content,
				WordCount: newWC,
				Action:    action,
			}, nil
		}

		currentContent = content
		currentWC = newWC
	}

	return &NormalizerOutput{
		Content:   bestContent,
		WordCount: bestWC,
		Action:    action,
	}, nil
}

func distanceToRange(count, min, max int) int {
	switch {
	case count < min:
		return min - count
	case count > max:
		return count - max
	default:
		return 0
	}
}
