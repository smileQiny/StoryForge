package app

import (
	"encoding/json"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

// TruthService handles truth file use cases.
type TruthService struct {
	truth *store.TruthStore
	books *store.BookStore
}

// NewTruthService creates a TruthService.
func NewTruthService(truth *store.TruthStore, books *store.BookStore) *TruthService {
	return &TruthService{truth: truth, books: books}
}

// TruthView is the aggregated view of all 7 truth files.
type TruthView struct {
	CurrentState     json.RawMessage `json:"currentState"`
	ParticleLedger   json.RawMessage `json:"particleLedger"`
	PendingHooks     json.RawMessage `json:"pendingHooks"`
	ChapterSummaries json.RawMessage `json:"chapterSummaries"`
	SubplotBoard     json.RawMessage `json:"subplotBoard"`
	EmotionalArcs    json.RawMessage `json:"emotionalArcs"`
	CharacterMatrix  json.RawMessage `json:"characterMatrix"`
}

// GetAll returns all 7 truth files as a combined view.
func (s *TruthService) GetAll(bookID string) (*TruthView, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	view := &TruthView{}
	for _, name := range store.AllTruthFiles {
		data, err := s.truth.ReadRaw(bookID, name)
		if err != nil {
			return nil, err
		}
		if data == nil {
			data = []byte("null")
		}
		switch name {
		case store.TruthCurrentState:
			view.CurrentState = data
		case store.TruthParticleLedger:
			view.ParticleLedger = data
		case store.TruthPendingHooks:
			view.PendingHooks = data
		case store.TruthChapterSummaries:
			view.ChapterSummaries = data
		case store.TruthSubplotBoard:
			view.SubplotBoard = data
		case store.TruthEmotionalArcs:
			view.EmotionalArcs = data
		case store.TruthCharacterMatrix:
			view.CharacterMatrix = data
		}
	}
	return view, nil
}

// GetFile returns the raw JSON of a single truth file.
func (s *TruthService) GetFile(bookID string, name store.TruthFileName) (json.RawMessage, error) {
	data, err := s.truth.ReadRaw(bookID, name)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return json.RawMessage("null"), nil
	}
	return data, nil
}

// UpdateFile writes a truth file from raw JSON input.
func (s *TruthService) UpdateFile(bookID string, name store.TruthFileName, raw json.RawMessage) error {
	// Validate it's valid JSON
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return s.truth.Write(bookID, name, v)
}

// GetRuntimeState loads and assembles the full RuntimeState from the 7 truth files.
func (s *TruthService) GetRuntimeState(bookID string) (*model.RuntimeState, error) {
	state := &model.RuntimeState{}

	if err := s.truth.Read(bookID, store.TruthCurrentState, &state.CurrentState); err != nil {
		return nil, err
	}
	if err := s.truth.Read(bookID, store.TruthParticleLedger, &state.ParticleLedger); err != nil {
		return nil, err
	}
	if err := s.truth.Read(bookID, store.TruthPendingHooks, &state.PendingHooks); err != nil {
		return nil, err
	}
	if err := s.truth.Read(bookID, store.TruthChapterSummaries, &state.ChapterSummaries); err != nil {
		return nil, err
	}
	if err := s.truth.Read(bookID, store.TruthSubplotBoard, &state.SubplotBoard); err != nil {
		return nil, err
	}
	if err := s.truth.Read(bookID, store.TruthEmotionalArcs, &state.EmotionalArcs); err != nil {
		return nil, err
	}
	if err := s.truth.Read(bookID, store.TruthCharacterMatrix, &state.CharacterMatrix); err != nil {
		return nil, err
	}
	return state, nil
}
