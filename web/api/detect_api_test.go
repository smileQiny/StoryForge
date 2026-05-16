package api_test

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/model"
	"storyforge/internal/store"
	"storyforge/web/api"
)

type detectHarness struct {
	handler http.Handler
}

func newDetectHarness(t *testing.T) *detectHarness {
	t.Helper()

	dir, err := os.MkdirTemp("", "storyforge-detect-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	books := store.NewBookStore(dir)
	chapters := store.NewChapterStore(dir)
	detectSvc := app.NewDetectService(books, chapters)
	detectH := api.NewDetectHandler(detectSvc)

	book := &model.BookConfig{
		ID:               "detect-book",
		Title:            "Detect Book",
		Genre:            "xuanhuan",
		Status:           model.BookStatusActive,
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	if err := books.Create(book); err != nil {
		t.Fatal(err)
	}

	content := "突然，他突然说道，然后继续解释。就在这个时候，所有人都沉默了。\n\n突然，他突然说道，然后继续解释。就在这个时候，所有人都沉默了。\n\n突然，他突然说道，然后继续解释。就在这个时候，所有人都沉默了。"
	meta := &model.ChapterMeta{
		Number:    1,
		Title:     "Chapter 1",
		Status:    model.ChapterStatusApproved,
		WordCount: len(content),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := chapters.SaveMeta(book.ID, meta); err != nil {
		t.Fatal(err)
	}
	if err := chapters.SaveContent(book.ID, 1, content); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	router.Get("/api/books/{bookID}/detect", detectH.Analyze)
	router.Post("/api/books/{bookID}/detect", detectH.Analyze)

	return &detectHarness{handler: router}
}

func TestDetect_GetQuery(t *testing.T) {
	h := newDetectHarness(t)

	w := do(t, h.handler, http.MethodGet, "/api/books/detect-book/detect?chapter=1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detect get: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		BookID                   string  `json:"bookId"`
		Chapter                  int     `json:"chapter"`
		FatigueWordHits          int     `json:"fatigueWordHits"`
		FatigueWordHitRate       float64 `json:"fatigueWordHitRate"`
		ParagraphEqualLengthRate float64 `json:"paragraphEqualLengthRate"`
		ClicheHits               int     `json:"clicheHits"`
		ClicheDensity            float64 `json:"clicheDensity"`
		RiskLevel                string  `json:"riskLevel"`
	}
	decodeJSON(t, w, &got)

	if got.BookID != "detect-book" || got.Chapter != 1 {
		t.Fatalf("unexpected book/chapter: %+v", got)
	}
	if got.FatigueWordHits == 0 || got.ClicheHits == 0 {
		t.Fatalf("expected hits to be detected: %+v", got)
	}
	if got.ParagraphEqualLengthRate < 0.99 {
		t.Fatalf("expected near-uniform paragraphs, got %.2f", got.ParagraphEqualLengthRate)
	}
	if got.RiskLevel != "high" {
		t.Fatalf("expected high risk, got %q", got.RiskLevel)
	}
}

func TestDetect_PostBody(t *testing.T) {
	h := newDetectHarness(t)

	w := do(t, h.handler, http.MethodPost, "/api/books/detect-book/detect", map[string]any{"chapter": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("detect post: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	decodeJSON(t, w, &got)
	if got["riskLevel"] != "high" {
		t.Fatalf("expected high risk, got %v", got["riskLevel"])
	}
}
