package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"storyforge/internal/store"
)

// StyleService analyzes chapter prose and stores style fingerprints.
type StyleService struct {
	books    *store.BookStore
	chapters *store.ChapterStore
	truth    *store.TruthStore
}

// NewStyleService creates a StyleService rooted in existing stores.
func NewStyleService(books *store.BookStore, chapters *store.ChapterStore, truth *store.TruthStore) *StyleService {
	return &StyleService{books: books, chapters: chapters, truth: truth}
}

// Analyze scans chapter content or supplied text and produces a deterministic style fingerprint.
func (s *StyleService) Analyze(bookID string, input StyleAnalyzeInput) (*StyleAnalyzeResult, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}

	sourceType := "book-chapters"
	chapterRange := "all"
	text := strings.TrimSpace(input.Text)
	if text == "" {
		chunks, rangeText, err := s.loadChapterTexts(bookID, input.ChapterFrom, input.ChapterTo)
		if err != nil {
			return nil, err
		}
		if len(chunks) == 0 {
			return nil, fmt.Errorf("no chapter content available for style analysis")
		}
		text = strings.Join(chunks, "\n\n")
		chapterRange = rangeText
	} else {
		sourceType = "raw-text"
	}

	stats, fingerprint := analyzeText(text)
	guidance := buildGuidance(stats, fingerprint)
	result := &StyleAnalyzeResult{
		BookID:        bookID,
		ChapterRange:  chapterRange,
		SourceType:    sourceType,
		Stats:         stats,
		Fingerprint:   fingerprint,
		StyleGuidance: guidance,
		StyleGuide:    strings.Join(guidance, "\n"),
	}
	if err := s.persist(bookID, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *StyleService) loadChapterTexts(bookID string, from, to int) ([]string, string, error) {
	metas, err := s.chapters.ListMeta(bookID)
	if err != nil {
		return nil, "", err
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Number < metas[j].Number })

	effectiveFrom := from
	effectiveTo := to
	if effectiveFrom <= 0 && len(metas) > 0 {
		effectiveFrom = metas[0].Number
	}
	if effectiveTo <= 0 && len(metas) > 0 {
		effectiveTo = metas[len(metas)-1].Number
	}
	if effectiveFrom > 0 && effectiveTo > 0 && effectiveFrom > effectiveTo {
		return nil, "", fmt.Errorf("chapterFrom cannot be greater than chapterTo")
	}

	var chunks []string
	for _, meta := range metas {
		if effectiveFrom > 0 && meta.Number < effectiveFrom {
			continue
		}
		if effectiveTo > 0 && meta.Number > effectiveTo {
			continue
		}
		content, err := s.chapters.GetContent(bookID, meta.Number)
		if err != nil {
			continue
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		chunks = append(chunks, content)
	}

	if len(chunks) == 0 {
		return nil, "", nil
	}
	if effectiveFrom > 0 && effectiveTo > 0 {
		return chunks, fmt.Sprintf("%d-%d", effectiveFrom, effectiveTo), nil
	}
	return chunks, "all", nil
}

func (s *StyleService) persist(bookID string, result *StyleAnalyzeResult) error {
	state, err := s.loadCurrentState(bookID)
	if err != nil {
		return err
	}
	state["styleAnalysis"] = result
	state["styleProfile"] = map[string]any{
		"source":      "analyze",
		"summary":     "deterministic chapter style analysis",
		"fingerprint": result.Fingerprint,
		"guidance":    result.StyleGuidance,
		"styleGuide":  result.StyleGuide,
		"importedAt":  time.Now().UTC().Format(time.RFC3339),
	}
	return s.writeCurrentState(bookID, state)
}

func (s *StyleService) loadCurrentState(bookID string) (map[string]any, error) {
	var state map[string]any
	if err := s.truth.Read(bookID, store.TruthCurrentState, &state); err != nil {
		return nil, err
	}
	if state == nil {
		state = make(map[string]any)
	}
	return state, nil
}

func (s *StyleService) writeCurrentState(bookID string, state map[string]any) error {
	return s.truth.Write(bookID, store.TruthCurrentState, state)
}

func tokenizeStyle(content string) []string {
	var tokens []string
	var current strings.Builder
	var currentClass tokenClass

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(current.String()))
		current.Reset()
		currentClass = tokenClassNone
	}

	for _, r := range content {
		class := classifyRune(r)
		if class == tokenClassNone {
			flush()
			continue
		}
		if currentClass != tokenClassNone && class != currentClass {
			flush()
		}
		currentClass = class
		current.WriteRune(r)
	}
	flush()
	return tokens
}

type tokenClass int

const (
	tokenClassNone tokenClass = iota
	tokenClassLatin
	tokenClassCJK
	tokenClassDigit
)

func classifyRune(r rune) tokenClass {
	switch {
	case unicode.IsLetter(r) && r <= unicode.MaxLatin1:
		return tokenClassLatin
	case unicode.IsDigit(r):
		return tokenClassDigit
	case unicode.In(r, unicode.Han):
		return tokenClassCJK
	default:
		return tokenClassNone
	}
}

func countSentences(content string) int {
	count := 0
	for _, r := range content {
		switch r {
		case '.', '!', '?', '。', '！', '？':
			count++
		}
	}
	return max(1, count)
}

func countParagraphs(content string) int {
	parts := strings.Split(content, "\n\n")
	count := 0
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return max(1, count)
}

func countDialogueLines(content string) int {
	lines := strings.Split(content, "\n")
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "—") || strings.HasPrefix(trimmed, "-") || strings.Contains(trimmed, "“") || strings.Contains(trimmed, "\"") {
			count++
		}
	}
	return count
}

func ratio(a, b int) float64 {
	if b <= 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func uniqueWordRatio(tokens map[string]int, totalWords int) float64 {
	if totalWords <= 0 {
		return 0
	}
	return float64(len(tokens)) / float64(totalWords)
}

func topWords(tokens map[string]int, limit int) []string {
	type pair struct {
		word  string
		count int
	}
	list := make([]pair, 0, len(tokens))
	for word, count := range tokens {
		if len(word) < 2 {
			continue
		}
		list = append(list, pair{word: word, count: count})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count == list[j].count {
			return list[i].word < list[j].word
		}
		return list[i].count > list[j].count
	})
	if len(list) > limit {
		list = list[:limit]
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		result = append(result, fmt.Sprintf("%s:%d", item.word, item.count))
	}
	return result
}

func buildStyleGuidance(fp map[string]any) []string {
	var guidance []string
	if avg, ok := fp["averageSentenceLength"].(float64); ok && avg > 28 {
		guidance = append(guidance, "Sentence length is long; tighten pacing with shorter clauses and stronger line breaks.")
	}
	if ratio, ok := fp["uniqueWordRatio"].(float64); ok && ratio < 0.35 {
		guidance = append(guidance, "Vocabulary repeats frequently; vary verbs, modifiers, and emotional descriptors.")
	}
	if ratio, ok := fp["dialogueRatio"].(float64); ok && ratio < 0.12 {
		guidance = append(guidance, "Dialogue is sparse; add more spoken interaction to improve movement and character voice.")
	}
	if avg, ok := fp["averageParagraphLength"].(float64); ok && avg > 140 {
		guidance = append(guidance, "Paragraphs are dense; break exposition into smaller beats and more visual spacing.")
	}
	if guidance == nil {
		guidance = []string{
			"Prose is balanced; preserve current cadence while keeping variation in scene length.",
		}
	}
	return guidance
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
