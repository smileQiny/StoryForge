package model

import "time"

// ChapterStatus represents the lifecycle state of a chapter.
type ChapterStatus string

const (
	ChapterStatusDraft         ChapterStatus = "draft"
	ChapterStatusAudited       ChapterStatus = "audited"
	ChapterStatusRevised       ChapterStatus = "revised"
	ChapterStatusPendingReview ChapterStatus = "pending-review"
	ChapterStatusApproved      ChapterStatus = "approved"
	ChapterStatusRejected      ChapterStatus = "rejected"
)

// ValidChapterStatuses is the set of all valid chapter statuses.
var ValidChapterStatuses = map[ChapterStatus]bool{
	ChapterStatusDraft:         true,
	ChapterStatusAudited:       true,
	ChapterStatusRevised:       true,
	ChapterStatusPendingReview: true,
	ChapterStatusApproved:      true,
	ChapterStatusRejected:      true,
}

// ChapterMeta holds metadata for a single chapter.
type ChapterMeta struct {
	Number    int           `json:"number"`
	Title     string        `json:"title"`
	Status    ChapterStatus `json:"status"`
	WordCount int           `json:"wordCount"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// Validate checks that the chapter metadata is valid.
func (c *ChapterMeta) Validate() error {
	if c.Number <= 0 {
		return &ValidationError{Field: "number", Message: "must be positive"}
	}
	if !ValidChapterStatuses[c.Status] {
		return &ValidationError{Field: "status", Message: "invalid chapter status"}
	}
	return nil
}
