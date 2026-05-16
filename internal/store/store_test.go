package store_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "storyforge-store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// --- TruthStore ---

func TestTruthStore_WriteRead(t *testing.T) {
	ts := store.NewTruthStore(tempDir(t))
	type payload struct {
		Value string `json:"value"`
	}
	p := payload{Value: "hello"}
	if err := ts.Write("book1", store.TruthCurrentState, p); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got payload
	if err := ts.Read("book1", store.TruthCurrentState, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Value != "hello" {
		t.Errorf("got %q, want %q", got.Value, "hello")
	}
}

func TestTruthStore_ReadMissing(t *testing.T) {
	ts := store.NewTruthStore(tempDir(t))
	var v any
	if err := ts.Read("book1", store.TruthCurrentState, &v); err != nil {
		t.Errorf("expected nil for missing file, got: %v", err)
	}
}

// --- BookStore ---

func newBook(id string) *model.BookConfig {
	return &model.BookConfig{
		ID:               id,
		Title:            "Test Book",
		Genre:            "xuanhuan",
		Status:           model.BookStatusDraft,
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
}

func TestBookStore_CRUD(t *testing.T) {
	bs := store.NewBookStore(tempDir(t))

	book := newBook("book-1")
	if err := bs.Create(book); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := bs.Get("book-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != book.Title {
		t.Errorf("title mismatch")
	}

	got.Title = "Updated"
	if err := bs.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := bs.Get("book-1")
	if got2.Title != "Updated" {
		t.Errorf("update not persisted")
	}

	books, err := bs.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(books) != 1 {
		t.Errorf("expected 1 book, got %d", len(books))
	}

	if err := bs.Delete("book-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := bs.Get("book-1"); err == nil {
		t.Error("expected error after delete")
	}
}

func TestBookStore_DuplicateCreate(t *testing.T) {
	bs := store.NewBookStore(tempDir(t))
	book := newBook("book-dup")
	_ = bs.Create(book)
	if err := bs.Create(book); err == nil {
		t.Error("expected error on duplicate create")
	}
}

// --- ChapterStore ---

func TestChapterStore_SaveGetContent(t *testing.T) {
	cs := store.NewChapterStore(tempDir(t))
	meta := &model.ChapterMeta{
		Number:    1,
		Title:     "Chapter One",
		Status:    model.ChapterStatusDraft,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := cs.SaveMeta("book1", meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	got, err := cs.GetMeta("book1", 1)
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if got.Title != meta.Title {
		t.Errorf("title mismatch")
	}

	if err := cs.SaveContent("book1", 1, "Once upon a time..."); err != nil {
		t.Fatalf("save content: %v", err)
	}
	content, err := cs.GetContent("book1", 1)
	if err != nil {
		t.Fatalf("get content: %v", err)
	}
	if content != "Once upon a time..." {
		t.Errorf("content mismatch: %q", content)
	}
}

func TestChapterStore_UpdateStatus(t *testing.T) {
	cs := store.NewChapterStore(tempDir(t))
	meta := &model.ChapterMeta{Number: 1, Status: model.ChapterStatusDraft, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_ = cs.SaveMeta("book1", meta)
	if err := cs.UpdateStatus("book1", 1, model.ChapterStatusApproved); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, _ := cs.GetMeta("book1", 1)
	if got.Status != model.ChapterStatusApproved {
		t.Errorf("status not updated")
	}
}

func TestRuntimeStore_DeleteFrom(t *testing.T) {
	rs := store.NewRuntimeStore(tempDir(t))
	bookID := "book-runtime"

	intent1 := &model.ChapterIntent{Chapter: 1, Goal: "g1"}
	intent2 := &model.ChapterIntent{Chapter: 2, Goal: "g2"}
	if err := rs.SaveIntent(bookID, intent1); err != nil {
		t.Fatalf("save intent1: %v", err)
	}
	if err := rs.SaveIntent(bookID, intent2); err != nil {
		t.Fatalf("save intent2: %v", err)
	}

	if err := rs.DeleteFrom(bookID, 2); err != nil {
		t.Fatalf("delete from: %v", err)
	}

	if _, err := rs.GetIntent(bookID, 1); err != nil {
		t.Fatalf("intent1 should remain, got error: %v", err)
	}
	if _, err := rs.GetIntent(bookID, 2); err == nil {
		t.Fatal("intent2 should be deleted")
	}
}

// --- SnapshotStore ---

func TestSnapshotStore_SaveLoadDelete(t *testing.T) {
	ss := store.NewSnapshotStore(tempDir(t))
	snap := &model.ChapterSnapshot{
		Chapter:   3,
		CreatedAt: time.Now().UTC(),
		State:     &model.RuntimeState{},
	}
	if err := ss.Save("book1", snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := ss.Load("book1", 3)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Chapter != 3 {
		t.Errorf("chapter mismatch")
	}
	if err := ss.Delete("book1", 3); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := ss.Load("book1", 3); err == nil {
		t.Error("expected error after delete")
	}
}

// --- FileLock concurrent write test ---

func TestFileLock_ConcurrentWrites(t *testing.T) {
	dir := tempDir(t)
	fl := store.NewFileLock()

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]int, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			unlock, err := fl.Lock(dir)
			if err != nil {
				t.Errorf("lock error: %v", err)
				return
			}
			results[idx] = idx + 1
			unlock()
		}(i)
	}
	wg.Wait()

	for i, v := range results {
		if v != i+1 {
			t.Errorf("results[%d] = %d, want %d", i, v, i+1)
		}
	}
}
