package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"storyforge/internal/agent"
	"storyforge/internal/app"
	"storyforge/internal/llm"
	"storyforge/internal/logging"
	"storyforge/internal/model"
)

const (
	doctorPingTimeoutDefault   = 5 * time.Second
	doctorPingTimeoutResponses = 12 * time.Second
)

type radarDoctorHandler struct {
	logger   *slog.Logger
	repoRoot string
	dataDir  string
	books    *app.BooksService
	chapters *app.ChaptersService
	config   *app.ConfigService
}

type doctorChecks struct {
	ConfigJSON    bool                 `json:"configJson"`
	ProjectEnv    bool                 `json:"projectEnv"`
	GlobalEnv     bool                 `json:"globalEnv"`
	BooksDir      bool                 `json:"booksDir"`
	LLMConnected  bool                 `json:"llmConnected"`
	BookCount     int                  `json:"bookCount"`
	Provider      string               `json:"provider,omitempty"`
	Model         string               `json:"model,omitempty"`
	ActiveProfile string               `json:"activeProfile,omitempty"`
	Profiles      []doctorProfileCheck `json:"profiles,omitempty"`
}

type doctorProfileCheck struct {
	Name       string `json:"name"`
	Language   string `json:"language,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	BaseURL    string `json:"baseUrl,omitempty"`
	IsDefault  bool   `json:"isDefault"`
	Configured bool   `json:"configured"`
	Connected  bool   `json:"connected"`
	Error      string `json:"error,omitempty"`
}

type radarResult struct {
	MarketSummary   string                `json:"marketSummary"`
	Recommendations []radarRecommendation `json:"recommendations"`
	Source          string                `json:"source,omitempty"`
}

type radarRecommendation struct {
	Confidence      float64  `json:"confidence"`
	Platform        string   `json:"platform"`
	Genre           string   `json:"genre"`
	Concept         string   `json:"concept"`
	Reasoning       string   `json:"reasoning"`
	BenchmarkTitles []string `json:"benchmarkTitles"`
}

type radarLibraryContext struct {
	Books         []*model.BookConfig
	GenreCounts   map[string]int
	PlatformCount map[string]int
	ChapterNotes  []string
	LogNotes      []string
}

func newRadarDoctorHandler(
	logger *slog.Logger,
	repoRoot string,
	dataDir string,
	books *app.BooksService,
	chapters *app.ChaptersService,
	config *app.ConfigService,
) *radarDoctorHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &radarDoctorHandler{
		logger:   logger,
		repoRoot: repoRoot,
		dataDir:  dataDir,
		books:    books,
		chapters: chapters,
		config:   config,
	}
}

func (h *radarDoctorHandler) doctor(w http.ResponseWriter, r *http.Request) {
	checks := h.collectDoctorChecks(r.Context())
	writeJSON(w, http.StatusOK, checks)
}

func (h *radarDoctorHandler) scan(w http.ResponseWriter, r *http.Request) {
	result, err := h.runRadarScan(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *radarDoctorHandler) collectDoctorChecks(ctx context.Context) doctorChecks {
	checks := doctorChecks{
		ConfigJSON: fileExists(filepath.Join(h.dataDir, "config.json")),
		ProjectEnv: fileExists(filepath.Join(h.repoRoot, ".env")),
		GlobalEnv:  fileExists(globalStoryForgeEnvPath()),
		BooksDir:   dirExists(h.dataDir),
	}

	books, err := h.books.List()
	if err == nil {
		checks.BookCount = len(books)
	}

	cfg, err := h.config.Get()
	if err != nil || cfg == nil {
		return checks
	}

	hydrated := hydrateProjectConfig(*cfg)
	checks.Provider = hydrated.LLM.Provider
	checks.Model = hydrated.LLM.Model
	checks.ActiveProfile = hydrated.DefaultLLMProfile
	checks.Profiles = h.collectProfileChecks(ctx, hydrated)
	for _, profile := range checks.Profiles {
		if profile.IsDefault {
			checks.LLMConnected = profile.Connected
			if checks.Provider == "" {
				checks.Provider = profile.Provider
			}
			if checks.Model == "" {
				checks.Model = profile.Model
			}
			break
		}
	}
	if checks.ActiveProfile == "" && len(checks.Profiles) > 0 {
		checks.ActiveProfile = checks.Profiles[0].Name
		checks.LLMConnected = checks.Profiles[0].Connected
	}
	return checks
}

func (h *radarDoctorHandler) collectProfileChecks(ctx context.Context, cfg model.ProjectConfig) []doctorProfileCheck {
	profiles := cfg.LLMProfiles
	if len(profiles) == 0 {
		profiles = []model.LLMProfile{{
			Name:     model.DefaultLLMProfileName,
			Language: cfg.Language,
			Provider: cfg.LLM.Provider,
			Model:    cfg.LLM.Model,
			BaseURL:  cfg.LLM.BaseURL,
			APIKey:   cfg.LLM.APIKey,
		}}
	}
	results := make([]doctorProfileCheck, len(profiles))
	var wg sync.WaitGroup
	for i, profile := range profiles {
		wg.Add(1)
		go func(index int, profile model.LLMProfile) {
			defer wg.Done()
			results[index] = buildDoctorProfileCheck(ctx, cfg, profile)
		}(i, profile)
	}
	wg.Wait()
	return results
}

func (h *radarDoctorHandler) runRadarScan(ctx context.Context) (*radarResult, error) {
	library, err := h.buildRadarLibraryContext()
	if err != nil {
		return nil, err
	}

	cfg, err := h.config.Get()
	if err != nil || cfg == nil {
		result := buildFallbackRadarResult(library)
		result.Source = "fallback"
		return result, nil
	}

	hydrated := hydrateProjectConfig(*cfg)
	result, err := h.runLLMRadar(ctx, library, hydrated)
	if err == nil && result != nil {
		result.Source = "llm"
		return result, nil
	}
	if err != nil {
		h.logger.Warn("radar scan fell back to heuristic mode", "component", "radar", "error", err.Error())
	}
	fallback := buildFallbackRadarResult(library)
	fallback.Source = "fallback"
	return fallback, nil
}

func (h *radarDoctorHandler) buildRadarLibraryContext() (*radarLibraryContext, error) {
	books, err := h.books.List()
	if err != nil {
		return nil, err
	}

	ctx := &radarLibraryContext{
		Books:         books,
		GenreCounts:   map[string]int{},
		PlatformCount: map[string]int{},
	}

	for _, book := range books {
		if genre := strings.TrimSpace(book.Genre); genre != "" {
			ctx.GenreCounts[genre]++
		}
		platform := strings.TrimSpace(book.Platform)
		if platform == "" {
			platform = "serial-web"
		}
		ctx.PlatformCount[platform]++

		metas, err := h.chapters.List(book.ID)
		if err != nil {
			continue
		}
		for _, meta := range metas {
			if len(ctx.ChapterNotes) >= 6 {
				break
			}
			content, _ := h.chapters.GetContent(book.ID, meta.Number)
			ctx.ChapterNotes = append(ctx.ChapterNotes, fmt.Sprintf(
				"%s 第%d章《%s》: %s",
				book.Title,
				meta.Number,
				firstNonEmpty(meta.Title, fmt.Sprintf("Chapter %d", meta.Number)),
				compactText(content, 180),
			))
		}
	}

	for _, entry := range logging.Recent(12, "") {
		if msg := strings.TrimSpace(entry.Message); msg != "" {
			ctx.LogNotes = append(ctx.LogNotes, compactText(msg, 120))
		}
	}
	return ctx, nil
}

func (h *radarDoctorHandler) runLLMRadar(
	ctx context.Context,
	library *radarLibraryContext,
	cfg model.ProjectConfig,
) (*radarResult, error) {
	agentCfg := resolveAgentLLMConfig(cfg, "radar")
	if !canPingLLMConfig(agentCfg) {
		return nil, fmt.Errorf("radar llm config incomplete")
	}

	router, err := llm.BuildFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	promptContext := buildRadarPromptContext(library)
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	resp, err := router.For("radar").Chat(reqCtx, llm.ChatRequest{
		Model: agentCfg.Model,
		Messages: []llm.Message{
			{
				Role: "system",
				Content: "You are the StoryForge market radar for serialized fiction. " +
					"Return only JSON with shape {\"marketSummary\":string,\"recommendations\":[{\"confidence\":number,\"platform\":string,\"genre\":string,\"concept\":string,\"reasoning\":string,\"benchmarkTitles\":string[]}]}." +
					" Focus on actionable opportunities for online fiction production.",
			},
			{
				Role:    "user",
				Content: promptContext,
			},
		},
		MaxTokens:   900,
		Temperature: 0.7,
	})
	if err != nil {
		return nil, err
	}

	var result radarResult
	if err := json.Unmarshal([]byte(agent.ExtractJSON(resp.Content)), &result); err != nil {
		return nil, fmt.Errorf("decode radar response: %w", err)
	}
	normalizeRadarResult(&result, library)
	if strings.TrimSpace(result.MarketSummary) == "" || len(result.Recommendations) == 0 {
		return nil, fmt.Errorf("empty radar payload")
	}
	return &result, nil
}

func buildDoctorProfileCheck(ctx context.Context, cfg model.ProjectConfig, profile model.LLMProfile) doctorProfileCheck {
	llmCfg := model.ResolveAgentLLMConfig(model.ProjectConfig{
		LLM:               cfg.LLM,
		LLMProfiles:       []model.LLMProfile{profile},
		DefaultLLMProfile: profile.Name,
	}, "")
	llmCfg = hydrateLLMConfig(llmCfg)

	check := doctorProfileCheck{
		Name:       profile.Name,
		Language:   profile.Language,
		Provider:   llmCfg.Provider,
		Model:      llmCfg.Model,
		BaseURL:    llmCfg.BaseURL,
		IsDefault:  strings.TrimSpace(profile.Name) == strings.TrimSpace(cfg.DefaultLLMProfile),
		Configured: canPingLLMConfig(llmCfg),
	}
	if !check.Configured {
		check.Error = "missing model or credentials/baseUrl"
		return check
	}

	ok, err := pingLLMConfig(ctx, llmCfg)
	check.Connected = ok
	if err != nil {
		check.Error = err.Error()
	}
	return check
}

func profileForTestProbe(profile model.LLMProfile) model.LLMProfile {
	probe := profile
	switch strings.ToLower(strings.TrimSpace(probe.Provider)) {
	case "claude":
		probe.SkipTLSVerify = true
	}
	return probe
}

func pingLLMConfig(ctx context.Context, cfg model.LLMConfig) (bool, error) {
	router, err := llm.BuildFromConfig(model.ProjectConfig{LLM: cfg})
	if err != nil {
		return false, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, doctorPingTimeout(cfg))
	defer cancel()

	_, err = router.For("doctor").Chat(reqCtx, llm.ChatRequest{
		Model: cfg.Model,
		Messages: []llm.Message{
			{Role: "user", Content: "ping"},
		},
		MaxTokens: 5,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func doctorPingTimeout(cfg model.LLMConfig) time.Duration {
	if strings.EqualFold(strings.TrimSpace(cfg.WireAPI), model.WireAPIResponses) {
		return doctorPingTimeoutResponses
	}
	return doctorPingTimeoutDefault
}

func canPingLLMConfig(cfg model.LLMConfig) bool {
	return strings.TrimSpace(cfg.Model) != "" && (strings.TrimSpace(cfg.APIKey) != "" || strings.TrimSpace(cfg.BaseURL) != "")
}

func buildRadarPromptContext(library *radarLibraryContext) string {
	var sections []string
	sections = append(sections, fmt.Sprintf("Books: %d", len(library.Books)))
	if genres := formatCountLines(library.GenreCounts); genres != "" {
		sections = append(sections, "Genres:\n"+genres)
	}
	if platforms := formatCountLines(library.PlatformCount); platforms != "" {
		sections = append(sections, "Platforms:\n"+platforms)
	}
	if len(library.Books) > 0 {
		var lines []string
		for _, book := range library.Books {
			lines = append(lines, fmt.Sprintf("- %s | genre=%s | platform=%s | status=%s", book.Title, book.Genre, firstNonEmpty(book.Platform, "serial-web"), book.Status))
		}
		sections = append(sections, "Current library:\n"+strings.Join(lines, "\n"))
	}
	if len(library.ChapterNotes) > 0 {
		sections = append(sections, "Recent chapter samples:\n- "+strings.Join(library.ChapterNotes, "\n- "))
	}
	if len(library.LogNotes) > 0 {
		sections = append(sections, "Recent ops signals:\n- "+strings.Join(library.LogNotes, "\n- "))
	}
	sections = append(sections, "Return 3 recommendations. Confidence must be between 0 and 1. Benchmark titles may reuse the current library if helpful.")
	return strings.Join(sections, "\n\n")
}

func buildFallbackRadarResult(library *radarLibraryContext) *radarResult {
	genres := topKeys(library.GenreCounts, 3)
	platforms := topKeys(library.PlatformCount, 3)
	benchmarks := benchmarkTitles(library.Books)

	if len(genres) == 0 {
		genres = []string{"xuanhuan", "urban", "romance"}
	}
	if len(platforms) == 0 {
		platforms = []string{"serial-web", "qidian", "fanfic"}
	}

	recommendations := make([]radarRecommendation, 0, 3)
	for i := 0; i < 3; i++ {
		genre := genres[min(i, len(genres)-1)]
		platform := platforms[min(i, len(platforms)-1)]
		confidence := 0.82 - (float64(i) * 0.12)
		recommendations = append(recommendations, radarRecommendation{
			Confidence: confidence,
			Platform:   platform,
			Genre:      genre,
			Concept:    fmt.Sprintf("%s 方向的高钩子长篇连载", strings.ToUpper(genre)),
			Reasoning: fmt.Sprintf(
				"当前书库里 %s 题材和 %s 平台出现频率较高，适合继续放大强冲突开篇、明确升级目标和连续章节钩子。",
				genre,
				platform,
			),
			BenchmarkTitles: benchmarks,
		})
	}

	summary := "当前市场雷达采用启发式回退模式。"
	if len(library.Books) == 0 {
		summary += " 书库还为空，建议先验证 2 到 3 个高概念题材切口，再决定主线投入。"
	} else {
		summary += fmt.Sprintf(" 已检测到 %d 本书，题材分布以 %s 为主，建议优先加深已有高势能赛道并提高章节钩子密度。", len(library.Books), strings.Join(genres, " / "))
	}

	return &radarResult{
		MarketSummary:   summary,
		Recommendations: recommendations,
	}
}

func normalizeRadarResult(result *radarResult, library *radarLibraryContext) {
	result.MarketSummary = strings.TrimSpace(result.MarketSummary)
	if len(result.Recommendations) > 3 {
		result.Recommendations = result.Recommendations[:3]
	}
	fallbackBenchmarks := benchmarkTitles(library.Books)
	for i := range result.Recommendations {
		rec := &result.Recommendations[i]
		rec.Confidence = clampConfidence(rec.Confidence)
		rec.Platform = firstNonEmpty(strings.TrimSpace(rec.Platform), "serial-web")
		rec.Genre = firstNonEmpty(strings.TrimSpace(rec.Genre), "general")
		rec.Concept = firstNonEmpty(strings.TrimSpace(rec.Concept), fmt.Sprintf("%s serial concept", rec.Genre))
		rec.Reasoning = firstNonEmpty(strings.TrimSpace(rec.Reasoning), "Use the current library and recent operations as the benchmark for the next production slot.")
		if len(rec.BenchmarkTitles) == 0 {
			rec.BenchmarkTitles = fallbackBenchmarks
		}
	}
}

func hydrateProjectConfig(cfg model.ProjectConfig) model.ProjectConfig {
	cfg.LLM = hydrateLLMConfig(cfg.LLM)
	for i := range cfg.LLMProfiles {
		profile := &cfg.LLMProfiles[i]
		hydrated := hydrateLLMConfig(model.LLMConfig{
			Provider:      profile.Provider,
			Model:         profile.Model,
			BaseURL:       profile.BaseURL,
			APIKey:        profile.APIKey,
			SkipTLSVerify: profile.SkipTLSVerify,
			Stream:        profile.Stream,
			Temperature:   profile.Temperature,
			MaxTokens:     profile.MaxTokens,
		})
		profile.Provider = hydrated.Provider
		profile.Model = hydrated.Model
		profile.BaseURL = hydrated.BaseURL
		profile.APIKey = hydrated.APIKey
		profile.SkipTLSVerify = hydrated.SkipTLSVerify
		profile.Stream = hydrated.Stream
		profile.Temperature = hydrated.Temperature
		profile.MaxTokens = hydrated.MaxTokens
	}
	cfg.LLM = model.ResolveDefaultLLMConfig(cfg)
	return cfg
}

func hydrateLLMConfig(cfg model.LLMConfig) model.LLMConfig {
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

func resolveAgentLLMConfig(cfg model.ProjectConfig, agentName string) model.LLMConfig {
	return model.ResolveAgentLLMConfig(cfg, agentName)
}

func formatCountLines(values map[string]int) string {
	keys := topKeys(values, len(values))
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("- %s: %d", key, values[key]))
	}
	return strings.Join(lines, "\n")
}

func topKeys(values map[string]int, limit int) []string {
	type pair struct {
		Key   string
		Count int
	}
	pairs := make([]pair, 0, len(values))
	for key, count := range values {
		pairs = append(pairs, pair{Key: key, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Key < pairs[j].Key
		}
		return pairs[i].Count > pairs[j].Count
	})
	if limit > len(pairs) {
		limit = len(pairs)
	}
	keys := make([]string, 0, limit)
	for _, item := range pairs[:limit] {
		keys = append(keys, item.Key)
	}
	return keys
}

func benchmarkTitles(books []*model.BookConfig) []string {
	if len(books) == 0 {
		return nil
	}
	copied := append([]*model.BookConfig(nil), books...)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].UpdatedAt.After(copied[j].UpdatedAt)
	})
	titles := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, book := range copied {
		title := strings.TrimSpace(book.Title)
		if title == "" || seen[title] {
			continue
		}
		seen[title] = true
		titles = append(titles, title)
		if len(titles) == 3 {
			break
		}
	}
	return titles
}

func compactText(text string, limit int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if clean == "" {
		return "暂无有效样本"
	}
	runes := []rune(clean)
	if len(runes) <= limit {
		return clean
	}
	return string(runes[:limit]) + "..."
}

func clampConfidence(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func globalStoryForgeEnvPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".storyforge", ".env")
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
