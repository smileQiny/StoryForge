package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

// ImportService handles import use cases for chapters, style, and canon.
type ImportService struct {
	dataDir  string
	books    *store.BookStore
	chapters *store.ChapterStore
	truth    *store.TruthStore
}

// NewImportService creates an ImportService.
func NewImportService(dataDir string, books *store.BookStore, chapters *store.ChapterStore, truth *store.TruthStore) *ImportService {
	return &ImportService{dataDir: dataDir, books: books, chapters: chapters, truth: truth}
}

// ImportedChapterInput describes a chapter imported from existing material.
type ImportedChapterInput struct {
	Number  int                 `json:"number"`
	Title   string              `json:"title"`
	Content string              `json:"content"`
	Summary string              `json:"summary,omitempty"`
	Status  model.ChapterStatus `json:"status,omitempty"`
}

// ImportChaptersInput is the payload for chapter import.
type ImportChaptersInput struct {
	Chapters []ImportedChapterInput `json:"chapters"`
}

// ImportChaptersResult summarizes the imported chapters.
type ImportChaptersResult struct {
	BookID                  string   `json:"bookId"`
	ImportedCount           int      `json:"importedCount"`
	TotalWords              int      `json:"totalWords"`
	Imported                []int    `json:"imported"`
	ReconstructedTruthFiles []string `json:"reconstructedTruthFiles"`
	NextChapter             int      `json:"nextChapter"`
}

// StyleImportInput stores a style fingerprint and guidance.
type StyleImportInput struct {
	Source      string         `json:"source"`
	Summary     string         `json:"summary,omitempty"`
	Fingerprint map[string]any `json:"fingerprint,omitempty"`
	Guidance    []string       `json:"guidance,omitempty"`
	Notes       string         `json:"notes,omitempty"`
}

// CanonImportInput stores canonical reference data for fanfic mode.
type CanonImportInput struct {
	Source     string   `json:"source"`
	BookID     string   `json:"bookId,omitempty"`
	Title      string   `json:"title,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Characters []string `json:"characters,omitempty"`
	Rules      []string `json:"rules,omitempty"`
	Notes      string   `json:"notes,omitempty"`
}

// FanficInitInput initializes a fanfic project with canon metadata and originality guardrails.
type FanficInitInput struct {
	Mode                model.FanficMode `json:"mode"`
	ParentBookID        string           `json:"parentBookId,omitempty"`
	SourceTitle         string           `json:"sourceTitle,omitempty"`
	SourceSummary       string           `json:"sourceSummary,omitempty"`
	Characters          []string         `json:"characters,omitempty"`
	Rules               []string         `json:"rules,omitempty"`
	DivergencePoint     string           `json:"divergencePoint"`
	OriginalPremise     string           `json:"originalPremise,omitempty"`
	ForbiddenCanonBeats []string         `json:"forbiddenCanonBeats,omitempty"`
	Notes               string           `json:"notes,omitempty"`
}

// FanficInitResult describes the initialized fanfic configuration.
type FanficInitResult struct {
	BookID              string           `json:"bookId"`
	Mode                model.FanficMode `json:"mode"`
	DivergencePoint     string           `json:"divergencePoint"`
	OriginalPremise     string           `json:"originalPremise,omitempty"`
	Guardrails          []string         `json:"guardrails"`
	ForbiddenCanonBeats []string         `json:"forbiddenCanonBeats,omitempty"`
}

// FanficState exposes the stored fanfic configuration for a book.
type FanficState struct {
	BookID       string           `json:"bookId"`
	Mode         model.FanficMode `json:"mode,omitempty"`
	ParentBookID string           `json:"parentBookId,omitempty"`
	Content      *string          `json:"content,omitempty"`
	Canon        map[string]any   `json:"canon,omitempty"`
	Profile      map[string]any   `json:"profile,omitempty"`
}

var (
	latinNamePattern = regexp.MustCompile(`\b[A-Z][a-z]{2,}\b`)
	cjkTokenPattern  = regexp.MustCompile(`[\p{Han}]{2,4}`)
)

// ImportChapterSummaries writes chapter content/meta and reconstructs the 7 truth files.
func (s *ImportService) ImportChapterSummaries(bookID string, input ImportChaptersInput) (*ImportChaptersResult, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	if len(input.Chapters) == 0 {
		return nil, fmt.Errorf("chapters is required")
	}

	imported := make([]int, 0, len(input.Chapters))
	summaries, err := s.loadChapterSummaries(bookID)
	if err != nil {
		return nil, err
	}
	summaryIndex := make(map[int]int, len(summaries))
	for i, row := range summaries {
		summaryIndex[row.Chapter] = i
	}

	now := time.Now().UTC()
	totalWords := 0
	for _, chapter := range input.Chapters {
		if chapter.Number <= 0 {
			return nil, fmt.Errorf("chapter number must be positive")
		}
		wordCount := countImportedWords(chapter.Content)
		meta := &model.ChapterMeta{
			Number:    chapter.Number,
			Title:     chapter.Title,
			Status:    normalizeImportedChapterStatus(chapter.Status),
			WordCount: wordCount,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.chapters.SaveMeta(bookID, meta); err != nil {
			return nil, err
		}
		if err := s.chapters.SaveContent(bookID, chapter.Number, chapter.Content); err != nil {
			return nil, err
		}

		row := model.ChapterSummaryRow{
			Chapter: chapter.Number,
			Title:   chapter.Title,
			Summary: chapter.Summary,
		}
		if row.Summary == "" {
			row.Summary = summarizeText(chapter.Content)
		}
		if idx, ok := summaryIndex[chapter.Number]; ok {
			summaries[idx] = row
		} else {
			summaries = append(summaries, row)
			summaryIndex[chapter.Number] = len(summaries) - 1
		}
		imported = append(imported, chapter.Number)
		totalWords += wordCount
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Chapter < summaries[j].Chapter
	})

	state := reverseEngineerRuntimeState(input.Chapters, summaries)
	if err := s.saveRuntimeState(bookID, state); err != nil {
		return nil, err
	}

	sort.Ints(imported)
	nextChapter := imported[len(imported)-1] + 1
	return &ImportChaptersResult{
		BookID:        bookID,
		ImportedCount: len(imported),
		TotalWords:    totalWords,
		Imported:      imported,
		ReconstructedTruthFiles: []string{
			string(store.TruthCurrentState),
			string(store.TruthParticleLedger),
			string(store.TruthPendingHooks),
			string(store.TruthChapterSummaries),
			string(store.TruthSubplotBoard),
			string(store.TruthEmotionalArcs),
			string(store.TruthCharacterMatrix),
		},
		NextChapter: nextChapter,
	}, nil
}

// ImportStyle writes a style fingerprint into current_state.json.
func (s *ImportService) ImportStyle(bookID string, input StyleImportInput) (map[string]any, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	state, err := s.loadCurrentState(bookID)
	if err != nil {
		return nil, err
	}
	state["styleProfile"] = map[string]any{
		"source":      input.Source,
		"summary":     input.Summary,
		"fingerprint": input.Fingerprint,
		"guidance":    input.Guidance,
		"notes":       input.Notes,
		"importedAt":  time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.writeCurrentState(bookID, state); err != nil {
		return nil, err
	}
	return state, nil
}

// ImportCanon writes canon data into current_state.json.
func (s *ImportService) ImportCanon(bookID string, input CanonImportInput) (map[string]any, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	state, err := s.loadCurrentState(bookID)
	if err != nil {
		return nil, err
	}
	state["fanficCanon"] = map[string]any{
		"source":     input.Source,
		"bookId":     input.BookID,
		"title":      input.Title,
		"summary":    input.Summary,
		"characters": input.Characters,
		"rules":      input.Rules,
		"notes":      input.Notes,
		"importedAt": time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.writeCurrentState(bookID, state); err != nil {
		return nil, err
	}
	return state, nil
}

// InitFanfic enables fanfic mode and persists the required originality divergence point.
func (s *ImportService) InitFanfic(bookID string, input FanficInitInput) (*FanficInitResult, error) {
	if input.Mode == model.FanficModeNone || !model.ValidFanficModes[input.Mode] {
		return nil, fmt.Errorf("mode must be one of inspired/alternate/continuation/reverse")
	}
	if strings.TrimSpace(input.DivergencePoint) == "" {
		return nil, fmt.Errorf("divergencePoint is required for fanfic mode")
	}

	book, err := s.books.Get(bookID)
	if err != nil {
		return nil, err
	}
	book.FanficMode = input.Mode
	if input.ParentBookID != "" {
		book.ParentBookID = input.ParentBookID
	}
	if err := s.books.Update(book); err != nil {
		return nil, err
	}

	state, err := s.loadCurrentState(bookID)
	if err != nil {
		return nil, err
	}
	guardrails := buildFanficGuardrails(input.Mode, input.DivergencePoint, input.ForbiddenCanonBeats)
	state["fanficCanon"] = map[string]any{
		"source":              "fanfic-init",
		"bookId":              input.ParentBookID,
		"title":               input.SourceTitle,
		"summary":             input.SourceSummary,
		"characters":          input.Characters,
		"rules":               input.Rules,
		"notes":               input.Notes,
		"divergencePoint":     input.DivergencePoint,
		"originalPremise":     input.OriginalPremise,
		"forbiddenCanonBeats": input.ForbiddenCanonBeats,
		"guardrails":          guardrails,
		"initializedAt":       time.Now().UTC().Format(time.RFC3339),
	}
	state["fanficProfile"] = map[string]any{
		"mode":                 input.Mode,
		"divergencePoint":      input.DivergencePoint,
		"originalPremise":      input.OriginalPremise,
		"forbiddenCanonBeats":  input.ForbiddenCanonBeats,
		"guardrails":           guardrails,
		"mustAvoidCanonRetell": true,
	}
	if err := s.writeCurrentState(bookID, state); err != nil {
		return nil, err
	}
	if err := s.writeFanficContent(bookID, state); err != nil {
		return nil, err
	}

	return &FanficInitResult{
		BookID:              bookID,
		Mode:                input.Mode,
		DivergencePoint:     input.DivergencePoint,
		OriginalPremise:     input.OriginalPremise,
		Guardrails:          guardrails,
		ForbiddenCanonBeats: input.ForbiddenCanonBeats,
	}, nil
}

// GetFanfic loads fanfic state from current_state.json and book metadata.
func (s *ImportService) GetFanfic(bookID string) (*FanficState, error) {
	book, err := s.books.Get(bookID)
	if err != nil {
		return nil, err
	}
	state, err := s.loadCurrentState(bookID)
	if err != nil {
		return nil, err
	}
	canon, _ := state["fanficCanon"].(map[string]any)
	profile, _ := state["fanficProfile"].(map[string]any)
	content := s.renderStoredFanficContent(state)
	return &FanficState{
		BookID:       bookID,
		Mode:         book.FanficMode,
		ParentBookID: book.ParentBookID,
		Content:      content,
		Canon:        canon,
		Profile:      profile,
	}, nil
}

// RefreshFanfic refreshes the stored fanfic canon payload without resetting the fanfic profile.
func (s *ImportService) RefreshFanfic(bookID string, input CanonImportInput) (*FanficState, error) {
	book, err := s.books.Get(bookID)
	if err != nil {
		return nil, err
	}
	if book.FanficMode == model.FanficModeNone {
		return nil, fmt.Errorf("book %q is not in fanfic mode", bookID)
	}

	state, err := s.loadCurrentState(bookID)
	if err != nil {
		return nil, err
	}
	existingCanon, _ := state["fanficCanon"].(map[string]any)
	if existingCanon == nil {
		existingCanon = map[string]any{}
	}

	existingCanon["source"] = input.Source
	existingCanon["bookId"] = input.BookID
	existingCanon["title"] = input.Title
	existingCanon["summary"] = input.Summary
	existingCanon["characters"] = input.Characters
	existingCanon["rules"] = input.Rules
	existingCanon["notes"] = input.Notes
	existingCanon["importedAt"] = time.Now().UTC().Format(time.RFC3339)
	state["fanficCanon"] = existingCanon
	if err := s.writeCurrentState(bookID, state); err != nil {
		return nil, err
	}
	if err := s.writeFanficContent(bookID, state); err != nil {
		return nil, err
	}

	profile, _ := state["fanficProfile"].(map[string]any)
	content := s.renderStoredFanficContent(state)
	return &FanficState{
		BookID:       bookID,
		Mode:         book.FanficMode,
		ParentBookID: book.ParentBookID,
		Content:      content,
		Canon:        existingCanon,
		Profile:      profile,
	}, nil
}

// ImportCanonFromBook builds a usable canon payload from another local book.
func (s *ImportService) ImportCanonFromBook(targetBookID, parentBookID string) (map[string]any, error) {
	parentBookID = strings.TrimSpace(parentBookID)
	if parentBookID == "" {
		return nil, fmt.Errorf("fromBookId is required")
	}
	parentBook, err := s.books.Get(parentBookID)
	if err != nil {
		return nil, err
	}

	state, err := s.loadCurrentState(parentBookID)
	if err != nil {
		return nil, err
	}
	summaries, err := s.loadChapterSummaries(parentBookID)
	if err != nil {
		return nil, err
	}

	input := CanonImportInput{
		Source:     "book-import",
		BookID:     parentBookID,
		Title:      parentBook.Title,
		Summary:    summarizeCanonSource(state, summaries),
		Characters: extractCanonCharacters(state),
		Rules:      buildCanonRules(parentBook, state),
		Notes:      buildCanonNotes(parentBook, state, summaries),
	}
	return s.ImportCanon(targetBookID, input)
}

func (s *ImportService) loadChapterSummaries(bookID string) ([]model.ChapterSummaryRow, error) {
	var summaries []model.ChapterSummaryRow
	if err := s.truth.Read(bookID, store.TruthChapterSummaries, &summaries); err != nil {
		return nil, err
	}
	if summaries == nil {
		return []model.ChapterSummaryRow{}, nil
	}
	return summaries, nil
}

func (s *ImportService) loadCurrentState(bookID string) (map[string]any, error) {
	var state map[string]any
	if err := s.truth.Read(bookID, store.TruthCurrentState, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = make(map[string]any)
	}
	return state, nil
}

func (s *ImportService) writeFanficContent(bookID string, state map[string]any) error {
	if strings.TrimSpace(s.dataDir) == "" {
		return nil
	}
	content := s.renderStoredFanficContent(state)
	if content == nil {
		return nil
	}
	storyDir := filepath.Join(s.dataDir, bookID, "story")
	if err := os.MkdirAll(storyDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(storyDir, "fanfic_canon.md"), []byte(*content), 0o644)
}

func (s *ImportService) renderStoredFanficContent(state map[string]any) *string {
	canon, _ := state["fanficCanon"].(map[string]any)
	profile, _ := state["fanficProfile"].(map[string]any)
	if len(canon) == 0 && len(profile) == 0 {
		return nil
	}

	var lines []string
	lines = append(lines, "# Fanfic Canon")
	if title := strings.TrimSpace(stringValue(canon["title"])); title != "" {
		lines = append(lines, "", "## Source", title)
	}
	if summary := strings.TrimSpace(stringValue(canon["summary"])); summary != "" {
		lines = append(lines, "", "## Summary", summary)
	}
	if divergence := strings.TrimSpace(stringValue(canon["divergencePoint"])); divergence == "" {
		divergence = strings.TrimSpace(stringValue(profile["divergencePoint"]))
		if divergence != "" {
			lines = append(lines, "", "## Divergence Point", divergence)
		}
	} else {
		lines = append(lines, "", "## Divergence Point", divergence)
	}
	if premise := strings.TrimSpace(stringValue(canon["originalPremise"])); premise == "" {
		premise = strings.TrimSpace(stringValue(profile["originalPremise"]))
		if premise != "" {
			lines = append(lines, "", "## Original Premise", premise)
		}
	} else {
		lines = append(lines, "", "## Original Premise", premise)
	}
	if chars := stringSlice(canon["characters"]); len(chars) > 0 {
		lines = append(lines, "", "## Characters")
		for _, item := range chars {
			lines = append(lines, "- "+item)
		}
	}
	if guardrails := stringSlice(canon["guardrails"]); len(guardrails) == 0 {
		guardrails = stringSlice(profile["guardrails"])
		if len(guardrails) > 0 {
			lines = append(lines, "", "## Guardrails")
			for _, item := range guardrails {
				lines = append(lines, "- "+item)
			}
		}
	} else {
		lines = append(lines, "", "## Guardrails")
		for _, item := range guardrails {
			lines = append(lines, "- "+item)
		}
	}
	if rules := stringSlice(canon["rules"]); len(rules) > 0 {
		lines = append(lines, "", "## Canon Rules")
		for _, item := range rules {
			lines = append(lines, "- "+item)
		}
	}
	if notes := strings.TrimSpace(stringValue(canon["notes"])); notes != "" {
		lines = append(lines, "", "## Notes", notes)
	}

	content := strings.TrimSpace(strings.Join(lines, "\n"))
	if content == "" {
		return nil
	}
	return &content
}

func (s *ImportService) writeCurrentState(bookID string, state map[string]any) error {
	return s.truth.Write(bookID, store.TruthCurrentState, state)
}

func (s *ImportService) saveRuntimeState(bookID string, st model.RuntimeState) error {
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

func normalizeImportedChapterStatus(status model.ChapterStatus) model.ChapterStatus {
	if status == "" {
		return model.ChapterStatusApproved
	}
	if model.ValidChapterStatuses[status] {
		return status
	}
	return model.ChapterStatusApproved
}

func summarizeText(content string) string {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return ""
	}
	if len(fields) > 40 {
		fields = fields[:40]
	}
	return strings.Join(fields, " ")
}

func countImportedWords(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
	}
	return count
}

func summarizeCanonSource(state map[string]any, summaries []model.ChapterSummaryRow) string {
	if latest := strings.TrimSpace(stringValue(state["latestSummary"])); latest != "" {
		return latest
	}
	if current, ok := state["importContinuation"].(map[string]any); ok {
		if latest := strings.TrimSpace(stringValue(current["latestSummary"])); latest != "" {
			return latest
		}
	}
	if len(summaries) > 0 {
		return strings.TrimSpace(summaries[len(summaries)-1].Summary)
	}
	return ""
}

func extractCanonCharacters(state map[string]any) []string {
	if current, ok := state["importContinuation"].(map[string]any); ok {
		if names := stringSlice(current["detectedCharacters"]); len(names) > 0 {
			return names
		}
	}
	if names := stringSlice(state["detectedCharacters"]); len(names) > 0 {
		return names
	}
	return nil
}

func buildCanonRules(book *model.BookConfig, state map[string]any) []string {
	rules := []string{
		fmt.Sprintf("Keep the canon genre consistent with %s.", book.Genre),
		fmt.Sprintf("Respect the established language setting: %s.", book.Language),
	}
	if latestTitle := strings.TrimSpace(stringValue(state["latestTitle"])); latestTitle != "" {
		rules = append(rules, "Continue from the latest known milestone: "+latestTitle+".")
	}
	return rules
}

func buildCanonNotes(book *model.BookConfig, state map[string]any, summaries []model.ChapterSummaryRow) string {
	parts := []string{
		fmt.Sprintf("Parent book: %s (%s).", book.Title, book.ID),
	}
	if summary := summarizeCanonSource(state, summaries); summary != "" {
		parts = append(parts, "Latest summary: "+summary)
	}
	if len(summaries) > 0 {
		parts = append(parts, fmt.Sprintf("Imported from %d summarized chapters.", len(summaries)))
	}
	return strings.Join(parts, " ")
}

func stringValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return ""
	}
}

func stringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		out := make([]string, 0, len(val))
		for _, item := range val {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s := strings.TrimSpace(stringValue(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func reverseEngineerRuntimeState(chapters []ImportedChapterInput, summaries []model.ChapterSummaryRow) model.RuntimeState {
	sortedChapters := append([]ImportedChapterInput(nil), chapters...)
	sort.Slice(sortedChapters, func(i, j int) bool {
		return sortedChapters[i].Number < sortedChapters[j].Number
	})

	sortedSummaries := append([]model.ChapterSummaryRow(nil), summaries...)
	sort.Slice(sortedSummaries, func(i, j int) bool {
		return sortedSummaries[i].Chapter < sortedSummaries[j].Chapter
	})

	names := detectLikelyNames(sortedChapters)
	keyPhrases := extractKeyPhrases(sortedChapters)
	latest := sortedChapters[len(sortedChapters)-1]
	latestSummary := summarizeText(latest.Content)
	for _, row := range sortedSummaries {
		if row.Chapter == latest.Number && row.Summary != "" {
			latestSummary = row.Summary
			break
		}
	}

	subplots := buildSubplots(sortedChapters)
	pendingHooks := buildPendingHooks(sortedChapters, sortedSummaries, keyPhrases)
	emotionalArcs := buildEmotionalArcs(sortedChapters, names)
	characterMatrix := buildCharacterMatrix(sortedChapters, names)
	wordCounts := make(map[string]int, len(sortedChapters))
	for _, chapter := range sortedChapters {
		wordCounts[strconv.Itoa(chapter.Number)] = countImportedWords(chapter.Content)
	}

	return model.RuntimeState{
		CurrentState: map[string]any{
			"importContinuation": map[string]any{
				"source":             "imported-chapters",
				"chapterCount":       len(sortedChapters),
				"nextChapter":        latest.Number + 1,
				"latestChapter":      latest.Number,
				"latestTitle":        latest.Title,
				"latestSummary":      latestSummary,
				"detectedCharacters": names,
				"keyPhrases":         keyPhrases,
				"readyToContinue":    true,
			},
			"lastChapter":   latest.Number,
			"latestTitle":   latest.Title,
			"latestSummary": latestSummary,
		},
		ParticleLedger: map[string]any{
			"detectedCharacters": names,
			"keyPhrases":         keyPhrases,
			"chapterWordCounts":  wordCounts,
			"continuationAnchor": latestSummary,
		},
		PendingHooks:     pendingHooks,
		ChapterSummaries: sortedSummaries,
		SubplotBoard:     subplots,
		EmotionalArcs:    emotionalArcs,
		CharacterMatrix:  characterMatrix,
	}
}

func detectLikelyNames(chapters []ImportedChapterInput) []string {
	counts := map[string]int{}
	for _, chapter := range chapters {
		for _, match := range latinNamePattern.FindAllString(chapter.Content+" "+chapter.Title, -1) {
			counts[match]++
		}
		for _, match := range cjkTokenPattern.FindAllString(chapter.Content+" "+chapter.Title, -1) {
			if utf8.RuneCountInString(match) < 2 || isCommonCJKToken(match) {
				continue
			}
			counts[match]++
		}
	}
	type pair struct {
		name  string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for name, count := range counts {
		if count < 2 {
			continue
		}
		pairs = append(pairs, pair{name: name, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].count > pairs[j].count
	})
	limit := len(pairs)
	if limit > 8 {
		limit = 8
	}
	result := make([]string, 0, limit)
	for _, item := range pairs[:limit] {
		result = append(result, item.name)
	}
	if len(result) == 0 {
		result = []string{"protagonist"}
	}
	return result
}

func extractKeyPhrases(chapters []ImportedChapterInput) []string {
	counts := map[string]int{}
	for _, chapter := range chapters {
		for _, token := range strings.FieldsFunc(chapter.Content+" "+chapter.Title, splitToken) {
			token = normalizeToken(token)
			if token == "" || isStopToken(token) {
				continue
			}
			counts[token]++
		}
		for _, token := range cjkTokenPattern.FindAllString(chapter.Content, -1) {
			if isCommonCJKToken(token) {
				continue
			}
			counts[token]++
		}
	}
	type pair struct {
		token string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for token, count := range counts {
		if count < 2 {
			continue
		}
		pairs = append(pairs, pair{token: token, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].token < pairs[j].token
		}
		return pairs[i].count > pairs[j].count
	})
	limit := len(pairs)
	if limit > 12 {
		limit = 12
	}
	result := make([]string, 0, limit)
	for _, item := range pairs[:limit] {
		result = append(result, item.token)
	}
	return result
}

func buildPendingHooks(chapters []ImportedChapterInput, summaries []model.ChapterSummaryRow, keyPhrases []string) []model.HookRecord {
	hooks := make([]model.HookRecord, 0, 4)
	for _, chapter := range chapters {
		summary := summarizeText(chapter.Content)
		for _, row := range summaries {
			if row.Chapter == chapter.Number && row.Summary != "" {
				summary = row.Summary
				break
			}
		}
		if !looksOpenEnded(chapter.Content, summary) {
			continue
		}
		hookType := "continuation"
		if len(keyPhrases) > 0 {
			hookType = keyPhrases[0]
		}
		hooks = append(hooks, model.HookRecord{
			HookID:              fmt.Sprintf("imported-hook-%d", chapter.Number),
			StartChapter:        chapter.Number,
			Type:                hookType,
			Status:              model.HookStatusOpen,
			LastAdvancedChapter: chapter.Number,
			ExpectedPayoff:      "continue imported storyline in upcoming chapters",
			PayoffTiming:        "next-arc",
			SeedExcerpt:         summary,
		})
	}
	if len(hooks) == 0 {
		last := chapters[len(chapters)-1]
		hooks = append(hooks, model.HookRecord{
			HookID:              fmt.Sprintf("imported-hook-%d", last.Number),
			StartChapter:        last.Number,
			Type:                "continuation",
			Status:              model.HookStatusOpen,
			LastAdvancedChapter: last.Number,
			ExpectedPayoff:      "continue imported storyline in upcoming chapters",
			PayoffTiming:        "next-chapter",
			SeedExcerpt:         summarizeText(last.Content),
		})
	}
	return hooks
}

func buildSubplots(chapters []ImportedChapterInput) []model.SubplotState {
	subplots := make([]model.SubplotState, 0, len(chapters))
	total := len(chapters)
	for idx, chapter := range chapters {
		progress := int(float64(idx+1) / float64(total) * 100)
		status := "resolved"
		if idx == total-1 {
			status = "progressing"
		}
		subplots = append(subplots, model.SubplotState{
			ID:       fmt.Sprintf("imported-arc-%d", chapter.Number),
			Title:    fallbackString(chapter.Title, fmt.Sprintf("Chapter %d imported arc", chapter.Number)),
			Status:   status,
			Progress: progress,
		})
	}
	return subplots
}

func buildEmotionalArcs(chapters []ImportedChapterInput, names []string) []model.EmotionalArcState {
	phase := "opening"
	count := len(chapters)
	switch {
	case count >= 6:
		phase = "late"
	case count >= 3:
		phase = "middle"
	}

	arcs := make([]model.EmotionalArcState, 0, len(names))
	for _, name := range names {
		arcs = append(arcs, model.EmotionalArcState{
			CharacterID: name,
			Arc:         inferEmotionalArc(chapters, name),
			Phase:       phase,
		})
	}
	return arcs
}

func buildCharacterMatrix(chapters []ImportedChapterInput, names []string) []model.CharacterMatrixEntry {
	matrix := make([]model.CharacterMatrixEntry, 0, len(names))
	for _, name := range names {
		relations := map[string]any{}
		for _, other := range names {
			if other == name {
				continue
			}
			if chaptersShareMention(chapters, name, other) {
				relations[other] = "co-appears"
			}
		}
		matrix = append(matrix, model.CharacterMatrixEntry{
			CharacterID: name,
			Knows: map[string]any{
				"lastSeenChapter": lastSeenChapter(chapters, name),
			},
			Relations: relations,
		})
	}
	return matrix
}

func looksOpenEnded(content, summary string) bool {
	text := content + " " + summary
	cues := []string{"?", "？", "秘密", "未", "still", "mystery", "suddenly", "但是", "然而", "却", "unknown", "promise"}
	for _, cue := range cues {
		if strings.Contains(strings.ToLower(text), strings.ToLower(cue)) {
			return true
		}
	}
	return false
}

func splitToken(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func normalizeToken(token string) string {
	token = strings.TrimSpace(strings.ToLower(token))
	if utf8.RuneCountInString(token) < 2 {
		return ""
	}
	return token
}

func isStopToken(token string) bool {
	stop := map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "that": {}, "this": {}, "from": {}, "chapter": {},
		"then": {}, "they": {}, "them": {}, "into": {}, "their": {}, "there": {}, "after": {}, "before": {},
		"第一章": {}, "第二章": {}, "第三章": {}, "他们": {}, "我们": {}, "不是": {}, "一个": {}, "没有": {}, "自己": {},
	}
	_, ok := stop[token]
	return ok
}

func isCommonCJKToken(token string) bool {
	common := map[string]struct{}{
		"第一章": {}, "第二章": {}, "第三章": {}, "第四章": {}, "第五章": {}, "然后": {}, "他们": {}, "我们": {},
		"这里": {}, "自己": {}, "突然": {}, "继续": {}, "时候": {}, "已经": {}, "没有": {}, "不是": {},
	}
	_, ok := common[token]
	return ok
}

func inferEmotionalArc(chapters []ImportedChapterInput, name string) string {
	positive := 0
	negative := 0
	for _, chapter := range chapters {
		if !strings.Contains(chapter.Content, name) {
			continue
		}
		text := chapter.Content
		for _, cue := range []string{"笑", "希望", "信任", "calm", "hope", "trust", "warm"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(cue)) {
				positive++
			}
		}
		for _, cue := range []string{"痛", "fear", "怒", "betray", "panic", "cold", "danger"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(cue)) {
				negative++
			}
		}
	}
	switch {
	case negative > positive:
		return "pressure"
	case positive > negative:
		return "recovery"
	default:
		return "rising_tension"
	}
}

func chaptersShareMention(chapters []ImportedChapterInput, left, right string) bool {
	for _, chapter := range chapters {
		if strings.Contains(chapter.Content, left) && strings.Contains(chapter.Content, right) {
			return true
		}
	}
	return false
}

func lastSeenChapter(chapters []ImportedChapterInput, name string) int {
	last := 0
	for _, chapter := range chapters {
		if strings.Contains(chapter.Content, name) || strings.Contains(chapter.Title, name) {
			last = chapter.Number
		}
	}
	return last
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func buildFanficGuardrails(mode model.FanficMode, divergencePoint string, forbiddenCanonBeats []string) []string {
	guardrails := []string{
		"Never retell canon plot beats verbatim after initialization.",
		"All new chapters must branch from the declared divergence point instead of replaying source scenes.",
		"Preserve recognizable canon constraints while generating new consequences.",
		fmt.Sprintf("Declared divergence point: %s", strings.TrimSpace(divergencePoint)),
	}
	switch mode {
	case model.FanficModeInspired:
		guardrails = append(guardrails, "Reuse inspiration tone and motifs only; build an original plotline.")
	case model.FanficModeAlternate:
		guardrails = append(guardrails, "Push the alternate timeline immediately and keep downstream causality original.")
	case model.FanficModeContinuation:
		guardrails = append(guardrails, "Treat canon as complete backstory and continue only with new post-canon developments.")
	case model.FanficModeReverse:
		guardrails = append(guardrails, "Anchor the narrative in a reversed viewpoint or power relation, not a replay of canon scenes.")
	}
	for _, beat := range forbiddenCanonBeats {
		beat = strings.TrimSpace(beat)
		if beat == "" {
			continue
		}
		guardrails = append(guardrails, "Forbidden canon beat: "+beat)
	}
	return guardrails
}
