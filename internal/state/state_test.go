package state_test

import (
	"testing"

	"storyforge/internal/model"
	"storyforge/internal/state"
)

func baseState() model.RuntimeState {
	return model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "h1", StartChapter: 1, Status: model.HookStatusOpen},
			{HookID: "h2", StartChapter: 2, Status: model.HookStatusProgressing},
		},
		ChapterSummaries: []model.ChapterSummaryRow{
			{Chapter: 1, Title: "Ch1", Summary: "intro"},
		},
	}
}

func TestApplyDelta_AdvanceHook(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter: 3,
		HookOps: model.HookOps{
			Advance: []model.HookAdvanceOp{{HookID: "h1", Chapter: 3}},
		},
	}
	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, h := range next.PendingHooks {
		if h.HookID == "h1" {
			if h.Status != model.HookStatusProgressing {
				t.Errorf("expected progressing, got %s", h.Status)
			}
			if h.LastAdvancedChapter != 3 {
				t.Errorf("expected lastAdvancedChapter=3, got %d", h.LastAdvancedChapter)
			}
			return
		}
	}
	t.Error("hook h1 not found")
}

func TestApplyDelta_ResolveHook(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter: 3,
		HookOps: model.HookOps{
			Resolve: []model.HookResolveOp{{HookID: "h2", Chapter: 3}},
		},
	}
	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, h := range next.PendingHooks {
		if h.HookID == "h2" && h.Status != model.HookStatusResolved {
			t.Errorf("expected resolved, got %s", h.Status)
		}
	}
}

func TestApplyDelta_CannotAdvanceResolved(t *testing.T) {
	s := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "h1", StartChapter: 1, Status: model.HookStatusResolved},
		},
	}
	delta := model.RuntimeStateDelta{
		Chapter: 4,
		HookOps: model.HookOps{
			Advance: []model.HookAdvanceOp{{HookID: "h1", Chapter: 4}},
		},
	}
	_, err := state.ApplyRuntimeStateDelta(s, delta)
	if err == nil {
		t.Error("expected error advancing resolved hook")
	}
}

func TestApplyDelta_AdvanceHookFallsBackToSameChapterMatch(t *testing.T) {
	s := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "hook-7-identity-mystery", StartChapter: 7, Type: "identity-mystery", Status: model.HookStatusOpen},
		},
	}
	delta := model.RuntimeStateDelta{
		Chapter: 8,
		HookOps: model.HookOps{
			Advance: []model.HookAdvanceOp{{HookID: "hook-7-lu-zhechuan-identity", Chapter: 8}},
		},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.PendingHooks[0].Status != model.HookStatusProgressing {
		t.Fatalf("expected progressing, got %s", next.PendingHooks[0].Status)
	}
	if next.PendingHooks[0].LastAdvancedChapter != 8 {
		t.Fatalf("expected lastAdvancedChapter=8, got %d", next.PendingHooks[0].LastAdvancedChapter)
	}
}

func TestApplyDelta_AdvanceHookIgnoresUnknownFallbackWhenAmbiguous(t *testing.T) {
	s := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "hook-7-identity-mystery", StartChapter: 7, Type: "identity-mystery", Status: model.HookStatusOpen},
			{HookID: "hook-7-conspiracy", StartChapter: 7, Type: "conspiracy", Status: model.HookStatusOpen},
		},
	}
	delta := model.RuntimeStateDelta{
		Chapter: 8,
		HookOps: model.HookOps{
			Advance: []model.HookAdvanceOp{{HookID: "hook-7-unknown", Chapter: 8}},
		},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, hook := range next.PendingHooks {
		if hook.Status != s.PendingHooks[i].Status {
			t.Fatalf("expected ambiguous missing hook op to be ignored, got %+v", next.PendingHooks)
		}
	}
}

func TestApplyDelta_AdvanceHookIgnoresRealSemanticGhostID(t *testing.T) {
	s := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "hook-7-identity-mystery", StartChapter: 7, Type: "identity-mystery", Status: model.HookStatusOpen},
			{HookID: "hook-7-process-trigger", StartChapter: 7, Type: "process-trigger", Status: model.HookStatusOpen},
			{HookID: "hook-7-antagonist-hidden-hand", StartChapter: 7, Type: "antagonist-hidden-hand", Status: model.HookStatusOpen},
		},
	}
	delta := model.RuntimeStateDelta{
		Chapter: 8,
		HookOps: model.HookOps{
			Advance: []model.HookAdvanceOp{{HookID: "hook-7-lu-name-on-index", Chapter: 8}},
		},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, hook := range next.PendingHooks {
		if hook.Status != s.PendingHooks[i].Status || hook.LastAdvancedChapter != s.PendingHooks[i].LastAdvancedChapter {
			t.Fatalf("expected unmatched semantic ghost id to be ignored, got %+v", next.PendingHooks)
		}
	}
}

func TestApplyDelta_NewHookCandidate(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter: 3,
		NewHookCandidates: []model.NewHookCandidate{
			{Type: "mystery", Description: "Who is X?", ExpectedPayoff: "chapter 10"},
		},
	}
	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(next.PendingHooks) != len(s.PendingHooks)+1 {
		t.Errorf("expected %d hooks, got %d", len(s.PendingHooks)+1, len(next.PendingHooks))
	}
}

func TestApplyDelta_NewHookCandidatesGetUniqueIDsWithinChapter(t *testing.T) {
	s := model.RuntimeState{}
	delta := model.RuntimeStateDelta{
		Chapter: 1,
		NewHookCandidates: []model.NewHookCandidate{
			{Type: "mystery", Description: "第一个谜团", ExpectedPayoff: "后续揭晓"},
			{Type: "mystery", Description: "第二个谜团", ExpectedPayoff: "中段回收"},
		},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(next.PendingHooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(next.PendingHooks))
	}
	if next.PendingHooks[0].HookID == next.PendingHooks[1].HookID {
		t.Fatalf("expected unique hook IDs, got duplicate %q", next.PendingHooks[0].HookID)
	}
	if err := state.ValidateRuntimeState(next); err != nil {
		t.Fatalf("expected valid runtime state, got %v", err)
	}
}

func TestApplyDelta_ChapterSummary(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter:        2,
		ChapterSummary: &model.ChapterSummaryRow{Chapter: 2, Title: "Ch2", Summary: "action"},
	}
	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(next.ChapterSummaries) != 2 {
		t.Errorf("expected 2 summaries, got %d", len(next.ChapterSummaries))
	}
}

func TestApplyDelta_ChapterSummaryUpsertsExistingChapter(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter:        1,
		ChapterSummary: &model.ChapterSummaryRow{Chapter: 1, Title: "Ch1 revised", Summary: "intro updated"},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(next.ChapterSummaries) != 1 {
		t.Fatalf("expected 1 summary after upsert, got %d", len(next.ChapterSummaries))
	}
	if next.ChapterSummaries[0].Title != "Ch1 revised" || next.ChapterSummaries[0].Summary != "intro updated" {
		t.Fatalf("expected updated summary, got %+v", next.ChapterSummaries[0])
	}
	if s.ChapterSummaries[0].Title != "Ch1" {
		t.Fatalf("original state mutated: %+v", s.ChapterSummaries[0])
	}
}

func TestApplyDelta_NewSubplotPreservesFields(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter: 2,
		SubplotOps: []map[string]any{
			{
				"id":       "sp1",
				"title":    "Guild politics",
				"status":   "active",
				"progress": 40,
			},
		},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(next.SubplotBoard) != 1 {
		t.Fatalf("expected 1 subplot, got %d", len(next.SubplotBoard))
	}
	if next.SubplotBoard[0].Title != "Guild politics" || next.SubplotBoard[0].Status != "active" || next.SubplotBoard[0].Progress != 40 {
		t.Fatalf("unexpected subplot state: %+v", next.SubplotBoard[0])
	}
}

func TestApplyDelta_CharacterMatrixMergesRelationsAndKnows(t *testing.T) {
	s := baseState()
	s.CharacterMatrix = []model.CharacterMatrixEntry{
		{
			CharacterID: "hero",
			Knows:       map[string]any{"gate": "sealed"},
			Relations:   map[string]any{"mentor": "tense"},
		},
	}
	delta := model.RuntimeStateDelta{
		Chapter: 2,
		CharacterMatrixOps: []map[string]any{
			{
				"characterId": "hero",
				"knows":       map[string]any{"trial": "started"},
				"relations":   map[string]any{"ally": "uncertain"},
			},
		},
	}

	next, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(next.CharacterMatrix) != 1 {
		t.Fatalf("expected 1 matrix row, got %d", len(next.CharacterMatrix))
	}
	row := next.CharacterMatrix[0]
	if row.Knows["gate"] != "sealed" || row.Knows["trial"] != "started" {
		t.Fatalf("expected merged knows map, got %+v", row.Knows)
	}
	if row.Relations["mentor"] != "tense" || row.Relations["ally"] != "uncertain" {
		t.Fatalf("expected merged relations map, got %+v", row.Relations)
	}
}

func TestApplyDelta_Immutability(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{
		Chapter: 3,
		HookOps: model.HookOps{
			Resolve: []model.HookResolveOp{{HookID: "h1", Chapter: 3}},
		},
	}
	_, err := state.ApplyRuntimeStateDelta(s, delta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Original state must be unchanged
	for _, h := range s.PendingHooks {
		if h.HookID == "h1" && h.Status != model.HookStatusOpen {
			t.Error("original state was mutated")
		}
	}
}

func TestApplyDelta_InvalidChapter(t *testing.T) {
	s := baseState()
	delta := model.RuntimeStateDelta{Chapter: 0}
	_, err := state.ApplyRuntimeStateDelta(s, delta)
	if err == nil {
		t.Error("expected error for chapter=0")
	}
}

func TestValidateRuntimeState_Valid(t *testing.T) {
	s := baseState()
	if err := state.ValidateRuntimeState(s); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateRuntimeState_DuplicateHook(t *testing.T) {
	s := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "h1", StartChapter: 1, Status: model.HookStatusOpen},
			{HookID: "h1", StartChapter: 2, Status: model.HookStatusOpen},
		},
	}
	if err := state.ValidateRuntimeState(s); err == nil {
		t.Error("expected error for duplicate hookId")
	}
}

func TestValidateRuntimeState_InvalidHookStatus(t *testing.T) {
	s := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "h1", StartChapter: 1, Status: "flying"},
		},
	}
	if err := state.ValidateRuntimeState(s); err == nil {
		t.Error("expected error for invalid hook status")
	}
}

func TestValidateRuntimeState_DuplicateSummary(t *testing.T) {
	s := model.RuntimeState{
		ChapterSummaries: []model.ChapterSummaryRow{
			{Chapter: 1, Summary: "a"},
			{Chapter: 1, Summary: "b"},
		},
	}
	if err := state.ValidateRuntimeState(s); err == nil {
		t.Error("expected error for duplicate chapter summary")
	}
}
