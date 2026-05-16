package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"storyforge/internal/model"
)

// Store handles persistence of Run, RunStage, and PromptTrace records.
// Layout:
//
//	<dataDir>/<bookID>/runs/<runID>/run.json
//	<dataDir>/<bookID>/runs/<runID>/traces/<stageName>.json
type Store struct {
	dataDir string
}

// NewStore creates a run Store rooted at dataDir.
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

func (s *Store) runDir(bookID, runID string) string {
	return filepath.Join(s.dataDir, bookID, "runs", runID)
}

func (s *Store) runPath(bookID, runID string) string {
	return filepath.Join(s.runDir(bookID, runID), "run.json")
}

func (s *Store) tracePath(bookID, runID, stageName string) string {
	safe := strings.ReplaceAll(stageName, "/", "-")
	return filepath.Join(s.runDir(bookID, runID), "traces", safe+".json")
}

// Create persists a new Run record.
func (s *Store) Create(run *model.Run) error {
	dir := s.runDir(run.BookID, run.ID)
	if err := os.MkdirAll(filepath.Join(dir, "traces"), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return s.writeRun(run)
}

// Get loads a Run by bookID and runID.
func (s *Store) Get(bookID, runID string) (*model.Run, error) {
	data, err := os.ReadFile(s.runPath(bookID, runID))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("run %q not found", runID)
	}
	if err != nil {
		return nil, err
	}
	var run model.Run
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("unmarshal run: %w", err)
	}
	return &run, nil
}

// Update saves changes to an existing Run.
func (s *Store) Update(run *model.Run) error {
	return s.writeRun(run)
}

// ListByBook returns all runs for a book, sorted by StartedAt descending.
func (s *Store) ListByBook(bookID string) ([]*model.Run, error) {
	runsDir := filepath.Join(s.dataDir, bookID, "runs")
	entries, err := os.ReadDir(runsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var runs []*model.Run
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		run, err := s.Get(bookID, e.Name())
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return runs, nil
}

// DeleteFromChapter removes all runs and traces with chapter >= fromChapter for a book.
func (s *Store) DeleteFromChapter(bookID string, fromChapter int) error {
	runs, err := s.ListByBook(bookID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.Chapter < fromChapter {
			continue
		}
		if err := os.RemoveAll(s.runDir(bookID, run.ID)); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(s.dataDir, "_traces", run.ID)); err != nil {
			return err
		}
	}
	return nil
}

// SaveTrace persists a PromptTrace for a stage.
func (s *Store) SaveTrace(trace *model.PromptTrace) error {
	// Determine bookID from run record
	path := s.tracePath("", trace.RunID, trace.StageName)
	// We need bookID — store traces under a global traces dir keyed by runID
	// Use a flat layout: <dataDir>/_traces/<runID>/<stageName>.json
	path = filepath.Join(s.dataDir, "_traces", trace.RunID, strings.ReplaceAll(trace.StageName, "/", "-")+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// ListTraces returns all PromptTrace records for a run.
func (s *Store) ListTraces(runID string) ([]*model.PromptTrace, error) {
	dir := filepath.Join(s.dataDir, "_traces", runID)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var traces []*model.PromptTrace
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var t model.PromptTrace
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		traces = append(traces, &t)
	}
	return traces, nil
}

// UpdateStageStatus updates a stage's status within a run.
func (s *Store) UpdateStageStatus(bookID, runID, stageName string, status model.StageStatus, usage *model.TokenUsage, errMsg string) error {
	run, err := s.Get(bookID, runID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for i, st := range run.Stages {
		if st.Name == stageName {
			run.Stages[i].Status = status
			run.Stages[i].EndedAt = &now
			if usage != nil {
				run.Stages[i].Usage = usage
			}
			if errMsg != "" {
				run.Stages[i].Error = errMsg
			}
			return s.writeRun(run)
		}
	}
	return fmt.Errorf("stage %q not found in run %q", stageName, runID)
}

func (s *Store) writeRun(run *model.Run) error {
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.runPath(run.BookID, run.ID), data)
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
