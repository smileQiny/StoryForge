package api_test

import (
	"net/http"
	"testing"

	"storyforge/internal/app"
	"storyforge/internal/model"
)

func TestImportChapters_ReconstructsTruthFiles(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "import-book",
		Title:            "Import Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/import-book/import/chapters", map[string]any{
		"chapters": []map[string]any{
			{
				"number":  1,
				"title":   "雾城来信",
				"content": "林秋收到苏晚寄来的密信，却没有立刻拆开。夜里，林秋看见城门外的黑塔再次亮起。",
				"summary": "林秋收到苏晚的密信，黑塔异象再次出现。",
			},
			{
				"number":  2,
				"title":   "黑塔回声",
				"content": "林秋和苏晚潜入黑塔，发现守钟人仍在等待某个约定。塔底的门后到底藏着什么？",
				"summary": "林秋与苏晚潜入黑塔，守钟人与塔底秘密引出新的悬念。",
			},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import chapters: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Imported                []int    `json:"imported"`
		ReconstructedTruthFiles []string `json:"reconstructedTruthFiles"`
		NextChapter             int      `json:"nextChapter"`
	}
	decodeJSON(t, w, &resp)
	if len(resp.Imported) != 2 || resp.NextChapter != 3 {
		t.Fatalf("unexpected import result: %+v", resp)
	}
	if len(resp.ReconstructedTruthFiles) != 7 {
		t.Fatalf("expected 7 truth files, got %d", len(resp.ReconstructedTruthFiles))
	}

	for _, path := range []string{
		"/api/books/import-book/truth/current_state.json",
		"/api/books/import-book/truth/particle_ledger.json",
		"/api/books/import-book/truth/pending_hooks.json",
		"/api/books/import-book/truth/chapter_summaries.json",
		"/api/books/import-book/truth/subplot_board.json",
		"/api/books/import-book/truth/emotional_arcs.json",
		"/api/books/import-book/truth/character_matrix.json",
	} {
		w = do(t, h, http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("truth file %s: expected 200, got %d: %s", path, w.Code, w.Body.String())
		}
	}

	w = do(t, h, http.MethodGet, "/api/books/import-book/truth/current_state.json", nil)
	var current map[string]any
	decodeJSON(t, w, &current)
	continuation, ok := current["importContinuation"].(map[string]any)
	if !ok {
		t.Fatalf("expected importContinuation scaffold, got %#v", current["importContinuation"])
	}
	if continuation["readyToContinue"] != true {
		t.Fatal("expected readyToContinue=true after import")
	}
	if int(continuation["nextChapter"].(float64)) != 3 {
		t.Fatalf("expected nextChapter=3, got %#v", continuation["nextChapter"])
	}

	w = do(t, h, http.MethodGet, "/api/books/import-book/truth/pending_hooks.json", nil)
	var hooks []map[string]any
	decodeJSON(t, w, &hooks)
	if len(hooks) == 0 {
		t.Fatal("expected pending hooks reconstructed from imported chapters")
	}
}

func TestImportStyleAndCanon_PersistToCurrentState(t *testing.T) {
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

	w = do(t, h, http.MethodPost, "/api/books/style-book/import/style", map[string]any{
		"source": "sample-text",
		"fingerprint": map[string]any{
			"voice": "concise",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import style: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/style-book/import/canon", map[string]any{
		"source":     "canon-wiki",
		"title":      "Original Work",
		"characters": []string{"林秋", "苏晚"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("import canon: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/books/style-book/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("current state: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var current map[string]any
	decodeJSON(t, w, &current)
	if current["styleProfile"] == nil {
		t.Fatal("expected styleProfile in current_state")
	}
	if current["fanficCanon"] == nil {
		t.Fatal("expected fanficCanon in current_state")
	}
}

func TestImportStyle_CompatTextPayloadPersistsSourceAndNotes(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "style-compat-book",
		Title:            "Style Compat Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPost, "/api/books/style-compat-book/style/import", map[string]any{
		"text":       "山门晨雾弥漫，主角抬头时看见石阶尽头的灯火。",
		"sourceName": "compat-sample",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("compat import style: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/books/style-compat-book/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("current state: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var current map[string]any
	decodeJSON(t, w, &current)
	styleProfile, ok := current["styleProfile"].(map[string]any)
	if !ok {
		t.Fatalf("expected styleProfile object, got %#v", current["styleProfile"])
	}
	if styleProfile["source"] != "compat-sample" {
		t.Fatalf("expected styleProfile.source to persist sourceName, got %#v", styleProfile["source"])
	}
	if styleProfile["notes"] == "" {
		t.Fatalf("expected styleProfile.notes to include original text, got %#v", styleProfile["notes"])
	}
}

func TestTruthUpdate_CompatWrappedContentParsesInnerJSON(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "truth-compat-book",
		Title:            "Truth Compat Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodPut, "/api/books/truth-compat-book/truth/current_state.json", map[string]any{
		"content": "{\n  \"compat\": true,\n  \"chapter\": 3\n}",
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("truth update: expected 204, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/api/books/truth-compat-book/truth/current_state.json", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get truth file: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var current map[string]any
	decodeJSON(t, w, &current)
	if current["compat"] != true || current["chapter"] != float64(3) {
		t.Fatalf("expected wrapped content to be parsed as inner JSON, got %+v", current)
	}
	if _, wrapped := current["content"]; wrapped {
		t.Fatalf("expected no wrapper object persisted, got %+v", current)
	}
}
