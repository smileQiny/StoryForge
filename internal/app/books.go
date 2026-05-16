package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

// BooksService handles book lifecycle use cases.
type BooksService struct {
	books    *store.BookStore
	truth    *store.TruthStore
	config   *ConfigService
	fileLock *store.FileLock
	dataDir  string
	logger   *slog.Logger
}

// NewBooksService creates a BooksService.
func NewBooksService(dataDir string, books *store.BookStore, truth *store.TruthStore, config *ConfigService, fileLock *store.FileLock, logger *slog.Logger) *BooksService {
	if logger == nil {
		logger = slog.Default()
	}
	return &BooksService{
		books:    books,
		truth:    truth,
		config:   config,
		fileLock: fileLock,
		dataDir:  dataDir,
		logger:   logger,
	}
}

// CreateBookInput is the input for creating a new book.
type CreateBookInput struct {
	ID               string
	Title            string
	Genre            string
	Brief            string
	Language         model.Language
	Platform         string
	TargetChapters   int
	ChapterWordCount int
	FanficMode       model.FanficMode
	ParentBookID     string
}

// Create creates a new book.
func (s *BooksService) Create(input CreateBookInput) (*model.BookConfig, error) {
	if input.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	now := time.Now().UTC()
	book := &model.BookConfig{
		ID:               input.ID,
		Title:            input.Title,
		Genre:            input.Genre,
		Language:         input.Language,
		Platform:         input.Platform,
		Status:           model.BookStatusDraft,
		TargetChapters:   input.TargetChapters,
		ChapterWordCount: input.ChapterWordCount,
		FanficMode:       input.FanficMode,
		ParentBookID:     input.ParentBookID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	bookDir := filepath.Join(s.dataDir, book.ID)
	unlock, err := s.fileLock.Lock(bookDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()

	if err := s.books.Create(book); err != nil {
		return nil, err
	}
	if err := s.bootstrapNewBook(book, input.Brief); err != nil {
		unlock()
		unlock = nil
		if deleteErr := s.books.Delete(book.ID); deleteErr != nil && !os.IsNotExist(deleteErr) {
			return nil, fmt.Errorf("%w (cleanup failed: %v)", err, deleteErr)
		}
		return nil, err
	}
	return book, nil
}

// List returns all books.
func (s *BooksService) List() ([]*model.BookConfig, error) {
	return s.books.List()
}

// Get returns a book by ID.
func (s *BooksService) Get(bookID string) (*model.BookConfig, error) {
	return s.books.Get(bookID)
}

// UpdateBookInput is the input for updating a book.
type UpdateBookInput struct {
	Title            *string
	Status           *model.BookStatus
	Language         *model.Language
	TargetChapters   *int
	ChapterWordCount *int
	Platform         *string
}

// Update applies partial updates to a book.
func (s *BooksService) Update(bookID string, input UpdateBookInput) (*model.BookConfig, error) {
	book, err := s.books.Get(bookID)
	if err != nil {
		return nil, err
	}
	if input.Title != nil {
		book.Title = *input.Title
	}
	if input.Status != nil {
		book.Status = *input.Status
	}
	if input.Language != nil {
		book.Language = *input.Language
	}
	if input.TargetChapters != nil {
		book.TargetChapters = *input.TargetChapters
	}
	if input.ChapterWordCount != nil {
		book.ChapterWordCount = *input.ChapterWordCount
	}
	if input.Platform != nil {
		book.Platform = *input.Platform
	}
	if err := s.books.Update(book); err != nil {
		return nil, err
	}
	return book, nil
}

// Delete removes a book and all its data.
func (s *BooksService) Delete(bookID string) error {
	return s.books.Delete(bookID)
}
