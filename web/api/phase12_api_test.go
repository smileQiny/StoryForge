package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"storyforge/internal/app"
	"storyforge/internal/model"
)

func TestStyleAnalyze_PersistsStyleProfile(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "style-book",
		Title:            "Style Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/style-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{"number": 1, "title": "One", "content": "第一章。\n\n角色说：“突然，门开了！”"},
			{"number": 2, "title": "Two", "content": "第二章。\n\n他们继续前进。"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/style-book/style/analyze", map[string]any{
		"chapterFrom": 1,
		"chapterTo":   2,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("style analyze: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, w, &resp)
	if resp["sourceType"] != "book-chapters" {
		t.Fatalf("unexpected sourceType: %v", resp["sourceType"])
	}
	if resp["styleGuide"] == "" {
		t.Fatal("expected styleGuide in response")
	}

	w = do(t, h, http.MethodGet, "/api/books/style-book/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("current_state: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var current map[string]any
	decodeJSON(t, w, &current)
	if current["styleProfile"] == nil {
		t.Fatal("expected styleProfile persisted in current_state")
	}
}

func TestDetect_AnalyzesChapterRisk(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "detect-book",
		Title:            "Detect Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/detect-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{"number": 1, "title": "One", "content": "突然，门开了。然后，他笑了。"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapter: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/books/detect-book/detect?chapter=1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("detect: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	decodeJSON(t, w, &resp)
	if resp["riskLevel"] == "" {
		t.Fatal("expected riskLevel in detection response")
	}
	if resp["fatigueWordHits"] == nil {
		t.Fatal("expected fatigueWordHits in detection response")
	}
}

func TestFanficInit_RequiresDivergencePointAndSupportsAllModes(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "fanfic-book",
		Title:            "Fanfic Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/fanfic-book/fanfic/init", map[string]any{
		"mode": "alternate",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing divergence point: expected 400, got %d: %s", w.Code, w.Body.String())
	}

	modes := []model.FanficMode{
		model.FanficModeInspired,
		model.FanficModeAlternate,
		model.FanficModeContinuation,
		model.FanficModeReverse,
	}
	for _, mode := range modes {
		w = do(t, h, http.MethodPost, "/api/books/fanfic-book/fanfic/init", map[string]any{
			"mode":                mode,
			"parentBookId":        "canon-book",
			"sourceTitle":         "Original Work",
			"sourceSummary":       "原作在王城大战后结束。",
			"divergencePoint":     "在王城大战当夜，主角没有离开，而是选择救下反派。",
			"originalPremise":     "主角与反派被迫结盟，建立新的地下秩序。",
			"forbiddenCanonBeats": []string{"复述王城大战原场景"},
		})
		if w.Code != http.StatusOK {
			t.Fatalf("fanfic init %s: expected 200, got %d: %s", mode, w.Code, w.Body.String())
		}
		var resp map[string]any
		decodeJSON(t, w, &resp)
		if resp["mode"] != string(mode) {
			t.Fatalf("expected mode %s, got %v", mode, resp["mode"])
		}
		if resp["divergencePoint"] == "" {
			t.Fatalf("expected divergencePoint in response for mode %s", mode)
		}
	}

	w = do(t, h, http.MethodGet, "/api/books/fanfic-book", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get book: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var book map[string]any
	decodeJSON(t, w, &book)
	if book["fanficMode"] != string(model.FanficModeReverse) {
		t.Fatalf("expected latest fanficMode persisted, got %v", book["fanficMode"])
	}

	w = do(t, h, http.MethodGet, "/api/books/fanfic-book/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("current state: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var current map[string]any
	decodeJSON(t, w, &current)
	profile, ok := current["fanficProfile"].(map[string]any)
	if !ok {
		t.Fatalf("expected fanficProfile in current_state, got %#v", current["fanficProfile"])
	}
	if profile["mustAvoidCanonRetell"] != true {
		t.Fatal("expected mustAvoidCanonRetell guardrail")
	}
	if profile["divergencePoint"] == "" {
		t.Fatal("expected divergencePoint persisted in fanficProfile")
	}
}

func TestWebhookNotifications_SignedAndFiltered(t *testing.T) {
	h := newTestHandler(t)

	type receivedWebhook struct {
		signature string
		eventType string
		body      map[string]any
	}
	received := make(chan receivedWebhook, 4)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode webhook body: %v", err)
		}
		received <- receivedWebhook{
			signature: r.Header.Get("X-StoryForge-Signature"),
			eventType: r.Header.Get("X-StoryForge-Event"),
			body:      body,
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	w := do(t, h, http.MethodGet, "/api/config", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cfg model.ProjectConfig
	decodeJSON(t, w, &cfg)
	cfg.Webhooks = []model.WebhookConfig{{
		Enabled:      true,
		URL:          webhook.URL,
		Secret:       "secret",
		EventFilters: []string{"run.queued"},
		TimeoutMS:    1000,
	}}

	w = do(t, h, http.MethodPut, "/api/config", cfg)
	if w.Code != http.StatusOK {
		t.Fatalf("update config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "webhook-book",
		Title:            "Webhook Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/webhook-book/plan", map[string]any{"chapter": 1})
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger plan: expected 202, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case got := <-received:
		if got.signature == "" {
			t.Fatal("expected signed webhook request")
		}
		if got.eventType != "run.queued" {
			t.Fatalf("expected run.queued event, got %s", got.eventType)
		}
		if got.body["type"] != "run.queued" {
			t.Fatalf("expected webhook payload type run.queued, got %v", got.body["type"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected webhook notification to be delivered")
	}

	cfg.Webhooks[0].EventFilters = []string{"review.*"}
	w = do(t, h, http.MethodPut, "/api/config", cfg)
	if w.Code != http.StatusOK {
		t.Fatalf("update filtered config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/webhook-book/plan", map[string]any{"chapter": 2})
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger filtered plan: expected 202, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case got := <-received:
		t.Fatalf("expected run.queued to be filtered out, got webhook %+v", got)
	case <-time.After(300 * time.Millisecond):
	}
}
