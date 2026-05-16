package app

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"storyforge/internal/store"
)

// DetectService analyzes a chapter for AI-trace patterns.
type DetectService struct {
	books    *store.BookStore
	chapters *store.ChapterStore
}

// NewDetectService creates a DetectService.
func NewDetectService(books *store.BookStore, chapters *store.ChapterStore) *DetectService {
	return &DetectService{books: books, chapters: chapters}
}

// DetectionResult is the output of a chapter detection pass.
type DetectionResult struct {
	BookID                   string    `json:"bookId"`
	Chapter                  int       `json:"chapter"`
	ChapterTitle             string    `json:"chapterTitle,omitempty"`
	TextLength               int       `json:"textLength"`
	ParagraphCount           int       `json:"paragraphCount"`
	FatigueWordHits          int       `json:"fatigueWordHits"`
	FatigueWordHitRate       float64   `json:"fatigueWordHitRate"`
	FatigueWordMatches       []string  `json:"fatigueWordMatches,omitempty"`
	ParagraphEqualLengthRate float64   `json:"paragraphEqualLengthRate"`
	ClicheHits               int       `json:"clicheHits"`
	ClicheDensity            float64   `json:"clicheDensity"`
	ClicheMatches            []string  `json:"clicheMatches,omitempty"`
	RiskLevel                string    `json:"riskLevel"`
	RiskScore                int       `json:"riskScore"`
	Reasons                  []string  `json:"reasons,omitempty"`
	AnalyzedAt               time.Time `json:"analyzedAt"`
}

// DetectionStats aggregates detection results across a whole book.
type DetectionStats struct {
	BookID                       string             `json:"bookId"`
	TotalChapters                int                `json:"totalChapters"`
	HighRiskChapters             int                `json:"highRiskChapters"`
	MediumRiskChapters           int                `json:"mediumRiskChapters"`
	LowRiskChapters              int                `json:"lowRiskChapters"`
	AverageRiskScore             float64            `json:"averageRiskScore"`
	AverageFatigueWordHitRate    float64            `json:"averageFatigueWordHitRate"`
	AverageParagraphEqualRate    float64            `json:"averageParagraphEqualLengthRate"`
	AverageClicheDensity         float64            `json:"averageClicheDensity"`
	MaxRiskChapter               *DetectionResult   `json:"maxRiskChapter,omitempty"`
	RiskLevelDistribution        map[string]int     `json:"riskLevelDistribution"`
	Chapters                     []*DetectionResult `json:"chapters"`
}

var fatigueTerms = []string{
	"突然",
	"然后",
	"接着",
	"忽然",
	"就在这个时候",
	"不知不觉",
	"眼前一亮",
	"毫无疑问",
	"与此同时",
	"总之",
	"原来如此",
	"说时迟那时快",
}

var clicheTerms = []string{
	"就在这个时候",
	"不知为何",
	"毫无疑问",
	"说时迟那时快",
	"总而言之",
	"与此同时",
	"眼前一亮",
	"原来如此",
	"没有想到",
	"在这一刻",
	"众所周知",
	"显而易见",
}

// AnalyzeChapter reads a chapter and computes detection metrics.
func (s *DetectService) AnalyzeChapter(bookID string, chapter int) (*DetectionResult, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	meta, err := s.chapters.GetMeta(bookID, chapter)
	if err != nil {
		return nil, err
	}
	content, err := s.chapters.GetContent(bookID, chapter)
	if err != nil {
		return nil, err
	}

	paragraphs := splitParagraphsDetect(content)
	textLen := utf8.RuneCountInString(content)

	fatigueHits, fatigueMatches := countPhraseHits(content, fatigueTerms)
	clicheHits, clicheMatches := countPhraseHits(content, clicheTerms)

	equalRate := paragraphEqualLengthRate(paragraphs)
	fatigueRate := ratePerK(textLen, fatigueHits)
	clicheDensity := ratePerK(textLen, clicheHits)

	riskScore, riskLevel, reasons := assessRisk(fatigueRate, equalRate, clicheDensity, fatigueHits, clicheHits)

	sort.Strings(fatigueMatches)
	sort.Strings(clicheMatches)

	return &DetectionResult{
		BookID:                   bookID,
		Chapter:                  chapter,
		ChapterTitle:             meta.Title,
		TextLength:               textLen,
		ParagraphCount:           len(paragraphs),
		FatigueWordHits:          fatigueHits,
		FatigueWordHitRate:       fatigueRate,
		FatigueWordMatches:       fatigueMatches,
		ParagraphEqualLengthRate: equalRate,
		ClicheHits:               clicheHits,
		ClicheDensity:            clicheDensity,
		ClicheMatches:            clicheMatches,
		RiskLevel:                riskLevel,
		RiskScore:                riskScore,
		Reasons:                  reasons,
		AnalyzedAt:               time.Now().UTC(),
	}, nil
}

// AnalyzeAll runs detection for every existing chapter in a book.
func (s *DetectService) AnalyzeAll(bookID string) ([]*DetectionResult, error) {
	if _, err := s.books.Get(bookID); err != nil {
		return nil, err
	}
	metas, err := s.chapters.ListMeta(bookID)
	if err != nil {
		return nil, err
	}
	if len(metas) == 0 {
		return []*DetectionResult{}, nil
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Number < metas[j].Number
	})

	results := make([]*DetectionResult, 0, len(metas))
	for _, meta := range metas {
		result, err := s.AnalyzeChapter(bookID, meta.Number)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// Stats computes aggregate detection stats for a book.
func (s *DetectService) Stats(bookID string) (*DetectionStats, error) {
	results, err := s.AnalyzeAll(bookID)
	if err != nil {
		return nil, err
	}

	stats := &DetectionStats{
		BookID:                bookID,
		TotalChapters:         len(results),
		RiskLevelDistribution: map[string]int{"high": 0, "medium": 0, "low": 0},
		Chapters:              results,
	}
	if len(results) == 0 {
		return stats, nil
	}

	var sumRiskScore, sumFatigueRate, sumEqualRate, sumClicheDensity float64
	for _, result := range results {
		sumRiskScore += float64(result.RiskScore)
		sumFatigueRate += result.FatigueWordHitRate
		sumEqualRate += result.ParagraphEqualLengthRate
		sumClicheDensity += result.ClicheDensity

		stats.RiskLevelDistribution[result.RiskLevel]++
		switch result.RiskLevel {
		case "high":
			stats.HighRiskChapters++
		case "medium":
			stats.MediumRiskChapters++
		default:
			stats.LowRiskChapters++
		}

		if stats.MaxRiskChapter == nil || result.RiskScore > stats.MaxRiskChapter.RiskScore {
			stats.MaxRiskChapter = result
		}
	}

	total := float64(len(results))
	stats.AverageRiskScore = sumRiskScore / total
	stats.AverageFatigueWordHitRate = sumFatigueRate / total
	stats.AverageParagraphEqualRate = sumEqualRate / total
	stats.AverageClicheDensity = sumClicheDensity / total
	return stats, nil
}

func splitParagraphsDetect(content string) []string {
	raw := strings.Split(content, "\n\n")
	paragraphs := make([]string, 0, len(raw))
	for _, part := range raw {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}
	if len(paragraphs) == 0 && strings.TrimSpace(content) != "" {
		return []string{strings.TrimSpace(content)}
	}
	return paragraphs
}

func countPhraseHits(content string, phrases []string) (int, []string) {
	hits := 0
	matches := make([]string, 0)
	for _, phrase := range phrases {
		c := strings.Count(content, phrase)
		if c == 0 {
			continue
		}
		hits += c
		for i := 0; i < c; i++ {
			matches = append(matches, phrase)
		}
	}
	return hits, matches
}

func paragraphEqualLengthRate(paragraphs []string) float64 {
	if len(paragraphs) < 2 {
		return 0
	}
	lengths := make([]int, 0, len(paragraphs))
	total := 0
	for _, p := range paragraphs {
		n := utf8.RuneCountInString(p)
		lengths = append(lengths, n)
		total += n
	}
	avg := float64(total) / float64(len(lengths))
	if avg == 0 {
		return 0
	}
	tolerance := avg * 0.2
	equal := 0
	for _, n := range lengths {
		if math.Abs(float64(n)-avg) <= tolerance {
			equal++
		}
	}
	return float64(equal) / float64(len(lengths))
}

func ratePerK(textLen, hits int) float64 {
	if textLen <= 0 || hits <= 0 {
		return 0
	}
	return float64(hits) * 1000 / float64(textLen)
}

func assessRisk(fatigueRate, equalRate, clicheDensity float64, fatigueHits, clicheHits int) (int, string, []string) {
	score := 0
	reasons := make([]string, 0, 3)

	if fatigueRate >= 4 {
		score += 3
		reasons = append(reasons, "fatigue words are dense")
	} else if fatigueRate >= 2 {
		score += 2
		reasons = append(reasons, "fatigue words are noticeable")
	} else if fatigueHits > 0 {
		score++
		reasons = append(reasons, "fatigue words appear")
	}

	if clicheDensity >= 4 {
		score += 3
		reasons = append(reasons, "clichés are dense")
	} else if clicheDensity >= 2 {
		score += 2
		reasons = append(reasons, "clichés are noticeable")
	} else if clicheHits > 0 {
		score++
		reasons = append(reasons, "clichés appear")
	}

	if equalRate >= 0.75 {
		score += 3
		reasons = append(reasons, "paragraph lengths are too uniform")
	} else if equalRate >= 0.5 {
		score += 2
		reasons = append(reasons, "paragraph lengths are fairly uniform")
	}

	level := "low"
	switch {
	case score >= 6:
		level = "high"
	case score >= 3:
		level = "medium"
	}

	return score, level, reasons
}
