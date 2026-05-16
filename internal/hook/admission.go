package hook

import (
	"strings"

	"storyforge/internal/model"
)

// AdmissionResult is the output of EvaluateHookAdmission.
type AdmissionResult struct {
	Admitted []model.NewHookCandidate
	Rejected []RejectedCandidate
}

// RejectedCandidate is a hook candidate that was rejected.
type RejectedCandidate struct {
	Candidate model.NewHookCandidate
	Reason    string
}

// EvaluateHookAdmission filters new hook candidates, rejecting duplicates
// and candidates that are too similar to existing active hooks.
func EvaluateHookAdmission(
	state model.RuntimeState,
	candidates []model.NewHookCandidate,
	maxNewPerChapter int,
) AdmissionResult {
	var result AdmissionResult

	admitted := 0
	for _, c := range candidates {
		if admitted >= maxNewPerChapter {
			result.Rejected = append(result.Rejected, RejectedCandidate{
				Candidate: c,
				Reason:    "exceeds max new hooks per chapter",
			})
			continue
		}

		if reason := checkDuplicate(state, c); reason != "" {
			result.Rejected = append(result.Rejected, RejectedCandidate{
				Candidate: c,
				Reason:    reason,
			})
			continue
		}

		result.Admitted = append(result.Admitted, c)
		admitted++
	}

	return result
}

// checkDuplicate returns a non-empty reason string if the candidate is too
// similar to an existing active hook.
func checkDuplicate(state model.RuntimeState, c model.NewHookCandidate) string {
	descNorm := normalize(c.Description)
	payoffNorm := normalize(c.ExpectedPayoff)

	for _, h := range state.PendingHooks {
		if IsResolved(h) {
			continue
		}

		// Same type + very similar description
		if h.Type == c.Type && similarity(normalize(h.ExpectedPayoff), payoffNorm) > 0.7 {
			return "duplicate payoff: similar hook already active (" + h.HookID + ")"
		}

		if similarity(normalize(h.ExpectedPayoff), descNorm) > 0.8 {
			return "duplicate description: similar hook already active (" + h.HookID + ")"
		}
	}

	return ""
}

// normalize lowercases and trims a string for comparison.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// similarity returns a rough Jaccard similarity between two strings based on
// word overlap. Returns a value in [0, 1].
func similarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}

	wordsA := tokenize(a)
	wordsB := tokenize(b)

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}

	intersection := 0
	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		if setA[w] {
			intersection++
		}
		setB[w] = true
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// tokenize splits a string into words.
func tokenize(s string) []string {
	return strings.Fields(s)
}
