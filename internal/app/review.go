package app

import (
	"context"
	"fmt"
	"path/filepath"

	"storyforge/internal/model"
	"storyforge/internal/run"
	"storyforge/internal/state"
	"storyforge/internal/store"
)

// ReviewService handles chapter review operations.
type ReviewService struct {
	dataDir   string
	books     *store.BookStore
	chapters  *store.ChapterStore
	truth     *store.TruthStore
	runtime   *store.RuntimeStore
	snapshots *store.SnapshotStore
	runs      *run.Store
	memory    *state.MemoryDB
	fileLock  *store.FileLock
}

// RejectResult describes the rollback result for a reject operation.
type RejectResult struct {
	RollbackToChapter int    `json:"rollbackToChapter"`
	DeletedFrom       int    `json:"deletedFrom"`
	Reason            string `json:"reason,omitempty"`
}

// NewReviewService creates a ReviewService.
func NewReviewService(
	dataDir string,
	books *store.BookStore,
	chapters *store.ChapterStore,
	truth *store.TruthStore,
	runtime *store.RuntimeStore,
	snapshots *store.SnapshotStore,
	runs *run.Store,
	memory *state.MemoryDB,
	fileLock *store.FileLock,
) *ReviewService {
	return &ReviewService{
		dataDir:   dataDir,
		books:     books,
		chapters:  chapters,
		truth:     truth,
		runtime:   runtime,
		snapshots: snapshots,
		runs:      runs,
		memory:    memory,
		fileLock:  fileLock,
	}
}

// Approve marks a chapter as approved.
func (s *ReviewService) Approve(bookID string, chapter int) error {
	if _, err := s.books.Get(bookID); err != nil {
		return err
	}
	if _, err := s.chapters.GetMeta(bookID, chapter); err != nil {
		return err
	}
	return s.chapters.UpdateStatus(bookID, chapter, model.ChapterStatusApproved)
}

// Reject rejects a chapter and rolls back state to chapter-1 snapshot.
// It clears downstream chapter artifacts (chapters/runtime/snapshots/runs) from the rejected chapter onward.
func (s *ReviewService) Reject(ctx context.Context, bookID string, chapter int, reason string) (*RejectResult, error) {
	if chapter <= 0 {
		return nil, fmt.Errorf("chapter must be positive")
	}
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	if _, err := s.chapters.GetMeta(bookID, chapter); err != nil {
		return nil, err
	}

	bookDir := filepath.Join(s.dataDir, bookID)
	unlock, err := s.fileLock.Lock(bookDir)
	if err != nil {
		return nil, err
	}
	defer unlock()

	rollbackTo := chapter - 1
	restored := model.RuntimeState{}
	if rollbackTo > 0 {
		snap, err := s.snapshots.Load(bookID, rollbackTo)
		if err != nil {
			return nil, fmt.Errorf("load snapshot for rollback: %w", err)
		}
		if snap.State != nil {
			restored = *snap.State
		}
	}

	if err := state.ValidateRuntimeState(restored); err != nil {
		return nil, fmt.Errorf("restored state invalid: %w", err)
	}
	if err := s.saveRuntimeState(bookID, restored); err != nil {
		return nil, fmt.Errorf("restore truth state: %w", err)
	}
	if err := s.chapters.DeleteFrom(bookID, chapter); err != nil {
		return nil, fmt.Errorf("delete chapters: %w", err)
	}
	if err := s.runtime.DeleteFrom(bookID, chapter); err != nil {
		return nil, fmt.Errorf("delete runtime artifacts: %w", err)
	}
	if err := s.snapshots.DeleteFrom(bookID, chapter); err != nil {
		return nil, fmt.Errorf("delete snapshots: %w", err)
	}
	if err := s.runs.DeleteFromChapter(bookID, chapter); err != nil {
		return nil, fmt.Errorf("delete runs: %w", err)
	}
	if s.memory != nil {
		if err := s.memory.DeleteFrom(ctx, bookID, chapter); err != nil {
			return nil, fmt.Errorf("delete memory: %w", err)
		}
	}

	return &RejectResult{
		RollbackToChapter: rollbackTo,
		DeletedFrom:       chapter,
		Reason:            reason,
	}, nil
}

func (s *ReviewService) saveRuntimeState(bookID string, st model.RuntimeState) error {
	writes := []struct {
		name store.TruthFileName
		val  any
	}{
		{store.TruthCurrentState, st.CurrentState},
		{store.TruthParticleLedger, st.ParticleLedger},
		{store.TruthPendingHooks, st.PendingHooks},
		{store.TruthChapterSummaries, st.ChapterSummaries},
		{store.TruthSubplotBoard, st.SubplotBoard},
		{store.TruthEmotionalArcs, st.EmotionalArcs},
		{store.TruthCharacterMatrix, st.CharacterMatrix},
	}
	for _, w := range writes {
		if err := s.truth.Write(bookID, w.name, w.val); err != nil {
			return err
		}
	}
	return nil
}
