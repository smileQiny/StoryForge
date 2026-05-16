package model

import "time"

// RunKind describes what kind of pipeline run this is.
type RunKind string

const (
	RunKindPlan         RunKind = "plan"
	RunKindCompose      RunKind = "compose"
	RunKindWrite        RunKind = "write"
	RunKindAudit        RunKind = "audit"
	RunKindRevise       RunKind = "revise"
	RunKindFullPipeline RunKind = "full-pipeline"
)

// RunStatus describes the current state of a run.
type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// RunTriggeredBy describes who triggered the run.
type RunTriggeredBy string

const (
	RunTriggeredByStudio    RunTriggeredBy = "studio"
	RunTriggeredByUser      RunTriggeredBy = "user"
	RunTriggeredByScheduler RunTriggeredBy = "scheduler"
	RunTriggeredBySystem    RunTriggeredBy = "system"
)

// StageStatus describes the state of a single pipeline stage.
type StageStatus string

const (
	StageStatusPending   StageStatus = "pending"
	StageStatusRunning   StageStatus = "running"
	StageStatusSucceeded StageStatus = "succeeded"
	StageStatusFailed    StageStatus = "failed"
	StageStatusSkipped   StageStatus = "skipped"
)

// Run is the top-level observable unit for a pipeline execution.
type Run struct {
	ID          string         `json:"id"`
	BookID      string         `json:"bookId"`
	Chapter     int            `json:"chapter"`
	Kind        RunKind        `json:"kind"`
	Mode        string         `json:"mode,omitempty"`
	Status      RunStatus      `json:"status"`
	TriggeredBy RunTriggeredBy `json:"triggeredBy"`
	StartedAt   time.Time      `json:"startedAt"`
	EndedAt     *time.Time     `json:"endedAt,omitempty"`
	Stages      []RunStage     `json:"stages,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// RunStage is a single stage within a run.
type RunStage struct {
	Name           string      `json:"name"`
	Role           string      `json:"role,omitempty"`
	Phase          string      `json:"phase,omitempty"`
	JobTitle       string      `json:"jobTitle,omitempty"`
	Responsibility string      `json:"responsibility,omitempty"`
	Status         StageStatus `json:"status"`
	StartedAt      *time.Time  `json:"startedAt,omitempty"`
	EndedAt        *time.Time  `json:"endedAt,omitempty"`
	Usage          *TokenUsage `json:"usage,omitempty"`
	Error          string      `json:"error,omitempty"`
}

// PromptTrace records the full prompt/response for a single LLM call.
type PromptTrace struct {
	RunID         string      `json:"runId"`
	StageName     string      `json:"stageName"`
	Role          string      `json:"role,omitempty"`
	PromptProfile string      `json:"promptProfile,omitempty"`
	SystemPrompt  string      `json:"systemPrompt,omitempty"`
	UserPrompt    string      `json:"userPrompt"`
	OutputSchema  string      `json:"outputSchema,omitempty"`
	ResponseText  string      `json:"responseText,omitempty"`
	Usage         *TokenUsage `json:"usage,omitempty"`
	Error         string      `json:"error,omitempty"`
}

// TokenUsage records token consumption for an LLM call.
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}
