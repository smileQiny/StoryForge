package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"storyforge/internal/model"
)

// SnapshotStore handles saving, loading, and deleting chapter snapshots.
// Layout: <dataDir>/<bookID>/snapshots/chapter-<NNNN>.json
type SnapshotStore struct {
	dataDir string
}

// NewSnapshotStore creates a SnapshotStore rooted at dataDir.
func NewSnapshotStore(dataDir string) *SnapshotStore {
	return &SnapshotStore{dataDir: dataDir}
}

func (ss *SnapshotStore) snapshotDir(bookID string) string {
	return filepath.Join(ss.dataDir, bookID, "snapshots")
}

func (ss *SnapshotStore) snapshotPath(bookID string, chapter int) string {
	return filepath.Join(ss.snapshotDir(bookID), fmt.Sprintf("chapter-%04d.json", chapter))
}

// Save writes a snapshot for the given chapter.
func (ss *SnapshotStore) Save(bookID string, snap *model.ChapterSnapshot) error {
	dir := ss.snapshotDir(bookID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(ss.snapshotPath(bookID, snap.Chapter), data)
}

// Load reads the snapshot for a chapter.
func (ss *SnapshotStore) Load(bookID string, chapter int) (*model.ChapterSnapshot, error) {
	data, err := os.ReadFile(ss.snapshotPath(bookID, chapter))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("snapshot for chapter %d not found", chapter)
	}
	if err != nil {
		return nil, err
	}
	var snap model.ChapterSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return &snap, nil
}

// Delete removes the snapshot for a chapter.
func (ss *SnapshotStore) Delete(bookID string, chapter int) error {
	path := ss.snapshotPath(bookID, chapter)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DeleteFrom removes all snapshots with chapter >= fromChapter.
func (ss *SnapshotStore) DeleteFrom(bookID string, fromChapter int) error {
	chapters, err := ss.ListChapters(bookID)
	if err != nil {
		return err
	}
	for _, ch := range chapters {
		if ch >= fromChapter {
			if err := ss.Delete(bookID, ch); err != nil {
				return err
			}
		}
	}
	return nil
}

// ListChapters returns the chapter numbers for which snapshots exist, sorted ascending.
func (ss *SnapshotStore) ListChapters(bookID string) ([]int, error) {
	entries, err := os.ReadDir(ss.snapshotDir(bookID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var chapters []int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "chapter-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		numStr := strings.TrimSuffix(strings.TrimPrefix(name, "chapter-"), ".json")
		n, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		chapters = append(chapters, n)
	}
	sort.Ints(chapters)
	return chapters, nil
}
