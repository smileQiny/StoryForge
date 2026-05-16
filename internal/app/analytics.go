package app

import (
	"storyforge/internal/model"
	"storyforge/internal/run"
	"storyforge/internal/store"
)

// BookAnalytics is a compact analytics snapshot for one book.
type BookAnalytics struct {
	BookID             string                      `json:"bookId"`
	TotalChapters      int                         `json:"totalChapters"`
	ChapterStatusCount map[model.ChapterStatus]int `json:"chapterStatusCount"`
	TotalRuns          int                         `json:"totalRuns"`
	RunStatusCount     map[model.RunStatus]int     `json:"runStatusCount"`
}

// AnalyticsService handles analytics read use cases.
type AnalyticsService struct {
	books    *store.BookStore
	chapters *store.ChapterStore
	runs     *run.Store
}

// NewAnalyticsService creates an AnalyticsService.
func NewAnalyticsService(books *store.BookStore, chapters *store.ChapterStore, runs *run.Store) *AnalyticsService {
	return &AnalyticsService{
		books:    books,
		chapters: chapters,
		runs:     runs,
	}
}

// BookOverview aggregates chapter and run counters for a given book.
func (s *AnalyticsService) BookOverview(bookID string) (*BookAnalytics, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}

	chapterMetas, err := s.chapters.ListMeta(bookID)
	if err != nil {
		return nil, err
	}
	runs, err := s.runs.ListByBook(bookID)
	if err != nil {
		return nil, err
	}

	resp := &BookAnalytics{
		BookID:             bookID,
		ChapterStatusCount: make(map[model.ChapterStatus]int),
		RunStatusCount:     make(map[model.RunStatus]int),
	}
	for _, meta := range chapterMetas {
		resp.TotalChapters++
		resp.ChapterStatusCount[meta.Status]++
	}
	for _, r := range runs {
		resp.TotalRuns++
		resp.RunStatusCount[r.Status]++
	}
	return resp, nil
}
