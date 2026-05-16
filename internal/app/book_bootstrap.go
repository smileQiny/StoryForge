package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"storyforge/internal/agent"
	"storyforge/internal/llm"
	"storyforge/internal/model"
	"storyforge/internal/store"
)

type foundationBundle struct {
	StoryBible   map[string]any
	Characters   map[string]any
	PlotOutline  map[string]any
	StyleGuide   map[string]any
	WritingBible map[string]any
	Review       foundationReviewSummary
	Source       string
}

type foundationReviewSummary struct {
	TotalScore      int              `json:"totalScore"`
	Passed          bool             `json:"passed"`
	OverallFeedback string           `json:"overallFeedback"`
	Scores          []map[string]any `json:"scores,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
}

const bootstrapFoundationTimeout = 10 * time.Minute

func newBootstrapFoundationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), bootstrapFoundationTimeout)
}

func (s *BooksService) bootstrapNewBook(book *model.BookConfig, brief string) error {
	bookDir := filepath.Join(s.dataDir, book.ID)
	foundation, err := s.buildFoundationBundle(book, brief)
	if err != nil {
		return err
	}

	if err := s.writeFoundationArtifacts(bookDir, foundation); err != nil {
		return err
	}
	state := buildInitialRuntimeState(book, brief, foundation)
	return s.writeInitialTruthFiles(book.ID, state)
}

func (s *BooksService) buildFoundationBundle(book *model.BookConfig, brief string) (foundationBundle, error) {
	brief = normalizeBookBrief(book, brief)
	bundle, err := s.generateFoundationWithLLM(book, brief)
	if err != nil {
		return foundationBundle{}, fmt.Errorf("book bootstrap requires a configured and reachable LLM: %w", err)
	}
	return bundle, nil
}

func (s *BooksService) generateFoundationWithLLM(book *model.BookConfig, brief string) (foundationBundle, error) {
	if s.config == nil {
		return foundationBundle{}, errors.New("project config service is unavailable")
	}
	cfg, err := s.config.Get()
	if err != nil {
		return foundationBundle{}, fmt.Errorf("load project config: %w", err)
	}
	if cfg == nil {
		return foundationBundle{}, errors.New("project config is missing")
	}
	*cfg = hydrateBootstrapProjectConfig(*cfg)

	architectCfg := resolveBootstrapAgentConfig(*cfg, "architect")
	reviewerCfg := resolveBootstrapAgentConfig(*cfg, "foundation_reviewer")
	if !canUseBootstrapLLM(architectCfg) || !canUseBootstrapLLM(reviewerCfg) {
		return foundationBundle{}, errors.New("architect/foundation_reviewer LLM profiles are incomplete")
	}

	router, err := llm.BuildFromConfig(*cfg)
	if err != nil {
		return foundationBundle{}, fmt.Errorf("build llm router: %w", err)
	}

	ctx, cancel := newBootstrapFoundationContext()
	defer cancel()

	architectBase, _, err := newValidatedBaseAgent(*cfg, router, "architect")
	if err != nil {
		return foundationBundle{}, fmt.Errorf("architect agent: %w", err)
	}
	reviewerBase, _, err := newValidatedBaseAgent(*cfg, router, "foundation_reviewer")
	if err != nil {
		return foundationBundle{}, fmt.Errorf("foundation reviewer agent: %w", err)
	}

	architect := agent.NewArchitect(architectBase)
	reviewer := agent.NewFoundationReviewer(reviewerBase)

	var reviewFeedback string
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		design, err := architect.Design(ctx, agent.ArchitectInput{
			Book:           *book,
			Brief:          brief,
			ReviewFeedback: reviewFeedback,
		})
		if err != nil {
			s.logger.Warn("architect foundation generation failed", "bookId", book.ID, "error", err.Error())
			return foundationBundle{}, fmt.Errorf("architect foundation generation failed: %w", err)
		}
		review, err := reviewer.Review(ctx, agent.FoundationReviewerInput{
			Book:      *book,
			Architect: design,
		})
		if err != nil {
			s.logger.Warn("foundation review failed", "bookId", book.ID, "error", err.Error())
			return foundationBundle{}, fmt.Errorf("foundation review failed: %w", err)
		}

		bundle := foundationBundle{
			StoryBible:   extractArchitectContent(design.WorldBuilding),
			Characters:   extractArchitectContent(design.Characters),
			PlotOutline:  extractArchitectContent(design.PlotOutline),
			StyleGuide:   extractArchitectContent(design.StyleGuide),
			WritingBible: extractArchitectContent(design.WritingBible),
			Review: foundationReviewSummary{
				TotalScore:      review.TotalScore,
				Passed:          review.Passed,
				OverallFeedback: strings.TrimSpace(review.OverallFeedback),
				Scores:          flattenFoundationScores(review.Scores),
				Metadata: map[string]any{
					"usage": map[string]any{
						"inputTokens":  safeUsage(review.Usage).InputTokens,
						"outputTokens": safeUsage(review.Usage).OutputTokens,
						"totalTokens":  safeUsage(review.Usage).TotalTokens,
					},
				},
			},
			Source: "llm",
		}

		if review.Passed {
			return normalizeFoundationBundle(book, brief, bundle), nil
		}
		reviewFeedback = review.OverallFeedback
		lastErr = fmt.Errorf("foundation review rejected bootstrap output: %s", strings.TrimSpace(review.OverallFeedback))
	}

	if lastErr != nil {
		return foundationBundle{}, lastErr
	}
	return foundationBundle{}, errors.New("foundation bootstrap failed without a passing review")
}

func (s *BooksService) writeFoundationArtifacts(bookDir string, foundation foundationBundle) error {
	storyDir := filepath.Join(bookDir, "story")
	foundationDir := filepath.Join(storyDir, "foundation")
	if err := os.MkdirAll(foundationDir, 0o755); err != nil {
		return err
	}

	files := map[string]any{
		"world_building.json": foundation.StoryBible,
		"characters.json":     foundation.Characters,
		"plot_outline.json":   foundation.PlotOutline,
		"style_guide.json":    foundation.StyleGuide,
		"writing_bible.json":  foundation.WritingBible,
		"review.json":         foundation.Review,
	}
	for name, value := range files {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(foundationDir, name), data, 0o644); err != nil {
			return err
		}
	}

	markdowns := map[string]string{
		"story_bible.md":    renderFoundationMarkdown("Story Bible", foundation.StoryBible),
		"characters.md":     renderFoundationMarkdown("Characters", foundation.Characters),
		"volume_outline.md": renderFoundationMarkdown("Volume Outline", foundation.PlotOutline),
		"style_guide.md":    renderFoundationMarkdown("Style Guide", foundation.StyleGuide),
		"book_rules.md":     renderFoundationMarkdown("Book Rules", foundation.WritingBible),
	}
	for name, content := range markdowns {
		if err := os.WriteFile(filepath.Join(storyDir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (s *BooksService) writeInitialTruthFiles(bookID string, state model.RuntimeState) error {
	if s.truth == nil {
		return nil
	}
	if err := s.truth.Write(bookID, store.TruthCurrentState, state.CurrentState); err != nil {
		return err
	}
	if err := s.truth.Write(bookID, store.TruthParticleLedger, state.ParticleLedger); err != nil {
		return err
	}
	if err := s.truth.Write(bookID, store.TruthPendingHooks, state.PendingHooks); err != nil {
		return err
	}
	if err := s.truth.Write(bookID, store.TruthChapterSummaries, state.ChapterSummaries); err != nil {
		return err
	}
	if err := s.truth.Write(bookID, store.TruthSubplotBoard, state.SubplotBoard); err != nil {
		return err
	}
	if err := s.truth.Write(bookID, store.TruthEmotionalArcs, state.EmotionalArcs); err != nil {
		return err
	}
	return s.truth.Write(bookID, store.TruthCharacterMatrix, state.CharacterMatrix)
}

func normalizeFoundationBundle(book *model.BookConfig, brief string, bundle foundationBundle) foundationBundle {
	if bundle.StoryBible == nil {
		bundle.StoryBible = map[string]any{}
	}
	if bundle.Characters == nil {
		bundle.Characters = map[string]any{}
	}
	if bundle.PlotOutline == nil {
		bundle.PlotOutline = map[string]any{}
	}
	if bundle.StyleGuide == nil {
		bundle.StyleGuide = map[string]any{}
	}
	if bundle.WritingBible == nil {
		bundle.WritingBible = map[string]any{}
	}
	bundle.StoryBible["title"] = firstBootstrapAny(bundle.StoryBible["title"], book.Title)
	bundle.StoryBible["genre"] = firstBootstrapAny(bundle.StoryBible["genre"], book.Genre)
	bundle.StoryBible["platform"] = firstBootstrapAny(bundle.StoryBible["platform"], firstBootstrapValue(book.Platform, "serial-web"))
	bundle.StoryBible["premise"] = firstBootstrapAny(bundle.StoryBible["premise"], brief)
	bundle.StyleGuide["summary"] = firstBootstrapAny(bundle.StyleGuide["summary"], "快节奏、可连载、以场景推进为先。")
	return bundle
}

func buildInitialRuntimeState(book *model.BookConfig, brief string, foundation foundationBundle) model.RuntimeState {
	foundationArtifacts := []map[string]any{
		{
			"key":            "storyBible",
			"title":          "基础世界观",
			"jobTitle":       "世界设定架构师",
			"responsibility": "定义这本书的世界底座",
			"backingFiles":   []string{"story/story_bible.md", "story/foundation/world_building.json"},
		},
		{
			"key":            "plotOutline",
			"title":          "卷纲",
			"jobTitle":       "长线剧情规划师",
			"responsibility": "规划长线推进节奏和关键转折",
			"backingFiles":   []string{"story/volume_outline.md", "story/foundation/plot_outline.json"},
		},
		{
			"key":            "writingBible",
			"title":          "规则",
			"jobTitle":       "写作规则制定者",
			"responsibility": "锁定写作边界、主角约束和题材规则",
			"backingFiles":   []string{"story/book_rules.md", "story/foundation/writing_bible.json"},
		},
		{
			"key":            "initialState",
			"title":          "初始状态",
			"jobTitle":       "初始运行态设定师",
			"responsibility": "给章节流水线一个可推进的起点",
			"backingFiles":   []string{"story/state/current_state.json"},
		},
		{
			"key":            "initialHooks",
			"title":          "初始 Hooks",
			"jobTitle":       "伏笔设计师",
			"responsibility": "设计初始悬念、伏笔和预期回收节奏",
			"backingFiles":   []string{"story/state/pending_hooks.json"},
		},
	}
	current := map[string]any{
		"book": map[string]any{
			"id":             book.ID,
			"title":          book.Title,
			"genre":          book.Genre,
			"platform":       firstBootstrapValue(book.Platform, "serial-web"),
			"language":       book.Language,
			"targetChapters": book.TargetChapters,
		},
		"foundation": map[string]any{
			"source":       foundation.Source,
			"brief":        brief,
			"storyBible":   foundation.StoryBible,
			"characters":   foundation.Characters,
			"plotOutline":  foundation.PlotOutline,
			"styleGuide":   foundation.StyleGuide,
			"writingBible": foundation.WritingBible,
			"review":       foundation.Review,
			"artifacts":    foundationArtifacts,
		},
		"styleProfile": map[string]any{
			"source":      foundation.Source,
			"summary":     foundation.StyleGuide["summary"],
			"fingerprint": map[string]any{},
			"guidance":    ensureStringSlice(foundation.StyleGuide["guidance"]),
		},
		"authorIntent":    brief,
		"currentFocus":    summarizeOpeningFocus(book, foundation),
		"lastBootstrapAt": time.Now().UTC().Format(time.RFC3339),
	}

	ledger := map[string]any{
		"coreConflict":   firstBootstrapAny(foundation.StoryBible["coreConflict"], brief),
		"worldAnchor":    firstBootstrapAny(foundation.StoryBible["worldAnchor"], book.Genre),
		"progressEngine": firstBootstrapAny(foundation.StoryBible["progressEngine"], "每章推进冲突与目标。"),
	}

	hooks := []model.HookRecord{
		{
			HookID:              "foundation-opening-hook",
			StartChapter:        1,
			Type:                "opening",
			Status:              model.HookStatusOpen,
			LastAdvancedChapter: 0,
			ExpectedPayoff:      summarizeOpeningFocus(book, foundation),
			PayoffTiming:        "opening-arc",
			SeedExcerpt:         compactBootstrapText(brief, 80),
		},
	}

	subplots := []model.SubplotState{
		{
			ID:       "main-line",
			Title:    summarizeOpeningFocus(book, foundation),
			Status:   "queued",
			Progress: 0,
		},
	}

	emotional := []model.EmotionalArcState{
		{
			CharacterID: "lead",
			Arc:         "survival-to-agency",
			Phase:       "setup",
		},
	}

	matrix := []model.CharacterMatrixEntry{
		{
			CharacterID: "lead",
			Knows: map[string]any{
				"goal": summarizeOpeningFocus(book, foundation),
			},
			Relations: map[string]any{
				"world": "still-learning",
			},
		},
	}

	return model.RuntimeState{
		CurrentState:     current,
		ParticleLedger:   ledger,
		PendingHooks:     hooks,
		ChapterSummaries: []model.ChapterSummaryRow{},
		SubplotBoard:     subplots,
		EmotionalArcs:    emotional,
		CharacterMatrix:  matrix,
	}
}

func extractArchitectContent(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err == nil {
		if content, ok := payload["content"].(map[string]any); ok {
			return content
		}
		if len(payload) > 0 {
			return payload
		}
	}
	var wrapper struct {
		Content map[string]any `json:"content"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Content != nil {
		return wrapper.Content
	}
	return map[string]any{}
}

func renderFoundationMarkdown(title string, value any) string {
	data, _ := json.MarshalIndent(value, "", "  ")
	return fmt.Sprintf("# %s\n\n```json\n%s\n```\n", title, string(data))
}

func flattenFoundationScores(scores []agent.FoundationScore) []map[string]any {
	if len(scores) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(scores))
	for _, score := range scores {
		rows = append(rows, map[string]any{
			"dimension": score.Dimension,
			"score":     score.Score,
			"feedback":  score.Feedback,
		})
	}
	return rows
}

func normalizeBookBrief(book *model.BookConfig, brief string) string {
	brief = strings.TrimSpace(brief)
	if brief != "" {
		return brief
	}
	return fmt.Sprintf("%s：一部 %s 题材、面向 %s 平台的长篇连载，目标 %d 章，每章约 %d 字。", book.Title, book.Genre, firstBootstrapValue(book.Platform, "serial-web"), maxInt(book.TargetChapters, 12), maxInt(book.ChapterWordCount, 3000))
}

func summarizeOpeningFocus(book *model.BookConfig, foundation foundationBundle) string {
	if opening, ok := foundation.PlotOutline["openingArc"].([]any); ok && len(opening) > 0 {
		if line, ok := opening[0].(string); ok && strings.TrimSpace(line) != "" {
			return line
		}
	}
	if opening := ensureStringSlice(foundation.PlotOutline["openingArc"]); len(opening) > 0 {
		return opening[0]
	}
	return fmt.Sprintf("在开篇迅速建立 %s 的核心冲突与升级目标。", book.Title)
}

func compactBootstrapText(text string, limit int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len([]rune(clean)) <= limit {
		return clean
	}
	return string([]rune(clean)[:limit]) + "..."
}

func ensureStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func hydrateBootstrapLLMConfig(cfg model.LLMConfig) model.LLMConfig {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return cfg
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "anthropic", "claude":
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	default:
		cfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	return cfg
}

func hydrateBootstrapProjectConfig(cfg model.ProjectConfig) model.ProjectConfig {
	cfg.LLM = hydrateBootstrapLLMConfig(cfg.LLM)
	for i := range cfg.LLMProfiles {
		profile := &cfg.LLMProfiles[i]
		hydrated := hydrateBootstrapLLMConfig(model.LLMConfig{
			Provider:       profile.Provider,
			Model:          profile.Model,
			BaseURL:        profile.BaseURL,
			APIKey:         profile.APIKey,
			WireAPI:        profile.WireAPI,
			Stream:         profile.Stream,
			Temperature:    profile.Temperature,
			MaxTokens:      profile.MaxTokens,
			ThinkingBudget: profile.ThinkingBudget,
		})
		profile.Provider = hydrated.Provider
		profile.Model = hydrated.Model
		profile.BaseURL = hydrated.BaseURL
		profile.APIKey = hydrated.APIKey
		profile.WireAPI = hydrated.WireAPI
		profile.Stream = hydrated.Stream
		profile.Temperature = hydrated.Temperature
		profile.MaxTokens = hydrated.MaxTokens
		profile.ThinkingBudget = hydrated.ThinkingBudget
	}
	cfg.LLM = model.ResolveDefaultLLMConfig(cfg)
	return cfg
}

func resolveBootstrapAgentConfig(cfg model.ProjectConfig, agentName string) model.LLMConfig {
	return model.ResolveAgentLLMConfig(cfg, agentName)
}

func canUseBootstrapLLM(cfg model.LLMConfig) bool {
	return strings.TrimSpace(cfg.Model) != "" && strings.TrimSpace(cfg.APIKey) != ""
}

func safeUsage(usage *model.TokenUsage) model.TokenUsage {
	if usage == nil {
		return model.TokenUsage{}
	}
	return *usage
}

func firstBootstrapValue(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstBootstrapAny(value any, fallback any) any {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return fallback
		}
		return typed
	case nil:
		return fallback
	default:
		return value
	}
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
