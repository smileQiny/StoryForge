package app

import (
	"testing"
	"time"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

func TestChaptersService_EditContentUpdatesStatusAfterManualEdit(t *testing.T) {
	cases := []struct {
		name string
		from model.ChapterStatus
		want model.ChapterStatus
	}{
		{"draft stays draft", model.ChapterStatusDraft, model.ChapterStatusDraft},
		{"audited becomes revised", model.ChapterStatusAudited, model.ChapterStatusRevised},
		{"revised stays revised", model.ChapterStatusRevised, model.ChapterStatusRevised},
		{"pending review becomes revised", model.ChapterStatusPendingReview, model.ChapterStatusRevised},
		{"approved becomes revised", model.ChapterStatusApproved, model.ChapterStatusRevised},
		{"rejected becomes revised", model.ChapterStatusRejected, model.ChapterStatusRevised},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			chapterStore := store.NewChapterStore(dir)
			bookStore := store.NewBookStore(dir)
			service := NewChaptersService(chapterStore, bookStore)

			book := &model.BookConfig{
				ID:               "manual-edit-book",
				Title:            "Manual Edit Book",
				Genre:            "other",
				Status:           model.BookStatusDraft,
				Language:         model.LanguageZH,
				ChapterWordCount: 8000,
				CreatedAt:        time.Now().UTC(),
				UpdatedAt:        time.Now().UTC(),
			}
			if err := bookStore.Create(book); err != nil {
				t.Fatalf("create book: %v", err)
			}
			chapterNum := i + 1
			if err := chapterStore.SaveMeta(book.ID, &model.ChapterMeta{
				Number:    chapterNum,
				Title:     "Chapter",
				Status:    tc.from,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}); err != nil {
				t.Fatalf("save meta: %v", err)
			}

			if err := service.EditContent(book.ID, chapterNum, "人工修订后的章节内容"); err != nil {
				t.Fatalf("edit content: %v", err)
			}

			meta, err := chapterStore.GetMeta(book.ID, chapterNum)
			if err != nil {
				t.Fatalf("get meta: %v", err)
			}
			if meta.Status != tc.want {
				t.Fatalf("status = %q, want %q", meta.Status, tc.want)
			}
			if meta.WordCount == 0 {
				t.Fatal("expected word count to be recomputed")
			}
		})
	}
}
