package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"storyforge/internal/model"
)

// ChapterStore handles chapter content and metadata on the filesystem.
// Layout: <dataDir>/<bookID>/chapters/<NNNN>/meta.json + content.md
type ChapterStore struct {
	dataDir string
}

// NewChapterStore creates a ChapterStore rooted at dataDir.
func NewChapterStore(dataDir string) *ChapterStore {
	return &ChapterStore{dataDir: dataDir}
}

func (cs *ChapterStore) chapterDir(bookID string, number int) string {
	return filepath.Join(cs.dataDir, bookID, "chapters", fmt.Sprintf("%04d", number))
}

// SaveMeta writes chapter metadata.
func (cs *ChapterStore) SaveMeta(bookID string, meta *model.ChapterMeta) error {
	if err := meta.Validate(); err != nil {
		return err
	}
	dir := cs.chapterDir(bookID, meta.Number)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "meta.json"), data)
}

// GetMeta loads chapter metadata.
func (cs *ChapterStore) GetMeta(bookID string, number int) (*model.ChapterMeta, error) {
	path := filepath.Join(cs.chapterDir(bookID, number), "meta.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("chapter %d not found", number)
	}
	if err != nil {
		return nil, err
	}
	var meta model.ChapterMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal meta: %w", err)
	}
	return &meta, nil
}

// ListMeta returns metadata for all chapters of a book, ordered by number.
func (cs *ChapterStore) ListMeta(bookID string) ([]*model.ChapterMeta, error) {
	chaptersDir := filepath.Join(cs.dataDir, bookID, "chapters")
	entries, err := os.ReadDir(chaptersDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var metas []*model.ChapterMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var num int
		if _, err := fmt.Sscanf(e.Name(), "%d", &num); err != nil {
			continue
		}
		meta, err := cs.GetMeta(bookID, num)
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

// SaveContent writes the chapter body text.
func (cs *ChapterStore) SaveContent(bookID string, number int, content string) error {
	dir := cs.chapterDir(bookID, number)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, "content.md"), []byte(content))
}

// GetContent reads the chapter body text.
func (cs *ChapterStore) GetContent(bookID string, number int) (string, error) {
	path := filepath.Join(cs.chapterDir(bookID, number), "content.md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("chapter %d content not found", number)
	}
	return string(data), err
}

// UpdateStatus updates only the chapter status field.
func (cs *ChapterStore) UpdateStatus(bookID string, number int, status model.ChapterStatus) error {
	meta, err := cs.GetMeta(bookID, number)
	if err != nil {
		return err
	}
	meta.Status = status
	meta.UpdatedAt = time.Now().UTC()
	return cs.SaveMeta(bookID, meta)
}

// DeleteFrom removes all chapters with number >= fromNumber.
func (cs *ChapterStore) DeleteFrom(bookID string, fromNumber int) error {
	metas, err := cs.ListMeta(bookID)
	if err != nil {
		return err
	}
	for _, m := range metas {
		if m.Number >= fromNumber {
			dir := cs.chapterDir(bookID, m.Number)
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
		}
	}
	return nil
}
