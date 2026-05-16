package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"storyforge/internal/agent"
	"storyforge/internal/llm"
	"storyforge/internal/model"
	pipe "storyforge/internal/pipeline"
	"storyforge/internal/run"
	"storyforge/internal/store"
)

type captureAuditProvider struct {
	response string
	lastReq  llm.ChatRequest
}

func (c *captureAuditProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (c *captureAuditProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.lastReq = req
	return &llm.ChatResponse{Content: c.response, Usage: llm.TokenUsage{InputTokens: 11, OutputTokens: 19, TotalTokens: 30}}, nil
}

func (c *captureAuditProvider) ChatWithTools(_ context.Context, req llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	c.lastReq = req
	return &llm.ToolResponse{Content: c.response}, nil
}

func (c *captureAuditProvider) Stream(_ context.Context, req llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	c.lastReq = req
	if cb != nil {
		_ = cb(c.response)
	}
	return &llm.ChatResponse{Content: c.response}, nil
}

type captureReflectProvider struct {
	response string
	lastReq  llm.ChatRequest
}

func (c *captureReflectProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (c *captureReflectProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.lastReq = req
	return &llm.ChatResponse{Content: c.response, Usage: llm.TokenUsage{InputTokens: 9, OutputTokens: 15, TotalTokens: 24}}, nil
}

func (c *captureReflectProvider) ChatWithTools(_ context.Context, req llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	c.lastReq = req
	return &llm.ToolResponse{Content: c.response}, nil
}

func (c *captureReflectProvider) Stream(_ context.Context, req llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	c.lastReq = req
	if cb != nil {
		_ = cb(c.response)
	}
	return &llm.ChatResponse{Content: c.response}, nil
}

func TestPipelineService_AuditChapter_LoadsRichContext(t *testing.T) {
	dir := t.TempDir()
	truth := store.NewTruthStore(dir)
	chapters := store.NewChapterStore(dir)
	runStore := run.NewStore(dir)
	service := &PipelineService{
		dataDir:     dir,
		chapters:    chapters,
		truth:       truth,
		runStore:    runStore,
		broadcaster: run.NewBroadcaster(),
	}

	book := &model.BookConfig{
		ID:       "book-rich-audit",
		Title:    "Rich Audit",
		Language: model.LanguageEN,
		Genre:    "litrpg",
	}
	currentState := model.RuntimeState{
		CurrentState:   map[string]any{"scene": "ruined gate"},
		ParticleLedger: map[string]any{"gold": 120},
		PendingHooks: []model.HookRecord{
			{HookID: "gate-secret", Type: "mystery", Status: model.HookStatusOpen, ExpectedPayoff: "open the gate"},
		},
		ChapterSummaries: []model.ChapterSummaryRow{
			{Chapter: 1, Title: "Debt", Summary: "Old debt returns."},
		},
		SubplotBoard: []model.SubplotState{
			{ID: "sp1", Title: "Guild politics", Status: "active", Progress: 40},
		},
		EmotionalArcs: []model.EmotionalArcState{
			{CharacterID: "hero", Arc: "grief", Phase: "shaken"},
		},
		CharacterMatrix: []model.CharacterMatrixEntry{
			{CharacterID: "hero", Relations: map[string]any{"mentor": "strained"}},
		},
	}
	if err := truth.Write(book.ID, store.TruthCurrentState, currentState.CurrentState); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthParticleLedger, currentState.ParticleLedger); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthPendingHooks, currentState.PendingHooks); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthChapterSummaries, currentState.ChapterSummaries); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthSubplotBoard, currentState.SubplotBoard); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthEmotionalArcs, currentState.EmotionalArcs); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthCharacterMatrix, currentState.CharacterMatrix); err != nil {
		t.Fatal(err)
	}

	if err := chapters.SaveMeta(book.ID, &model.ChapterMeta{
		Number:    1,
		Title:     "Chapter 1",
		Status:    model.ChapterStatusApproved,
		WordCount: 16,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := chapters.SaveContent(book.ID, 1, "Chapter 1 full text with the ruined gate foreshadowing."); err != nil {
		t.Fatal(err)
	}

	storyDir := filepath.Join(dir, book.ID, "story")
	if err := os.MkdirAll(storyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"style_guide.md":    "Short, tense sentences.",
		"story_bible.md":    "The gate was sealed centuries ago.",
		"volume_outline.md": "Volume 1: reopen the gate.",
		"parent_canon.md":   "Mainline canon excerpt",
		"fanfic_canon.md":   "Fanfic canon excerpt",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(storyDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	provider := &captureAuditProvider{response: `{"chapter":2,"passed":true,"issues":[],"dimensions":[]}`}
	exec := &pipelineExecution{
		book:     book,
		profiles: map[string]string{"auditor": "auditor"},
		agents: pipe.Agents{
			Auditor: agent.NewAuditor(agent.NewBaseAgent("auditor", provider, "test-model")),
		},
	}
	rec := run.NewRecorder(&model.Run{
		ID:        "run-audit-rich",
		BookID:    book.ID,
		Chapter:   2,
		Kind:      model.RunKindAudit,
		Status:    model.RunStatusRunning,
		StartedAt: time.Now().UTC(),
		Stages:    []model.RunStage{{Name: "audit", Status: model.StageStatusRunning}},
	}, runStore, service.broadcaster)

	_, err := service.auditChapter(
		context.Background(),
		rec,
		exec,
		2,
		"Chapter 2 text at the ruined gate.",
		"Chapter 1 summary",
		currentState,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(provider.lastReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(provider.lastReq.Messages))
	}
	user := provider.lastReq.Messages[1].Content
	checks := []string{
		"Chapter 1 full text with the ruined gate foreshadowing.",
		`"scene": "ruined gate"`,
		"Guild politics",
		"Short, tense sentences.",
		"The gate was sealed centuries ago.",
		"Volume 1: reopen the gate.",
		"Mainline canon excerpt",
		"Fanfic canon excerpt",
	}
	for _, check := range checks {
		if !strings.Contains(user, check) {
			t.Fatalf("expected audit prompt to include %q; got:\n%s", check, user)
		}
	}
}

func TestPipelineService_ReflectChapter_LoadsRichContext(t *testing.T) {
	dir := t.TempDir()
	truth := store.NewTruthStore(dir)
	chapters := store.NewChapterStore(dir)
	runStore := run.NewStore(dir)
	service := &PipelineService{
		dataDir:     dir,
		chapters:    chapters,
		truth:       truth,
		runStore:    runStore,
		broadcaster: run.NewBroadcaster(),
	}

	book := &model.BookConfig{
		ID:       "book-rich-reflect",
		Title:    "Rich Reflect",
		Language: model.LanguageEN,
		Genre:    "litrpg",
	}
	currentState := model.RuntimeState{
		CurrentState: map[string]any{"scene": "ruined gate"},
		PendingHooks: []model.HookRecord{
			{HookID: "gate-secret", StartChapter: 1, Type: "mystery", Status: model.HookStatusOpen, ExpectedPayoff: "open the gate"},
		},
		ChapterSummaries: []model.ChapterSummaryRow{
			{Chapter: 1, Title: "Debt", Summary: "Old debt returns."},
		},
		SubplotBoard: []model.SubplotState{
			{ID: "sp1", Title: "Guild politics", Status: "active", Progress: 40},
		},
		EmotionalArcs: []model.EmotionalArcState{
			{CharacterID: "hero", Arc: "grief", Phase: "shaken"},
		},
		CharacterMatrix: []model.CharacterMatrixEntry{
			{CharacterID: "hero", Relations: map[string]any{"mentor": "strained"}},
		},
	}
	if err := truth.Write(book.ID, store.TruthCurrentState, currentState.CurrentState); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthPendingHooks, currentState.PendingHooks); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthChapterSummaries, currentState.ChapterSummaries); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthSubplotBoard, currentState.SubplotBoard); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthEmotionalArcs, currentState.EmotionalArcs); err != nil {
		t.Fatal(err)
	}
	if err := truth.Write(book.ID, store.TruthCharacterMatrix, currentState.CharacterMatrix); err != nil {
		t.Fatal(err)
	}

	provider := &captureReflectProvider{response: `{"chapter":2,"chapterSummary":{"chapter":2,"title":"Ch2","summary":"Gate reopens"}}`}
	exec := &pipelineExecution{
		book:     book,
		profiles: map[string]string{"reflector": "reflector"},
		agents: pipe.Agents{
			Reflector: agent.NewReflector(agent.NewBaseAgent("reflector", provider, "test-model")),
		},
	}
	rec := run.NewRecorder(&model.Run{
		ID:        "run-reflect-rich",
		BookID:    book.ID,
		Chapter:   2,
		Kind:      model.RunKindFullPipeline,
		Status:    model.RunStatusRunning,
		StartedAt: time.Now().UTC(),
		Stages:    []model.RunStage{{Name: "reflect", Status: model.StageStatusRunning}},
	}, runStore, service.broadcaster)

	_, err := service.reflectChapter(
		context.Background(),
		rec,
		exec,
		2,
		"Chapter 2 text at the ruined gate.",
		[]model.ObservedFact{{Kind: "event", Subject: "Hero", Content: "reached gate", Chapter: 2}},
		currentState,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(provider.lastReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(provider.lastReq.Messages))
	}
	user := provider.lastReq.Messages[1].Content
	checks := []string{
		`"scene": "ruined gate"`,
		"gate-secret",
		"Old debt returns.",
		"Guild politics",
		"shaken",
		"strained",
	}
	for _, check := range checks {
		if !strings.Contains(user, check) {
			t.Fatalf("expected reflect prompt to include %q; got:\n%s", check, user)
		}
	}
}
