package prompt_test

import (
	"strings"
	"testing"

	"storyforge/internal/prompt"
)

func defaultCtx() prompt.PromptContext {
	return prompt.PromptContext{
		Language:      "zh",
		Genre:         "玄幻",
		BookTitle:     "测试小说",
		ChapterNumber: 5,
		WordCountMin:  2500,
		WordCountMax:  3500,
		Goal:          "主角突破境界",
		MustKeep:      []string{"主角的秘密"},
		MustAvoid:     []string{"信息堆砌"},
		ContextBundle: "[上下文内容]",
		RuleStack:     "[规则栈]",
		HookAgenda:    "[伏笔排班]",
		BookRules:     "主角不能轻易暴露实力",
		StyleGuide:    "简洁有力，节奏紧凑",
	}
}

func TestBuilder_WriterZH_ContainsAllSections(t *testing.T) {
	b := prompt.DefaultBuilder()
	ctx := defaultCtx()

	system, user, err := b.Build(prompt.RoleWriter, "zh", ctx)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}

	// System prompt should contain genre intro
	if !strings.Contains(system+user, "玄幻") {
		t.Error("expected genre in prompt")
	}
	if !strings.Contains(system+user, "测试小说") {
		t.Error("expected book title in prompt")
	}
	if !strings.Contains(system+user, "主角突破境界") {
		t.Error("expected goal in prompt")
	}
	if !strings.Contains(system+user, "2500") {
		t.Error("expected word count min in prompt")
	}
	if !strings.Contains(system+user, "主角的秘密") {
		t.Error("expected mustKeep in prompt")
	}
	if !strings.Contains(system+user, "主角不能轻易暴露实力") {
		t.Error("expected book rules in prompt")
	}
	if !strings.Contains(system+user, "简洁有力") {
		t.Error("expected style guide in prompt")
	}
}

func TestBuilder_WriterEN_ContainsAllSections(t *testing.T) {
	b := prompt.DefaultBuilder()
	ctx := defaultCtx()
	ctx.Language = "en"
	ctx.Genre = "LitRPG"
	ctx.BookTitle = "Test Novel"
	ctx.Goal = "Protagonist levels up"
	ctx.MustKeep = []string{"protagonist's secret"}
	ctx.BookRules = "No power creep without cost"
	ctx.StyleGuide = "Fast-paced, punchy prose"

	system, user, err := b.Build(prompt.RoleWriter, "en", ctx)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}

	combined := system + user
	checks := []string{"LitRPG", "Test Novel", "Protagonist levels up", "protagonist's secret", "No power creep"}
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Errorf("expected %q in prompt", check)
		}
	}
}

func TestBuilder_ObserverZH(t *testing.T) {
	b := prompt.DefaultBuilder()
	ctx := defaultCtx()
	ctx.DraftBody = "萧炎突破了斗师境界，周围的能量波动引起了众人注意。"

	system, user, err := b.Build(prompt.RoleObserver, "zh", ctx)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	combined := system + user
	if !strings.Contains(combined, "萧炎突破了斗师境界") {
		t.Error("expected draft body in observer prompt")
	}
	if !strings.Contains(combined, "JSON") {
		t.Error("expected JSON output format instruction")
	}
}

func TestBuilder_ReflectorZH(t *testing.T) {
	b := prompt.DefaultBuilder()
	ctx := defaultCtx()
	ctx.DraftBody = "主角完成了突破。"
	ctx.TruthSnapshot = `{"pendingHooks": []}`

	_, user, err := b.Build(prompt.RoleReflector, "zh", ctx)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	if !strings.Contains(user, "RuntimeStateDelta") {
		t.Error("expected RuntimeStateDelta in reflector prompt")
	}
}

func TestBuilder_AuditorZH_WithDimensions(t *testing.T) {
	b := prompt.DefaultBuilder()
	ctx := defaultCtx()
	ctx.DraftBody = "章节内容..."
	ctx.TruthSnapshot = "{}"
	ctx.AuditDimensions = []string{"continuity", "character_consistency", "hook_progress"}

	_, user, err := b.Build(prompt.RoleAuditor, "zh", ctx)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	for _, dim := range ctx.AuditDimensions {
		if !strings.Contains(user, dim) {
			t.Errorf("expected dimension %q in auditor prompt", dim)
		}
	}
}

func TestBuilder_FanficCanon_OnlyWhenSet(t *testing.T) {
	b := prompt.DefaultBuilder()

	// Without fanfic canon
	ctx := defaultCtx()
	ctx.FanficCanon = ""
	_, user, _ := b.Build(prompt.RoleWriter, "zh", ctx)
	if strings.Contains(user, "同人正典") {
		t.Error("fanfic canon section should be empty when not set")
	}

	// With fanfic canon
	ctx.FanficCanon = "原作设定：主角是斗气大陆的天才"
	_, user, _ = b.Build(prompt.RoleWriter, "zh", ctx)
	if !strings.Contains(user, "原作设定") {
		t.Error("expected fanfic canon in prompt when set")
	}
}

func TestStaticLoader(t *testing.T) {
	loader := prompt.NewStaticLoader(map[string]string{
		"zh/writer/genre_intro": "你好，{{.Genre}}",
		"writer/genre_intro":    "Hello, {{.Genre}}",
	})

	text, err := loader.Load("writer/genre_intro", "zh")
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if text != "你好，{{.Genre}}" {
		t.Errorf("unexpected text: %q", text)
	}

	text, err = loader.Load("writer/genre_intro", "fr")
	if err != nil {
		t.Fatalf("load fallback error: %v", err)
	}
	if text != "Hello, {{.Genre}}" {
		t.Errorf("unexpected fallback text: %q", text)
	}
}

func TestSchemaRegistry(t *testing.T) {
	sr := prompt.NewSchemaRegistry()
	schema, ok := sr.Get(prompt.RoleWriter)
	if !ok {
		t.Error("expected writer schema")
	}
	if len(schema) == 0 {
		t.Error("expected non-empty schema")
	}
}
