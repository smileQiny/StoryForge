package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"storyforge/internal/model"
)

// roundtrip marshals v to JSON and unmarshals into a new value of the same type.
func roundtrip[T any](t *testing.T, v T) T {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestBookConfig_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	b := model.BookConfig{
		ID:               "book-1",
		Title:            "Test Book",
		Genre:            "xuanhuan",
		Status:           model.BookStatusActive,
		Language:         model.LanguageZH,
		TargetChapters:   100,
		ChapterWordCount: 3000,
		FanficMode:       model.FanficModeNone,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	got := roundtrip(t, b)
	if got.ID != b.ID || got.Title != b.Title || got.Language != b.Language {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
}

func TestBookConfig_Validate(t *testing.T) {
	valid := model.BookConfig{
		ID:               "book-1",
		Title:            "Test",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*model.BookConfig)
	}{
		{"missing id", func(b *model.BookConfig) { b.ID = "" }},
		{"missing title", func(b *model.BookConfig) { b.Title = "" }},
		{"missing genre", func(b *model.BookConfig) { b.Genre = "" }},
		{"invalid language", func(b *model.BookConfig) { b.Language = "fr" }},
		{"zero word count", func(b *model.BookConfig) { b.ChapterWordCount = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := valid
			tc.mutate(&b)
			if err := b.Validate(); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}

func TestChapterMeta_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := model.ChapterMeta{
		Number:    1,
		Title:     "Chapter One",
		Status:    model.ChapterStatusDraft,
		WordCount: 3000,
		CreatedAt: now,
		UpdatedAt: now,
	}
	got := roundtrip(t, c)
	if got.Number != c.Number || got.Status != c.Status {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
}

func TestChapterMeta_Validate(t *testing.T) {
	valid := model.ChapterMeta{Number: 1, Status: model.ChapterStatusDraft}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	bad := model.ChapterMeta{Number: 0, Status: model.ChapterStatusDraft}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for zero number")
	}

	bad2 := model.ChapterMeta{Number: 1, Status: "unknown"}
	if err := bad2.Validate(); err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestRuntimeStateDelta_RoundTrip(t *testing.T) {
	delta := model.RuntimeStateDelta{
		Chapter: 5,
		HookOps: model.HookOps{
			Advance: []model.HookAdvanceOp{{HookID: "h1", Chapter: 5}},
		},
		NewHookCandidates: []model.NewHookCandidate{
			{Type: "mystery", Description: "Who is the stranger?", ExpectedPayoff: "chapter 10"},
		},
		ChapterSummary: &model.ChapterSummaryRow{
			Chapter: 5, Title: "The Stranger", Summary: "A stranger arrives.",
		},
	}
	got := roundtrip(t, delta)
	if got.Chapter != delta.Chapter {
		t.Errorf("chapter mismatch: got %d", got.Chapter)
	}
	if len(got.HookOps.Advance) != 1 || got.HookOps.Advance[0].HookID != "h1" {
		t.Errorf("hook ops mismatch: got %+v", got.HookOps)
	}
	if got.ChapterSummary == nil || got.ChapterSummary.Title != "The Stranger" {
		t.Errorf("chapter summary mismatch")
	}
}

func TestChapterIntent_RoundTrip(t *testing.T) {
	intent := model.ChapterIntent{
		Chapter:   3,
		Goal:      "Introduce the antagonist",
		MustKeep:  []string{"protagonist's secret"},
		MustAvoid: []string{"info dump"},
		HookAgenda: model.HookAgenda{
			MustAdvance:     []string{"h1"},
			EligibleResolve: []string{"h2"},
			StaleDebt:       []string{"h3"},
		},
	}
	got := roundtrip(t, intent)
	if got.Chapter != intent.Chapter || got.Goal != intent.Goal {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
	if len(got.HookAgenda.MustAdvance) != 1 {
		t.Errorf("hook agenda mismatch")
	}
}

func TestAuditReport_RoundTrip(t *testing.T) {
	report := model.AuditReport{
		Chapter: 2,
		Passed:  false,
		Issues: []model.AuditIssue{
			{Dimension: "continuity", Severity: "critical", Summary: "Character teleported"},
		},
		Dimensions: []model.AuditDimensionResult{
			{Key: "continuity", Passed: false, Score: 40},
		},
	}
	got := roundtrip(t, report)
	if got.Passed != false || len(got.Issues) != 1 {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
	if !got.HasCriticalIssues() {
		t.Error("expected critical issues")
	}
}

func TestRun_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	run := model.Run{
		ID:          "run-1",
		BookID:      "book-1",
		Chapter:     1,
		Kind:        model.RunKindFullPipeline,
		Status:      model.RunStatusRunning,
		TriggeredBy: model.RunTriggeredByStudio,
		StartedAt:   now,
		Stages: []model.RunStage{
			{Name: "plan", Status: model.StageStatusSucceeded},
		},
	}
	got := roundtrip(t, run)
	if got.ID != run.ID || got.Kind != run.Kind || got.Status != run.Status {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
	if len(got.Stages) != 1 || got.Stages[0].Name != "plan" {
		t.Errorf("stages mismatch")
	}
}

func TestChapterSnapshot_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	snap := model.ChapterSnapshot{
		Chapter:   3,
		CreatedAt: now,
		State: &model.RuntimeState{
			PendingHooks: []model.HookRecord{
				{HookID: "h1", Status: model.HookStatusOpen, StartChapter: 1},
			},
		},
	}
	got := roundtrip(t, snap)
	if got.Chapter != snap.Chapter {
		t.Errorf("chapter mismatch")
	}
	if got.State == nil || len(got.State.PendingHooks) != 1 {
		t.Errorf("state mismatch")
	}
}

func TestCoreAuditDimensions_Count(t *testing.T) {
	if len(model.CoreAuditDimensions) != 11 {
		t.Errorf("expected 11 core dimensions, got %d", len(model.CoreAuditDimensions))
	}
}

func TestFanficAuditDimensions_Count(t *testing.T) {
	if len(model.FanficAuditDimensions) != 6 {
		t.Errorf("expected 6 fanfic dimensions, got %d", len(model.FanficAuditDimensions))
	}
}
