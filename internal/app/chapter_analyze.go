package app

import (
	"context"
	"fmt"

	"storyforge/internal/agent"
	"storyforge/internal/model"
	"storyforge/internal/state"
)

// ChapterAnalyzeResult is the parity-friendly response payload for chapter analysis.
type ChapterAnalyzeResult struct {
	BookID          string                  `json:"bookId"`
	Chapter         int                     `json:"chapter"`
	ChapterTitle    string                  `json:"chapterTitle,omitempty"`
	Content         string                  `json:"content,omitempty"`
	Facts           []model.ObservedFact    `json:"facts"`
	Delta           model.RuntimeStateDelta `json:"delta"`
	CurrentState    model.RuntimeState      `json:"currentState"`
	NextState       model.RuntimeState      `json:"nextState"`
	PreviousSummary string                  `json:"previousSummary,omitempty"`
}

// ChapterAnalyzeService provides a read-only observer + reflector compatibility flow.
type ChapterAnalyzeService struct {
	pipeline *PipelineService
}

// NewChapterAnalyzeService creates a ChapterAnalyzeService.
func NewChapterAnalyzeService(pipeline *PipelineService) *ChapterAnalyzeService {
	return &ChapterAnalyzeService{pipeline: pipeline}
}

// Analyze executes observer + reflector for an existing chapter without mutating stored truth state.
func (s *ChapterAnalyzeService) Analyze(ctx context.Context, bookID string, chapter int) (*ChapterAnalyzeResult, error) {
	if s == nil || s.pipeline == nil {
		return nil, fmt.Errorf("chapter analyze service is not configured")
	}
	if chapter <= 0 {
		return nil, fmt.Errorf("chapter must be positive")
	}

	exec, err := s.pipeline.prepareExecution(bookID)
	if err != nil {
		return nil, err
	}
	meta, err := s.pipeline.chapters.GetMeta(bookID, chapter)
	if err != nil {
		return nil, err
	}
	content, err := s.pipeline.chapters.GetContent(bookID, chapter)
	if err != nil {
		return nil, err
	}
	currentState, err := s.pipeline.loadRuntimeState(bookID)
	if err != nil {
		return nil, err
	}

	observed, err := exec.agents.Observer.Observe(ctx, agent.ObserverInput{
		Book:        *exec.book,
		Chapter:     chapter,
		ChapterText: content,
		State:       currentState,
	})
	if err != nil {
		return nil, err
	}

	textCtx := reflectTextContext(currentState)
	reflected, err := exec.agents.Reflector.Reflect(ctx, agent.ReflectorInput{
		Book:                 *exec.book,
		Chapter:              chapter,
		ChapterText:          content,
		Facts:                observed.Facts,
		State:                currentState,
		CurrentStateText:     textCtx.CurrentStateText,
		HooksText:            textCtx.HooksText,
		ChapterSummariesText: textCtx.ChapterSummariesText,
		SubplotBoardText:     textCtx.SubplotBoardText,
		EmotionalArcsText:    textCtx.EmotionalArcsText,
		CharacterMatrixText:  textCtx.CharacterMatrixText,
		PreviousSummary:      previousSummary(currentState, chapter),
	})
	if err != nil {
		return nil, err
	}

	nextState, err := state.ApplyRuntimeStateDelta(currentState, reflected.Delta)
	if err != nil {
		return nil, err
	}
	if err := state.ValidateRuntimeState(nextState); err != nil {
		return nil, err
	}

	return &ChapterAnalyzeResult{
		BookID:          bookID,
		Chapter:         chapter,
		ChapterTitle:    meta.Title,
		Content:         content,
		Facts:           observed.Facts,
		Delta:           reflected.Delta,
		CurrentState:    currentState,
		NextState:       nextState,
		PreviousSummary: previousSummary(currentState, chapter),
	}, nil
}
