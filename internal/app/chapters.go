package app

import (
	"fmt"
	"time"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

// ChaptersService handles chapter use cases.
type ChaptersService struct {
	chapters *store.ChapterStore
	books    *store.BookStore
}

// NewChaptersService creates a ChaptersService.
func NewChaptersService(chapters *store.ChapterStore, books *store.BookStore) *ChaptersService {
	return &ChaptersService{chapters: chapters, books: books}
}

// List returns all chapter metadata for a book.
func (s *ChaptersService) List(bookID string) ([]*model.ChapterMeta, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	return s.chapters.ListMeta(bookID)
}

// GetContent returns the content of a chapter.
func (s *ChaptersService) GetContent(bookID string, number int) (string, error) {
	return s.chapters.GetContent(bookID, number)
}

// GetMeta returns the metadata of a chapter.
func (s *ChaptersService) GetMeta(bookID string, number int) (*model.ChapterMeta, error) {
	return s.chapters.GetMeta(bookID, number)
}

// EditContent allows manual editing of a chapter's content.
func (s *ChaptersService) EditContent(bookID string, number int, content string) error {
	meta, err := s.chapters.GetMeta(bookID, number)
	if err != nil {
		return fmt.Errorf("chapter %d not found: %w", number, err)
	}
	if err := s.chapters.SaveContent(bookID, number, content); err != nil {
		return err
	}
	// Recount words (rough estimate)
	meta.WordCount = countWords(content)
	meta.Status = statusAfterManualEdit(meta.Status)
	meta.UpdatedAt = time.Now().UTC()
	return s.chapters.SaveMeta(bookID, meta)
}

func statusAfterManualEdit(status model.ChapterStatus) model.ChapterStatus {
	switch status {
	case model.ChapterStatusDraft:
		return model.ChapterStatusDraft
	default:
		return model.ChapterStatusRevised
	}
}

// countWords is a simple whitespace-based word counter.
func countWords(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
	}
	return count
}
