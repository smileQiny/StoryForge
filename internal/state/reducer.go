package state

import (
	"fmt"
	"strconv"
	"strings"

	"storyforge/internal/model"
)

// ApplyRuntimeStateDelta applies a delta to a RuntimeState immutably.
// It returns a new state; the original is never modified.
func ApplyRuntimeStateDelta(state model.RuntimeState, delta model.RuntimeStateDelta) (model.RuntimeState, error) {
	if err := validateDelta(delta); err != nil {
		return model.RuntimeState{}, fmt.Errorf("invalid delta: %w", err)
	}

	next := copyState(state)

	// Apply current state patch
	if delta.CurrentStatePatch != nil {
		applyCurrentStatePatch(&next, delta.CurrentStatePatch)
	}

	// Apply hook ops
	if err := applyHookOps(&next, delta.HookOps, delta.Chapter); err != nil {
		return model.RuntimeState{}, fmt.Errorf("hook ops: %w", err)
	}

	// Admit new hook candidates
	existingHookIDs := make(map[string]struct{}, len(next.PendingHooks))
	for _, hook := range next.PendingHooks {
		existingHookIDs[hook.HookID] = struct{}{}
	}
	for _, candidate := range delta.NewHookCandidates {
		next.PendingHooks = append(next.PendingHooks, model.HookRecord{
			HookID:         generateHookID(candidate, delta.Chapter, existingHookIDs),
			StartChapter:   delta.Chapter,
			Type:           candidate.Type,
			Status:         model.HookStatusOpen,
			ExpectedPayoff: candidate.ExpectedPayoff,
			PayoffTiming:   candidate.PayoffTiming,
		})
	}

	// Append chapter summary
	if delta.ChapterSummary != nil {
		replaced := false
		for i, summary := range next.ChapterSummaries {
			if summary.Chapter != delta.ChapterSummary.Chapter {
				continue
			}
			next.ChapterSummaries[i] = *delta.ChapterSummary
			replaced = true
			break
		}
		if !replaced {
			next.ChapterSummaries = append(next.ChapterSummaries, *delta.ChapterSummary)
		}
	}

	// Apply subplot ops
	for _, op := range delta.SubplotOps {
		applySubplotOp(&next, op)
	}

	// Apply emotional arc ops
	for _, op := range delta.EmotionalArcOps {
		applyEmotionalArcOp(&next, op)
	}

	// Apply character matrix ops
	for _, op := range delta.CharacterMatrixOps {
		applyCharacterMatrixOp(&next, op)
	}

	return next, nil
}

func validateDelta(delta model.RuntimeStateDelta) error {
	if delta.Chapter <= 0 {
		return fmt.Errorf("chapter must be positive, got %d", delta.Chapter)
	}
	for _, adv := range delta.HookOps.Advance {
		if adv.HookID == "" {
			return fmt.Errorf("advance op missing hookId")
		}
	}
	for _, res := range delta.HookOps.Resolve {
		if res.HookID == "" {
			return fmt.Errorf("resolve op missing hookId")
		}
	}
	return nil
}

func copyState(s model.RuntimeState) model.RuntimeState {
	next := model.RuntimeState{}

	// Deep copy maps
	if s.CurrentState != nil {
		next.CurrentState = make(map[string]any, len(s.CurrentState))
		for k, v := range s.CurrentState {
			next.CurrentState[k] = v
		}
	}
	if s.ParticleLedger != nil {
		next.ParticleLedger = make(map[string]any, len(s.ParticleLedger))
		for k, v := range s.ParticleLedger {
			next.ParticleLedger[k] = v
		}
	}

	// Copy slices
	next.PendingHooks = make([]model.HookRecord, len(s.PendingHooks))
	copy(next.PendingHooks, s.PendingHooks)

	next.ChapterSummaries = make([]model.ChapterSummaryRow, len(s.ChapterSummaries))
	copy(next.ChapterSummaries, s.ChapterSummaries)

	next.SubplotBoard = make([]model.SubplotState, len(s.SubplotBoard))
	copy(next.SubplotBoard, s.SubplotBoard)

	next.EmotionalArcs = make([]model.EmotionalArcState, len(s.EmotionalArcs))
	copy(next.EmotionalArcs, s.EmotionalArcs)

	next.CharacterMatrix = make([]model.CharacterMatrixEntry, len(s.CharacterMatrix))
	for i, entry := range s.CharacterMatrix {
		next.CharacterMatrix[i] = model.CharacterMatrixEntry{
			CharacterID: entry.CharacterID,
			Knows:       cloneAnyMap(entry.Knows),
			Relations:   cloneAnyMap(entry.Relations),
		}
	}

	return next
}

func applyCurrentStatePatch(state *model.RuntimeState, patch *model.CurrentStatePatch) {
	if state.CurrentState == nil {
		state.CurrentState = make(map[string]any)
	}
	for _, u := range patch.CharacterUpdates {
		if id, ok := u["id"].(string); ok {
			state.CurrentState["char:"+id] = u
		}
	}
	for _, u := range patch.LocationUpdates {
		if id, ok := u["id"].(string); ok {
			state.CurrentState["loc:"+id] = u
		}
	}
}

func applyHookOps(state *model.RuntimeState, ops model.HookOps, chapter int) error {
	hookIndex := buildHookIndex(state.PendingHooks)

	for _, adv := range ops.Advance {
		idx, ok := resolveHookIndex(state.PendingHooks, hookIndex, adv.HookID)
		if !ok {
			continue
		}
		h := &state.PendingHooks[idx]
		if h.Status == model.HookStatusResolved {
			return fmt.Errorf("cannot advance resolved hook %q", adv.HookID)
		}
		h.Status = model.HookStatusProgressing
		h.LastAdvancedChapter = chapter
	}

	for _, res := range ops.Resolve {
		idx, ok := resolveHookIndex(state.PendingHooks, hookIndex, res.HookID)
		if !ok {
			continue
		}
		state.PendingHooks[idx].Status = model.HookStatusResolved
	}

	for _, def := range ops.Defer {
		idx, ok := resolveHookIndex(state.PendingHooks, hookIndex, def.HookID)
		if !ok {
			continue
		}
		h := &state.PendingHooks[idx]
		if h.Status == model.HookStatusResolved {
			return fmt.Errorf("cannot defer resolved hook %q", def.HookID)
		}
		h.Status = model.HookStatusDeferred
	}

	return nil
}

func buildHookIndex(hooks []model.HookRecord) map[string]int {
	idx := make(map[string]int, len(hooks))
	for i, h := range hooks {
		idx[h.HookID] = i
	}
	return idx
}

func resolveHookIndex(hooks []model.HookRecord, hookIndex map[string]int, requested string) (int, bool) {
	if idx, ok := hookIndex[requested]; ok {
		return idx, true
	}

	chapter, requestedType, ok := parseHookReference(requested)
	if !ok {
		return 0, false
	}

	chapterMatches := make([]int, 0, 2)
	typeMatches := make([]int, 0, 2)
	for i, hook := range hooks {
		if hook.StartChapter != chapter {
			continue
		}
		chapterMatches = append(chapterMatches, i)
		if hookTypeMatches(requestedType, hook.Type) {
			typeMatches = append(typeMatches, i)
		}
	}
	if len(typeMatches) == 1 {
		return typeMatches[0], true
	}
	if len(chapterMatches) == 1 {
		return chapterMatches[0], true
	}
	return 0, false
}

func parseHookReference(hookID string) (int, string, bool) {
	parts := strings.Split(hookID, "-")
	if len(parts) < 3 || parts[0] != "hook" {
		return 0, "", false
	}
	chapter, err := strconv.Atoi(parts[1])
	if err != nil || chapter <= 0 {
		return 0, "", false
	}
	return chapter, strings.Join(parts[2:], "-"), true
}

func hookTypeMatches(requestedType, actualType string) bool {
	requestedType = sanitizeID(requestedType)
	actualType = sanitizeID(actualType)
	if requestedType == "" || actualType == "" {
		return false
	}
	return requestedType == actualType ||
		strings.Contains(requestedType, actualType) ||
		strings.Contains(actualType, requestedType)
}

func applySubplotOp(state *model.RuntimeState, op map[string]any) {
	id, _ := op["id"].(string)
	if id == "" {
		return
	}
	for i, s := range state.SubplotBoard {
		if s.ID == id {
			if title, ok := op["title"].(string); ok && title != "" {
				state.SubplotBoard[i].Title = title
			}
			if status, ok := op["status"].(string); ok {
				state.SubplotBoard[i].Status = status
			}
			if progress, ok := intFromAny(op["progress"]); ok {
				state.SubplotBoard[i].Progress = progress
			}
			return
		}
	}
	// New subplot
	state.SubplotBoard = append(state.SubplotBoard, model.SubplotState{
		ID:       id,
		Title:    stringOrEmpty(op, "title"),
		Status:   stringOrEmpty(op, "status"),
		Progress: intOrZero(op, "progress"),
	})
}

func applyEmotionalArcOp(state *model.RuntimeState, op map[string]any) {
	charID, _ := op["characterId"].(string)
	if charID == "" {
		return
	}
	for i, a := range state.EmotionalArcs {
		if a.CharacterID == charID {
			if arc, ok := op["arc"].(string); ok {
				state.EmotionalArcs[i].Arc = arc
			}
			if phase, ok := op["phase"].(string); ok {
				state.EmotionalArcs[i].Phase = phase
			}
			return
		}
	}
	state.EmotionalArcs = append(state.EmotionalArcs, model.EmotionalArcState{
		CharacterID: charID,
		Arc:         stringOrEmpty(op, "arc"),
		Phase:       stringOrEmpty(op, "phase"),
	})
}

func applyCharacterMatrixOp(state *model.RuntimeState, op map[string]any) {
	charID, _ := op["characterId"].(string)
	if charID == "" {
		return
	}
	for i, e := range state.CharacterMatrix {
		if e.CharacterID == charID {
			state.CharacterMatrix[i].Knows = mergeAnyMap(state.CharacterMatrix[i].Knows, mapFromAny(op["knows"]))
			state.CharacterMatrix[i].Relations = mergeAnyMap(state.CharacterMatrix[i].Relations, mapFromAny(op["relations"]))
			return
		}
	}
	state.CharacterMatrix = append(state.CharacterMatrix, model.CharacterMatrixEntry{
		CharacterID: charID,
		Knows:       cloneAnyMap(mapFromAny(op["knows"])),
		Relations:   cloneAnyMap(mapFromAny(op["relations"])),
	})
}

func intOrZero(op map[string]any, key string) int {
	v, ok := intFromAny(op[key])
	if !ok {
		return 0
	}
	return v
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func mapFromAny(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeAnyMap(dst, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func generateHookID(c model.NewHookCandidate, chapter int, existing map[string]struct{}) string {
	base := fmt.Sprintf("hook-%d-%s", chapter, sanitizeID(c.Type))
	candidate := base
	for suffix := 2; ; suffix++ {
		if _, taken := existing[candidate]; !taken {
			existing[candidate] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func sanitizeID(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '-' {
			out = append(out, b)
		} else if b >= 'A' && b <= 'Z' {
			out = append(out, b+32)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func stringOrEmpty(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
