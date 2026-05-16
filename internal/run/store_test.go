package run_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"storyforge/internal/model"
	"storyforge/internal/run"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "storyforge-run-store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestStore_DeleteFromChapter(t *testing.T) {
	dir := tempDir(t)
	rs := run.NewStore(dir)
	bookID := "book-1"

	now := time.Now().UTC()
	run1 := &model.Run{
		ID:        "run-ch1",
		BookID:    bookID,
		Chapter:   1,
		Kind:      model.RunKindPlan,
		Status:    model.RunStatusSucceeded,
		StartedAt: now,
	}
	run2 := &model.Run{
		ID:        "run-ch2",
		BookID:    bookID,
		Chapter:   2,
		Kind:      model.RunKindPlan,
		Status:    model.RunStatusSucceeded,
		StartedAt: now.Add(time.Second),
	}
	if err := rs.Create(run1); err != nil {
		t.Fatalf("create run1: %v", err)
	}
	if err := rs.Create(run2); err != nil {
		t.Fatalf("create run2: %v", err)
	}
	if err := rs.SaveTrace(&model.PromptTrace{RunID: run2.ID, StageName: "plan", UserPrompt: "hello"}); err != nil {
		t.Fatalf("save trace: %v", err)
	}

	if err := rs.DeleteFromChapter(bookID, 2); err != nil {
		t.Fatalf("delete from chapter: %v", err)
	}

	runs, err := rs.ListByBook(bookID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run1.ID {
		t.Fatalf("unexpected runs after delete: %+v", runs)
	}

	if _, err := os.Stat(filepath.Join(dir, "_traces", run2.ID)); !os.IsNotExist(err) {
		t.Fatalf("expected traces of run2 deleted, stat err=%v", err)
	}
}
