package hook

import (
	"strings"

	"storyforge/internal/model"
)

// SeedExcerpt is the result of extracting a seed scene for a hook.
type SeedExcerpt struct {
	HookID  string
	Chapter int
	Excerpt string
}

// ExtractSeedExcerpt finds the original seed scene for a hook from the
// chapter summaries. It looks for the chapter where the hook was introduced
// (StartChapter) and extracts the relevant portion of the summary.
func ExtractSeedExcerpt(hook model.HookRecord, summaries []model.ChapterSummaryRow) SeedExcerpt {
	result := SeedExcerpt{HookID: hook.HookID}

	// Find the chapter summary for the hook's start chapter
	for _, row := range summaries {
		if row.Chapter != hook.StartChapter {
			continue
		}

		result.Chapter = row.Chapter
		result.Excerpt = extractRelevantExcerpt(hook, row)
		return result
	}

	// Fallback: search all summaries for hook mentions
	for _, row := range summaries {
		if containsHookReference(hook, row) {
			result.Chapter = row.Chapter
			result.Excerpt = extractRelevantExcerpt(hook, row)
			return result
		}
	}

	return result
}

// extractRelevantExcerpt pulls the most relevant portion of a summary for a hook.
func extractRelevantExcerpt(hook model.HookRecord, row model.ChapterSummaryRow) string {
	// Prefer hookUpdates field if it mentions this hook
	if row.HookUpdates != "" && containsHookID(hook.HookID, row.HookUpdates) {
		return trimExcerpt(row.HookUpdates)
	}

	// Fall back to the full summary
	summary := row.Summary
	if summary == "" {
		return ""
	}

	// Try to find the sentence(s) most relevant to the hook
	sentences := splitSentences(summary)
	for _, s := range sentences {
		lower := strings.ToLower(s)
		if strings.Contains(lower, strings.ToLower(hook.Type)) ||
			strings.Contains(lower, strings.ToLower(hook.ExpectedPayoff)) {
			return trimExcerpt(s)
		}
	}

	// Return first 200 chars of summary as fallback
	return trimExcerpt(summary)
}

// containsHookReference checks if a summary row references the given hook.
func containsHookReference(hook model.HookRecord, row model.ChapterSummaryRow) bool {
	if containsHookID(hook.HookID, row.HookUpdates) {
		return true
	}
	lower := strings.ToLower(row.Summary)
	return strings.Contains(lower, strings.ToLower(hook.Type)) ||
		strings.Contains(lower, strings.ToLower(hook.ExpectedPayoff))
}

// containsHookID checks if text contains the hookId.
func containsHookID(hookID, text string) bool {
	return strings.Contains(text, hookID)
}

// splitSentences splits text into sentences on common punctuation.
func splitSentences(text string) []string {
	var sentences []string
	var buf strings.Builder

	for _, r := range text {
		buf.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '。' || r == '！' || r == '？' {
			s := strings.TrimSpace(buf.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		s := strings.TrimSpace(buf.String())
		if s != "" {
			sentences = append(sentences, s)
		}
	}
	return sentences
}

// trimExcerpt truncates an excerpt to a reasonable length.
func trimExcerpt(s string) string {
	const maxLen = 300
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	// Try to cut at a sentence boundary
	for i := maxLen; i > maxLen/2; i-- {
		if s[i] == '.' || s[i] == '!' || s[i] == '?' {
			return s[:i+1]
		}
	}
	return s[:maxLen] + "..."
}
