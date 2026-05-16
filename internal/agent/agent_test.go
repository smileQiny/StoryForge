package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"storyforge/internal/agent"
	"storyforge/internal/hook"
	"storyforge/internal/llm"
	"storyforge/internal/model"
)

// mockProvider is a simple mock LLM provider for testing.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (m *mockProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ChatResponse{Content: m.response, Usage: llm.TokenUsage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}}, nil
}

func (m *mockProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.ToolResponse{Content: m.response}, nil
}

func (m *mockProvider) Stream(_ context.Context, _ llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if cb != nil {
		_ = cb(m.response)
	}
	return &llm.ChatResponse{Content: m.response}, nil
}

func newBase(role, response string) *agent.BaseAgent {
	return agent.NewBaseAgent(role, &mockProvider{response: response}, "test-model")
}

type captureProvider struct {
	response string
	lastReq  llm.ChatRequest
}

func (c *captureProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (c *captureProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.lastReq = req
	return &llm.ChatResponse{
		Content: c.response,
		Usage:   llm.TokenUsage{InputTokens: 12, OutputTokens: 18, TotalTokens: 30},
	}, nil
}

func (c *captureProvider) ChatWithTools(_ context.Context, req llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	c.lastReq = req
	return &llm.ToolResponse{Content: c.response}, nil
}

func (c *captureProvider) Stream(_ context.Context, req llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	c.lastReq = req
	if cb != nil {
		_ = cb(c.response)
	}
	return &llm.ChatResponse{Content: c.response}, nil
}

type sequenceProvider struct {
	responses []string
	calls     int
}

func (s *sequenceProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (s *sequenceProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if len(s.responses) == 0 {
		return &llm.ChatResponse{Content: ""}, nil
	}
	idx := s.calls
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	s.calls++
	return &llm.ChatResponse{
		Content: s.responses[idx],
		Usage:   llm.TokenUsage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	}, nil
}

func (s *sequenceProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	resp, err := s.Chat(context.Background(), llm.ChatRequest{})
	if err != nil {
		return nil, err
	}
	return &llm.ToolResponse{Content: resp.Content}, nil
}

func (s *sequenceProvider) Stream(_ context.Context, _ llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	resp, err := s.Chat(context.Background(), llm.ChatRequest{})
	if err != nil {
		return nil, err
	}
	if cb != nil {
		_ = cb(resp.Content)
	}
	return resp, nil
}

type capabilityProvider struct {
	caps       llm.Capabilities
	chatResp   string
	streamHits int
	chatHits   int
}

func (c *capabilityProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	c.chatHits++
	return &llm.ChatResponse{Content: c.chatResp, Usage: llm.TokenUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}}, nil
}

func (c *capabilityProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	return &llm.ToolResponse{Content: c.chatResp}, nil
}

func (c *capabilityProvider) Stream(_ context.Context, _ llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	c.streamHits++
	if cb != nil {
		_ = cb("stream")
	}
	return &llm.ChatResponse{Content: "stream"}, nil
}

func (c *capabilityProvider) Capabilities() llm.Capabilities {
	return c.caps
}

// --- BaseAgent tests ---

func TestBaseAgent_Chat(t *testing.T) {
	base := newBase("test", "hello world")
	content, usage, err := base.Chat(context.Background(), "system", "user")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
	if usage == nil || usage.TotalTokens != 30 {
		t.Error("expected usage with 30 total tokens")
	}
}

func TestBaseAgent_Stream(t *testing.T) {
	base := newBase("test", "streamed content")
	var received string
	content, _, err := base.Stream(context.Background(), "system", "user", func(token string) error {
		received += token
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if content != "streamed content" {
		t.Errorf("expected 'streamed content', got %q", content)
	}
	if received != "streamed content" {
		t.Errorf("callback received %q", received)
	}
}

func TestBaseAgent_StreamFallsBackWhenProviderLacksStreaming(t *testing.T) {
	provider := &capabilityProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    false,
			SupportsToolCalls:    true,
			SupportsSystemPrompt: true,
		},
		chatResp: "fallback-chat",
	}
	base := agent.NewBaseAgent("test", provider, "test-model")

	var received string
	content, usage, err := base.Stream(context.Background(), "system", "user", func(token string) error {
		received += token
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if content != "fallback-chat" {
		t.Fatalf("expected chat fallback content, got %q", content)
	}
	if usage == nil || usage.TotalTokens != 5 {
		t.Fatalf("expected chat fallback usage, got %+v", usage)
	}
	if provider.streamHits != 0 {
		t.Fatalf("expected stream path not to be used, got %d hits", provider.streamHits)
	}
	if provider.chatHits != 1 {
		t.Fatalf("expected one chat fallback call, got %d", provider.chatHits)
	}
	if received != "fallback-chat" {
		t.Fatalf("expected callback to receive fallback chat content, got %q", received)
	}
}

func TestBaseAgent_ChatWithToolsRejectsUnsupportedCapability(t *testing.T) {
	provider := &capabilityProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    true,
			SupportsToolCalls:    false,
			SupportsSystemPrompt: true,
		},
		chatResp: "unused",
	}
	base := agent.NewBaseAgent("architect", provider, "test-model")

	_, err := base.ChatWithTools(context.Background(), "system", "user", []llm.Tool{{
		Name:        "submit_world",
		Description: "submit world state",
		Parameters:  llm.ObjectSchema(map[string]llm.PropertyDef{"content": {Type: "string"}}, []string{"content"}),
	}})
	if err == nil {
		t.Fatal("expected unsupported tool-call capability error")
	}
	if !strings.Contains(err.Error(), "does not support tool calling") {
		t.Fatalf("expected clear tool capability error, got %v", err)
	}
}

func TestValidateProviderForRole_ArchitectRequiresToolCalling(t *testing.T) {
	provider := &capabilityProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    true,
			SupportsToolCalls:    false,
			SupportsSystemPrompt: true,
		},
	}

	err := agent.ValidateProviderForRole("architect", "test-model", provider)
	if err == nil {
		t.Fatal("expected architect capability validation to fail")
	}
	if !strings.Contains(err.Error(), "tool calling") {
		t.Fatalf("expected missing tool calling in error, got %v", err)
	}
}

func TestValidateProviderForRole_WriterOnlyPrefersStreaming(t *testing.T) {
	provider := &capabilityProvider{
		caps: llm.Capabilities{
			SupportsChat:         true,
			SupportsStreaming:    false,
			SupportsToolCalls:    true,
			SupportsSystemPrompt: true,
		},
	}

	if err := agent.ValidateProviderForRole("writer", "test-model", provider); err != nil {
		t.Fatalf("expected writer validation to allow missing preferred streaming, got %v", err)
	}

	missing := agent.MissingPreferredCapabilities("writer", provider)
	if len(missing) != 1 || missing[0] != "streaming" {
		t.Fatalf("expected missing preferred streaming, got %#v", missing)
	}
}

func TestValidateProviderForRole_DefaultRequiresChat(t *testing.T) {
	provider := &capabilityProvider{
		caps: llm.Capabilities{
			SupportsChat:         false,
			SupportsStreaming:    true,
			SupportsToolCalls:    true,
			SupportsSystemPrompt: true,
		},
	}

	err := agent.ValidateProviderForRole("planner", "test-model", provider)
	if err == nil {
		t.Fatal("expected default role capability validation to fail without chat")
	}
	if !strings.Contains(err.Error(), "chat") {
		t.Fatalf("expected missing chat in error, got %v", err)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{`{"key":"val"}`, `{"key":"val"}`},
		{"```json\n{\"key\":\"val\"}\n```", `{"key":"val"}`},
		{"Here is the result:\n```\n{\"key\":\"val\"}\n```", `{"key":"val"}`},
		{"[1,2,3]", "[1,2,3]"},
	}
	for _, c := range cases {
		got := agent.ExtractJSON(c.input)
		if strings.TrimSpace(got) != c.want {
			t.Errorf("ExtractJSON(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// --- PlannerCore tests ---

func TestPlannerCore_Compute_NoLLM(t *testing.T) {
	core := agent.PlannerCore{}
	state := model.RuntimeState{
		PendingHooks: []model.HookRecord{
			{HookID: "h1", StartChapter: 1, LastAdvancedChapter: 1, Status: model.HookStatusOpen},
		},
	}
	input := agent.PlannerInput{
		Book:         model.BookConfig{Title: "Test", Language: "en", Genre: "litrpg"},
		Chapter:      8,
		State:        state,
		AgendaConfig: hook.DefaultAgendaConfig(),
	}

	intent := core.Compute(input)
	if intent.Chapter != 8 {
		t.Errorf("expected chapter 8, got %d", intent.Chapter)
	}
	// h1 age = 7 >= stale(5) — should appear in stale debt
	if len(intent.HookAgenda.StaleDebt) == 0 {
		t.Error("expected stale debt in agenda")
	}
}

func TestPlannerCore_FindOutlineNode_Chinese(t *testing.T) {
	core := agent.PlannerCore{}
	outline := "第1章 序幕\n主角登场\n\n第2章 危机\n危机爆发\n\n第3章 转折\n转折点"
	input := agent.PlannerInput{
		Book:         model.BookConfig{Language: "zh", Genre: "xuanhuan"},
		Chapter:      2,
		OutlineText:  outline,
		AgendaConfig: hook.DefaultAgendaConfig(),
	}
	intent := core.Compute(input)
	if !strings.Contains(intent.OutlineNode, "第2章") {
		t.Errorf("expected outline node for chapter 2, got %q", intent.OutlineNode)
	}
}

func TestPlannerCore_FindOutlineNode_English(t *testing.T) {
	core := agent.PlannerCore{}
	outline := "Chapter 1: The Beginning\nHero arrives.\n\nChapter 2: The Conflict\nConflict starts."
	input := agent.PlannerInput{
		Book:         model.BookConfig{Language: "en", Genre: "litrpg"},
		Chapter:      2,
		OutlineText:  outline,
		AgendaConfig: hook.DefaultAgendaConfig(),
	}
	intent := core.Compute(input)
	if !strings.Contains(intent.OutlineNode, "Chapter 2") {
		t.Errorf("expected outline node for Chapter 2, got %q", intent.OutlineNode)
	}
}

// --- Composer tests ---

func TestComposer_Compose(t *testing.T) {
	base := newBase("composer", "")
	c := agent.NewComposer(base)

	state := model.RuntimeState{
		ChapterSummaries: []model.ChapterSummaryRow{
			{Chapter: 1, Summary: "Hero arrives."},
			{Chapter: 2, Summary: "Conflict begins."},
		},
		PendingHooks: []model.HookRecord{
			{HookID: "h1", StartChapter: 1, Type: "mystery", ExpectedPayoff: "secret revealed", Status: model.HookStatusOpen},
		},
	}
	input := agent.ComposerInput{
		Book:    model.BookConfig{Language: "en", Genre: "litrpg"},
		Chapter: 3,
		Intent: model.ChapterIntent{
			Chapter:   3,
			MustKeep:  []string{"hero's sword"},
			MustAvoid: []string{"time travel"},
		},
		State:       state,
		TokenBudget: 2000,
	}

	pkg, stack, trace, err := c.Compose(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if pkg == nil || len(pkg.SelectedContext) == 0 {
		t.Error("expected non-empty context package")
	}
	if stack == nil || len(stack.Sections.Hard) == 0 {
		t.Error("expected hard rules in rule stack")
	}
	if trace == nil {
		t.Error("expected non-nil trace")
	}
	// Must-keep and must-avoid should appear in hard rules
	found := false
	for _, r := range stack.Sections.Hard {
		if strings.Contains(r, "hero's sword") {
			found = true
		}
	}
	if !found {
		t.Error("expected must-keep rule in hard rules")
	}
}

// --- Writer tests ---

func TestWriter_Write(t *testing.T) {
	content := strings.Repeat("The hero walked through the forest. ", 50) // ~1800 chars
	base := newBase("writer", content)
	w := agent.NewWriter(base)

	input := agent.WriterInput{
		Book:       model.BookConfig{Title: "Test", Language: "en", Genre: "litrpg"},
		Chapter:    1,
		Intent:     model.ChapterIntent{Chapter: 1, Goal: "Hero enters the dungeon"},
		LengthSpec: agent.LengthSpec{Min: 500, Target: 1500, Max: 3000},
	}

	out, err := w.Write(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Content == "" {
		t.Error("expected non-empty content")
	}
	if out.WordCount == 0 {
		t.Error("expected non-zero word count")
	}
}

func TestWriter_NeedsNormalize(t *testing.T) {
	// Very short content
	base := newBase("writer", "Short.")
	w := agent.NewWriter(base)

	input := agent.WriterInput{
		Book:       model.BookConfig{Title: "Test", Language: "en", Genre: "litrpg"},
		Chapter:    1,
		LengthSpec: agent.LengthSpec{Min: 1000, Target: 2000, Max: 3000},
	}

	out, err := w.Write(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.NeedsNormalize {
		t.Error("expected NeedsNormalize=true for short content")
	}
}

func TestWriter_WritePassesMaxTokensBudgetForLongChapterTargets(t *testing.T) {
	provider := &captureProvider{response: strings.Repeat("雾港钟楼。", 200)}
	w := agent.NewWriter(agent.NewBaseAgent("writer", provider, "test-model"))

	_, err := w.Write(context.Background(), agent.WriterInput{
		Book:       model.BookConfig{Title: "Test", Language: "zh", Genre: "suspense"},
		Chapter:    1,
		Intent:     model.ChapterIntent{Chapter: 1, Goal: "Keep the tension climbing"},
		LengthSpec: agent.LengthSpec{Min: 6400, Target: 8000, Max: 9600},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.lastReq.MaxTokens != 12000 {
		t.Fatalf("expected writer max tokens 12000, got %d", provider.lastReq.MaxTokens)
	}
}

func TestRecommendedComposeTokenBudgetScalesWithChapterLength(t *testing.T) {
	if got := agent.RecommendedComposeTokenBudget(0); got != 4000 {
		t.Fatalf("expected default compose budget 4000, got %d", got)
	}
	if got := agent.RecommendedComposeTokenBudget(8000); got != 8000 {
		t.Fatalf("expected compose budget 8000 for long chapter target, got %d", got)
	}
	if got := agent.RecommendedComposeTokenBudget(20000); got != 12000 {
		t.Fatalf("expected compose budget ceiling 12000, got %d", got)
	}
}

// --- Observer tests ---

func TestObserver_Observe(t *testing.T) {
	factsJSON := `[{"kind":"character","subject":"Hero","content":"Hero entered the dungeon","chapter":1}]`
	provider := &captureProvider{response: factsJSON}
	o := agent.NewObserver(agent.NewBaseAgent("observer", provider, "test-model"))

	out, err := o.Observe(context.Background(), agent.ObserverInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     1,
		ChapterText: "The hero entered the dungeon.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Facts) != 1 {
		t.Errorf("expected 1 fact, got %d", len(out.Facts))
	}
	if out.Facts[0].Kind != "character" {
		t.Errorf("expected kind=character, got %q", out.Facts[0].Kind)
	}
	if provider.lastReq.MaxTokens != 900 {
		t.Fatalf("expected observer max tokens 900, got %d", provider.lastReq.MaxTokens)
	}
}

func TestObserver_InvalidJSON_NoError(t *testing.T) {
	base := newBase("observer", "not json at all")
	o := agent.NewObserver(base)

	out, err := o.Observe(context.Background(), agent.ObserverInput{
		Book: model.BookConfig{Language: "en"}, Chapter: 1, ChapterText: "text",
	})
	if err != nil {
		t.Fatal("expected no error on invalid JSON")
	}
	if len(out.Facts) != 0 {
		t.Error("expected empty facts on parse failure")
	}
}

// --- Reflector tests ---

func TestReflector_Reflect(t *testing.T) {
	deltaJSON := `{"chapter":1,"chapterSummary":{"chapter":1,"title":"Ch1","summary":"Hero enters dungeon"}}`
	base := newBase("reflector", deltaJSON)
	r := agent.NewReflector(base)

	out, err := r.Reflect(context.Background(), agent.ReflectorInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     1,
		ChapterText: "The hero entered the dungeon.",
		Facts:       []model.ObservedFact{{Kind: "event", Subject: "Hero", Content: "entered dungeon", Chapter: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Delta.Chapter != 1 {
		t.Errorf("expected chapter 1, got %d", out.Delta.Chapter)
	}
	if out.Delta.ChapterSummary == nil {
		t.Error("expected chapter summary in delta")
	}
}

func TestReflector_Reflect_IncludesRichContext(t *testing.T) {
	deltaJSON := `{"chapter":2,"chapterSummary":{"chapter":2,"title":"Ch2","summary":"Gate reopens"}}`
	provider := &captureProvider{response: deltaJSON}
	r := agent.NewReflector(agent.NewBaseAgent("reflector", provider, "test-model"))

	_, err := r.Reflect(context.Background(), agent.ReflectorInput{
		Book:                 model.BookConfig{Language: "en"},
		Chapter:              2,
		ChapterText:          "The hero reached the ruined gate.",
		Facts:                []model.ObservedFact{{Kind: "event", Subject: "Hero", Content: "reached gate", Chapter: 2}},
		State:                model.RuntimeState{},
		CurrentStateText:     `{"scene":"ruined-gate"}`,
		HooksText:            `[{"hookId":"gate-secret"}]`,
		ChapterSummariesText: `[{"chapter":1,"summary":"old debt"}]`,
		SubplotBoardText:     `[{"id":"sp1","title":"Guild politics"}]`,
		EmotionalArcsText:    `[{"characterId":"hero","phase":"shaken"}]`,
		CharacterMatrixText:  `[{"characterId":"hero","relations":{"mentor":"strained"}}]`,
		PreviousSummary:      "Chapter 1 summary",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(provider.lastReq.Messages) != 2 {
		t.Fatalf("expected 2 chat messages, got %d", len(provider.lastReq.Messages))
	}
	user := provider.lastReq.Messages[1].Content
	checks := []string{
		`{"scene":"ruined-gate"}`,
		"gate-secret",
		"old debt",
		"Guild politics",
		"shaken",
		"strained",
		"Chapter 1 summary",
	}
	for _, check := range checks {
		if !strings.Contains(user, check) {
			t.Fatalf("expected reflector prompt to include %q; got:\n%s", check, user)
		}
	}
	if provider.lastReq.MaxTokens != 1000 {
		t.Fatalf("expected reflector max tokens 1000, got %d", provider.lastReq.MaxTokens)
	}
}

// --- Normalizer tests ---

func TestNormalizer_Unchanged(t *testing.T) {
	base := newBase("normalizer", "")
	n := agent.NewNormalizer(base)

	text := strings.Repeat("a", 1500)
	out, err := n.Normalize(context.Background(), agent.NormalizerInput{
		ChapterText: text,
		TargetMin:   1000,
		TargetMax:   2000,
		Language:    "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Action != "unchanged" {
		t.Errorf("expected unchanged, got %q", out.Action)
	}
}

func TestNormalizer_Expand(t *testing.T) {
	expanded := strings.Repeat("b", 1500)
	base := newBase("normalizer", expanded)
	n := agent.NewNormalizer(base)

	out, err := n.Normalize(context.Background(), agent.NormalizerInput{
		ChapterText: "Short.",
		TargetMin:   1000,
		TargetMax:   2000,
		Language:    "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Action != "expanded" {
		t.Errorf("expected expanded, got %q", out.Action)
	}
}

func TestNormalizer_RetriesExpansionUntilWithinRange(t *testing.T) {
	provider := &sequenceProvider{
		responses: []string{
			strings.Repeat("b", 700),
			strings.Repeat("c", 1300),
		},
	}
	n := agent.NewNormalizer(agent.NewBaseAgent("normalizer", provider, "test-model"))

	out, err := n.Normalize(context.Background(), agent.NormalizerInput{
		ChapterText: "Short.",
		TargetMin:   1000,
		TargetMax:   2000,
		Language:    "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 normalization attempts, got %d", provider.calls)
	}
	if out.WordCount < 1000 || out.WordCount > 2000 {
		t.Fatalf("expected normalized content within range, got %d", out.WordCount)
	}
	if out.Content != strings.Repeat("c", 1300) {
		t.Fatalf("expected final content from second pass, got length %d", len(out.Content))
	}
}

// --- Auditor tests ---

func TestAuditor_Audit(t *testing.T) {
	reportJSON := `{
		"chapter": 1,
		"passed": true,
		"issues": [],
		"dimensions": [{"key":"continuity","passed":true,"score":90}]
	}`
	base := newBase("auditor", reportJSON)
	a := agent.NewAuditor(base)

	report, usage, err := a.Audit(context.Background(), agent.AuditorInput{
		Book:        model.BookConfig{Language: "en", Genre: "litrpg"},
		Chapter:     1,
		ChapterText: "The hero fought the monster.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed {
		t.Error("expected passed=true")
	}
	if usage == nil {
		t.Error("expected non-nil usage")
	}
}

func TestAuditor_ActiveDimensions_Fanfic(t *testing.T) {
	// Fanfic mode should activate fanfic dimensions
	reportJSON := `{"chapter":1,"passed":true,"issues":[],"dimensions":[]}`
	base := newBase("auditor", reportJSON)
	a := agent.NewAuditor(base)

	_, _, err := a.Audit(context.Background(), agent.AuditorInput{
		Book: model.BookConfig{
			Language:   "en",
			Genre:      "isekai",
			FanficMode: model.FanficModeAlternate,
		},
		Chapter:     1,
		ChapterText: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuditor_DisabledDimensions(t *testing.T) {
	reportJSON := `{"chapter":1,"passed":true,"issues":[],"dimensions":[]}`
	base := newBase("auditor", reportJSON)
	a := agent.NewAuditor(base)

	_, _, err := a.Audit(context.Background(), agent.AuditorInput{
		Book: model.BookConfig{
			Language:                "en",
			Genre:                   "litrpg",
			DisabledAuditDimensions: []string{"ai_trace", "pacing"},
		},
		Chapter:     1,
		ChapterText: "text",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuditor_Audit_IncludesRichContext(t *testing.T) {
	reportJSON := `{"chapter":1,"passed":true,"issues":[],"dimensions":[]}`
	provider := &captureProvider{response: reportJSON}
	a := agent.NewAuditor(agent.NewBaseAgent("auditor", provider, "test-model"))

	_, _, err := a.Audit(context.Background(), agent.AuditorInput{
		Book:                 model.BookConfig{Language: "en", Genre: "litrpg"},
		Chapter:              2,
		ChapterText:          "The hero returned to the ruined gate.",
		PreviousSummary:      "Chapter 1 summary",
		PreviousChapterText:  "Chapter 1 full text",
		CurrentStateText:     `{"scene":"ruined-gate"}`,
		ParticleLedgerText:   `{"gold":120}`,
		HooksText:            `[{"hookId":"gate-secret"}]`,
		ChapterSummariesText: `[{"chapter":1,"summary":"old debt"}]`,
		SubplotBoardText:     `[{"id":"sp1","title":"Guild politics"}]`,
		EmotionalArcsText:    `[{"characterId":"hero","phase":"shaken"}]`,
		CharacterMatrixText:  `[{"characterId":"hero","relations":{"mentor":"strained"}}]`,
		StyleGuideText:       "Short, tense sentences.",
		StoryBibleText:       "The gate was sealed centuries ago.",
		VolumeOutlineText:    "Volume 1: reopen the gate.",
		ParentCanonText:      "Mainline canon excerpt",
		FanficCanonText:      "Fanfic canon excerpt",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(provider.lastReq.Messages) != 2 {
		t.Fatalf("expected 2 chat messages, got %d", len(provider.lastReq.Messages))
	}

	user := provider.lastReq.Messages[1].Content
	checks := []string{
		"Chapter 1 full text",
		`{"scene":"ruined-gate"}`,
		`{"gold":120}`,
		"Guild politics",
		"strained",
		"Short, tense sentences.",
		"The gate was sealed centuries ago.",
		"Volume 1: reopen the gate.",
		"Mainline canon excerpt",
		"Fanfic canon excerpt",
	}
	for _, check := range checks {
		if !strings.Contains(user, check) {
			t.Fatalf("expected auditor user prompt to include %q; got:\n%s", check, user)
		}
	}
}

// --- Reviser tests ---

func TestReviser_NoCriticalIssues(t *testing.T) {
	base := newBase("reviser", "")
	r := agent.NewReviser(base)

	out, err := r.Revise(context.Background(), agent.ReviserInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     1,
		ChapterText: "Original text.",
		Report: model.AuditReport{
			Issues: []model.AuditIssue{
				{Dimension: "pacing", Severity: "warning", Summary: "slightly slow"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "Original text." {
		t.Error("expected original text when no critical issues")
	}
	if len(out.MarkedIssues) != 1 {
		t.Errorf("expected 1 marked issue, got %d", len(out.MarkedIssues))
	}
}

func TestReviser_AntiDetectModeRevisesWithoutCriticalIssues(t *testing.T) {
	provider := &captureProvider{response: "Smoothed chapter text."}
	r := agent.NewReviser(agent.NewBaseAgent("reviser", provider, "test-model"))

	out, err := r.Revise(context.Background(), agent.ReviserInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     1,
		ChapterText: "Original text.",
		Mode:        "anti-detect",
		AntiDetect:  true,
		Report: model.AuditReport{
			Issues: []model.AuditIssue{
				{Dimension: "style", Severity: "warning", Summary: "slightly repetitive"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "Smoothed chapter text." {
		t.Fatalf("expected anti-detect mode to revise content, got %q", out.Content)
	}
}

func TestReviser_FixCriticalIssues(t *testing.T) {
	fixed := "Fixed chapter text."
	base := newBase("reviser", fixed)
	r := agent.NewReviser(base)

	out, err := r.Revise(context.Background(), agent.ReviserInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     1,
		ChapterText: "Broken text.",
		Report: model.AuditReport{
			Issues: []model.AuditIssue{
				{Dimension: "continuity", Severity: "critical", Summary: "character teleported"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != fixed {
		t.Errorf("expected fixed content, got %q", out.Content)
	}
	if len(out.FixedIssues) != 1 {
		t.Errorf("expected 1 fixed issue, got %d", len(out.FixedIssues))
	}
}

func TestReviser_AntiDetectModeAddsPromptGuidance(t *testing.T) {
	provider := &captureProvider{response: "Adjusted chapter text."}
	r := agent.NewReviser(agent.NewBaseAgent("reviser", provider, "test-model"))

	_, err := r.Revise(context.Background(), agent.ReviserInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     2,
		ChapterText: "Original text.",
		AntiDetect:  true,
		Report: model.AuditReport{
			Issues: []model.AuditIssue{
				{Dimension: "style", Severity: "critical", Summary: "too repetitive"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	system := provider.lastReq.Messages[0].Content
	if !strings.Contains(system, "reduce AI-detection patterns") {
		t.Fatalf("expected anti-detect guidance in reviser prompt; got:\n%s", system)
	}
}

func TestReviser_CriticalIssuesAddAcceptanceChecklistGuidance(t *testing.T) {
	provider := &captureProvider{response: "Adjusted chapter text."}
	r := agent.NewReviser(agent.NewBaseAgent("reviser", provider, "test-model"))

	_, err := r.Revise(context.Background(), agent.ReviserInput{
		Book:        model.BookConfig{Language: "en"},
		Chapter:     3,
		ChapterText: "Original text.",
		Report: model.AuditReport{
			Issues: []model.AuditIssue{
				{
					Dimension:  "continuity",
					Severity:   "critical",
					Summary:    "missing external witness scene",
					Evidence:   "outline requires witness confirmation",
					Suggestion: "add the witness call",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	system := provider.lastReq.Messages[0].Content
	if !strings.Contains(system, "acceptance checklist") || !strings.Contains(system, "silently verify") {
		t.Fatalf("expected critical issue checklist guidance in reviser prompt; got:\n%s", system)
	}
}

// --- FoundationReviewer tests ---

func TestFoundationReviewer_Pass(t *testing.T) {
	resultJSON := `{"totalScore":85,"passed":true,"scores":[],"overallFeedback":"Good foundation."}`
	base := newBase("foundation_reviewer", resultJSON)
	fr := agent.NewFoundationReviewer(base)

	result, err := fr.Review(context.Background(), agent.FoundationReviewerInput{
		Book:      model.BookConfig{Language: "en", Genre: "litrpg"},
		Architect: &agent.ArchitectOutput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Error("expected passed=true")
	}
	if result.TotalScore != 85 {
		t.Errorf("expected score 85, got %d", result.TotalScore)
	}
}

func TestFoundationReviewer_Fail_LowScore(t *testing.T) {
	resultJSON := `{"totalScore":70,"passed":false,"scores":[],"overallFeedback":"Needs work."}`
	base := newBase("foundation_reviewer", resultJSON)
	fr := agent.NewFoundationReviewer(base)

	result, err := fr.Review(context.Background(), agent.FoundationReviewerInput{
		Book:      model.BookConfig{Language: "en", Genre: "litrpg"},
		Architect: &agent.ArchitectOutput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Error("expected passed=false for score 70")
	}
}

func TestFoundationReviewer_Fanfic_RequiresDivergencePoint(t *testing.T) {
	// No divergence point in response — should force passed=false
	resultJSON := `{"totalScore":90,"passed":true,"scores":[],"overallFeedback":"Good."}`
	base := newBase("foundation_reviewer", resultJSON)
	fr := agent.NewFoundationReviewer(base)

	result, err := fr.Review(context.Background(), agent.FoundationReviewerInput{
		Book: model.BookConfig{
			Language:   "en",
			Genre:      "isekai",
			FanficMode: model.FanficModeAlternate,
		},
		Architect: &agent.ArchitectOutput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Error("expected passed=false when fanfic divergence point is missing")
	}
}

func TestFoundationReviewer_AcceptsNumericScoresArray(t *testing.T) {
	resultJSON := `{"totalScore":91,"passed":true,"scores":[92,90,91,89,93],"overallFeedback":"Good foundation."}`
	base := newBase("foundation_reviewer", resultJSON)
	fr := agent.NewFoundationReviewer(base)

	result, err := fr.Review(context.Background(), agent.FoundationReviewerInput{
		Book:      model.BookConfig{Language: "zh", Genre: "xuanhuan"},
		Architect: &agent.ArchitectOutput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Scores) != 5 {
		t.Fatalf("expected 5 normalized scores, got %+v", result.Scores)
	}
	if result.Scores[0].Dimension == "" || result.Scores[0].Score != 92 {
		t.Fatalf("expected normalized first score, got %+v", result.Scores[0])
	}
}

// --- Radar tests ---

func TestRadar_Skip(t *testing.T) {
	base := newBase("radar", "")
	r := agent.NewRadar(base)

	out, err := r.Scan(context.Background(), agent.RadarInput{Skip: true})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Skipped {
		t.Error("expected Skipped=true")
	}
}

func TestRadar_Scan(t *testing.T) {
	signalsJSON := `[{"kind":"ai_trace","text":"It is worth noting that","position":42,"confidence":0.9}]`
	base := newBase("radar", signalsJSON)
	r := agent.NewRadar(base)

	out, err := r.Scan(context.Background(), agent.RadarInput{
		ChapterText:  "It is worth noting that the hero arrived.",
		Language:     "en",
		FatigueWords: []string{"suddenly", "immediately"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(out.Signals))
	}
	if out.Signals[0].Kind != "ai_trace" {
		t.Errorf("expected ai_trace, got %q", out.Signals[0].Kind)
	}
}

// --- Architect mock server test ---

func TestArchitect_Design_ToolCalls(t *testing.T) {
	// Use a mock HTTP server to simulate tool-call response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":          "msg_test",
			"type":        "message",
			"role":        "assistant",
			"content":     []map[string]any{},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 20},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Use mock provider directly for simplicity
	toolResp := &llm.ToolResponse{
		ToolCalls: []llm.ToolCall{
			{Name: "submit_world_building", Arguments: json.RawMessage(`{"content":{"setting":"fantasy world"}}`)},
			{Name: "submit_characters", Arguments: json.RawMessage(`{"content":{"protagonist":"hero"}}`)},
		},
	}
	mockProv := &mockToolProvider{resp: toolResp}
	base := agent.NewBaseAgent("architect", mockProv, "test-model")
	a := agent.NewArchitect(base)

	out, err := a.Design(context.Background(), agent.ArchitectInput{
		Book:  model.BookConfig{Title: "Test", Language: "en", Genre: "litrpg"},
		Brief: "A hero enters a dungeon.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.WorldBuilding == nil {
		t.Error("expected world building output")
	}
	if out.Characters == nil {
		t.Error("expected characters output")
	}
}

func TestArchitect_Design_FallsBackToStructuredJSONContent(t *testing.T) {
	mockProv := &mockToolProvider{
		resp: &llm.ToolResponse{
			Content: `{
				"worldBuilding": {"setting": "fog harbor"},
				"characters": {"protagonist": "沈知微"},
				"plotOutline": {"acts": ["return", "investigate", "reveal"]},
				"styleGuide": {"tone": "gothic suspense"},
				"writingBible": {"rules": ["钟声响起后不能离港"]}
			}`,
		},
	}
	base := agent.NewBaseAgent("architect", mockProv, "test-model")
	a := agent.NewArchitect(base)

	out, err := a.Design(context.Background(), agent.ArchitectInput{
		Book:  model.BookConfig{Title: "Test", Language: "zh", Genre: "horror"},
		Brief: "A fog harbor rewrites memory.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(out.WorldBuilding) == "" {
		t.Fatal("expected world building output from structured content")
	}
	if string(out.WritingBible) == "" {
		t.Fatal("expected writing bible output from structured content")
	}
}

func TestArchitect_Design_FallsBackWhenToolArgumentsAreEmpty(t *testing.T) {
	mockProv := &mockToolProvider{
		resp: &llm.ToolResponse{
			ToolCalls: []llm.ToolCall{
				{Name: "submit_world_building", Arguments: json.RawMessage(`{}`)},
				{Name: "submit_characters", Arguments: json.RawMessage(`{}`)},
			},
		},
		chatResp: &llm.ChatResponse{
			Content: `{
				"worldBuilding": {"setting": "fog harbor"},
				"characters": {"protagonist": "沈知微"},
				"plotOutline": {"acts": ["return", "investigate", "reveal"]},
				"styleGuide": {"tone": "gothic suspense"},
				"writingBible": {"rules": ["钟声响起后不能离港"]}
			}`,
		},
	}
	base := agent.NewBaseAgent("architect", mockProv, "test-model")
	a := agent.NewArchitect(base)

	out, err := a.Design(context.Background(), agent.ArchitectInput{
		Book:  model.BookConfig{Title: "Test", Language: "zh", Genre: "horror"},
		Brief: "A fog harbor rewrites memory.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(out.StyleGuide) == "" {
		t.Fatal("expected style guide output from chat fallback")
	}
}

// mockToolProvider supports ChatWithTools.
type mockToolProvider struct {
	resp     *llm.ToolResponse
	chatResp *llm.ChatResponse
}

func (m *mockToolProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsChat:         true,
		SupportsStreaming:    true,
		SupportsToolCalls:    true,
		SupportsSystemPrompt: true,
	}
}

func (m *mockToolProvider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.chatResp != nil {
		return m.chatResp, nil
	}
	return &llm.ChatResponse{}, nil
}
func (m *mockToolProvider) ChatWithTools(_ context.Context, _ llm.ChatRequest, _ []llm.Tool) (*llm.ToolResponse, error) {
	return m.resp, nil
}
func (m *mockToolProvider) Stream(_ context.Context, _ llm.ChatRequest, cb llm.StreamCallback) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{}, nil
}

// suppress unused import
var _ = fmt.Sprintf
