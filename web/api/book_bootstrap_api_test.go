package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"storyforge/internal/app"
	"storyforge/internal/model"
)

func TestBooks_CreateRequiresLLMForFoundationArtifactsAndTruthFiles(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "bootstrap-book",
		Title:            "Bootstrap Book",
		Brief:            "少年误入一座会吞噬命运的神殿，被迫在升级和失控之间求活。",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		Platform:         "qidian",
		TargetChapters:   120,
		ChapterWordCount: 3200,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "llm") {
		t.Fatalf("expected llm-related error, got %s", w.Body.String())
	}

	bookDir := filepath.Join(env.dir, "bootstrap-book")
	if _, err := os.Stat(bookDir); !os.IsNotExist(err) {
		t.Fatalf("expected failed bootstrap to remove %s, stat err=%v", bookDir, err)
	}
}

func TestBooks_CreateUsesLLMFoundationWhenConfigured(t *testing.T) {
	env := newTestEnv(t)
	requestCount := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode stub request: %v", err)
		}
		requestCount++
		if r.URL.Path == "/responses" {
			content := `{"totalScore":91,"passed":true,"scores":[{"dimension":"plot_structure","score":92,"feedback":"good"}],"overallFeedback":"Strong foundation."}`
			if requestCount == 1 {
				content = `{"worldBuilding":{"premise":"LLM premise","worldAnchor":"Floating citadel"},"characters":{"lead":[{"name":"林渊","goal":"守住神殿碎片"}]},"plotOutline":{"openingArc":["第一卷从神殿裂缝爆发开始。"]},"styleGuide":{"summary":"迅猛推进","guidance":["每章结尾留钩子"]},"writingBible":{"mustKeep":["升级有代价"],"mustAvoid":["空转章节"]}}`
			}
			fmt.Fprintf(w, `{"id":"resp-test","model":"gpt-test","output":[{"type":"message","content":[{"type":"output_text","text":%q}]}],"usage":{"input_tokens":9,"output_tokens":10,"total_tokens":19}}`, content)
			return
		}
		if _, ok := body["tools"]; ok {
			fmt.Fprintf(w, `{"id":"chatcmpl-architect","object":"chat.completion","created":%d,"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"tool-1","type":"function","function":{"name":"submit_world_building","arguments":"{\"content\":{\"premise\":\"LLM premise\",\"worldAnchor\":\"Floating citadel\"}}"}},{"id":"tool-2","type":"function","function":{"name":"submit_characters","arguments":"{\"content\":{\"lead\":[{\"name\":\"林渊\",\"goal\":\"守住神殿碎片\"}]}}"}},{"id":"tool-3","type":"function","function":{"name":"submit_plot_outline","arguments":"{\"content\":{\"openingArc\":[\"第一卷从神殿裂缝爆发开始。\"]}}"}},{"id":"tool-4","type":"function","function":{"name":"submit_style_guide","arguments":"{\"content\":{\"summary\":\"迅猛推进\",\"guidance\":[\"每章结尾留钩子\"]}}"}},{"id":"tool-5","type":"function","function":{"name":"submit_writing_bible","arguments":"{\"content\":{\"mustKeep\":[\"升级有代价\"],\"mustAvoid\":[\"空转章节\"]}}"}}]}}],"usage":{"prompt_tokens":12,"completion_tokens":18,"total_tokens":30}}`, time.Now().Unix())
			return
		}

		fmt.Fprintf(w, `{"id":"chatcmpl-reviewer","object":"chat.completion","created":%d,"choices":[{"index":0,"message":{"role":"assistant","content":"{\"totalScore\":91,\"passed\":true,\"scores\":[{\"dimension\":\"plot_structure\",\"score\":92,\"feedback\":\"good\"}],\"overallFeedback\":\"Strong foundation.\"}"}}],"usage":{"prompt_tokens":9,"completion_tokens":10,"total_tokens":19}}`, time.Now().Unix())
	}))
	defer stub.Close()

	config := model.ProjectConfig{
		DataDir:            env.dir,
		MaxConcurrentBooks: 1,
		LLM: model.LLMConfig{
			Provider: "openai",
			Model:    "gpt-test",
			BaseURL:  stub.URL,
			APIKey:   "test-api-key",
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	w := do(t, env.handler, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "llm-book",
		Title:            "LLM Book",
		Brief:            "一个被命运裂缝选中的底层少年，必须在城邦崩塌前掌控失序力量。",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		Platform:         "qidian",
		TargetChapters:   200,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create llm book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(env.dir, "llm-book", "story", "state", "current_state.json"))
	if err != nil {
		t.Fatalf("read current_state: %v", err)
	}
	var current map[string]any
	if err := json.Unmarshal(raw, &current); err != nil {
		t.Fatalf("unmarshal current_state: %v", err)
	}
	foundation, ok := current["foundation"].(map[string]any)
	if !ok {
		t.Fatalf("expected foundation block, got %#v", current["foundation"])
	}
	if foundation["source"] != "llm" {
		t.Fatalf("expected llm source, got %#v", foundation["source"])
	}
	artifacts, ok := foundation["artifacts"].([]any)
	if !ok {
		t.Fatalf("expected foundation artifacts, got %#v", foundation["artifacts"])
	}
	if len(artifacts) != 5 {
		t.Fatalf("expected 5 documented foundation artifacts, got %d", len(artifacts))
	}
	firstArtifact, ok := artifacts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first artifact object, got %#v", artifacts[0])
	}
	if firstArtifact["key"] != "storyBible" || firstArtifact["title"] != "基础世界观" {
		t.Fatalf("expected story bible artifact metadata, got %#v", firstArtifact)
	}
	review, ok := foundation["review"].(map[string]any)
	if !ok {
		t.Fatalf("expected review block, got %#v", foundation["review"])
	}
	if review["totalScore"] != float64(91) {
		t.Fatalf("expected review score 91, got %#v", review["totalScore"])
	}

	storyBible, err := os.ReadFile(filepath.Join(env.dir, "llm-book", "story", "story_bible.md"))
	if err != nil {
		t.Fatalf("read story bible: %v", err)
	}
	if !strings.Contains(string(storyBible), "Floating citadel") {
		t.Fatalf("expected llm story bible content, got %s", string(storyBible))
	}
}
