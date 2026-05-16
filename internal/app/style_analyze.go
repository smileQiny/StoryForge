package app

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"storyforge/internal/store"
)

// StyleAnalyzeInput controls how text is selected for analysis.
type StyleAnalyzeInput struct {
	Text        string `json:"text,omitempty"`
	ChapterFrom int    `json:"chapterFrom,omitempty"`
	ChapterTo   int    `json:"chapterTo,omitempty"`
}

// StyleAnalyzeResult is the output of style analysis.
type StyleAnalyzeResult struct {
	BookID        string           `json:"bookId"`
	ChapterRange  string           `json:"chapterRange"`
	SourceType    string           `json:"sourceType"` // book-chapters / raw-text
	Stats         StyleStats       `json:"stats"`
	Fingerprint   StyleFingerprint `json:"fingerprint"`
	StyleGuidance []string         `json:"styleGuidance"`
	StyleGuide    string           `json:"styleGuide"`
}

// StyleStats captures basic writing statistics.
type StyleStats struct {
	TotalChars            int     `json:"totalChars"`
	TotalWords            int     `json:"totalWords"`
	ParagraphCount        int     `json:"paragraphCount"`
	SentenceCount         int     `json:"sentenceCount"`
	AverageSentenceLength float64 `json:"averageSentenceLength"`
	AverageParagraphWords float64 `json:"averageParagraphWords"`
	DialogueRatio         float64 `json:"dialogueRatio"`
	ExclamationDensity    float64 `json:"exclamationDensity"`
	EllipsisDensity       float64 `json:"ellipsisDensity"`
}

// StyleFingerprint captures derived style markers.
type StyleFingerprint struct {
	LexicalDiversity float64 `json:"lexicalDiversity"`
	AverageWordLength float64 `json:"averageWordLength"`
	ShortSentenceRatio float64 `json:"shortSentenceRatio"`
	LongSentenceRatio  float64 `json:"longSentenceRatio"`
}

// StyleAnalyzeService performs deterministic style analysis.
type StyleAnalyzeService struct {
	books    *store.BookStore
	chapters *store.ChapterStore
}

// NewStyleAnalyzeService creates a StyleAnalyzeService.
func NewStyleAnalyzeService(books *store.BookStore, chapters *store.ChapterStore) *StyleAnalyzeService {
	return &StyleAnalyzeService{books: books, chapters: chapters}
}

// Analyze builds style statistics and guidance from raw text or chapter range.
func (s *StyleAnalyzeService) Analyze(bookID string, input StyleAnalyzeInput) (*StyleAnalyzeResult, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}

	sourceType := "book-chapters"
	rangeText := "all"
	text := strings.TrimSpace(input.Text)
	if text == "" {
		chunks, chapterRange, err := s.loadChapterTexts(bookID, input.ChapterFrom, input.ChapterTo)
		if err != nil {
			return nil, err
		}
		rangeText = chapterRange
		text = strings.Join(chunks, "\n\n")
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("no chapter content available for analysis")
		}
	} else {
		sourceType = "raw-text"
	}

	stats, fp := analyzeText(text)
	guidance := buildGuidance(stats, fp)
	return &StyleAnalyzeResult{
		BookID:        bookID,
		ChapterRange:  rangeText,
		SourceType:    sourceType,
		Stats:         stats,
		Fingerprint:   fp,
		StyleGuidance: guidance,
		StyleGuide:    strings.Join(guidance, "\n"),
	}, nil
}

// AnalyzeText runs style analysis on raw text without requiring a book.
func (s *StyleAnalyzeService) AnalyzeText(text string) (*StyleAnalyzeResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	stats, fp := analyzeText(text)
	guidance := buildGuidance(stats, fp)
	return &StyleAnalyzeResult{
		BookID:        "",
		ChapterRange:  "raw",
		SourceType:    "raw-text",
		Stats:         stats,
		Fingerprint:   fp,
		StyleGuidance: guidance,
		StyleGuide:    strings.Join(guidance, "\n"),
	}, nil
}

func (s *StyleAnalyzeService) loadChapterTexts(bookID string, from, to int) ([]string, string, error) {
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

var sentenceSplitRE = regexp.MustCompile(`[。！？!?\.]+`)
var wordSplitRE = regexp.MustCompile(`[\p{L}\p{N}_]+`)

func analyzeText(text string) (StyleStats, StyleFingerprint) {
	trimmed := strings.TrimSpace(text)
	paragraphs := splitParagraphs(trimmed)
	sentences := splitSentences(trimmed)
	words := extractWords(trimmed)

	totalChars := len([]rune(trimmed))
	totalWords := len(words)

	stats := StyleStats{
		TotalChars:     totalChars,
		TotalWords:     totalWords,
		ParagraphCount: len(paragraphs),
		SentenceCount:  len(sentences),
	}
	if stats.SentenceCount > 0 {
		stats.AverageSentenceLength = round2(float64(totalWords) / float64(stats.SentenceCount))
	}
	if stats.ParagraphCount > 0 {
		stats.AverageParagraphWords = round2(float64(totalWords) / float64(stats.ParagraphCount))
	}
	stats.DialogueRatio = round2(dialogueRatio(paragraphs))
	stats.ExclamationDensity = round2(float64(strings.Count(trimmed, "!")+strings.Count(trimmed, "！")) / math.Max(1, float64(stats.SentenceCount)))
	stats.EllipsisDensity = round2(float64(strings.Count(trimmed, "...")+strings.Count(trimmed, "……")) / math.Max(1, float64(stats.SentenceCount)))

	unique := make(map[string]struct{}, len(words))
	totalWordLen := 0
	shortSentences := 0
	longSentences := 0
	for _, w := range words {
		unique[strings.ToLower(w)] = struct{}{}
		totalWordLen += len([]rune(w))
	}
	for _, s := range sentences {
		wc := len(extractWords(s))
		if wc <= 8 {
			shortSentences++
		}
		if wc >= 20 {
			longSentences++
		}
	}

	fp := StyleFingerprint{}
	if totalWords > 0 {
		fp.LexicalDiversity = round2(float64(len(unique)) / float64(totalWords))
		fp.AverageWordLength = round2(float64(totalWordLen) / float64(totalWords))
	}
	if len(sentences) > 0 {
		fp.ShortSentenceRatio = round2(float64(shortSentences) / float64(len(sentences)))
		fp.LongSentenceRatio = round2(float64(longSentences) / float64(len(sentences)))
	}
	return stats, fp
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitSentences(text string) []string {
	parts := sentenceSplitRE.Split(text, -1)
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func extractWords(text string) []string {
	return wordSplitRE.FindAllString(text, -1)
}

func dialogueRatio(paragraphs []string) float64 {
	if len(paragraphs) == 0 {
		return 0
	}
	dialogueParagraphs := 0
	for _, p := range paragraphs {
		if strings.ContainsAny(p, `"'“”‘’`) || strings.HasPrefix(p, "—") {
			dialogueParagraphs++
		}
	}
	return float64(dialogueParagraphs) / float64(len(paragraphs))
}

func buildGuidance(stats StyleStats, fp StyleFingerprint) []string {
	var out []string
	if fp.LexicalDiversity < 0.45 {
		out = append(out, "词汇复用较高：可增加同义替换和场景细节词，降低重复表达。")
	} else {
		out = append(out, "词汇多样性良好：保持关键词复现的同时继续扩展动作与感官词汇。")
	}
	if fp.ShortSentenceRatio > 0.55 {
		out = append(out, "短句占比较高：适度引入复句与从句，增强叙述层次。")
	}
	if fp.LongSentenceRatio > 0.35 {
		out = append(out, "长句占比较高：在冲突和动作段落使用更短句提升节奏。")
	}
	if stats.DialogueRatio < 0.2 {
		out = append(out, "对话密度偏低：关键场景加入角色互动可提升代入感。")
	}
	if stats.ExclamationDensity > 0.3 {
		out = append(out, "感叹符密度偏高：建议用动作或心理描写替代部分强调符号。")
	}
	if len(out) == 0 {
		out = append(out, "风格分布均衡：可围绕核心叙事目标微调句长与信息密度。")
	}
	return out
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
