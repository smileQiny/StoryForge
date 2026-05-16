package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	storyforge "storyforge"
	"storyforge/internal/app"
	applogging "storyforge/internal/logging"
	"storyforge/internal/model"
	"storyforge/internal/run"
	"storyforge/internal/state"
	"storyforge/internal/store"
	"storyforge/web/api"
)

type testEnv struct {
	handler http.Handler
	dir     string
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)
	return env.handler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir, err := os.MkdirTemp("", "storyforge-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	genresFS, err := storyforge.GenresFS()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := api.NewDefaultServices(dir, genresFS)
	handler, err := api.NewHandlerWithServices(applogging.NewLogger(slog.LevelInfo), svc, dir)
	if err != nil {
		t.Fatal(err)
	}
	return &testEnv{handler: handler, dir: dir}
}

func do(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, w.Body.String())
	}
}

func configurePipelineLLM(t *testing.T, dir string) {
	t.Helper()

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modelName, _ := body["model"].(string)
		stream, _ := body["stream"].(bool)

		if modelName == "architect-test" {
			if _, hasTools := body["tools"]; !hasTools {
				content := pipelineStubContent(modelName)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      "chatcmpl-architect-chat",
					"object":  "chat.completion",
					"created": time.Now().Unix(),
					"model":   modelName,
					"choices": []map[string]any{{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": content,
						},
						"finish_reason": "stop",
					}},
					"usage": map[string]any{
						"prompt_tokens":     12,
						"completion_tokens": 18,
						"total_tokens":      30,
					},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "chatcmpl-bootstrap-architect",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   modelName,
				"choices": []map[string]any{{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]any{
							{"id": "tool-1", "type": "function", "function": map[string]any{"name": "submit_world_building", "arguments": "{\"content\":{\"premise\":\"主角坠入失控地宫，被迫以试炼换生路。\",\"worldAnchor\":\"地宫与城邦势力围绕失序能源展开争夺。\",\"coreConflict\":\"主角必须在变强与失控之间找到边界。\"}}"}},
							{"id": "tool-2", "type": "function", "function": map[string]any{"name": "submit_characters", "arguments": "{\"content\":{\"lead\":[{\"name\":\"林渊\",\"goal\":\"活着走出地宫并查清失序能源真相\"}],\"supporting\":[{\"name\":\"沈瑜\",\"role\":\"表面合作、实则另藏任务\"}]}}"}},
							{"id": "tool-3", "type": "function", "function": map[string]any{"name": "submit_plot_outline", "arguments": "{\"content\":{\"openingArc\":[\"主角在开篇坠入地宫，并确认第一道试炼已经开始。\"],\"midArc\":[\"主角逐步发现城邦与地宫能源实验的联系。\"],\"payoff\":[\"失序能源真相在阶段高潮首次公开。\"]}}"}},
							{"id": "tool-4", "type": "function", "function": map[string]any{"name": "submit_style_guide", "arguments": "{\"content\":{\"summary\":\"高压推进，章节结尾必须留钩子。\",\"guidance\":[\"优先展示冲突升级\",\"避免解释性空话\"]}}"}},
							{"id": "tool-5", "type": "function", "function": map[string]any{"name": "submit_writing_bible", "arguments": "{\"content\":{\"mustKeep\":[\"升级必须伴随代价\"],\"mustAvoid\":[\"连续空转章节\"]}}"}},
						},
					},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{
					"prompt_tokens":     12,
					"completion_tokens": 18,
					"total_tokens":      30,
				},
			})
			return
		}

		content := pipelineStubContent(modelName)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, chunk := range chunkText(content, 180) {
				payload, _ := json.Marshal(map[string]any{
					"choices": []map[string]any{{
						"delta": map[string]any{"content": chunk},
					}},
				})
				fmt.Fprintf(w, "data: %s\n\n", payload)
			}
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-pipeline-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 18,
				"total_tokens":      30,
			},
		})
	}))
	t.Cleanup(stub.Close)

	baseProfile := func(name, modelName string) model.LLMProfile {
		return model.LLMProfile{
			Name:     name,
			Language: "zh",
			Provider: "openai",
			Model:    modelName,
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
			WireAPI:  "chat",
		}
	}

	cfg := model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            dir,
		MaxConcurrentBooks: 1,
		DefaultLLMProfile:  "generic",
		LLM: model.LLMConfig{
			Provider: "openai",
			Model:    "generic-test",
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
			WireAPI:  "chat",
		},
		LLMProfiles: []model.LLMProfile{
			baseProfile("generic", "generic-test"),
			baseProfile("architect", "architect-test"),
			baseProfile("foundation-reviewer", "foundation-reviewer-test"),
			baseProfile("planner", "planner-test"),
			baseProfile("writer", "writer-test"),
			baseProfile("observer", "observer-test"),
			baseProfile("reflector", "reflector-test"),
			baseProfile("auditor", "auditor-test"),
			baseProfile("reviser", "reviser-test"),
			baseProfile("normalizer", "normalizer-test"),
		},
		AgentLLMBindings: []model.AgentLLMBinding{
			{Agent: "architect", Profile: "architect"},
			{Agent: "foundation_reviewer", Profile: "foundation-reviewer"},
			{Agent: "planner", Profile: "planner"},
			{Agent: "writer", Profile: "writer"},
			{Agent: "observer", Profile: "observer"},
			{Agent: "reflector", Profile: "reflector"},
			{Agent: "auditor", Profile: "auditor"},
			{Agent: "reviser", Profile: "reviser"},
			{Agent: "normalizer", Profile: "normalizer"},
		},
	}
	if _, err := app.NewConfigService(dir).Update(cfg); err != nil {
		t.Fatalf("configure pipeline llm: %v", err)
	}
}

func pipelineStubContent(modelName string) string {
	switch modelName {
	case "planner-test":
		return `{"goal":"主角闯入地宫并确认试炼开始","sceneDirective":"压低环境光，突出机关压迫感","moodDirective":"紧张克制"}`
	case "foundation-reviewer-test":
		return `{"totalScore":93,"passed":true,"scores":[{"dimension":"plot_structure","score":93,"feedback":"good"}],"overallFeedback":"Strong foundation.","fanficDivergencePoint":"故事从主角拒绝原作终局选择的那一刻开始偏离。"}`
	case "writer-test", "normalizer-test", "reviser-test", "generic-test":
		return strings.Repeat("主角踏入地宫，火把映出潮湿石壁，脚步声在甬道里回荡，他知道真正的试炼才刚刚开始。", 40)
	case "observer-test":
		return `[{"kind":"event","subject":"主角","content":"踏入地宫并准备试炼","chapter":1}]`
	case "reflector-test":
		return `{"chapter":1,"chapterSummary":{"chapter":1,"title":"地宫试炼","summary":"主角进入地宫并确认第一道试炼开启。","hookUpdates":"试炼主线启动"}}`
	case "auditor-test":
		return `{"chapter":1,"passed":true,"issues":[],"dimensions":[{"key":"continuity","passed":true,"score":92,"notes":"ok"}]}`
	default:
		return `{}`
	}
}

func chunkText(text string, size int) []string {
	if size <= 0 || len(text) <= size {
		return []string{text}
	}
	chunks := make([]string, 0, (len(text)+size-1)/size)
	for len(text) > 0 {
		if len(text) <= size {
			chunks = append(chunks, text)
			break
		}
		chunks = append(chunks, text[:size])
		text = text[size:]
	}
	return chunks
}

func waitForRunCompletion(t *testing.T, h http.Handler, bookID, runID string, timeout time.Duration) model.Run {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		w := do(t, h, "GET", "/api/runs/"+runID+"?bookId="+bookID, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("get run: expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var runDetail model.Run
		decodeJSON(t, w, &runDetail)
		if runDetail.Status == model.RunStatusSucceeded || runDetail.Status == model.RunStatusFailed {
			return runDetail
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for run completion, last status %s", runDetail.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// --- /healthz ---

func TestHealthz(t *testing.T) {
	h := newTestHandler(t)
	w := do(t, h, "GET", "/healthz", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Books CRUD ---

func TestBooks_CreateListGetUpdateDelete(t *testing.T) {
	h := newTestHandler(t)

	// Create
	w := do(t, h, "POST", "/api/books", app.CreateBookInput{
		ID:               "book-test",
		Title:            "My Novel",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List
	w = do(t, h, "GET", "/api/books", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var books []*model.BookConfig
	decodeJSON(t, w, &books)
	if len(books) != 1 {
		t.Errorf("expected 1 book, got %d", len(books))
	}

	// Get
	w = do(t, h, "GET", "/api/books/book-test", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}
	var book model.BookConfig
	decodeJSON(t, w, &book)
	if book.Title != "My Novel" {
		t.Errorf("title mismatch: %q", book.Title)
	}

	// Update
	newTitle := "Updated Novel"
	w = do(t, h, "PUT", "/api/books/book-test", app.UpdateBookInput{Title: &newTitle})
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Delete
	w = do(t, h, "DELETE", "/api/books/book-test", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", w.Code)
	}

	// Get after delete
	w = do(t, h, "GET", "/api/books/book-test", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w.Code)
	}
}

// --- Truth files ---

func TestTruth_GetAllAndUpdate(t *testing.T) {
	h := newTestHandler(t)

	// Create book first
	do(t, h, "POST", "/api/books", app.CreateBookInput{
		ID: "b1", Title: "T", Genre: "xuanhuan", Language: model.LanguageZH, ChapterWordCount: 3000,
	})

	// Get all (empty)
	w := do(t, h, "GET", "/api/books/b1/truth", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get truth: expected 200, got %d", w.Code)
	}

	// Update a truth file
	payload := map[string]any{"characters": []string{"hero"}}
	w = do(t, h, "PUT", "/api/books/b1/truth/current_state.json", payload)
	if w.Code != http.StatusNoContent {
		t.Fatalf("update truth: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Get single file
	w = do(t, h, "GET", "/api/books/b1/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get truth file: expected 200, got %d", w.Code)
	}
}

// --- Pipeline trigger ---

func TestPipeline_Trigger(t *testing.T) {
	h := newTestHandler(t)
	do(t, h, "POST", "/api/books", app.CreateBookInput{
		ID: "b1", Title: "T", Genre: "xuanhuan", Language: model.LanguageZH, ChapterWordCount: 3000,
	})

	w := do(t, h, "POST", "/api/books/b1/plan", map[string]int{"chapter": 1})
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger plan: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, w, &resp)
	if resp["runId"] == "" {
		t.Error("expected runId in response")
	}
}

func TestBooks_GetDecodesNonASCIIBookIDPathParam(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	create := do(t, env.handler, http.MethodPost, "/api/books", map[string]any{
		"id":               "雾港停钟楼-20260422-0942",
		"title":            "雾港停钟楼-20260422-0942",
		"genre":            "suspense",
		"language":         "zh",
		"targetChapters":   10,
		"chapterWordCount": 8000,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create book status = %d, body=%s", create.Code, create.Body.String())
	}

	resp := do(t, env.handler, http.MethodGet, "/api/books/"+url.PathEscape("雾港停钟楼-20260422-0942"), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("get book status = %d, body=%s", resp.Code, resp.Body.String())
	}

	var book model.BookConfig
	decodeJSON(t, resp, &book)
	if book.ID != "雾港停钟楼-20260422-0942" {
		t.Fatalf("expected decoded book id, got %q", book.ID)
	}
}

func TestPipeline_TriggerWriteNextDecodesNonASCIIBookIDPathParam(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	create := do(t, env.handler, http.MethodPost, "/api/books", map[string]any{
		"id":               "雾港停钟楼-20260422-0942",
		"title":            "雾港停钟楼-20260422-0942",
		"genre":            "suspense",
		"language":         "zh",
		"targetChapters":   10,
		"chapterWordCount": 8000,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create book status = %d, body=%s", create.Code, create.Body.String())
	}

	resp := do(t, env.handler, http.MethodPost, "/api/books/"+url.PathEscape("雾港停钟楼-20260422-0942")+"/write-next", nil)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("trigger write-next status = %d, body=%s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	decodeJSON(t, resp, &payload)
	if strings.TrimSpace(fmt.Sprint(payload["runId"])) == "" {
		t.Fatalf("expected runId in response, got %v", payload)
	}
}

func TestRuns_GetDecodesNonASCIIRunIDPathParam(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	create := do(t, env.handler, http.MethodPost, "/api/books", map[string]any{
		"id":               "雾港停钟楼-20260422-0942",
		"title":            "雾港停钟楼-20260422-0942",
		"genre":            "suspense",
		"language":         "zh",
		"targetChapters":   10,
		"chapterWordCount": 8000,
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create book status = %d, body=%s", create.Code, create.Body.String())
	}

	trigger := do(t, env.handler, http.MethodPost, "/api/books/"+url.PathEscape("雾港停钟楼-20260422-0942")+"/write-next", nil)
	if trigger.Code != http.StatusAccepted {
		t.Fatalf("trigger write-next status = %d, body=%s", trigger.Code, trigger.Body.String())
	}

	var payload map[string]any
	decodeJSON(t, trigger, &payload)
	runID := fmt.Sprint(payload["runId"])

	getRun := do(t, env.handler, http.MethodGet, "/api/runs/"+url.PathEscape(runID)+"?bookId="+url.QueryEscape("雾港停钟楼-20260422-0942"), nil)
	if getRun.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body=%s", getRun.Code, getRun.Body.String())
	}

	var runRecord model.Run
	decodeJSON(t, getRun, &runRecord)
	if runRecord.ID != runID {
		t.Fatalf("expected run id %q, got %q", runID, runRecord.ID)
	}
}

func TestPipeline_TriggerCompletesAndWritesTrace(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)
	do(t, env.handler, "POST", "/api/books", app.CreateBookInput{
		ID: "b2", Title: "T", Genre: "xuanhuan", Language: model.LanguageZH, ChapterWordCount: 3000,
	})

	w := do(t, env.handler, "POST", "/api/books/b2/write", map[string]int{"chapter": 1})
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger write: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	decodeJSON(t, w, &resp)
	runID := resp["runId"]
	if runID == "" {
		t.Fatal("expected runId in response")
	}

	runDetail := waitForRunCompletion(t, env.handler, "b2", runID, 3*time.Second)
	if runDetail.Status != model.RunStatusSucceeded {
		t.Fatalf("expected run succeeded, got %s", runDetail.Status)
	}

	w = do(t, env.handler, "GET", "/api/runs/"+runID+"/traces", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get traces: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var traces []*model.PromptTrace
	decodeJSON(t, w, &traces)
	if len(traces) == 0 {
		t.Fatal("expected at least one trace after simulated run")
	}
}

// --- Config API ---

func TestConfig_GetAndUpdate(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, "GET", "/api/config", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got model.ProjectConfig
	decodeJSON(t, w, &got)
	if got.LLM.Provider == "" || got.LLM.Model == "" {
		t.Fatalf("expected default llm config, got: %+v", got.LLM)
	}

	updated := model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            got.DataDir,
		MaxConcurrentBooks: 3,
		DefaultLLMProfile:  "anthropic-main",
		LLM:                model.LLMConfig{Provider: "anthropic", Model: "claude-sonnet-4-5"},
		LLMProfiles: []model.LLMProfile{{
			Name:     "anthropic-main",
			Language: "zh",
			Provider: "anthropic",
			Model:    "claude-sonnet-4-5",
		}},
	}
	w = do(t, h, "PUT", "/api/config", updated)
	if w.Code != http.StatusOK {
		t.Fatalf("update config: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var gotUpdated model.ProjectConfig
	decodeJSON(t, w, &gotUpdated)
	if gotUpdated.MaxConcurrentBooks != 3 || gotUpdated.LLM.Provider != "anthropic" {
		t.Fatalf("unexpected updated config: %+v", gotUpdated)
	}
}

func TestConfig_ModelRoutes(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, "GET", "/api/config/models", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get config/models: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var routes map[string]any
	decodeJSON(t, w, &routes)
	if routes["global"] == nil {
		t.Fatalf("expected global model route, got %+v", routes)
	}
	if routes["profiles"] == nil {
		t.Fatalf("expected profiles in model route response, got %+v", routes)
	}

	w = do(t, h, "PUT", "/api/config", model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            "",
		MaxConcurrentBooks: 1,
		DefaultLLMProfile:  "claude-main",
		LLM:                model.LLMConfig{Provider: "anthropic", Model: "claude-sonnet-4-5"},
		LLMProfiles: []model.LLMProfile{{
			Name:     "claude-main",
			Language: "zh",
			Provider: "anthropic",
			Model:    "claude-sonnet-4-5",
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, "PUT", "/api/config/models/writer", model.AgentLLMBinding{
		Profile: "claude-main",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put config/models: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &routes)
	agents, ok := routes["agents"].([]any)
	if !ok || len(agents) == 0 {
		t.Fatalf("expected non-empty agent overrides, got %+v", routes["agents"])
	}
}

func TestConfig_TestProfile(t *testing.T) {
	env := newTestEnv(t)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/responses" {
			fmt.Fprintf(
				w,
				`{"id":"resp-test","model":"gpt-5.4-mini","output":[{"type":"message","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":4,"output_tokens":1,"total_tokens":5}}`,
			)
			return
		}
		fmt.Fprintf(
			w,
			`{"id":"chatcmpl-test","object":"chat.completion","created":%d,"model":"gpt-5.4-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`,
			time.Now().Unix(),
		)
	}))
	defer stub.Close()

	payload := model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            env.dir,
		MaxConcurrentBooks: 1,
		DefaultLLMProfile:  "test-openai",
		LLM: model.LLMConfig{
			Provider: "openai",
			Model:    "gpt-5.4-mini",
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
		},
		LLMProfiles: []model.LLMProfile{{
			Name:     "test-openai",
			Language: "zh",
			Provider: "openai",
			Model:    "gpt-5.4-mini",
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
		}},
	}

	w := do(t, env.handler, "POST", "/api/config/profiles/test-openai/test", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("test profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got map[string]any
	decodeJSON(t, w, &got)
	if got["name"] != "test-openai" {
		t.Fatalf("expected tested profile name, got %+v", got)
	}
	if got["connected"] != true {
		t.Fatalf("expected connected=true, got %+v", got)
	}
}

func TestConfig_TestProfile_AutoEnablesSkipTLSForClaudeProfiles(t *testing.T) {
	env := newTestEnv(t)
	stub := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"pong"}],"usage":{"input_tokens":4,"output_tokens":1}}`)
	}))
	defer stub.Close()

	payload := model.ProjectConfig{
		Name:               "StoryForge",
		Language:           "zh",
		DataDir:            env.dir,
		MaxConcurrentBooks: 1,
		DefaultLLMProfile:  "claude-native",
		LLM: model.LLMConfig{
			Provider: "claude",
			Model:    "claude-sonnet-4-6",
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
		},
		LLMProfiles: []model.LLMProfile{{
			Name:     "claude-native",
			Language: "zh",
			Provider: "claude",
			Model:    "claude-sonnet-4-6",
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
		}},
	}

	w := do(t, env.handler, "POST", "/api/config/profiles/claude-native/test", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("test claude profile: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got map[string]any
	decodeJSON(t, w, &got)
	if got["connected"] != true {
		t.Fatalf("expected connected=true with auto skip tls for claude probe, got %+v", got)
	}
}

// --- Genres API ---

func TestGenres_ListAndGet(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, "GET", "/api/genres?language=zh", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list genres: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var genres []map[string]any
	decodeJSON(t, w, &genres)
	if len(genres) == 0 {
		t.Fatal("expected non-empty zh genres list")
	}

	w = do(t, h, "GET", "/api/genres/zh/xuanhuan", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get genre: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var genre map[string]any
	decodeJSON(t, w, &genre)
	if genre["id"] != "xuanhuan" {
		t.Fatalf("expected id=xuanhuan, got %v", genre["id"])
	}
}

// --- Analytics API ---

func TestAnalytics_BookOverview(t *testing.T) {
	h := newTestHandler(t)
	do(t, h, "POST", "/api/books", app.CreateBookInput{
		ID: "b-analytics", Title: "T", Genre: "xuanhuan", Language: model.LanguageZH, ChapterWordCount: 3000,
	})

	w := do(t, h, "GET", "/api/analytics/books/b-analytics/overview", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("analytics overview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, w, &resp)
	if resp["bookId"] != "b-analytics" {
		t.Fatalf("book id mismatch: %v", resp["bookId"])
	}
}

// --- SSE broadcaster ---

func TestBroadcaster_SubscribePublish(t *testing.T) {
	b := run.NewBroadcaster()
	ch, cancel := b.Subscribe("run-1")
	defer cancel()

	event := run.RunEvent{RunID: "run-1", Type: "progress", Message: "hello"}
	b.Publish("run-1", event)

	select {
	case got := <-ch:
		if got.Message != "hello" {
			t.Errorf("message mismatch: %q", got.Message)
		}
	default:
		t.Error("expected event in channel")
	}
}

func TestReviewReject_RemovesMemoryFromRealService(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)
	h := env.handler

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "review-memory-book",
		Title:            "Review Memory Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	chapters := store.NewChapterStore(env.dir)
	truths := store.NewTruthStore(env.dir)
	snapshots := store.NewSnapshotStore(env.dir)
	memory, err := state.OpenMemoryDB(filepath.Join(env.dir, "memory.db"))
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = memory.Close() })

	now := time.Now().UTC()
	for _, chapter := range []struct {
		number int
		title  string
		status model.ChapterStatus
		body   string
	}{
		{1, "Chapter 1", model.ChapterStatusApproved, "chapter one"},
		{2, "Chapter 2", model.ChapterStatusPendingReview, "chapter two"},
	} {
		meta := &model.ChapterMeta{
			Number:    chapter.number,
			Title:     chapter.title,
			Status:    chapter.status,
			WordCount: len(chapter.body),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := chapters.SaveMeta("review-memory-book", meta); err != nil {
			t.Fatalf("save meta: %v", err)
		}
		if err := chapters.SaveContent("review-memory-book", chapter.number, chapter.body); err != nil {
			t.Fatalf("save content: %v", err)
		}
	}

	snapshot := &model.ChapterSnapshot{
		Chapter:   1,
		CreatedAt: now,
		State: &model.RuntimeState{
			CurrentState: map[string]any{"scene": "before chapter 2"},
			ChapterSummaries: []model.ChapterSummaryRow{
				{Chapter: 1, Title: "Chapter 1", Summary: "chapter one"},
			},
		},
	}
	if err := snapshots.Save("review-memory-book", snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := truths.Write("review-memory-book", store.TruthCurrentState, map[string]any{"scene": "after chapter 2"}); err != nil {
		t.Fatalf("seed current state: %v", err)
	}
	if err := memory.Insert(context.Background(), state.MemoryEntry{
		BookID:    "review-memory-book",
		Chapter:   1,
		Kind:      "summary",
		Subject:   "chapter",
		Content:   "chapter 1 memory",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert chapter 1 memory: %v", err)
	}
	if err := memory.Insert(context.Background(), state.MemoryEntry{
		BookID:    "review-memory-book",
		Chapter:   2,
		Kind:      "summary",
		Subject:   "chapter",
		Content:   "chapter 2 memory",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("insert chapter 2 memory: %v", err)
	}

	w = do(t, h, http.MethodPost, "/api/books/review-memory-book/chapters/2/reject", map[string]any{"reason": "rollback"})
	if w.Code != http.StatusOK {
		t.Fatalf("reject: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	entries, err := memory.Recall(context.Background(), state.MemoryQuery{
		BookID:  "review-memory-book",
		Chapter: 2,
		Limit:   20,
	})
	if err != nil {
		t.Fatalf("recall memory: %v", err)
	}
	for _, entry := range entries {
		if entry.Chapter >= 2 {
			t.Fatalf("expected chapter >= 2 memory to be deleted, got %+v", entry)
		}
	}
}

func TestBroadcaster_CancelUnsubscribes(t *testing.T) {
	b := run.NewBroadcaster()
	_, cancel := b.Subscribe("run-2")
	cancel()
	// Publishing after cancel should not panic
	b.Publish("run-2", run.RunEvent{RunID: "run-2", Type: "complete"})
}

// --- Ensure NewDefaultServices is exported ---
var _ *api.Services = (*api.Services)(nil)

func init() {
	// Ensure store types are accessible
	_ = store.NewBookStore
	_ = store.NewChapterStore
}
