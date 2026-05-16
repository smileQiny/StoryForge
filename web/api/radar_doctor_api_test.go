package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"storyforge/internal/app"
	"storyforge/internal/model"
	"storyforge/internal/store"
)

type doctorResponse struct {
	ConfigJSON    *bool            `json:"configJson"`
	ProjectEnv    *bool            `json:"projectEnv"`
	GlobalEnv     *bool            `json:"globalEnv"`
	BooksDir      *bool            `json:"booksDir"`
	LLMConnected  *bool            `json:"llmConnected"`
	BookCount     *int             `json:"bookCount"`
	ActiveProfile string           `json:"activeProfile"`
	Profiles      []map[string]any `json:"profiles"`
}

type radarRecommendation struct {
	Confidence      float64  `json:"confidence"`
	Platform        string   `json:"platform"`
	Genre           string   `json:"genre"`
	Concept         string   `json:"concept"`
	Reasoning       string   `json:"reasoning"`
	BenchmarkTitles []string `json:"benchmarkTitles"`
}

type radarResponse struct {
	MarketSummary   string                `json:"marketSummary"`
	Recommendations []radarRecommendation `json:"recommendations"`
}

type openAIStub struct {
	server     *httptest.Server
	chatCalls  atomic.Int32
	modelCalls atomic.Int32
	model      string
	content    string
	delay      time.Duration
}

type anthropicStub struct {
	server    *httptest.Server
	chatCalls atomic.Int32
	model     string
	content   string
}

func newOpenAIStub(t *testing.T, modelName, content string) *openAIStub {
	t.Helper()

	stub := &openAIStub{
		model:   modelName,
		content: content,
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if stub.delay > 0 {
			time.Sleep(stub.delay)
		}

		switch r.Method {
		case http.MethodGet:
			stub.modelCalls.Add(1)
			fmt.Fprintf(w, `{"object":"list","data":[{"id":%q,"object":"model"}]}`, stub.model)
		default:
			stub.chatCalls.Add(1)
			if r.URL.Path == "/responses" {
				fmt.Fprintf(
					w,
					`{"id":"resp-test","model":%q,"output":[{"type":"message","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":4,"output_tokens":1,"total_tokens":5}}`,
					stub.model,
					stub.content,
				)
				return
			}
			fmt.Fprintf(
				w,
				`{"id":"chatcmpl-test","object":"chat.completion","created":%d,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`,
				time.Now().Unix(),
				stub.model,
				stub.content,
			)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func newDelayedOpenAIStub(t *testing.T, modelName, content string, delay time.Duration) *openAIStub {
	t.Helper()
	stub := newOpenAIStub(t, modelName, content)
	stub.delay = delay
	return stub
}

func (s *openAIStub) baseURL() string {
	return s.server.URL
}

func newAnthropicStub(t *testing.T, modelName, content string) *anthropicStub {
	t.Helper()

	stub := &anthropicStub{
		model:   modelName,
		content: content,
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stub.chatCalls.Add(1)
		fmt.Fprintf(w, `{"id":"msg-test","model":%q,"role":"assistant","content":[{"type":"text","text":%q}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`, stub.model, stub.content)
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *anthropicStub) baseURL() string {
	return s.server.URL
}

func writeProjectConfig(t *testing.T, dataDir string, llm model.LLMConfig) {
	t.Helper()

	cfg := model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            dataDir,
		MaxConcurrentBooks: 1,
		LLM:                llm,
	}
	writeFullProjectConfig(t, dataDir, cfg)
}

func writeFullProjectConfig(t *testing.T, dataDir string, cfg model.ProjectConfig) {
	t.Helper()

	cfg.DataDir = dataDir
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func mustCreateBook(t *testing.T, env *testEnv, bookID string) {
	t.Helper()
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, "POST", "/api/books", app.CreateBookInput{
		ID:               bookID,
		Title:            "Contract Test Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		Platform:         "qidian",
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func mustSeedChapter(t *testing.T, dataDir, bookID string, number int, content string) {
	t.Helper()

	now := time.Now().UTC()
	chapters := store.NewChapterStore(dataDir)
	if err := chapters.SaveMeta(bookID, &model.ChapterMeta{
		Number:    number,
		Title:     fmt.Sprintf("Chapter %d", number),
		Status:    model.ChapterStatusApproved,
		WordCount: len([]rune(content)),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save chapter meta: %v", err)
	}
	if err := chapters.SaveContent(bookID, number, content); err != nil {
		t.Fatalf("save chapter content: %v", err)
	}
}

func TestDoctor_DefaultStructuredChecks(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodGet, "/api/doctor", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("doctor: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got doctorResponse
	decodeJSON(t, w, &got)

	if got.ConfigJSON == nil || got.ProjectEnv == nil || got.GlobalEnv == nil || got.BooksDir == nil || got.LLMConnected == nil {
		t.Fatalf("doctor should return structured checks, got %+v", got)
	}
	if got.BookCount == nil {
		t.Fatalf("doctor should return bookCount, got %+v", got)
	}
	if *got.BookCount != 0 {
		t.Fatalf("expected empty test env to report bookCount=0, got %d", *got.BookCount)
	}
	if len(got.Profiles) == 0 {
		t.Fatalf("doctor should return at least one profile, got %+v", got)
	}
}

func TestDoctor_LLMConnectedWithStubOpenAI(t *testing.T) {
	env := newTestEnv(t)
	stub := newOpenAIStub(t, "doctor-stub-model", "pong")

	writeProjectConfig(t, env.dir, model.LLMConfig{
		Provider: "openai",
		Model:    "doctor-stub-model",
		BaseURL:  stub.baseURL(),
		APIKey:   "test-api-key",
	})

	w := do(t, env.handler, http.MethodGet, "/api/doctor", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("doctor llm connectivity: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got doctorResponse
	decodeJSON(t, w, &got)

	if got.LLMConnected == nil {
		t.Fatalf("doctor response missing llmConnected: %+v", got)
	}
	if !*got.LLMConnected {
		t.Fatalf("expected llmConnected=true when config points to reachable stub and apiKey exists, got %+v", got)
	}
	if stub.chatCalls.Load() == 0 && stub.modelCalls.Load() == 0 {
		t.Fatalf("expected doctor to exercise stub openai-compatible service")
	}
}

func TestDoctor_ResponsesProbeAllowsSlowCompatibleEndpoints(t *testing.T) {
	env := newTestEnv(t)
	stub := newDelayedOpenAIStub(t, "doctor-responses-model", "pong", 6*time.Second)

	writeProjectConfig(t, env.dir, model.LLMConfig{
		Provider: "openai",
		Model:    "doctor-responses-model",
		BaseURL:  stub.baseURL(),
		APIKey:   "test-api-key",
		WireAPI:  model.WireAPIResponses,
	})

	w := do(t, env.handler, http.MethodGet, "/api/doctor", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("doctor slow responses connectivity: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got doctorResponse
	decodeJSON(t, w, &got)

	if got.LLMConnected == nil || !*got.LLMConnected {
		t.Fatalf("expected llmConnected=true for a slow but healthy responses endpoint, got %+v", got)
	}
	if stub.chatCalls.Load() == 0 {
		t.Fatalf("expected doctor to hit the responses stub")
	}
}

func TestDoctor_ReportsProfilesAndSupportsCustomProvider(t *testing.T) {
	env := newTestEnv(t)
	openAICompat := newOpenAIStub(t, "compat-model", "pong")
	anthropicCompat := newAnthropicStub(t, "claude-test", "pong")

	writeFullProjectConfig(t, env.dir, model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		MaxConcurrentBooks: 1,
		DefaultLLMProfile:  "compat",
		LLM: model.LLMConfig{
			Provider: "custom",
			Model:    "compat-model",
			BaseURL:  openAICompat.baseURL(),
			APIKey:   "compat-key",
		},
		LLMProfiles: []model.LLMProfile{
			{
				Name:     "compat",
				Language: "zh",
				Provider: "custom",
				Model:    "compat-model",
				BaseURL:  openAICompat.baseURL(),
				APIKey:   "compat-key",
			},
			{
				Name:     "anthropic-native",
				Language: "en",
				Provider: "anthropic",
				Model:    "claude-test",
				BaseURL:  anthropicCompat.baseURL(),
				APIKey:   "anthropic-key",
			},
		},
		AgentLLMBindings: []model.AgentLLMBinding{
			{Agent: "writer", Profile: "compat"},
			{Agent: "auditor", Profile: "anthropic-native"},
		},
	})

	w := do(t, env.handler, http.MethodGet, "/api/doctor", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("doctor multi-profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got doctorResponse
	decodeJSON(t, w, &got)

	if got.ActiveProfile != "compat" {
		t.Fatalf("expected activeProfile=compat, got %+v", got.ActiveProfile)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("expected 2 profile checks, got %+v", got.Profiles)
	}
	if openAICompat.chatCalls.Load() == 0 {
		t.Fatalf("expected custom provider profile to hit openai-compatible stub")
	}
	if anthropicCompat.chatCalls.Load() == 0 {
		t.Fatalf("expected anthropic profile to hit anthropic stub")
	}
}

func TestRadar_ScanReturnsSummaryAndRecommendations(t *testing.T) {
	env := newTestEnv(t)
	mustCreateBook(t, env, "radar-book")
	mustSeedChapter(t, env.dir, "radar-book", 1, "少年在雨夜中踏入废弃神殿，发现命运裂缝。")

	stub := newOpenAIStub(
		t,
		"radar-stub-model",
		`{"marketSummary":"玄幻男频近期偏好高概念开场与明确升级钩子。","recommendations":[{"confidence":0.91,"platform":"qidian","genre":"xuanhuan","concept":"神殿裂缝流","reasoning":"高概念入口清晰，利于连载转化。","benchmarkTitles":["裂缝求生手册"]}]}`,
	)
	writeProjectConfig(t, env.dir, model.LLMConfig{
		Provider: "openai",
		Model:    "radar-stub-model",
		BaseURL:  stub.baseURL(),
		APIKey:   "test-api-key",
	})

	w := do(t, env.handler, http.MethodPost, "/api/radar/scan", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("radar scan: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got radarResponse
	decodeJSON(t, w, &got)

	if strings.TrimSpace(got.MarketSummary) == "" {
		t.Fatalf("expected marketSummary in radar response, got %+v", got)
	}
	if len(got.Recommendations) == 0 {
		t.Fatalf("expected recommendations in radar response, got %+v", got)
	}
	if strings.TrimSpace(got.Recommendations[0].Concept) == "" {
		t.Fatalf("expected first recommendation concept to be populated, got %+v", got.Recommendations[0])
	}
	if stub.chatCalls.Load() == 0 {
		t.Fatalf("expected radar scan to exercise stub openai-compatible service")
	}
}

func TestRadar_ScanReadsLatestConfigFromDisk(t *testing.T) {
	env := newTestEnv(t)
	mustCreateBook(t, env, "radar-reload-book")
	mustSeedChapter(t, env.dir, "radar-reload-book", 1, "第一章内容用于验证配置热读取。")

	stale := newOpenAIStub(
		t,
		"stale-radar-model",
		`{"marketSummary":"stale summary","recommendations":[{"confidence":0.35,"platform":"qidian","genre":"xuanhuan","concept":"stale concept","reasoning":"stale reasoning","benchmarkTitles":["stale title"]}]}`,
	)
	fresh := newOpenAIStub(
		t,
		"fresh-radar-model",
		`{"marketSummary":"fresh summary","recommendations":[{"confidence":0.88,"platform":"qidian","genre":"xuanhuan","concept":"fresh concept","reasoning":"fresh reasoning","benchmarkTitles":["fresh title"]}]}`,
	)

	writeProjectConfig(t, env.dir, model.LLMConfig{
		Provider: "openai",
		Model:    "stale-radar-model",
		BaseURL:  stale.baseURL(),
		APIKey:   "test-api-key",
	})

	first := do(t, env.handler, http.MethodPost, "/api/radar/scan", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first radar scan: expected 200, got %d: %s", first.Code, first.Body.String())
	}
	var gotFirst radarResponse
	decodeJSON(t, first, &gotFirst)
	if !strings.Contains(gotFirst.MarketSummary, "stale") {
		t.Fatalf("expected first radar response to reflect stale config, got %+v", gotFirst)
	}

	writeProjectConfig(t, env.dir, model.LLMConfig{
		Provider: "openai",
		Model:    "fresh-radar-model",
		BaseURL:  fresh.baseURL(),
		APIKey:   "test-api-key",
	})

	second := do(t, env.handler, http.MethodPost, "/api/radar/scan", nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second radar scan: expected 200, got %d: %s", second.Code, second.Body.String())
	}
	var gotSecond radarResponse
	decodeJSON(t, second, &gotSecond)

	if !strings.Contains(gotSecond.MarketSummary, "fresh") {
		t.Fatalf("expected second radar response to reflect fresh config from disk, got %+v", gotSecond)
	}
	if gotFirst.MarketSummary == gotSecond.MarketSummary {
		t.Fatalf("expected radar scan to reload config.json between requests, both responses were %+v", gotSecond)
	}
	if stale.chatCalls.Load() == 0 {
		t.Fatalf("expected first radar scan to hit stale stub")
	}
	if fresh.chatCalls.Load() == 0 {
		t.Fatalf("expected second radar scan to hit fresh stub after config overwrite")
	}
}
