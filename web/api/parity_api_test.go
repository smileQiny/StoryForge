package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"storyforge/internal/app"
	"storyforge/internal/model"
)

func TestPipeline_AuditReviseRewriteParityEndpoints(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "parity-book",
		Title:            "Parity Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/parity-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{"number": 1, "title": "Imported", "content": "旧稿内容。"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/parity-book/audit/1", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("audit: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var auditResp map[string]string
	decodeJSON(t, w, &auditResp)
	auditRun := waitForRunCompletion(t, env.handler, "parity-book", auditResp["runId"], 3*time.Second)
	if auditRun.Kind != model.RunKindAudit || auditRun.Status != model.RunStatusSucceeded {
		t.Fatalf("unexpected audit run: %+v", auditRun)
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/parity-book/revise/1", map[string]any{"mode": "spot-fix"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("revise: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var reviseResp map[string]string
	decodeJSON(t, w, &reviseResp)
	reviseRun := waitForRunCompletion(t, env.handler, "parity-book", reviseResp["runId"], 3*time.Second)
	if reviseRun.Kind != model.RunKindRevise || reviseRun.Status != model.RunStatusSucceeded {
		t.Fatalf("unexpected revise run: %+v", reviseRun)
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/parity-book/rewrite/1", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("rewrite: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var rewriteResp map[string]any
	decodeJSON(t, w, &rewriteResp)
	runID, _ := rewriteResp["runId"].(string)
	rewriteRun := waitForRunCompletion(t, env.handler, "parity-book", runID, 5*time.Second)
	if rewriteRun.Kind != model.RunKindFullPipeline || rewriteRun.Status != model.RunStatusSucceeded {
		t.Fatalf("unexpected rewrite run: %+v", rewriteRun)
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/parity-book/chapters/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get rewritten chapter: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var chapter map[string]any
	decodeJSON(t, w, &chapter)
	content, _ := chapter["content"].(string)
	if content == "" || content == "旧稿内容。" {
		t.Fatalf("expected rewritten content, got %q", content)
	}
}

func TestDetectAllAndStatsParityEndpoints(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "detect-all-book",
		Title:            "Detect All Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/detect-all-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{"number": 1, "title": "High", "content": "突然，他突然说道。然后他继续。就在这个时候，所有人都沉默了。\n\n突然，他突然说道。然后他继续。就在这个时候，所有人都沉默了。"},
			{"number": 2, "title": "Low", "content": "山门在晨雾里缓缓显形，主角停下脚步，听见溪水从石缝间穿过。"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/detect-all-book/detect-all", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detect-all: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var allResp struct {
		BookID  string           `json:"bookId"`
		Results []map[string]any `json:"results"`
	}
	decodeJSON(t, w, &allResp)
	if allResp.BookID != "detect-all-book" || len(allResp.Results) != 2 {
		t.Fatalf("unexpected detect-all response: %+v", allResp)
	}

	w = do(t, h, http.MethodGet, "/api/books/detect-all-book/detect/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detect stats: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var stats map[string]any
	decodeJSON(t, w, &stats)
	if stats["totalChapters"] != float64(2) {
		t.Fatalf("expected totalChapters=2, got %v", stats["totalChapters"])
	}
	if stats["maxRiskChapter"] == nil {
		t.Fatal("expected maxRiskChapter in stats response")
	}
}

func TestFanfic_ShowAndRefreshParityEndpoints(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "fanfic-refresh-book",
		Title:            "Fanfic Refresh Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/fanfic-refresh-book/fanfic/init", map[string]any{
		"mode":            "reverse",
		"parentBookId":    "canon-book",
		"sourceTitle":     "Original Work",
		"sourceSummary":   "原作大纲",
		"divergencePoint": "主角在终局前提前得知真相。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("fanfic init: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/books/fanfic-refresh-book/fanfic", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get fanfic: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var initial map[string]any
	decodeJSON(t, w, &initial)
	if initial["mode"] != string(model.FanficModeReverse) {
		t.Fatalf("expected reverse mode, got %v", initial["mode"])
	}
	if initial["content"] == nil {
		t.Fatal("expected fanfic markdown content")
	}

	w = do(t, h, http.MethodPost, "/api/books/fanfic-refresh-book/fanfic/refresh", map[string]any{
		"sourceText": "补充了反派支线资料，并强调不能复述终局大战。",
		"sourceName": "refresh",
		"title":      "Refreshed Canon",
		"bookId":     "canon-book",
		"characters": []string{"主角", "反派"},
		"rules":      []string{"不能复述终局大战"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh fanfic: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var refreshed map[string]any
	decodeJSON(t, w, &refreshed)
	canon, ok := refreshed["canon"].(map[string]any)
	if !ok {
		t.Fatalf("expected canon payload, got %#v", refreshed["canon"])
	}
	if canon["title"] != "Refreshed Canon" {
		t.Fatalf("expected refreshed title, got %v", canon["title"])
	}
	if refreshed["ok"] != true {
		t.Fatalf("expected ok=true, got %+v", refreshed)
	}
}

func TestProjectAndBookCompatAliases(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books/create", app.CreateBookInput{
		ID:               "compat-book",
		Title:            "Compat Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("compat create: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	bookReady := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		getBook := do(t, env.handler, http.MethodGet, "/api/books/compat-book", nil)
		if getBook.Code == http.StatusOK {
			bookReady = true
			break
		}
	}
	if !bookReady {
		t.Fatal("compat book did not finish creating in time")
	}

	w = do(t, env.handler, http.MethodPut, "/api/project", map[string]any{
		"language":    "en",
		"temperature": 0.2,
		"maxTokens":   2048,
		"stream":      true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("project update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodGet, "/api/project", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("project get: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var project map[string]any
	decodeJSON(t, w, &project)
	if project["language"] != "en" || project["languageExplicit"] != true {
		t.Fatalf("unexpected project payload: %+v", project)
	}
	if project["temperature"] != 0.2 || project["maxTokens"] != float64(2048) || project["stream"] != true {
		t.Fatalf("expected project llm overrides reflected immediately, got %+v", project)
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/compat-book/create-status", nil)
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Fatalf("create-status: expected 404 missing or 200 creating/error, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/project/language", map[string]any{"language": "en"})
	if w.Code != http.StatusOK {
		t.Fatalf("project language: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPut, "/api/project/model-overrides", map[string]any{
		"overrides": map[string]any{
			"writer": map[string]any{
				"provider": "openai",
				"model":    "gpt-5.4-mini",
			},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("model overrides: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodGet, "/api/project/model-overrides", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get model overrides: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var overrides map[string]any
	decodeJSON(t, w, &overrides)
	overrideMap, ok := overrides["overrides"].(map[string]any)
	if !ok || overrideMap["writer"] == nil {
		t.Fatal("expected model overrides payload")
	}

	w = do(t, env.handler, http.MethodPut, "/api/project/notify", map[string]any{
		"channels": []map[string]any{{
			"enabled": true,
			"url":     "https://example.invalid/webhook",
			"secret":  "test-secret",
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("notify update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodGet, "/api/project/notify", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("notify get: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/compat-book/write-next", nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("write-next: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var writeResp map[string]any
	decodeJSON(t, w, &writeResp)
	runID, _ := writeResp["runId"].(string)
	writeRun := waitForRunCompletion(t, env.handler, "compat-book", runID, 5*time.Second)
	if writeRun.Status != model.RunStatusSucceeded || writeRun.Chapter != 1 {
		t.Fatalf("unexpected write-next run: %+v", writeRun)
	}
	expectedStages := []string{"plan", "compose", "write", "observe", "reflect", "normalize", "audit", "revise", "persist"}
	if len(writeRun.Stages) != len(expectedStages) {
		t.Fatalf("expected %d stages on write run, got %+v", len(expectedStages), writeRun.Stages)
	}
	for i, want := range expectedStages {
		if writeRun.Stages[i].Name != want {
			t.Fatalf("write run stage[%d] = %q, want %q", i, writeRun.Stages[i].Name, want)
		}
	}
	if writeRun.Stages[0].JobTitle != "章节规划师" || writeRun.Stages[0].Responsibility == "" {
		t.Fatalf("expected plan stage metadata to align with overview doc, got %+v", writeRun.Stages[0])
	}
	if writeRun.Stages[8].JobTitle != "持久化执行器" || writeRun.Stages[8].Phase != "persisting" {
		t.Fatalf("expected persist stage metadata to align with overview doc, got %+v", writeRun.Stages[8])
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/compat-book/analytics", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("compat analytics: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCompatCreatePersistsBootstrapBrief(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books/create", map[string]any{
		"title":            "Brief Book",
		"genre":            "xuanhuan",
		"language":         "zh",
		"platform":         "tomato",
		"chapterWordCount": 3200,
		"targetChapters":   120,
		"brief":            "主角被卷入一座吞噬记忆的古城，必须在失忆前拼出真相。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("compat create with brief: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	bookReady := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		getBook := do(t, env.handler, http.MethodGet, "/api/books/brief-book", nil)
		if getBook.Code == http.StatusOK {
			bookReady = true
			break
		}
	}
	if !bookReady {
		t.Fatal("brief book did not finish creating in time")
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/brief-book/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get current_state: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var current map[string]any
	decodeJSON(t, w, &current)
	if current["authorIntent"] != "主角被卷入一座吞噬记忆的古城，必须在失忆前拼出真相。" {
		t.Fatalf("expected authorIntent to persist brief, got %#v", current["authorIntent"])
	}

	foundation, ok := current["foundation"].(map[string]any)
	if !ok {
		t.Fatalf("expected foundation block, got %#v", current["foundation"])
	}
	if foundation["brief"] != "主角被卷入一座吞噬记忆的古城，必须在失忆前拼出真相。" {
		t.Fatalf("expected foundation brief to persist, got %#v", foundation["brief"])
	}
}

func TestCompatCreatePreservesSmallTargetChapterCounts(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books/create", map[string]any{
		"title":            "Short Arc Book",
		"genre":            "horror",
		"language":         "zh",
		"platform":         "other",
		"chapterWordCount": 8000,
		"targetChapters":   10,
		"brief":            "十章内完成真相揭示与情感回收的悬疑短篇。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("compat create short arc: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	bookReady := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		getBook := do(t, env.handler, http.MethodGet, "/api/books/short-arc-book", nil)
		if getBook.Code == http.StatusOK {
			var detail map[string]any
			decodeJSON(t, getBook, &detail)
			if detail["targetChapters"] != float64(10) {
				t.Fatalf("expected targetChapters=10, got %+v", detail)
			}
			bookReady = true
			break
		}
	}
	if !bookReady {
		t.Fatal("short arc book did not finish creating in time")
	}
}

func TestCompatCreateReportsBootstrapFailureWhenLLMUnavailable(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodPost, "/api/books/create", map[string]any{
		"title":            "Broken Brief Book",
		"genre":            "xuanhuan",
		"language":         "zh",
		"platform":         "tomato",
		"chapterWordCount": 3200,
		"targetChapters":   120,
		"brief":            "主角被卷入一座吞噬记忆的古城，必须在失忆前拼出真相。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("compat create start: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var start map[string]any
	decodeJSON(t, w, &start)
	if start["status"] != "creating" {
		t.Fatalf("expected creating status, got %+v", start)
	}

	var status map[string]any
	failed := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		w = do(t, env.handler, http.MethodGet, "/api/books/broken-brief-book/create-status", nil)
		if w.Code != http.StatusOK {
			continue
		}
		decodeJSON(t, w, &status)
		if status["status"] == "error" {
			failed = true
			break
		}
	}
	if !failed {
		t.Fatalf("expected compat create to fail when llm is unavailable, last status=%+v", status)
	}
	if errText, _ := status["error"].(string); errText == "" || !strings.Contains(strings.ToLower(errText), "llm") {
		t.Fatalf("expected llm-related error in create-status, got %+v", status)
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/broken-brief-book", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected failed compat create not to persist book, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGenreCRUDParityEndpoints(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/genres/create", map[string]any{
		"id":              "custom-parity",
		"name":            "Custom Parity",
		"language":        "zh",
		"numericalSystem": true,
		"fatigueWords":    []string{"忽然"},
		"rules":           []string{"保持升级节奏"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("genre create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/genres/zh/custom-parity", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("genre get: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPut, "/api/genres/custom-parity", map[string]any{
		"id":              "custom-parity",
		"name":            "Custom Parity Updated",
		"language":        "zh",
		"powerScaling":    true,
		"auditDimensions": []string{"continuity"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("genre update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/genres/custom-parity/copy?language=zh", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("genre copy: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodDelete, "/api/genres/custom-parity?language=zh", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("genre delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRootFanficInitCompatEndpoint(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/fanfic/init", map[string]any{
		"title":      "Compat Fanfic",
		"sourceText": "原作终局之后，新的裂缝再度打开。",
		"mode":       "au",
		"genre":      "xuanhuan",
		"language":   "zh",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("root fanfic init: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	decodeJSON(t, w, &resp)
	bookID, _ := resp["bookId"].(string)
	if bookID == "" {
		t.Fatal("expected bookId from root fanfic init")
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/"+bookID+"/fanfic", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get root-created fanfic: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fanfic map[string]any
	decodeJSON(t, w, &fanfic)
	if fanfic["content"] == nil {
		t.Fatalf("expected fanfic content in compat payload, got %+v", fanfic)
	}
}

func TestAgentAndGlobalStyleAnalyzeCompatEndpoints(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/agent", map[string]any{
		"instruction": "给出一句简短的写作建议。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("agent endpoint: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var agentResp map[string]any
	decodeJSON(t, w, &agentResp)
	if agentResp["response"] == "" {
		t.Fatalf("expected agent response, got %+v", agentResp)
	}

	w = do(t, env.handler, http.MethodPost, "/api/style/analyze", map[string]any{
		"text": "山门晨雾弥漫，主角抬头时看见石阶尽头的灯火。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("global style analyze: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var styleResp map[string]any
	decodeJSON(t, w, &styleResp)
	if styleResp["sourceType"] != "raw-text" {
		t.Fatalf("expected raw-text style analysis, got %v", styleResp["sourceType"])
	}
}

func TestChapterAnalyzeParityEndpoint(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "analyze-book",
		Title:            "Analyze Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/analyze-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{"number": 1, "title": "地宫试炼", "content": "主角踏入地宫，试炼正式开启。"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/analyze-book/chapters/1/analyze", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("chapter analyze: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, w, &resp)
	if resp["bookId"] != "analyze-book" || resp["chapter"] != float64(1) {
		t.Fatalf("unexpected chapter analyze response: %+v", resp)
	}
	if resp["facts"] == nil || resp["delta"] == nil {
		t.Fatalf("expected facts and delta in response, got %+v", resp)
	}
	if resp["currentState"] == nil || resp["nextState"] == nil {
		t.Fatalf("expected currentState and nextState in response, got %+v", resp)
	}
	if resp["chapterTitle"] != "地宫试炼" {
		t.Fatalf("expected chapterTitle in response, got %+v", resp["chapterTitle"])
	}
}

func TestReviseParityEndpoint_PersistsRequestedMode(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "revise-mode-book",
		Title:            "Revise Mode Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/revise-mode-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{"number": 1, "title": "旧稿", "content": "旧稿内容。"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/books/revise-mode-book/revise/1", map[string]any{"mode": "anti-detect"})
	if w.Code != http.StatusAccepted {
		t.Fatalf("revise: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var reviseResp map[string]any
	decodeJSON(t, w, &reviseResp)
	if reviseResp["mode"] != "anti-detect" {
		t.Fatalf("expected accepted revise response to echo mode, got %+v", reviseResp)
	}
	runID, _ := reviseResp["runId"].(string)
	_ = waitForRunCompletion(t, env.handler, "revise-mode-book", runID, 3*time.Second)

	w = do(t, env.handler, http.MethodGet, "/api/runs/"+runID+"?bookId=revise-mode-book", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get run: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var runDetail map[string]any
	decodeJSON(t, w, &runDetail)
	if runDetail["mode"] != "anti-detect" {
		t.Fatalf("expected run detail to retain requested revise mode, got %+v", runDetail)
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/revise-mode-book/chapters/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get revised chapter: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var chapter map[string]any
	decodeJSON(t, w, &chapter)
	content, _ := chapter["content"].(string)
	if content == "" || content == "旧稿内容。" {
		t.Fatalf("expected anti-detect revise mode to rewrite chapter content, got %q", content)
	}
}

func TestLegacyImportCompatPayloads(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "legacy-import-book",
		Title:            "Legacy Import Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/legacy-import-book/import/chapters", map[string]any{
		"text": "第一章 雾城来信\n林秋收到密信。\n\n第二章 黑塔回声\n黑塔再次亮起。",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("legacy import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var importResp map[string]any
	decodeJSON(t, w, &importResp)
	if importResp["importedCount"] != float64(2) || importResp["nextChapter"] != float64(3) {
		t.Fatalf("unexpected legacy import response: %+v", importResp)
	}

	w = do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "legacy-canon-source",
		Title:            "Legacy Canon Source",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create canon source: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	_ = do(t, h, http.MethodPost, "/api/books/legacy-canon-source/import/chapters", map[string]any{
		"chapters": []map[string]any{{"number": 1, "title": "Canon", "content": "林秋在黑塔前立誓。"}},
	})

	w = do(t, h, http.MethodPost, "/api/books/legacy-import-book/import/canon", map[string]any{
		"fromBookId": "legacy-canon-source",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("legacy import canon: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var canonResp map[string]any
	decodeJSON(t, w, &canonResp)
	if canonResp["fanficCanon"] == nil || canonResp["ok"] != true {
		t.Fatalf("unexpected legacy canon response: %+v", canonResp)
	}
}
