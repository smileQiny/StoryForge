package model

// HookStatus represents the lifecycle state of a foreshadowing hook.
type HookStatus string

const (
	HookStatusOpen        HookStatus = "open"
	HookStatusProgressing HookStatus = "progressing"
	HookStatusDeferred    HookStatus = "deferred"
	HookStatusResolved    HookStatus = "resolved"
)

// ValidHookStatuses is the set of all valid hook statuses.
var ValidHookStatuses = map[HookStatus]bool{
	HookStatusOpen:        true,
	HookStatusProgressing: true,
	HookStatusDeferred:    true,
	HookStatusResolved:    true,
}

// HookRecord represents a single foreshadowing hook (伏笔).
type HookRecord struct {
	HookID              string     `json:"hookId"`
	StartChapter        int        `json:"startChapter"`
	Type                string     `json:"type"`
	Status              HookStatus `json:"status"`
	LastAdvancedChapter int        `json:"lastAdvancedChapter"`
	ExpectedPayoff      string     `json:"expectedPayoff"`
	PayoffTiming        string     `json:"payoffTiming,omitempty"`
	SeedExcerpt         string     `json:"seedExcerpt,omitempty"`
}

// HookOps describes mutations to apply to the hook list.
type HookOps struct {
	Advance  []HookAdvanceOp  `json:"advance,omitempty"`
	Resolve  []HookResolveOp  `json:"resolve,omitempty"`
	Defer    []HookDeferOp    `json:"defer,omitempty"`
}

// HookAdvanceOp advances a hook's progress.
type HookAdvanceOp struct {
	HookID  string `json:"hookId"`
	Chapter int    `json:"chapter"`
	Note    string `json:"note,omitempty"`
}

// HookResolveOp marks a hook as resolved.
type HookResolveOp struct {
	HookID  string `json:"hookId"`
	Chapter int    `json:"chapter"`
	Summary string `json:"summary,omitempty"`
}

// HookDeferOp defers a hook.
type HookDeferOp struct {
	HookID string `json:"hookId"`
	Reason string `json:"reason,omitempty"`
}

// NewHookCandidate is a proposed new hook to be admitted.
type NewHookCandidate struct {
	Type           string `json:"type"`
	Description    string `json:"description"`
	ExpectedPayoff string `json:"expectedPayoff"`
	PayoffTiming   string `json:"payoffTiming,omitempty"`
}

// ChapterSummaryRow is a single chapter's summary entry.
type ChapterSummaryRow struct {
	Chapter     int    `json:"chapter"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	HookUpdates string `json:"hookUpdates,omitempty"`
}

// CurrentStatePatch is a partial update to the current world state.
type CurrentStatePatch struct {
	CharacterUpdates  []map[string]any `json:"characterUpdates,omitempty"`
	LocationUpdates   []map[string]any `json:"locationUpdates,omitempty"`
	RelationshipOps   []map[string]any `json:"relationshipOps,omitempty"`
	KnowledgeOps      []map[string]any `json:"knowledgeOps,omitempty"`
}

// RuntimeState is the authoritative world state (maps to 7 truth files).
type RuntimeState struct {
	CurrentState    map[string]any   `json:"currentState"`
	ParticleLedger  map[string]any   `json:"particleLedger"`
	PendingHooks    []HookRecord     `json:"pendingHooks"`
	ChapterSummaries []ChapterSummaryRow `json:"chapterSummaries"`
	SubplotBoard    []SubplotState   `json:"subplotBoard"`
	EmotionalArcs   []EmotionalArcState `json:"emotionalArcs"`
	CharacterMatrix []CharacterMatrixEntry `json:"characterMatrix"`
}

// SubplotState tracks a subplot's progress.
type SubplotState struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

// EmotionalArcState tracks an emotional arc.
type EmotionalArcState struct {
	CharacterID string `json:"characterId"`
	Arc         string `json:"arc"`
	Phase       string `json:"phase"`
}

// CharacterMatrixEntry tracks character interactions and information boundaries.
type CharacterMatrixEntry struct {
	CharacterID string         `json:"characterId"`
	Knows       map[string]any `json:"knows,omitempty"`
	Relations   map[string]any `json:"relations,omitempty"`
}

// RuntimeStateDelta is the output of Reflector — a set of mutations to apply.
type RuntimeStateDelta struct {
	Chapter            int                `json:"chapter"`
	CurrentStatePatch  *CurrentStatePatch `json:"currentStatePatch,omitempty"`
	HookOps            HookOps            `json:"hookOps"`
	NewHookCandidates  []NewHookCandidate `json:"newHookCandidates,omitempty"`
	ChapterSummary     *ChapterSummaryRow `json:"chapterSummary,omitempty"`
	SubplotOps         []map[string]any   `json:"subplotOps,omitempty"`
	EmotionalArcOps    []map[string]any   `json:"emotionalArcOps,omitempty"`
	CharacterMatrixOps []map[string]any   `json:"characterMatrixOps,omitempty"`
}

// ObservedFact is a single fact extracted by the Observer agent.
type ObservedFact struct {
	Kind    string `json:"kind"` // character/location/event/hook/resource/relation/knowledge/emotion/subplot
	Subject string `json:"subject"`
	Content string `json:"content"`
	Chapter int    `json:"chapter"`
}
