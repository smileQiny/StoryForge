package model

import "time"

// ChapterSnapshot is a point-in-time snapshot of the full truth state for a chapter.
type ChapterSnapshot struct {
	Chapter   int            `json:"chapter"`
	CreatedAt time.Time      `json:"createdAt"`
	State     *RuntimeState  `json:"state"`
}
