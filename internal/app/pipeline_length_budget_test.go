package app

import (
	"context"
	"testing"

	"storyforge/internal/agent"
	"storyforge/internal/model"
	pipe "storyforge/internal/pipeline"
)

func TestPipelineService_ComposeContextScalesTokenBudgetWithChapterTarget(t *testing.T) {
	service := &PipelineService{}
	book := &model.BookConfig{
		ID:               "budget-book",
		Title:            "Budget Book",
		Language:         model.LanguageZH,
		Genre:            "suspense",
		ChapterWordCount: 8000,
	}
	exec := &pipelineExecution{
		book: book,
		agents: pipe.Agents{
			Composer: agent.NewComposer(agent.NewBaseAgent("composer", &captureAuditProvider{}, "test-model")),
		},
		runnerCfg: pipe.DefaultRunnerConfig(),
	}

	_, _, trace, err := service.composeContext(
		context.Background(),
		nil,
		exec,
		1,
		model.RuntimeState{},
		model.ChapterIntent{Chapter: 1},
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if trace.TokenBudget != 8000 {
		t.Fatalf("expected compose token budget 8000, got %d", trace.TokenBudget)
	}
}
