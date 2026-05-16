package state

import (
	"fmt"

	"storyforge/internal/model"
)

// ValidateRuntimeState checks structural integrity of a RuntimeState.
// Returns an error if the state is inconsistent or contains bad data.
func ValidateRuntimeState(s model.RuntimeState) error {
	if err := validateHooks(s.PendingHooks); err != nil {
		return fmt.Errorf("hooks: %w", err)
	}
	if err := validateChapterSummaries(s.ChapterSummaries); err != nil {
		return fmt.Errorf("chapter summaries: %w", err)
	}
	if err := validateSubplots(s.SubplotBoard); err != nil {
		return fmt.Errorf("subplot board: %w", err)
	}
	return nil
}

func validateHooks(hooks []model.HookRecord) error {
	seen := make(map[string]bool, len(hooks))
	for i, h := range hooks {
		if h.HookID == "" {
			return fmt.Errorf("hook[%d]: missing hookId", i)
		}
		if seen[h.HookID] {
			return fmt.Errorf("hook[%d]: duplicate hookId %q", i, h.HookID)
		}
		seen[h.HookID] = true
		if !model.ValidHookStatuses[h.Status] {
			return fmt.Errorf("hook %q: invalid status %q", h.HookID, h.Status)
		}
		if h.StartChapter <= 0 {
			return fmt.Errorf("hook %q: startChapter must be positive", h.HookID)
		}
	}
	return nil
}

func validateChapterSummaries(summaries []model.ChapterSummaryRow) error {
	seen := make(map[int]bool, len(summaries))
	for i, s := range summaries {
		if s.Chapter <= 0 {
			return fmt.Errorf("summary[%d]: chapter must be positive", i)
		}
		if seen[s.Chapter] {
			return fmt.Errorf("summary[%d]: duplicate chapter %d", i, s.Chapter)
		}
		seen[s.Chapter] = true
	}
	return nil
}

func validateSubplots(subplots []model.SubplotState) error {
	seen := make(map[string]bool, len(subplots))
	for i, s := range subplots {
		if s.ID == "" {
			return fmt.Errorf("subplot[%d]: missing id", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("subplot[%d]: duplicate id %q", i, s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}
