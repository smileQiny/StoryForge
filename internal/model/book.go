package model

import "time"

// BookStatus represents the lifecycle state of a book.
type BookStatus string

const (
	BookStatusDraft     BookStatus = "draft"
	BookStatusActive    BookStatus = "active"
	BookStatusPaused    BookStatus = "paused"
	BookStatusCompleted BookStatus = "completed"
	BookStatusArchived  BookStatus = "archived"
)

// FanficMode represents the fanfiction mode for a book.
type FanficMode string

const (
	FanficModeNone         FanficMode = ""
	FanficModeInspired     FanficMode = "inspired"     // 灵感借鉴
	FanficModeAlternate    FanficMode = "alternate"    // 平行世界
	FanficModeContinuation FanficMode = "continuation" // 续写
	FanficModeReverse      FanficMode = "reverse"      // 反转视角
)

// ValidFanficModes enumerates the supported fanfic modes.
var ValidFanficModes = map[FanficMode]bool{
	FanficModeInspired:     true,
	FanficModeAlternate:    true,
	FanficModeContinuation: true,
	FanficModeReverse:      true,
}

// Language represents the writing language.
type Language string

const (
	LanguageZH Language = "zh"
	LanguageEN Language = "en"
)

// BookConfig is the primary configuration for a book project.
type BookConfig struct {
	ID                      string     `json:"id"`
	Title                   string     `json:"title"`
	Platform                string     `json:"platform,omitempty"`
	Genre                   string     `json:"genre"`
	Status                  BookStatus `json:"status"`
	Language                Language   `json:"language"`
	TargetChapters          int        `json:"targetChapters"`
	ChapterWordCount        int        `json:"chapterWordCount"`
	ParentBookID            string     `json:"parentBookId,omitempty"`
	FanficMode              FanficMode `json:"fanficMode,omitempty"`
	DisabledAuditDimensions []string   `json:"disabledAuditDimensions,omitempty"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

// Validate checks that required fields are set and values are valid.
func (b *BookConfig) Validate() error {
	if b.ID == "" {
		return &ValidationError{Field: "id", Message: "required"}
	}
	if b.Title == "" {
		return &ValidationError{Field: "title", Message: "required"}
	}
	if b.Genre == "" {
		return &ValidationError{Field: "genre", Message: "required"}
	}
	if b.Language != LanguageZH && b.Language != LanguageEN {
		return &ValidationError{Field: "language", Message: "must be zh or en"}
	}
	if b.ChapterWordCount <= 0 {
		return &ValidationError{Field: "chapterWordCount", Message: "must be positive"}
	}
	if b.FanficMode != FanficModeNone && !ValidFanficModes[b.FanficMode] {
		return &ValidationError{Field: "fanficMode", Message: "must be one of inspired/alternate/continuation/reverse"}
	}
	return nil
}
