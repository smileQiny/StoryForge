package model

// ContextPackage is the compiled context bundle for a chapter, output of Composer.
type ContextPackage struct {
	Chapter         int             `json:"chapter"`
	SelectedContext []ContextSource `json:"selectedContext"`
	TokenBudgetUsed int             `json:"tokenBudgetUsed"`
	ExcludedSources []string        `json:"excludedSources,omitempty"`
}

// ContextSource is a single piece of context selected for injection.
type ContextSource struct {
	Kind      string  `json:"kind"` // summary/hook/fact/matrix/state/bible/rules
	Label     string  `json:"label"`
	Content   string  `json:"content"`
	Relevance float64 `json:"relevance"`
	Chapter   int     `json:"chapter,omitempty"`
}

// RuleStack is the compiled rule set for a chapter.
type RuleStack struct {
	Layers        []RuleLayer       `json:"layers"`
	Sections      RuleStackSections `json:"sections"`
	OverrideEdges []OverrideEdge    `json:"overrideEdges,omitempty"`
}

// RuleLayer is a named, prioritized layer of rules.
type RuleLayer struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Rules    []Rule `json:"rules"`
}

// RuleStackSections groups rules by enforcement level.
type RuleStackSections struct {
	Hard       []string `json:"hard,omitempty"`
	Soft       []string `json:"soft,omitempty"`
	Diagnostic []string `json:"diagnostic,omitempty"`
}

// OverrideEdge records a rule override relationship between layers.
type OverrideEdge struct {
	HigherLayer string `json:"higherLayer"`
	LowerLayer  string `json:"lowerLayer"`
	Reason      string `json:"reason,omitempty"`
}

// Rule is a single writing constraint.
type Rule struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Severity string `json:"severity"` // hard/soft/diagnostic
	Source   string `json:"source"`   // base/genre/book/intent/runtime
}

// ChapterTrace is the trace of context selection decisions for a chapter.
type ChapterTrace struct {
	Chapter         int      `json:"chapter"`
	MemoryQueryTerms []string `json:"memoryQueryTerms,omitempty"`
	RecalledSources []string `json:"recalledSources,omitempty"`
	InjectedSources []string `json:"injectedSources,omitempty"`
	ExcludedSources []string `json:"excludedSources,omitempty"`
	TokenBudget     int      `json:"tokenBudget"`
	TokenUsed       int      `json:"tokenUsed"`
}
