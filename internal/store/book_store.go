package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"storyforge/internal/model"
)

// BookStore handles book CRUD on the filesystem.
// Each book lives at: <dataDir>/<bookID>/book.json
type BookStore struct {
	dataDir string
}

// NewBookStore creates a BookStore rooted at dataDir.
func NewBookStore(dataDir string) *BookStore {
	return &BookStore{dataDir: dataDir}
}

func (bs *BookStore) bookPath(bookID string) string {
	return filepath.Join(bs.dataDir, bookID, "book.json")
}

// Create writes a new book config. Returns error if it already exists.
func (bs *BookStore) Create(book *model.BookConfig) error {
	if err := book.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(bs.dataDir, book.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	path := bs.bookPath(book.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("book %q already exists", book.ID)
	}
	return bs.write(book)
}

// Get loads a book by ID.
func (bs *BookStore) Get(bookID string) (*model.BookConfig, error) {
	data, err := os.ReadFile(bs.bookPath(bookID))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("book %q not found", bookID)
	}
	if err != nil {
		return nil, err
	}
	var book model.BookConfig
	if err := json.Unmarshal(data, &book); err != nil {
		return nil, fmt.Errorf("unmarshal book: %w", err)
	}
	return &book, nil
}

// List returns all books in the data directory.
func (bs *BookStore) List() ([]*model.BookConfig, error) {
	entries, err := os.ReadDir(bs.dataDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var books []*model.BookConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		book, err := bs.Get(e.Name())
		if err != nil {
			continue // skip dirs without book.json
		}
		books = append(books, book)
	}
	return books, nil
}

// Update saves changes to an existing book.
func (bs *BookStore) Update(book *model.BookConfig) error {
	if err := book.Validate(); err != nil {
		return err
	}
	if _, err := bs.Get(book.ID); err != nil {
		return err
	}
	book.UpdatedAt = time.Now().UTC()
	return bs.write(book)
}

// Delete removes a book directory entirely.
func (bs *BookStore) Delete(bookID string) error {
	dir := filepath.Join(bs.dataDir, bookID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("book %q not found", bookID)
	}
	return os.RemoveAll(dir)
}

func (bs *BookStore) write(book *model.BookConfig) error {
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(bs.bookPath(book.ID), data)
}
