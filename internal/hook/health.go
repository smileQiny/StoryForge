package hook

import "storyforge/internal/model"

// HookHealthReport is the output of AnalyzeHookHealth.
type HookHealthReport struct {
	// Stale contains hooks that haven't been advanced for StaleThreshold chapters.
	Stale []HookDebtEntry
	// Overdue contains hooks that haven't been advanced for OverdueThreshold chapters.
	Overdue []HookDebtEntry
	// TotalActive is the count of non-resolved hooks.
	TotalActive int
	// TotalResolved is the count of resolved hooks.
	TotalResolved int
}

// HookDebtEntry describes a hook with a debt problem.
type HookDebtEntry struct {
	Hook          model.HookRecord
	ChaptersSince int
	DebtKind      string // "stale" or "overdue"
}

// AnalyzeHookHealth audits the hook list and identifies debt.
// currentChapter is the chapter number that was just written.
func AnalyzeHookHealth(state model.RuntimeState, currentChapter int, cfg AgendaConfig) HookHealthReport {
	var report HookHealthReport

	for _, h := range state.PendingHooks {
		if IsResolved(h) {
			report.TotalResolved++
			continue
		}
		report.TotalActive++

		age := ChaptersSinceAdvance(h, currentChapter)

		switch {
		case age >= cfg.OverdueThreshold:
			report.Overdue = append(report.Overdue, HookDebtEntry{
				Hook:          h,
				ChaptersSince: age,
				DebtKind:      "overdue",
			})
			// Also include in stale for completeness
			report.Stale = append(report.Stale, HookDebtEntry{
				Hook:          h,
				ChaptersSince: age,
				DebtKind:      "stale",
			})
		case age >= cfg.StaleThreshold:
			report.Stale = append(report.Stale, HookDebtEntry{
				Hook:          h,
				ChaptersSince: age,
				DebtKind:      "stale",
			})
		}
	}

	return report
}
