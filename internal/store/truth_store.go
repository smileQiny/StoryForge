package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TruthFileName enumerates the 7 canonical truth files.
type TruthFileName string

const (
	TruthCurrentState    TruthFileName = "current_state.json"
	TruthParticleLedger  TruthFileName = "particle_ledger.json"
	TruthPendingHooks    TruthFileName = "pending_hooks.json"
	TruthChapterSummaries TruthFileName = "chapter_summaries.json"
	TruthSubplotBoard    TruthFileName = "subplot_board.json"
	TruthEmotionalArcs   TruthFileName = "emotional_arcs.json"
	TruthCharacterMatrix TruthFileName = "character_matrix.json"
)

// AllTruthFiles is the ordered list of all 7 truth files.
var AllTruthFiles = []TruthFileName{
	TruthCurrentState,
	TruthParticleLedger,
	TruthPendingHooks,
	TruthChapterSummaries,
	TruthSubplotBoard,
	TruthEmotionalArcs,
	TruthCharacterMatrix,
}

// TruthStore handles reading and writing the 7 truth files for a book.
// Each file is stored as JSON (authority) with an optional Markdown projection.
type TruthStore struct {
	dataDir string
}

// NewTruthStore creates a TruthStore rooted at dataDir.
func NewTruthStore(dataDir string) *TruthStore {
	return &TruthStore{dataDir: dataDir}
}

func (ts *TruthStore) stateDir(bookID string) string {
	return filepath.Join(ts.dataDir, bookID, "story", "state")
}

// ReadRaw reads the raw JSON bytes of a truth file.
func (ts *TruthStore) ReadRaw(bookID string, name TruthFileName) ([]byte, error) {
	path := filepath.Join(ts.stateDir(bookID), string(name))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil // file not yet created is valid
	}
	return data, err
}

// Read unmarshals a truth file into v.
func (ts *TruthStore) Read(bookID string, name TruthFileName, v any) error {
	data, err := ts.ReadRaw(bookID, name)
	if err != nil {
		return err
	}
	if data == nil {
		return nil // not yet created
	}
	return json.Unmarshal(data, v)
}

// Write marshals v and writes it as the truth file, then writes a Markdown projection.
func (ts *TruthStore) Write(bookID string, name TruthFileName, v any) error {
	dir := ts.stateDir(bookID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	jsonPath := filepath.Join(dir, string(name))
	if err := writeFileAtomic(jsonPath, data); err != nil {
		return fmt.Errorf("write json: %w", err)
	}

	// Write Markdown projection alongside the JSON
	mdPath := jsonPath[:len(jsonPath)-len(".json")] + ".md"
	md := renderMarkdownProjection(name, data)
	if err := writeFileAtomic(mdPath, []byte(md)); err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}

	return nil
}

// WriteRaw writes raw JSON bytes directly (used by snapshot restore).
func (ts *TruthStore) WriteRaw(bookID string, name TruthFileName, data []byte) error {
	dir := ts.stateDir(bookID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, string(name)), data)
}

// writeFileAtomic writes data to path via a temp file + rename for atomicity.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// renderMarkdownProjection produces a human-readable Markdown view of a truth file.
func renderMarkdownProjection(name TruthFileName, jsonData []byte) string {
	header := map[TruthFileName]string{
		TruthCurrentState:     "# Current State\n\n",
		TruthParticleLedger:   "# Particle Ledger\n\n",
		TruthPendingHooks:     "# Pending Hooks\n\n",
		TruthChapterSummaries: "# Chapter Summaries\n\n",
		TruthSubplotBoard:     "# Subplot Board\n\n",
		TruthEmotionalArcs:    "# Emotional Arcs\n\n",
		TruthCharacterMatrix:  "# Character Matrix\n\n",
	}
	h := header[name]
	return h + "```json\n" + string(jsonData) + "\n```\n"
}
