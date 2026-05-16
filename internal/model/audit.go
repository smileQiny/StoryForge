package model

// AuditDimensionTier classifies audit dimensions by scope.
type AuditDimensionTier string

const (
	TierCore  AuditDimensionTier = "core"
	TierGenre AuditDimensionTier = "genre"
	TierBook  AuditDimensionTier = "book"
)

// AuditDimensionDef defines a single audit dimension.
type AuditDimensionDef struct {
	ID        int                `json:"id"`
	Key       string             `json:"key"`
	NameZH    string             `json:"nameZh"`
	NameEN    string             `json:"nameEn"`
	Tier      AuditDimensionTier `json:"tier"`
	GenreReq  []string           `json:"genreReq,omitempty"` // genre flags required to activate
	DefaultOn bool               `json:"defaultOn"`
	Severity  string             `json:"severity"` // critical/warning/info
}

// CoreAuditDimensions are the 11 dimensions always active for all books.
var CoreAuditDimensions = []AuditDimensionDef{
	{ID: 1, Key: "continuity", NameZH: "连续性", NameEN: "Continuity", Tier: TierCore, DefaultOn: true, Severity: "critical"},
	{ID: 2, Key: "character_consistency", NameZH: "角色一致性", NameEN: "Character Consistency", Tier: TierCore, DefaultOn: true, Severity: "critical"},
	{ID: 3, Key: "plot_logic", NameZH: "情节逻辑", NameEN: "Plot Logic", Tier: TierCore, DefaultOn: true, Severity: "critical"},
	{ID: 4, Key: "hook_progress", NameZH: "伏笔推进", NameEN: "Hook Progress", Tier: TierCore, DefaultOn: true, Severity: "warning"},
	{ID: 5, Key: "word_count", NameZH: "字数达标", NameEN: "Word Count", Tier: TierCore, DefaultOn: true, Severity: "warning"},
	{ID: 6, Key: "ai_trace", NameZH: "AI 痕迹", NameEN: "AI Trace", Tier: TierCore, DefaultOn: true, Severity: "warning"},
	{ID: 7, Key: "cross_chapter_emotion", NameZH: "跨章情绪", NameEN: "Cross-Chapter Emotion", Tier: TierCore, DefaultOn: true, Severity: "info"},
	{ID: 8, Key: "title_clustering", NameZH: "标题聚集", NameEN: "Title Clustering", Tier: TierCore, DefaultOn: true, Severity: "info"},
	{ID: 9, Key: "ending_repetition", NameZH: "结尾重复", NameEN: "Ending Repetition", Tier: TierCore, DefaultOn: true, Severity: "warning"},
	{ID: 10, Key: "pacing", NameZH: "节奏", NameEN: "Pacing", Tier: TierCore, DefaultOn: true, Severity: "info"},
	{ID: 11, Key: "scene_grounding", NameZH: "场景落地", NameEN: "Scene Grounding", Tier: TierCore, DefaultOn: true, Severity: "warning"},
}

// GenreAuditDimensions are dimensions activated by specific genre flags.
var GenreAuditDimensions = []AuditDimensionDef{
	{ID: 12, Key: "numerical", NameZH: "数值系统", NameEN: "Numerical System", Tier: TierGenre, GenreReq: []string{"litrpg"}, DefaultOn: false, Severity: "critical"},
	{ID: 13, Key: "power_scaling", NameZH: "战力标尺", NameEN: "Power Scaling", Tier: TierGenre, GenreReq: []string{"litrpg", "xianxia", "xuanhuan"}, DefaultOn: false, Severity: "warning"},
	{ID: 14, Key: "system_notification", NameZH: "系统通知格式", NameEN: "System Notification Format", Tier: TierGenre, GenreReq: []string{"litrpg", "system-apocalypse"}, DefaultOn: false, Severity: "warning"},
	{ID: 15, Key: "cultivation_realm", NameZH: "修炼境界", NameEN: "Cultivation Realm", Tier: TierGenre, GenreReq: []string{"xianxia", "cultivation"}, DefaultOn: false, Severity: "critical"},
	{ID: 16, Key: "dungeon_mechanics", NameZH: "地下城机制", NameEN: "Dungeon Mechanics", Tier: TierGenre, GenreReq: []string{"dungeon-core"}, DefaultOn: false, Severity: "warning"},
	{ID: 17, Key: "isekai_rules", NameZH: "异世界规则", NameEN: "Isekai Rules", Tier: TierGenre, GenreReq: []string{"isekai"}, DefaultOn: false, Severity: "info"},
}

// FanficAuditDimensions are the 6 dimensions activated when fanficMode is set.
var FanficAuditDimensions = []AuditDimensionDef{
	{ID: 18, Key: "canon_divergence", NameZH: "正典分岔点", NameEN: "Canon Divergence", Tier: TierBook, DefaultOn: false, Severity: "critical"},
	{ID: 19, Key: "original_plot_copy", NameZH: "原作情节复述", NameEN: "Original Plot Copy", Tier: TierBook, DefaultOn: false, Severity: "critical"},
	{ID: 20, Key: "character_ooc", NameZH: "角色出戏", NameEN: "Character OOC", Tier: TierBook, DefaultOn: false, Severity: "warning"},
	{ID: 21, Key: "world_rule_violation", NameZH: "世界规则违反", NameEN: "World Rule Violation", Tier: TierBook, DefaultOn: false, Severity: "critical"},
	{ID: 22, Key: "canon_character_respect", NameZH: "正典角色尊重", NameEN: "Canon Character Respect", Tier: TierBook, DefaultOn: false, Severity: "warning"},
	{ID: 23, Key: "fanfic_originality", NameZH: "同人原创性", NameEN: "Fanfic Originality", Tier: TierBook, DefaultOn: false, Severity: "warning"},
}

// AuditReport is the output of the Auditor agent.
type AuditReport struct {
	Chapter    int                    `json:"chapter"`
	Passed     bool                   `json:"passed"`
	Issues     []AuditIssue           `json:"issues,omitempty"`
	Dimensions []AuditDimensionResult `json:"dimensions"`
}

// AuditIssue is a single issue found during auditing.
type AuditIssue struct {
	Dimension  string `json:"dimension"`
	Severity   string `json:"severity"` // critical/warning/info
	Summary    string `json:"summary"`
	Evidence   string `json:"evidence,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

// AuditDimensionResult is the result for a single audit dimension.
type AuditDimensionResult struct {
	Key    string `json:"key"`
	Passed bool   `json:"passed"`
	Score  int    `json:"score"`
	Notes  string `json:"notes,omitempty"`
}

// HasCriticalIssues returns true if any critical issues exist.
func (r *AuditReport) HasCriticalIssues() bool {
	for _, issue := range r.Issues {
		if issue.Severity == "critical" {
			return true
		}
	}
	return false
}
