package hook

import (
	"strings"

	"storyforge/internal/model"
)

// MentionKind classifies how a hook was referenced in a chapter.
type MentionKind string

const (
	// MentionKindReal indicates the hook was genuinely advanced (plot moved forward).
	MentionKindReal MentionKind = "real"
	// MentionKindPassive indicates the hook was only mentioned in passing.
	MentionKindPassive MentionKind = "passive"
)

// MentionAnalysis is the result of classifying a hook mention.
type MentionAnalysis struct {
	HookID   string
	Kind     MentionKind
	Evidence string
}

// passiveIndicators are phrases that suggest a hook was only mentioned, not advanced.
var passiveIndicators = []string{
	"mentioned",
	"recalled",
	"remembered",
	"thought about",
	"wondered about",
	"briefly",
	"in passing",
	"still unresolved",
	"yet to be",
	"had not yet",
	"hadn't yet",
	"still pending",
	"lingered",
	"remained",
	"as before",
}

// realAdvanceIndicators are phrases that suggest genuine plot advancement.
var realAdvanceIndicators = []string{
	"revealed",
	"discovered",
	"confronted",
	"resolved",
	"answered",
	"explained",
	"confirmed",
	"denied",
	"progressed",
	"advanced",
	"unfolded",
	"happened",
	"occurred",
	"took place",
	"finally",
	"at last",
}

// ClassifyMention determines whether a hook mention in chapterText represents
// a real advance or a passive mention.
// hookDescription is the hook's expectedPayoff or description for context matching.
func ClassifyMention(hookID, hookDescription, chapterText string) MentionAnalysis {
	lower := strings.ToLower(chapterText)

	realScore := 0
	passiveScore := 0
	var evidence string

	for _, ind := range realAdvanceIndicators {
		if strings.Contains(lower, ind) {
			realScore++
			if evidence == "" {
				evidence = ind
			}
		}
	}

	for _, ind := range passiveIndicators {
		if strings.Contains(lower, ind) {
			passiveScore++
		}
	}

	kind := MentionKindReal
	if passiveScore > realScore {
		kind = MentionKindPassive
		evidence = "passive indicators outweigh real advance indicators"
	}

	return MentionAnalysis{
		HookID:   hookID,
		Kind:     kind,
		Evidence: evidence,
	}
}

// FilterRealAdvances returns only the hooks from ops whose chapter text
// indicates a genuine advance (not just a mention).
// chapterTexts maps hookId -> relevant excerpt from the chapter.
func FilterRealAdvances(
	ops []model.HookAdvanceOp,
	hooks []model.HookRecord,
	chapterTexts map[string]string,
) []model.HookAdvanceOp {
	hookDesc := make(map[string]string, len(hooks))
	for _, h := range hooks {
		hookDesc[h.HookID] = h.ExpectedPayoff
	}

	var real []model.HookAdvanceOp
	for _, op := range ops {
		text, ok := chapterTexts[op.HookID]
		if !ok {
			// No text provided — assume real advance
			real = append(real, op)
			continue
		}
		analysis := ClassifyMention(op.HookID, hookDesc[op.HookID], text)
		if analysis.Kind == MentionKindReal {
			real = append(real, op)
		}
	}
	return real
}
