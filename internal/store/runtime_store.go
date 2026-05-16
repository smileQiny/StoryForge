package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"storyforge/internal/model"
)

// RuntimeStore handles reading and writing per-chapter runtime artifacts.
// Layout: <dataDir>/<bookID>/runtime/chapter-<NNNN>.<kind>.<ext>
type RuntimeStore struct {
	dataDir string
}

// NewRuntimeStore creates a RuntimeStore rooted at dataDir.
func NewRuntimeStore(dataDir string) *RuntimeStore {
	return &RuntimeStore{dataDir: dataDir}
}

func (rs *RuntimeStore) runtimeDir(bookID string) string {
	return filepath.Join(rs.dataDir, bookID, "story", "runtime")
}

func (rs *RuntimeStore) chapterPrefix(bookID string, chapter int) string {
	return filepath.Join(rs.runtimeDir(bookID), fmt.Sprintf("chapter-%04d", chapter))
}

// SaveIntent writes the ChapterIntent for a chapter.
func (rs *RuntimeStore) SaveIntent(bookID string, intent *model.ChapterIntent) error {
	return rs.writeJSON(rs.chapterPrefix(bookID, intent.Chapter)+".intent.json", intent)
}

// GetIntent reads the ChapterIntent for a chapter.
func (rs *RuntimeStore) GetIntent(bookID string, chapter int) (*model.ChapterIntent, error) {
	var v model.ChapterIntent
	if err := rs.readJSON(rs.chapterPrefix(bookID, chapter)+".intent.json", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// SaveContext writes the ContextPackage for a chapter.
func (rs *RuntimeStore) SaveContext(bookID string, ctx *model.ContextPackage) error {
	return rs.writeJSON(rs.chapterPrefix(bookID, ctx.Chapter)+".context.json", ctx)
}

// GetContext reads the ContextPackage for a chapter.
func (rs *RuntimeStore) GetContext(bookID string, chapter int) (*model.ContextPackage, error) {
	var v model.ContextPackage
	if err := rs.readJSON(rs.chapterPrefix(bookID, chapter)+".context.json", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// SaveRuleStack writes the RuleStack for a chapter.
func (rs *RuntimeStore) SaveRuleStack(bookID string, chapter int, rs2 *model.RuleStack) error {
	return rs.writeJSON(rs.chapterPrefix(bookID, chapter)+".rule-stack.json", rs2)
}

// GetRuleStack reads the RuleStack for a chapter.
func (rs *RuntimeStore) GetRuleStack(bookID string, chapter int) (*model.RuleStack, error) {
	var v model.RuleStack
	if err := rs.readJSON(rs.chapterPrefix(bookID, chapter)+".rule-stack.json", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// SaveTrace writes the ChapterTrace for a chapter.
func (rs *RuntimeStore) SaveTrace(bookID string, trace *model.ChapterTrace) error {
	return rs.writeJSON(rs.chapterPrefix(bookID, trace.Chapter)+".trace.json", trace)
}

// GetTrace reads the ChapterTrace for a chapter.
func (rs *RuntimeStore) GetTrace(bookID string, chapter int) (*model.ChapterTrace, error) {
	var v model.ChapterTrace
	if err := rs.readJSON(rs.chapterPrefix(bookID, chapter)+".trace.json", &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (rs *RuntimeStore) writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func (rs *RuntimeStore) readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("runtime artifact not found: %s", filepath.Base(path))
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// DeleteFrom removes all runtime artifacts with chapter >= fromChapter.
func (rs *RuntimeStore) DeleteFrom(bookID string, fromChapter int) error {
	dir := rs.runtimeDir(bookID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "chapter-") || len(name) < len("chapter-0000.") {
			continue
		}
		chapterStr := name[len("chapter-") : len("chapter-")+4]
		chapter, err := strconv.Atoi(chapterStr)
		if err != nil {
			continue
		}
		if chapter >= fromChapter {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}
