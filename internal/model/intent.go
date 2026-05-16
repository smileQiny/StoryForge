package model

// ChapterIntent is the output of the Planner — the task definition for a chapter.
type ChapterIntent struct {
	Chapter        int              `json:"chapter"`
	Goal           string           `json:"goal"`
	OutlineNode    string           `json:"outlineNode,omitempty"`
	SceneDirective string           `json:"sceneDirective,omitempty"`
	ArcDirective   string           `json:"arcDirective,omitempty"`
	MoodDirective  string           `json:"moodDirective,omitempty"`
	TitleDirective string           `json:"titleDirective,omitempty"`
	MustKeep       []string         `json:"mustKeep,omitempty"`
	MustAvoid      []string         `json:"mustAvoid,omitempty"`
	StyleEmphasis  []string         `json:"styleEmphasis,omitempty"`
	Conflicts      []ChapterConflict `json:"conflicts,omitempty"`
	HookAgenda     HookAgenda       `json:"hookAgenda"`
}

// ChapterConflict describes a conflict to be developed in the chapter.
type ChapterConflict struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Stakes      string `json:"stakes,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
}

// HookAgenda is the hook scheduling plan for a chapter.
type HookAgenda struct {
	PressureMap          []HookPressure `json:"pressureMap,omitempty"`
	MustAdvance          []string       `json:"mustAdvance,omitempty"`
	EligibleResolve      []string       `json:"eligibleResolve,omitempty"`
	StaleDebt            []string       `json:"staleDebt,omitempty"`
	AvoidNewHookFamilies []string       `json:"avoidNewHookFamilies,omitempty"`
}

// HookPressure describes the pressure level and movement for a hook.
type HookPressure struct {
	HookID   string `json:"hookId"`
	Type     string `json:"type"`
	Movement string `json:"movement"` // quiet-hold/refresh/advance/partial-payoff/full-payoff
	Pressure string `json:"pressure"` // low/medium/high/critical
	Phase    string `json:"phase"`    // opening/middle/late
	Reason   string `json:"reason"`   // fresh-promise/building-debt/stale-promise/ripe-payoff/overdue-payoff/long-arc-hold
}
