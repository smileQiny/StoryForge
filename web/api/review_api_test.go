package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/model"
	"storyforge/internal/state"
	"storyforge/internal/store"
)

type reviewHarness struct {
	handler   http.Handler
	books     *store.BookStore
	chapters  *store.ChapterStore
	truths    *store.TruthStore
	snapshots *store.SnapshotStore
	runtime   *store.RuntimeStore
	memory    *state.MemoryDB
}

func newReviewHarness(t *testing.T) *reviewHarness {
	t.Helper()

	dir, err := os.MkdirTemp("", "storyforge-review-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	books := store.NewBookStore(dir)
	chapters := store.NewChapterStore(dir)
	truths := store.NewTruthStore(dir)
	snapshots := store.NewSnapshotStore(dir)
	runtime := store.NewRuntimeStore(dir)
	memory, err := state.OpenMemoryDB(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	h := &reviewHarness{
		books:     books,
		chapters:  chapters,
		truths:    truths,
		snapshots: snapshots,
		runtime:   runtime,
		memory:    memory,
	}

	router := chi.NewRouter()
	router.Post("/api/books/{bookID}/chapters/{chapterNum}/approve", h.approve)
	router.Post("/api/books/{bookID}/chapters/{chapterNum}/reject", h.reject)

	h.handler = router
	return h
}

func (h *reviewHarness) approve(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	chapterNum, ok := parseChapterNumber(w, r)
	if !ok {
		return
	}
	if _, err := h.books.Get(bookID); err != nil {
		reviewWriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := h.chapters.UpdateStatus(bookID, chapterNum, model.ChapterStatusApproved); err != nil {
		reviewWriteError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *reviewHarness) reject(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	chapterNum, ok := parseChapterNumber(w, r)
	if !ok {
		return
	}

	snap, err := h.snapshots.Load(bookID, chapterNum)
	if err != nil {
		reviewWriteError(w, http.StatusNotFound, err.Error())
		return
	}

	if err := restoreTruthFiles(h.truths, bookID, snap.State); err != nil {
		reviewWriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.chapters.DeleteFrom(bookID, chapterNum); err != nil {
		reviewWriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.snapshots.DeleteFrom(bookID, chapterNum); err != nil {
		reviewWriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.runtime.DeleteFrom(bookID, chapterNum); err != nil {
		reviewWriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.memory.DeleteFrom(r.Context(), bookID, chapterNum); err != nil {
		reviewWriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseChapterNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := chi.URLParam(r, "chapterNum")
	chapterNum, err := strconv.Atoi(raw)
	if err == nil && chapterNum > 0 {
		return chapterNum, true
	}
	reviewWriteError(w, http.StatusBadRequest, "invalid chapter number: "+raw)
	return 0, false
}

func reviewWriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func restoreTruthFiles(truths *store.TruthStore, bookID string, state *model.RuntimeState) error {
	if state == nil {
		state = &model.RuntimeState{}
	}
	if err := truths.Write(bookID, store.TruthCurrentState, state.CurrentState); err != nil {
		return err
	}
	if err := truths.Write(bookID, store.TruthParticleLedger, state.ParticleLedger); err != nil {
		return err
	}
	if err := truths.Write(bookID, store.TruthPendingHooks, state.PendingHooks); err != nil {
		return err
	}
	if err := truths.Write(bookID, store.TruthChapterSummaries, state.ChapterSummaries); err != nil {
		return err
	}
	if err := truths.Write(bookID, store.TruthSubplotBoard, state.SubplotBoard); err != nil {
		return err
	}
	if err := truths.Write(bookID, store.TruthEmotionalArcs, state.EmotionalArcs); err != nil {
		return err
	}
	if err := truths.Write(bookID, store.TruthCharacterMatrix, state.CharacterMatrix); err != nil {
		return err
	}
	return nil
}

func seedReviewBook(t *testing.T, h *reviewHarness) (string, *model.RuntimeState) {
	t.Helper()

	book := &model.BookConfig{
		ID:               "review-book",
		Title:            "Review Book",
		Genre:            "xuanhuan",
		Status:           model.BookStatusActive,
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if err := h.books.Create(book); err != nil {
		t.Fatal(err)
	}

	for _, chapter := range []struct {
		number int
		title  string
		status model.ChapterStatus
		body   string
	}{
		{1, "Chapter 1", model.ChapterStatusApproved, "chapter one"},
		{2, "Chapter 2", model.ChapterStatusPendingReview, "chapter two"},
		{3, "Chapter 3", model.ChapterStatusDraft, "chapter three"},
	} {
		meta := &model.ChapterMeta{
			Number:    chapter.number,
			Title:     chapter.title,
			Status:    chapter.status,
			WordCount: len(chapter.body),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := h.chapters.SaveMeta(book.ID, meta); err != nil {
			t.Fatal(err)
		}
		if err := h.chapters.SaveContent(book.ID, chapter.number, chapter.body); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := &model.ChapterSnapshot{
		Chapter:   2,
		CreatedAt: time.Now().UTC(),
		State: &model.RuntimeState{
			CurrentState: map[string]any{"scene": "before-chapter-2"},
			ParticleLedger: map[string]any{
				"gold": 10,
			},
			PendingHooks: []model.HookRecord{
				{
					HookID:         "hook-1",
					StartChapter:   1,
					Type:           "seed",
					Status:         model.HookStatusOpen,
					ExpectedPayoff: "chapter 4",
				},
			},
			ChapterSummaries: []model.ChapterSummaryRow{
				{Chapter: 1, Title: "Chapter 1", Summary: "before"},
			},
			SubplotBoard: []model.SubplotState{
				{ID: "subplot-1", Title: "subplot", Status: "open", Progress: 10},
			},
			EmotionalArcs: []model.EmotionalArcState{
				{CharacterID: "hero", Arc: "growth", Phase: "opening"},
			},
			CharacterMatrix: []model.CharacterMatrixEntry{
				{CharacterID: "hero", Knows: map[string]any{"truth": true}},
			},
		},
	}
	if err := h.snapshots.Save(book.ID, snapshot); err != nil {
		t.Fatal(err)
	}

	afterState := &model.RuntimeState{
		CurrentState: map[string]any{"scene": "after-chapter-2"},
		ParticleLedger: map[string]any{
			"gold": 99,
		},
		PendingHooks: []model.HookRecord{
			{
				HookID:         "hook-1",
				StartChapter:   1,
				Type:           "seed",
				Status:         model.HookStatusProgressing,
				ExpectedPayoff: "chapter 4",
			},
		},
		ChapterSummaries: []model.ChapterSummaryRow{
			{Chapter: 1, Title: "Chapter 1", Summary: "before"},
			{Chapter: 2, Title: "Chapter 2", Summary: "after"},
		},
		SubplotBoard: []model.SubplotState{
			{ID: "subplot-1", Title: "subplot", Status: "progressing", Progress: 80},
		},
		EmotionalArcs: []model.EmotionalArcState{
			{CharacterID: "hero", Arc: "growth", Phase: "middle"},
		},
		CharacterMatrix: []model.CharacterMatrixEntry{
			{CharacterID: "hero", Knows: map[string]any{"truth": false}},
		},
	}
	if err := restoreTruthFiles(h.truths, book.ID, afterState); err != nil {
		t.Fatal(err)
	}

	for _, chapter := range []struct {
		num int
		msg string
	}{
		{2, "chapter 2 memory"},
		{3, "chapter 3 memory"},
	} {
		if err := h.memory.Insert(context.Background(), state.MemoryEntry{
			BookID:    book.ID,
			Chapter:   chapter.num,
			Kind:      "summary",
			Subject:   "chapter",
			Content:   chapter.msg,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	return book.ID, snapshot.State
}

func TestReviewApproveAndRejectRollback(t *testing.T) {
	h := newReviewHarness(t)
	bookID, snapshotState := seedReviewBook(t, h)

	w := do(t, h.handler, http.MethodPost, "/api/books/"+bookID+"/chapters/2/approve", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("approve: expected 204, got %d: %s", w.Code, w.Body.String())
	}
	meta, err := h.chapters.GetMeta(bookID, 2)
	if err != nil {
		t.Fatalf("load approved chapter: %v", err)
	}
	if meta.Status != model.ChapterStatusApproved {
		t.Fatalf("expected approved status, got %s", meta.Status)
	}

	w = do(t, h.handler, http.MethodPost, "/api/books/"+bookID+"/chapters/2/reject", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("reject: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := h.chapters.GetMeta(bookID, 2); err == nil {
		t.Fatal("expected chapter 2 to be deleted after reject")
	}
	if _, err := h.chapters.GetMeta(bookID, 3); err == nil {
		t.Fatal("expected downstream chapter 3 to be deleted after reject")
	}
	if _, err := h.snapshots.Load(bookID, 2); err == nil {
		t.Fatal("expected chapter 2 snapshot to be deleted after reject")
	}
	if _, err := h.runtime.GetIntent(bookID, 2); err == nil {
		t.Fatal("expected runtime artifacts for chapter 2 to be deleted after reject")
	}

	var restored map[string]any
	if err := h.truths.Read(bookID, store.TruthCurrentState, &restored); err != nil {
		t.Fatalf("read restored truth: %v", err)
	}
	if restored["scene"] != snapshotState.CurrentState["scene"] {
		t.Fatalf("expected scene %v, got %v", snapshotState.CurrentState["scene"], restored["scene"])
	}

	entries, err := h.memory.Recall(context.Background(), state.MemoryQuery{
		BookID:  bookID,
		Chapter: 2,
		Limit:   20,
	})
	if err != nil {
		t.Fatalf("recall after reject: %v", err)
	}
	for _, entry := range entries {
		if entry.Chapter >= 2 {
			t.Fatalf("expected memory entries >= chapter 2 to be deleted, got chapter %d", entry.Chapter)
		}
	}
}

func TestReviewRejectMissingSnapshot(t *testing.T) {
	h := newReviewHarness(t)
	bookID, _ := seedReviewBook(t, h)

	w := do(t, h.handler, http.MethodPost, "/api/books/"+bookID+"/chapters/9/reject", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing snapshot, got %d", w.Code)
	}
}

func TestReviewApproveMissingChapter(t *testing.T) {
	h := newReviewHarness(t)
	bookID, _ := seedReviewBook(t, h)

	w := do(t, h.handler, http.MethodPost, "/api/books/"+bookID+"/chapters/9/approve", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing chapter, got %d", w.Code)
	}
}
