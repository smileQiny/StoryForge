package api_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/model"
	"storyforge/internal/store"
	"storyforge/web/api"
)

type exportHarness struct {
	handler  http.Handler
	books    *store.BookStore
	chapters *store.ChapterStore
}

func newExportHarness(t *testing.T) *exportHarness {
	t.Helper()
	dir := t.TempDir()

	books := store.NewBookStore(dir)
	chapters := store.NewChapterStore(dir)
	svc := app.NewExportService(dir, books, chapters)

	router := chi.NewRouter()
	router.Get("/api/books/{bookID}/export", func(w http.ResponseWriter, r *http.Request) {
		api.NewExportHandler(svc).ExportBook(w, r)
	})
	router.Post("/api/books/{bookID}/export-save", func(w http.ResponseWriter, r *http.Request) {
		api.NewExportHandler(svc).SaveBook(w, r)
	})

	return &exportHarness{handler: router, books: books, chapters: chapters}
}

func (h *exportHarness) seedBook(t *testing.T, bookID string) {
	t.Helper()
	now := time.Now().UTC()
	book := &model.BookConfig{
		ID:               bookID,
		Title:            "Export Book",
		Genre:            "xuanhuan",
		Status:           model.BookStatusActive,
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := h.books.Create(book); err != nil {
		t.Fatal(err)
	}
}

func (h *exportHarness) seedChapter(t *testing.T, bookID string, number int, title, content string) {
	t.Helper()
	now := time.Now().UTC()
	meta := &model.ChapterMeta{
		Number:    number,
		Title:     title,
		Status:    model.ChapterStatusApproved,
		WordCount: len(content),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.chapters.SaveMeta(bookID, meta); err != nil {
		t.Fatal(err)
	}
	if err := h.chapters.SaveContent(bookID, number, content); err != nil {
		t.Fatal(err)
	}
}

func TestExportBook_TextAndMarkdown(t *testing.T) {
	h := newExportHarness(t)
	h.seedBook(t, "export-book")
	h.seedChapter(t, "export-book", 2, "Second", "chapter two body")
	h.seedChapter(t, "export-book", 1, "First", "chapter one body")

	w := do(t, h.handler, http.MethodGet, "/api/books/export-book/export?format=txt", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("txt export: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("txt export: unexpected content type %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `export-book.txt`) {
		t.Fatalf("txt export: unexpected disposition %q", cd)
	}
	body := w.Body.String()
	firstIdx := strings.Index(body, "Chapter 1: First")
	secondIdx := strings.Index(body, "Chapter 2: Second")
	if firstIdx == -1 || secondIdx == -1 || firstIdx > secondIdx {
		t.Fatalf("txt export not sorted by chapter: %q", body)
	}

	w = do(t, h.handler, http.MethodGet, "/api/books/export-book/export?format=md", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("md export: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/markdown") {
		t.Fatalf("md export: unexpected content type %q", ct)
	}
	if !strings.Contains(w.Body.String(), "# Chapter 1: First") {
		t.Fatalf("md export missing chapter heading: %q", w.Body.String())
	}

	w = do(t, h.handler, http.MethodGet, "/api/books/export-book/export?format=epub", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("html export: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("html export: unexpected content type %q", ct)
	}
	if !strings.Contains(w.Body.String(), "<h2 id=\"ch1\">Chapter 1: First</h2>") {
		t.Fatalf("html export missing chapter heading: %q", w.Body.String())
	}
}

func TestExportBook_InvalidFormat(t *testing.T) {
	h := newExportHarness(t)
	h.seedBook(t, "export-book")

	w := do(t, h.handler, http.MethodGet, "/api/books/export-book/export?format=pdf", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid format: expected 400, got %d", w.Code)
	}
}

func TestExportSave_ApprovedOnly(t *testing.T) {
	h := newExportHarness(t)
	h.seedBook(t, "export-book")
	h.seedChapter(t, "export-book", 1, "Approved", "approved body")
	h.seedChapter(t, "export-book", 2, "Draft", "draft body")

	meta, err := h.chapters.GetMeta("export-book", 2)
	if err != nil {
		t.Fatal(err)
	}
	meta.Status = model.ChapterStatusDraft
	if err := h.chapters.SaveMeta("export-book", meta); err != nil {
		t.Fatal(err)
	}

	w := do(t, h.handler, http.MethodPost, "/api/books/export-book/export-save", map[string]any{
		"format":       "epub",
		"approvedOnly": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("export save: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, w, &resp)
	if resp["chapters"] != float64(1) {
		t.Fatalf("expected 1 exported chapter, got %v", resp["chapters"])
	}
	path, _ := resp["path"].(string)
	if !strings.HasSuffix(path, "export-book/export-book.html") {
		t.Fatalf("unexpected export path %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "Approved") {
		t.Fatalf("expected approved chapter content in export: %q", body)
	}
	if strings.Contains(body, "Draft") {
		t.Fatalf("did not expect draft chapter in approved-only export: %q", body)
	}
}
