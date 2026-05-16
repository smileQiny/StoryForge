package pipeline_test

import (
	"context"
	"testing"
	"time"

	"storyforge/internal/agent"
	"storyforge/internal/llm"
	"storyforge/internal/model"
	"storyforge/internal/pipeline"
	"storyforge/internal/run"
	"storyforge/internal/store"
)

// ── mock LLM provider ────────────────────────────────────────────────────────

type mockProvider struct{ response string }

func (m *mockProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (m *mockProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: m.response, Usage: llm.TokenUsage{TotalTokens: 10}}, nil
}
func (m *mockProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	return &llm.ToolResponse{Content: m.response}, nil
}
func (m *mockProvider) Stream(_ context.Context, _ llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	if cb != nil {
		_ = cb(m.response)
	}
	return &llm.ChatResponse{Content: m.response}, nil
}

func newBase(role, resp string) *agent.BaseAgent {
	return agent.NewBaseAgent(role, &mockProvider{response: resp}, "mock")
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeAgents() pipeline.Agents {
	chapterContent := "The hero entered the dungeon and fought the monster. The secret was finally revealed."

	observerResp := `[{"kind":"event","subject":"Hero","content":"entered dungeon","chapter":1}]`
	reflectorResp := `{"chapter":1,"chapterSummary":{"chapter":1,"title":"Ch1","summary":"Hero enters dungeon"}}`
	auditResp := `{"chapter":1,"passed":true,"issues":[],"dimensions":[{"key":"continuity","passed":true,"score":90}]}`

	return pipeline.Agents{
		Planner:    agent.NewPlanner(newBase("planner", `{"goal":"Hero enters dungeon","sceneDirective":"dungeon entrance"}`)),
		Composer:   agent.NewComposer(newBase("composer", "")),
		Writer:     agent.NewWriter(newBase("writer", chapterContent)),
		Observer:   agent.NewObserver(newBase("observer", observerResp)),
		Reflector:  agent.NewReflector(newBase("reflector", reflectorResp)),
		Normalizer: agent.NewNormalizer(newBase("normalizer", chapterContent)),
		Auditor:    agent.NewAuditor(newBase("auditor", auditResp)),
		Reviser:    agent.NewReviser(newBase("reviser", chapterContent)),
		Radar:      agent.NewRadar(newBase("radar", `[]`)),
	}
}

func makeStores(t *testing.T) pipeline.Stores {
	t.Helper()
	dir := t.TempDir()
	return pipeline.Stores{
		Truth:    store.NewTruthStore(dir),
		Chapter:  store.NewChapterStore(dir),
		Runtime:  store.NewRuntimeStore(dir),
		Snapshot: store.NewSnapshotStore(dir),
		Run:      run.NewStore(dir),
	}
}

func makeBook() model.BookConfig {
	return model.BookConfig{
		ID:               "book1",
		Title:            "Test Book",
		Genre:            "litrpg",
		Language:         model.LanguageEN,
		ChapterWordCount: 100,
		Status:           model.BookStatusActive,
	}
}

func createRun(t *testing.T, stores pipeline.Stores, book model.BookConfig, chapter int) *model.Run {
	t.Helper()
	now := time.Now().UTC()
	r := &model.Run{
		ID:          "run-test-001",
		BookID:      book.ID,
		Chapter:     chapter,
		Kind:        model.RunKindFullPipeline,
		Status:      model.RunStatusQueued,
		TriggeredBy: model.RunTriggeredByUser,
		StartedAt:   now,
		Stages: []model.RunStage{
			{Name: "plan", Status: model.StageStatusPending},
			{Name: "compose", Status: model.StageStatusPending},
			{Name: "write", Status: model.StageStatusPending},
			{Name: "observe", Status: model.StageStatusPending},
			{Name: "reflect", Status: model.StageStatusPending},
			{Name: "normalize", Status: model.StageStatusPending},
			{Name: "audit", Status: model.StageStatusPending},
			{Name: "revise", Status: model.StageStatusPending},
			{Name: "persist", Status: model.StageStatusPending},
		},
	}
	if err := stores.Run.Create(r); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return r
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestRunner_FullPipeline(t *testing.T) {
	stores := makeStores(t)
	book := makeBook()
	r := createRun(t, stores, book, 1)

	bc := run.NewBroadcaster()
	cfg := pipeline.DefaultRunnerConfig()
	runner := pipeline.NewRunner(makeAgents(), stores, cfg, bc)

	// Subscribe to events
	events, cancel := bc.Subscribe(r.ID)
	defer cancel()

	err := runner.Run(context.Background(), pipeline.RunInput{
		Book:    book,
		Chapter: 1,
		RunID:   r.ID,
	})
	if err != nil {
		t.Fatalf("pipeline run failed: %v", err)
	}

	// Verify run completed
	updated, err := stores.Run.Get(book.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != model.RunStatusSucceeded {
		t.Errorf("expected succeeded, got %s", updated.Status)
	}

	// Verify chapter content was saved
	content, err := stores.Chapter.GetContent(book.ID, 1)
	if err != nil {
		t.Fatalf("get chapter content: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty chapter content")
	}

	// Verify at least one SSE event was emitted
	select {
	case ev := <-events:
		if ev.RunID != r.ID {
			t.Errorf("expected runId %q, got %q", r.ID, ev.RunID)
		}
	default:
		t.Error("expected at least one SSE event")
	}
}

func TestRunner_StageOrder(t *testing.T) {
	stores := makeStores(t)
	book := makeBook()
	r := createRun(t, stores, book, 1)

	bc := run.NewBroadcaster()
	events, cancel := bc.Subscribe(r.ID)
	defer cancel()

	cfg := pipeline.DefaultRunnerConfig()
	runner := pipeline.NewRunner(makeAgents(), stores, cfg, bc)

	if err := runner.Run(context.Background(), pipeline.RunInput{
		Book: book, Chapter: 1, RunID: r.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Collect all stage_start events and verify order
	var stageOrder []string
	for {
		select {
		case ev := <-events:
			if ev.Type == "stage_start" {
				stageOrder = append(stageOrder, ev.Stage)
			}
		default:
			goto done
		}
	}
done:
	requiredPrefix := []string{"plan", "compose", "write", "observe", "reflect"}
	for i, expected := range requiredPrefix {
		if i >= len(stageOrder) {
			t.Errorf("missing stage %q at position %d", expected, i)
			continue
		}
		if stageOrder[i] != expected {
			t.Errorf("stage[%d] = %q, want %q", i, stageOrder[i], expected)
		}
	}
	if len(stageOrder) < len(requiredPrefix)+2 {
		t.Fatalf("expected audit and persist after prefix, got %v", stageOrder)
	}
	auditIndex := len(requiredPrefix)
	if stageOrder[auditIndex] == "normalize" {
		auditIndex++
	}
	if stageOrder[auditIndex] != "audit" {
		t.Fatalf("expected audit after write/settle stages, got %v", stageOrder)
	}
	if stageOrder[len(stageOrder)-1] != "persist" {
		t.Fatalf("expected persist as final stage, got %v", stageOrder)
	}
}

func TestRunner_ComposeBudgetScalesWithChapterTarget(t *testing.T) {
	stores := makeStores(t)
	book := makeBook()
	book.ID = "book-compose-budget"
	book.ChapterWordCount = 8000
	r := createRun(t, stores, book, 1)

	runner := pipeline.NewRunner(makeAgents(), stores, pipeline.DefaultRunnerConfig(), run.NewBroadcaster())
	if err := runner.Run(context.Background(), pipeline.RunInput{
		Book:    book,
		Chapter: 1,
		RunID:   r.ID,
	}); err != nil {
		t.Fatal(err)
	}

	trace, err := stores.Runtime.GetTrace(book.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if trace.TokenBudget != agent.RecommendedComposeTokenBudget(book.ChapterWordCount) {
		t.Fatalf("expected compose token budget %d, got %d", agent.RecommendedComposeTokenBudget(book.ChapterWordCount), trace.TokenBudget)
	}
}

func TestRunner_AuditPassNoRevise(t *testing.T) {
	stores := makeStores(t)
	book := makeBook()
	r := createRun(t, stores, book, 1)

	bc := run.NewBroadcaster()
	cfg := pipeline.DefaultRunnerConfig()
	runner := pipeline.NewRunner(makeAgents(), stores, cfg, bc)

	if err := runner.Run(context.Background(), pipeline.RunInput{
		Book: book, Chapter: 1, RunID: r.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Audit passed → revise stage should be skipped or not run as a retry
	updated, _ := stores.Run.Get(book.ID, r.ID)
	for _, s := range updated.Stages {
		if s.Name == "revise" && s.Status == model.StageStatusFailed {
			t.Error("revise should not fail when audit passes")
		}
	}
}

func TestRunner_WithOutline(t *testing.T) {
	stores := makeStores(t)
	book := makeBook()
	r := createRun(t, stores, book, 1)

	bc := run.NewBroadcaster()
	cfg := pipeline.DefaultRunnerConfig()
	runner := pipeline.NewRunner(makeAgents(), stores, cfg, bc)

	outline := "Chapter 1: The Beginning\nHero enters the dungeon for the first time."

	if err := runner.Run(context.Background(), pipeline.RunInput{
		Book:        book,
		Chapter:     1,
		RunID:       r.ID,
		OutlineText: outline,
	}); err != nil {
		t.Fatal(err)
	}

	updated, _ := stores.Run.Get(book.ID, r.ID)
	if updated.Status != model.RunStatusSucceeded {
		t.Errorf("expected succeeded, got %s", updated.Status)
	}
}

func TestRunner_SSETokenEvents(t *testing.T) {
	stores := makeStores(t)
	book := makeBook()
	r := createRun(t, stores, book, 1)

	bc := run.NewBroadcaster()
	events, cancel := bc.Subscribe(r.ID)
	defer cancel()

	cfg := pipeline.DefaultRunnerConfig()
	runner := pipeline.NewRunner(makeAgents(), stores, cfg, bc)

	if err := runner.Run(context.Background(), pipeline.RunInput{
		Book: book, Chapter: 1, RunID: r.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Drain events and look for token events
	tokenCount := 0
	for {
		select {
		case ev := <-events:
			if ev.Type == "token" {
				tokenCount++
			}
		default:
			goto done2
		}
	}
done2:
	if tokenCount == 0 {
		t.Error("expected at least one token SSE event during write stage")
	}
}

func TestScheduler_MaxConcurrent(t *testing.T) {
	// Verify the semaphore is created with the right capacity
	cfg := pipeline.DefaultSchedulerConfig()
	if cfg.MaxConcurrentBooks != 3 {
		t.Errorf("expected MaxConcurrentBooks=3, got %d", cfg.MaxConcurrentBooks)
	}
}
