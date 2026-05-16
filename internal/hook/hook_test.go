package hook_test

import (
	"testing"

	"storyforge/internal/hook"
	"storyforge/internal/model"
)

// --- lifecycle tests ---

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from    model.HookStatus
		to      model.HookStatus
		allowed bool
	}{
		{model.HookStatusOpen, model.HookStatusProgressing, true},
		{model.HookStatusOpen, model.HookStatusDeferred, true},
		{model.HookStatusOpen, model.HookStatusResolved, false},
		{model.HookStatusProgressing, model.HookStatusResolved, true},
		{model.HookStatusProgressing, model.HookStatusDeferred, true},
		{model.HookStatusProgressing, model.HookStatusOpen, false},
		{model.HookStatusDeferred, model.HookStatusProgressing, true},
		{model.HookStatusDeferred, model.HookStatusResolved, true},
		{model.HookStatusDeferred, model.HookStatusOpen, false},
		{model.HookStatusResolved, model.HookStatusOpen, false},
		{model.HookStatusResolved, model.HookStatusProgressing, false},
		{model.HookStatusResolved, model.HookStatusDeferred, false},
	}

	for _, c := range cases {
		got := hook.CanTransition(c.from, c.to)
		if got != c.allowed {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", c.from, c.to, got, c.allowed)
		}
	}
}

func TestValidateTransition_Error(t *testing.T) {
	err := hook.ValidateTransition("h1", model.HookStatusResolved, model.HookStatusOpen)
	if err == nil {
		t.Fatal("expected error for resolved -> open")
	}
}

func TestChaptersSinceAdvance(t *testing.T) {
	h := model.HookRecord{
		HookID:              "h1",
		StartChapter:        3,
		LastAdvancedChapter: 5,
		Status:              model.HookStatusProgressing,
	}
	got := hook.ChaptersSinceAdvance(h, 10)
	if got != 5 {
		t.Errorf("expected 5, got %d", got)
	}

	// No advance yet — use StartChapter
	h2 := model.HookRecord{
		HookID:       "h2",
		StartChapter: 3,
		Status:       model.HookStatusOpen,
	}
	got2 := hook.ChaptersSinceAdvance(h2, 8)
	if got2 != 5 {
		t.Errorf("expected 5, got %d", got2)
	}
}

// --- agenda tests ---

func makeState(hooks ...model.HookRecord) model.RuntimeState {
	return model.RuntimeState{PendingHooks: hooks}
}

func TestBuildHookAgenda_StaleDebt(t *testing.T) {
	cfg := hook.DefaultAgendaConfig()
	state := makeState(
		model.HookRecord{
			HookID:              "h1",
			StartChapter:        1,
			LastAdvancedChapter: 1,
			Status:              model.HookStatusOpen,
		},
	)

	// currentChapter = 7, age = 6 >= staleThreshold(5)
	result := hook.BuildHookAgenda(state, 7, cfg)

	if len(result.StaleDebt) != 1 {
		t.Errorf("expected 1 stale hook, got %d", len(result.StaleDebt))
	}
	if result.PressureMap["h1"] == 0 {
		t.Error("expected non-zero pressure for h1")
	}
}

func TestBuildHookAgenda_MustAdvance(t *testing.T) {
	cfg := hook.DefaultAgendaConfig()
	state := makeState(
		model.HookRecord{
			HookID:              "h1",
			StartChapter:        1,
			LastAdvancedChapter: 1,
			Status:              model.HookStatusOpen,
		},
	)

	// age = 11 >= overdueThreshold(10)
	result := hook.BuildHookAgenda(state, 12, cfg)

	if len(result.MustAdvance) != 1 {
		t.Errorf("expected 1 must-advance hook, got %d", len(result.MustAdvance))
	}
}

func TestBuildHookAgenda_EligibleResolve(t *testing.T) {
	cfg := hook.DefaultAgendaConfig()
	state := makeState(
		model.HookRecord{
			HookID:              "h1",
			StartChapter:        1,
			LastAdvancedChapter: 3,
			Status:              model.HookStatusProgressing,
		},
	)

	// age = 3 >= 2, progressing -> eligible
	result := hook.BuildHookAgenda(state, 6, cfg)

	if len(result.EligibleResolve) != 1 {
		t.Errorf("expected 1 eligible-resolve hook, got %d", len(result.EligibleResolve))
	}
}

func TestBuildHookAgenda_ResolvedSkipped(t *testing.T) {
	cfg := hook.DefaultAgendaConfig()
	state := makeState(
		model.HookRecord{
			HookID: "h1",
			Status: model.HookStatusResolved,
		},
	)

	result := hook.BuildHookAgenda(state, 10, cfg)
	if len(result.PressureMap) != 0 {
		t.Error("resolved hooks should not appear in pressure map")
	}
}

// --- health tests ---

func TestAnalyzeHookHealth(t *testing.T) {
	cfg := hook.DefaultAgendaConfig()
	state := makeState(
		model.HookRecord{HookID: "stale", StartChapter: 1, LastAdvancedChapter: 1, Status: model.HookStatusOpen},
		model.HookRecord{HookID: "overdue", StartChapter: 1, LastAdvancedChapter: 1, Status: model.HookStatusProgressing},
		model.HookRecord{HookID: "resolved", Status: model.HookStatusResolved},
		model.HookRecord{HookID: "fresh", StartChapter: 9, LastAdvancedChapter: 9, Status: model.HookStatusOpen},
	)

	// stale: age=9 (>=5), overdue: age=11 (>=10), fresh: age=3
	report := hook.AnalyzeHookHealth(state, 12, cfg)

	if report.TotalResolved != 1 {
		t.Errorf("expected 1 resolved, got %d", report.TotalResolved)
	}
	if report.TotalActive != 3 {
		t.Errorf("expected 3 active, got %d", report.TotalActive)
	}

	// "overdue" hook should appear in both stale and overdue
	foundOverdue := false
	for _, e := range report.Overdue {
		if e.Hook.HookID == "overdue" {
			foundOverdue = true
		}
	}
	if !foundOverdue {
		t.Error("expected 'overdue' hook in Overdue list")
	}

	// "stale" hook: lastAdvanced=1, current=12, age=11 >= overdue(10) — appears in both
	// But we want to test a hook that's stale but not overdue. Use "fresh" hook age=3 < stale.
	// The "stale" hook is actually overdue here. Let's just verify it appears in stale list.
	foundStale := false
	for _, e := range report.Stale {
		if e.Hook.HookID == "stale" {
			foundStale = true
		}
	}
	if !foundStale {
		t.Error("expected 'stale' hook in Stale list")
	}
}

// --- admission tests ---

func TestEvaluateHookAdmission_Duplicate(t *testing.T) {
	state := makeState(
		model.HookRecord{
			HookID:         "h1",
			Type:           "mystery",
			ExpectedPayoff: "the killer is revealed",
			Status:         model.HookStatusOpen,
		},
	)

	candidates := []model.NewHookCandidate{
		{
			Type:           "mystery",
			Description:    "who is the killer",
			ExpectedPayoff: "the killer is revealed",
		},
	}

	result := hook.EvaluateHookAdmission(state, candidates, 3)
	if len(result.Admitted) != 0 {
		t.Error("expected duplicate to be rejected")
	}
	if len(result.Rejected) != 1 {
		t.Errorf("expected 1 rejected, got %d", len(result.Rejected))
	}
}

func TestEvaluateHookAdmission_MaxPerChapter(t *testing.T) {
	state := model.RuntimeState{}
	candidates := []model.NewHookCandidate{
		{Type: "a", Description: "hook a", ExpectedPayoff: "payoff a"},
		{Type: "b", Description: "hook b", ExpectedPayoff: "payoff b"},
		{Type: "c", Description: "hook c", ExpectedPayoff: "payoff c"},
		{Type: "d", Description: "hook d", ExpectedPayoff: "payoff d"},
	}

	result := hook.EvaluateHookAdmission(state, candidates, 2)
	if len(result.Admitted) != 2 {
		t.Errorf("expected 2 admitted, got %d", len(result.Admitted))
	}
	if len(result.Rejected) != 2 {
		t.Errorf("expected 2 rejected, got %d", len(result.Rejected))
	}
}

// --- seed tests ---

func TestExtractSeedExcerpt_ByStartChapter(t *testing.T) {
	h := model.HookRecord{
		HookID:         "h1",
		StartChapter:   3,
		Type:           "mystery",
		ExpectedPayoff: "the artifact is cursed",
	}
	summaries := []model.ChapterSummaryRow{
		{Chapter: 1, Summary: "The hero arrives in town."},
		{Chapter: 3, Summary: "The hero discovers a mysterious artifact. It seems cursed.", HookUpdates: "h1: artifact introduced"},
		{Chapter: 5, Summary: "The hero fights a monster."},
	}

	result := hook.ExtractSeedExcerpt(h, summaries)
	if result.Chapter != 3 {
		t.Errorf("expected chapter 3, got %d", result.Chapter)
	}
	if result.Excerpt == "" {
		t.Error("expected non-empty excerpt")
	}
}

func TestExtractSeedExcerpt_Fallback(t *testing.T) {
	h := model.HookRecord{
		HookID:         "h1",
		StartChapter:   99, // not in summaries
		Type:           "mystery",
		ExpectedPayoff: "cursed",
	}
	summaries := []model.ChapterSummaryRow{
		{Chapter: 3, Summary: "The hero discovers a mysterious artifact. It seems cursed."},
	}

	result := hook.ExtractSeedExcerpt(h, summaries)
	// Should fall back to searching by content
	if result.Excerpt == "" {
		t.Error("expected non-empty excerpt from fallback search")
	}
}

func TestExtractSeedExcerpt_NotFound(t *testing.T) {
	h := model.HookRecord{
		HookID:         "h1",
		StartChapter:   99,
		Type:           "romance",
		ExpectedPayoff: "they get married",
	}
	summaries := []model.ChapterSummaryRow{
		{Chapter: 1, Summary: "The hero fights a dragon."},
	}

	result := hook.ExtractSeedExcerpt(h, summaries)
	if result.Excerpt != "" {
		t.Error("expected empty excerpt when hook not found")
	}
}

// --- mention tests ---

func TestClassifyMention_Real(t *testing.T) {
	text := "The hero finally revealed the secret of the ancient artifact, confirming it was cursed."
	result := hook.ClassifyMention("h1", "artifact is cursed", text)
	if result.Kind != hook.MentionKindReal {
		t.Errorf("expected real advance, got %s (evidence: %s)", result.Kind, result.Evidence)
	}
}

func TestClassifyMention_Passive(t *testing.T) {
	text := "The hero briefly mentioned the artifact, still wondering about it in passing."
	result := hook.ClassifyMention("h1", "artifact is cursed", text)
	if result.Kind != hook.MentionKindPassive {
		t.Errorf("expected passive mention, got %s", result.Kind)
	}
}

func TestFilterRealAdvances(t *testing.T) {
	ops := []model.HookAdvanceOp{
		{HookID: "h1", Chapter: 5},
		{HookID: "h2", Chapter: 5},
	}
	hooks := []model.HookRecord{
		{HookID: "h1", ExpectedPayoff: "secret revealed"},
		{HookID: "h2", ExpectedPayoff: "mystery solved"},
	}
	texts := map[string]string{
		"h1": "The secret was finally revealed to everyone.",
		"h2": "He briefly mentioned the mystery in passing.",
	}

	real := hook.FilterRealAdvances(ops, hooks, texts)
	if len(real) != 1 || real[0].HookID != "h1" {
		t.Errorf("expected only h1 as real advance, got %v", real)
	}
}
