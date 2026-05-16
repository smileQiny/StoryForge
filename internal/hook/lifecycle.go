// Package hook implements foreshadowing (伏笔) lifecycle management.
package hook

import (
	"fmt"

	"storyforge/internal/model"
)

// ErrInvalidTransition is returned when a state transition is not allowed.
type ErrInvalidTransition struct {
	HookID string
	From   model.HookStatus
	To     model.HookStatus
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("hook %s: invalid transition %s -> %s", e.HookID, e.From, e.To)
}

// allowedTransitions defines valid state machine edges.
// open -> progressing, deferred
// progressing -> deferred, resolved
// deferred -> progressing, resolved
// resolved -> (terminal, no transitions)
var allowedTransitions = map[model.HookStatus]map[model.HookStatus]bool{
	model.HookStatusOpen: {
		model.HookStatusProgressing: true,
		model.HookStatusDeferred:    true,
	},
	model.HookStatusProgressing: {
		model.HookStatusDeferred: true,
		model.HookStatusResolved: true,
	},
	model.HookStatusDeferred: {
		model.HookStatusProgressing: true,
		model.HookStatusResolved:    true,
	},
	model.HookStatusResolved: {},
}

// CanTransition reports whether a transition from -> to is valid.
func CanTransition(from, to model.HookStatus) bool {
	targets, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// ValidateTransition returns an error if the transition is not allowed.
func ValidateTransition(hookID string, from, to model.HookStatus) error {
	if !CanTransition(from, to) {
		return &ErrInvalidTransition{HookID: hookID, From: from, To: to}
	}
	return nil
}

// IsActive returns true if the hook is still in play (not resolved).
func IsActive(h model.HookRecord) bool {
	return h.Status != model.HookStatusResolved
}

// IsOpen returns true if the hook has not yet been advanced.
func IsOpen(h model.HookRecord) bool {
	return h.Status == model.HookStatusOpen
}

// IsProgressing returns true if the hook is actively being developed.
func IsProgressing(h model.HookRecord) bool {
	return h.Status == model.HookStatusProgressing
}

// IsDeferred returns true if the hook has been postponed.
func IsDeferred(h model.HookRecord) bool {
	return h.Status == model.HookStatusDeferred
}

// IsResolved returns true if the hook has been paid off.
func IsResolved(h model.HookRecord) bool {
	return h.Status == model.HookStatusResolved
}

// ChaptersSinceAdvance returns how many chapters have passed since the hook
// was last advanced (or since it started, if never advanced).
func ChaptersSinceAdvance(h model.HookRecord, currentChapter int) int {
	last := h.LastAdvancedChapter
	if last == 0 {
		last = h.StartChapter
	}
	diff := currentChapter - last
	if diff < 0 {
		return 0
	}
	return diff
}
