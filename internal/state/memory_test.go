package state_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"storyforge/internal/state"
)

func TestMemoryDB_InsertRecall(t *testing.T) {
	dir, err := os.MkdirTemp("", "storyforge-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := state.OpenMemoryDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	entries := []state.MemoryEntry{
		{BookID: "b1", Chapter: 1, Kind: "fact", Subject: "hero", Content: "hero is brave", Tags: "character", CreatedAt: time.Now()},
		{BookID: "b1", Chapter: 2, Kind: "hook", Subject: "mystery", Content: "who is the stranger", Tags: "hook,mystery", CreatedAt: time.Now()},
		{BookID: "b1", Chapter: 5, Kind: "summary", Subject: "ch5", Content: "battle scene", Tags: "action", CreatedAt: time.Now()},
	}
	for _, e := range entries {
		if err := db.Insert(ctx, e); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Recall all for book
	results, err := db.Recall(ctx, state.MemoryQuery{BookID: "b1", Chapter: 3, Limit: 10})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Recall by kind
	hooks, err := db.Recall(ctx, state.MemoryQuery{BookID: "b1", Chapter: 3, Kind: "hook", Limit: 10})
	if err != nil {
		t.Fatalf("recall hooks: %v", err)
	}
	if len(hooks) != 1 {
		t.Errorf("expected 1 hook, got %d", len(hooks))
	}

	// Recall by term
	brave, err := db.Recall(ctx, state.MemoryQuery{BookID: "b1", Chapter: 1, Terms: []string{"brave"}, Limit: 10})
	if err != nil {
		t.Fatalf("recall by term: %v", err)
	}
	if len(brave) != 1 || brave[0].Subject != "hero" {
		t.Errorf("term search mismatch: %+v", brave)
	}
}

func TestMemoryDB_DeleteFrom(t *testing.T) {
	dir, err := os.MkdirTemp("", "storyforge-memory-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	db, err := state.OpenMemoryDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		_ = db.Insert(ctx, state.MemoryEntry{
			BookID: "b1", Chapter: i, Kind: "fact",
			Subject: "s", Content: "c", CreatedAt: time.Now(),
		})
	}

	if err := db.DeleteFrom(ctx, "b1", 3); err != nil {
		t.Fatalf("delete from: %v", err)
	}

	results, _ := db.Recall(ctx, state.MemoryQuery{BookID: "b1", Chapter: 1, Limit: 20})
	if len(results) != 2 {
		t.Errorf("expected 2 remaining entries, got %d", len(results))
	}
}
