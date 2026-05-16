package hook

import "storyforge/internal/model"

// AgendaConfig controls thresholds for hook agenda generation.
type AgendaConfig struct {
	// StaleThreshold is the number of chapters after which an active hook is
	// considered stale (default 5).
	StaleThreshold int
	// OverdueThreshold is the number of chapters after which a stale hook
	// becomes overdue (default 10).
	OverdueThreshold int
	// MaxNewHooksPerChapter caps how many new hooks can be introduced.
	MaxNewHooksPerChapter int
}

// DefaultAgendaConfig returns sensible defaults.
func DefaultAgendaConfig() AgendaConfig {
	return AgendaConfig{
		StaleThreshold:        5,
		OverdueThreshold:      10,
		MaxNewHooksPerChapter: 3,
	}
}

// HookAgendaResult is the output of BuildHookAgenda.
type HookAgendaResult struct {
	// PressureMap maps hookId -> pressure score (higher = more urgent).
	PressureMap map[string]int
	// MustAdvance contains hooks that MUST be advanced this chapter.
	MustAdvance []model.HookRecord
	// EligibleResolve contains hooks that could be resolved this chapter.
	EligibleResolve []model.HookRecord
	// StaleDebt contains hooks that have gone too long without advancement.
	StaleDebt []model.HookRecord
}

// BuildHookAgenda computes the hook agenda for the upcoming chapter.
// currentChapter is the chapter number about to be written.
func BuildHookAgenda(state model.RuntimeState, currentChapter int, cfg AgendaConfig) HookAgendaResult {
	result := HookAgendaResult{
		PressureMap: make(map[string]int),
	}

	for _, h := range state.PendingHooks {
		if !IsActive(h) {
			continue
		}

		age := ChaptersSinceAdvance(h, currentChapter)
		pressure := computePressure(h, age, cfg)
		result.PressureMap[h.HookID] = pressure

		switch {
		case age >= cfg.OverdueThreshold:
			result.StaleDebt = append(result.StaleDebt, h)
			result.MustAdvance = append(result.MustAdvance, h)
		case age >= cfg.StaleThreshold:
			result.StaleDebt = append(result.StaleDebt, h)
		}

		if IsProgressing(h) && age >= 2 {
			result.EligibleResolve = append(result.EligibleResolve, h)
		}
	}

	return result
}

// computePressure returns a pressure score for a hook.
// Higher score = more urgent to advance.
func computePressure(h model.HookRecord, age int, cfg AgendaConfig) int {
	score := age * 10

	switch h.Status {
	case model.HookStatusOpen:
		score += 5
	case model.HookStatusProgressing:
		score += 15
	case model.HookStatusDeferred:
		score += 20
	}

	if age >= cfg.OverdueThreshold {
		score += 50
	} else if age >= cfg.StaleThreshold {
		score += 25
	}

	return score
}
