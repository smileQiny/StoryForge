package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"storyforge/internal/agent"
	"storyforge/internal/hook"
	"storyforge/internal/llm"
	"storyforge/internal/model"
	"storyforge/internal/notify"
	pipe "storyforge/internal/pipeline"
	"storyforge/internal/run"
	"storyforge/internal/state"
	"storyforge/internal/store"
)

// PipelineService orchestrates real chapter pipeline operations.
type PipelineService struct {
	dataDir     string
	books       *store.BookStore
	chapters    *store.ChapterStore
	truth       *store.TruthStore
	runtime     *store.RuntimeStore
	snapshots   *store.SnapshotStore
	config      *ConfigService
	runStore    *run.Store
	broadcaster *run.Broadcaster
	notifier    *notify.WebhookDispatcher
}

type pipelineExecution struct {
	book        *model.BookConfig
	agents      pipe.Agents
	runnerCfg   pipe.RunnerConfig
	outlineText string
	profiles    map[string]string
}

type auditTextContext struct {
	PreviousChapterText  string
	CurrentStateText     string
	ParticleLedgerText   string
	HooksText            string
	ChapterSummariesText string
	SubplotBoardText     string
	EmotionalArcsText    string
	CharacterMatrixText  string
	StyleGuideText       string
	StoryBibleText       string
	VolumeOutlineText    string
	ParentCanonText      string
	FanficCanonText      string
}

// NewPipelineService creates a PipelineService.
func NewPipelineService(
	dataDir string,
	books *store.BookStore,
	chapters *store.ChapterStore,
	truth *store.TruthStore,
	runtime *store.RuntimeStore,
	snapshots *store.SnapshotStore,
	config *ConfigService,
	runStore *run.Store,
	broadcaster *run.Broadcaster,
	notifier *notify.WebhookDispatcher,
) *PipelineService {
	return &PipelineService{
		dataDir:     dataDir,
		books:       books,
		chapters:    chapters,
		truth:       truth,
		runtime:     runtime,
		snapshots:   snapshots,
		config:      config,
		runStore:    runStore,
		broadcaster: broadcaster,
		notifier:    notifier,
	}
}

// TriggerInput is the common input for triggering a pipeline operation.
type TriggerInput struct {
	BookID      string
	Chapter     int
	Kind        model.RunKind
	ReviseMode  string
	TriggeredBy model.RunTriggeredBy
}

// Trigger creates a Run record and starts the real pipeline asynchronously.
func (s *PipelineService) Trigger(ctx context.Context, input TriggerInput) (*model.Run, error) {
	if input.BookID == "" {
		return nil, fmt.Errorf("bookId is required")
	}
	if input.Chapter <= 0 {
		return nil, fmt.Errorf("chapter must be positive")
	}
	if _, err := s.books.Get(input.BookID); err != nil {
		return nil, err
	}

	runID := generateRunID(input.BookID, input.Chapter, input.Kind)
	now := time.Now().UTC()

	r := &model.Run{
		ID:          runID,
		BookID:      input.BookID,
		Chapter:     input.Chapter,
		Kind:        input.Kind,
		Mode:        NormalizeReviseMode(input.ReviseMode),
		Status:      model.RunStatusQueued,
		TriggeredBy: input.TriggeredBy,
		StartedAt:   now,
		Stages:      defaultStages(input.Kind),
	}

	if err := s.runStore.Create(r); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}
	s.broadcaster.Publish(runID, run.RunEvent{
		RunID:     runID,
		Type:      "queued",
		Message:   "run queued",
		Timestamp: now,
	})
	_ = s.notifier.Notify(ctx, notify.Event{
		Type:      "run.queued",
		Timestamp: now,
		BookID:    input.BookID,
		Chapter:   input.Chapter,
		RunID:     runID,
		Status:    string(model.RunStatusQueued),
		Payload: map[string]any{
			"kind":        input.Kind,
			"triggeredBy": input.TriggeredBy,
		},
	})

	go s.executeRun(context.Background(), r, input)
	return r, nil
}

func (s *PipelineService) executeRun(ctx context.Context, runRecord *model.Run, input TriggerInput) {
	rec := run.NewRecorder(runRecord, s.runStore, s.broadcaster)
	exec, err := s.prepareExecution(input.BookID)
	if err != nil {
		_ = rec.Fail(err)
		s.notifyCompletion(ctx, input.BookID, input.Chapter, runRecord.ID, model.RunStatusFailed, err)
		return
	}

	switch input.Kind {
	case model.RunKindPlan:
		err = s.runPlan(ctx, rec, runRecord.ID, exec, input.Chapter, true)
	case model.RunKindCompose:
		err = s.runCompose(ctx, rec, runRecord.ID, exec, input.Chapter, true)
	case model.RunKindWrite:
		err = s.runWrite(ctx, rec, runRecord.ID, exec, input.Chapter, true)
	case model.RunKindAudit:
		err = s.runAudit(ctx, rec, runRecord.ID, exec, input.Chapter, true)
	case model.RunKindRevise:
		err = s.runRevise(ctx, rec, runRecord.ID, exec, input.Chapter, runRecord.Mode, true)
	case model.RunKindFullPipeline:
		err = s.runFullPipeline(ctx, rec, runRecord.ID, exec, input.Chapter)
	default:
		err = fmt.Errorf("unsupported run kind %q", input.Kind)
	}

	status := model.RunStatusSucceeded
	if err != nil {
		status = model.RunStatusFailed
	}
	s.notifyCompletion(ctx, input.BookID, input.Chapter, runRecord.ID, status, err)
}

// GetRun returns a run by ID.
func (s *PipelineService) GetRun(bookID, runID string) (*model.Run, error) {
	return s.runStore.Get(bookID, runID)
}

// ListRuns returns all runs for a book.
func (s *PipelineService) ListRuns(bookID string) ([]*model.Run, error) {
	return s.runStore.ListByBook(bookID)
}

// GetTraces returns all prompt traces for a run.
func (s *PipelineService) GetTraces(runID string) ([]*model.PromptTrace, error) {
	return s.runStore.ListTraces(runID)
}

func defaultStages(kind model.RunKind) []model.RunStage {
	var names []string
	switch kind {
	case model.RunKindPlan:
		names = []string{"plan"}
	case model.RunKindCompose:
		names = []string{"compose"}
	case model.RunKindWrite:
		names = []string{"write", "normalize", "persist"}
	case model.RunKindAudit:
		names = []string{"audit", "persist"}
	case model.RunKindRevise:
		names = []string{"revise", "persist"}
	case model.RunKindFullPipeline:
		names = []string{"plan", "compose", "write", "observe", "reflect", "normalize", "audit", "revise", "persist"}
	default:
		names = []string{string(kind)}
	}
	stages := make([]model.RunStage, len(names))
	for i, n := range names {
		stages[i] = buildRunStage(n)
	}
	return stages
}

func buildRunStage(name string) model.RunStage {
	stage := model.RunStage{Name: name, Status: model.StageStatusPending}
	switch name {
	case "plan":
		stage.Role = "planner"
		stage.Phase = "planning"
		stage.JobTitle = "章节规划师"
		stage.Responsibility = "决定这一章要完成什么、推进哪些 hooks、遵守哪些硬约束"
	case "compose":
		stage.Role = "composer"
		stage.Phase = "planning"
		stage.JobTitle = "上下文编排师"
		stage.Responsibility = "整理运行时上下文、规则栈和章节追踪信息"
	case "write":
		stage.Role = "writer"
		stage.Phase = "writing"
		stage.JobTitle = "章节写手"
		stage.Responsibility = "基于规划和上下文写出章节正文，并产出可结算的章节结果"
	case "normalize":
		stage.Role = "normalizer"
		stage.Phase = "writing"
		stage.JobTitle = "篇幅校正器"
		stage.Responsibility = "把章节长度拉回目标区间，不改变核心剧情结论"
	case "audit":
		stage.Role = "auditor"
		stage.Phase = "auditing"
		stage.JobTitle = "连续性审计师"
		stage.Responsibility = "检查设定、时间线、角色、节奏和文本风险"
	case "revise":
		stage.Role = "reviser"
		stage.Phase = "revising"
		stage.JobTitle = "修稿编辑"
		stage.Responsibility = "按审计问题做定点修复或受控改写"
	case "observe":
		stage.Role = "observer"
		stage.Phase = "writing"
		stage.JobTitle = "章节观察员"
		stage.Responsibility = "从正文抽取显式事实、事件和状态变化"
	case "reflect":
		stage.Role = "reflector"
		stage.Phase = "writing"
		stage.JobTitle = "状态结算器"
		stage.Responsibility = "把章节事实合并回 truth files 和运行时状态"
	case "persist":
		stage.Role = "persister"
		stage.Phase = "persisting"
		stage.JobTitle = "持久化执行器"
		stage.Responsibility = "把章节正文、truth files、快照和索引同步写入磁盘"
	}
	return stage
}

func generateRunID(bookID string, chapter int, kind model.RunKind) string {
	return fmt.Sprintf("%s-ch%04d-%s-%d", bookID, chapter, kind, time.Now().UnixMilli())
}

func (s *PipelineService) prepareExecution(bookID string) (*pipelineExecution, error) {
	book, err := s.books.Get(bookID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.config.Get()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	hydrateProjectLLMConfig(cfg)
	router, err := llm.BuildFromConfig(*cfg)
	if err != nil {
		return nil, fmt.Errorf("build llm router: %w", err)
	}
	profiles := make(map[string]string)
	mkBase := func(name string) (*agent.BaseAgent, error) {
		base, profile, err := newValidatedBaseAgent(*cfg, router, name)
		if err != nil {
			return nil, err
		}
		profiles[name] = profile
		return base, nil
	}

	plannerBase, err := mkBase("planner")
	if err != nil {
		return nil, fmt.Errorf("planner agent: %w", err)
	}
	composerBase, err := mkBase("composer")
	if err != nil {
		return nil, fmt.Errorf("composer agent: %w", err)
	}
	writerBase, err := mkBase("writer")
	if err != nil {
		return nil, fmt.Errorf("writer agent: %w", err)
	}
	observerBase, err := mkBase("observer")
	if err != nil {
		return nil, fmt.Errorf("observer agent: %w", err)
	}
	reflectorBase, err := mkBase("reflector")
	if err != nil {
		return nil, fmt.Errorf("reflector agent: %w", err)
	}
	normalizerBase, err := mkBase("normalizer")
	if err != nil {
		return nil, fmt.Errorf("normalizer agent: %w", err)
	}
	auditorBase, err := mkBase("auditor")
	if err != nil {
		return nil, fmt.Errorf("auditor agent: %w", err)
	}
	reviserBase, err := mkBase("reviser")
	if err != nil {
		return nil, fmt.Errorf("reviser agent: %w", err)
	}
	radarBase, err := mkBase("radar")
	if err != nil {
		return nil, fmt.Errorf("radar agent: %w", err)
	}

	return &pipelineExecution{
		book: book,
		agents: pipe.Agents{
			Planner:    agent.NewPlanner(plannerBase),
			Composer:   agent.NewComposer(composerBase),
			Writer:     agent.NewWriter(writerBase),
			Observer:   agent.NewObserver(observerBase),
			Reflector:  agent.NewReflector(reflectorBase),
			Normalizer: agent.NewNormalizer(normalizerBase),
			Auditor:    agent.NewAuditor(auditorBase),
			Reviser:    agent.NewReviser(reviserBase),
			Radar:      agent.NewRadar(radarBase),
		},
		runnerCfg:   pipe.DefaultRunnerConfig(),
		outlineText: s.loadOutlineText(bookID),
		profiles:    profiles,
	}, nil
}

func (s *PipelineService) runPlan(
	ctx context.Context,
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	chapter int,
	recordStage bool,
) error {
	currentState, err := s.loadRuntimeState(exec.book.ID)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load state: %w", err))
	}
	intent, usage, err := s.planIntent(ctx, rec, exec, chapter, currentState, recordStage)
	if err != nil {
		return err
	}
	return s.completeOrPersistPlan(rec, runID, exec, intent, usage)
}

func (s *PipelineService) completeOrPersistPlan(
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	intent *model.ChapterIntent,
	usage *model.TokenUsage,
) error {
	if err := s.runtime.SaveIntent(exec.book.ID, intent); err != nil {
		return s.failRun(rec, "plan", fmt.Errorf("save intent: %w", err))
	}
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID:         runID,
		StageName:     "plan",
		Role:          "planner",
		PromptProfile: exec.profiles["planner"],
		ResponseText:  mustJSON(intent),
		Usage:         usage,
	})
	return rec.Complete()
}

func (s *PipelineService) runCompose(
	ctx context.Context,
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	chapter int,
	recordStage bool,
) error {
	currentState, err := s.loadRuntimeState(exec.book.ID)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load state: %w", err))
	}
	intent, _, err := s.planIntent(ctx, rec, exec, chapter, currentState, false)
	if err != nil {
		return err
	}
	if err := s.runtime.SaveIntent(exec.book.ID, intent); err != nil {
		return s.failRun(rec, "compose", fmt.Errorf("save intent: %w", err))
	}
	pkg, ruleStack, chTrace, err := s.composeContext(ctx, rec, exec, chapter, currentState, *intent, recordStage)
	if err != nil {
		return err
	}
	if err := s.persistComposeArtifacts(exec.book.ID, chapter, pkg, ruleStack, chTrace); err != nil {
		return s.failRun(rec, "compose", err)
	}
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID:         runID,
		StageName:     "compose",
		Role:          "composer",
		PromptProfile: exec.profiles["composer"],
		ResponseText:  mustJSON(map[string]any{"context": pkg, "ruleStack": ruleStack, "trace": chTrace}),
	})
	return rec.Complete()
}

func (s *PipelineService) runWrite(
	ctx context.Context,
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	chapter int,
	recordStage bool,
) error {
	currentState, err := s.loadRuntimeState(exec.book.ID)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load state: %w", err))
	}
	intent, _, err := s.planIntent(ctx, rec, exec, chapter, currentState, false)
	if err != nil {
		return err
	}
	pkg, ruleStack, chTrace, err := s.composeContext(ctx, rec, exec, chapter, currentState, *intent, false)
	if err != nil {
		return err
	}
	if err := s.runtime.SaveIntent(exec.book.ID, intent); err != nil {
		return s.failRun(rec, "write", fmt.Errorf("save intent: %w", err))
	}
	if err := s.persistComposeArtifacts(exec.book.ID, chapter, pkg, ruleStack, chTrace); err != nil {
		return s.failRun(rec, "write", err)
	}
	chapterText, _, err := s.writeDraft(ctx, rec, exec, chapter, *intent, *pkg, *ruleStack, recordStage)
	if err != nil {
		return err
	}
	chapterText, _, err = s.normalizeDraft(ctx, rec, exec, chapterText, recordStage)
	if err != nil {
		return err
	}
	if recordStage {
		_ = rec.StageStart("persist", "persister")
	}
	if err := s.persistChapter(exec.book.ID, chapter, chapterText, model.ChapterStatusDraft); err != nil {
		return s.failRun(rec, "persist", fmt.Errorf("persist chapter: %w", err))
	}
	if recordStage {
		_ = rec.StageSucceed("persist", nil)
	}
	return rec.Complete()
}

func (s *PipelineService) runAudit(
	ctx context.Context,
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	chapter int,
	recordStage bool,
) error {
	chapterText, err := s.chapters.GetContent(exec.book.ID, chapter)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load chapter content: %w", err))
	}
	currentState, err := s.loadRuntimeState(exec.book.ID)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load state: %w", err))
	}
	report, err := s.auditChapter(ctx, rec, exec, chapter, chapterText, previousSummary(currentState, chapter), currentState, recordStage)
	if err != nil {
		return err
	}
	status := model.ChapterStatusAudited
	if report.Passed && !report.HasCriticalIssues() {
		status = model.ChapterStatusPendingReview
	}
	if recordStage {
		_ = rec.StageStart("persist", "persister")
	}
	if err := s.persistChapter(exec.book.ID, chapter, chapterText, status); err != nil {
		return s.failRun(rec, "persist", fmt.Errorf("persist chapter: %w", err))
	}
	if recordStage {
		_ = rec.StageSucceed("persist", nil)
	}
	return rec.Complete()
}

func (s *PipelineService) runRevise(
	ctx context.Context,
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	chapter int,
	mode string,
	recordStage bool,
) error {
	chapterText, err := s.chapters.GetContent(exec.book.ID, chapter)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load chapter content: %w", err))
	}
	currentState, err := s.loadRuntimeState(exec.book.ID)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load state: %w", err))
	}
	report, err := s.auditChapter(ctx, rec, exec, chapter, chapterText, previousSummary(currentState, chapter), currentState, false)
	if err != nil {
		return err
	}
	if !report.HasCriticalIssues() && !ReviseModeRequiresRewrite(mode) {
		_ = rec.StageSkip("revise", "no critical issues detected")
		if recordStage {
			_ = rec.StageStart("persist", "persister")
		}
		if err := s.persistChapter(exec.book.ID, chapter, chapterText, model.ChapterStatusPendingReview); err != nil {
			return s.failRun(rec, "persist", fmt.Errorf("persist chapter: %w", err))
		}
		if recordStage {
			_ = rec.StageSucceed("persist", nil)
		}
		return rec.Complete()
	}

	revisedText, err := s.reviseChapter(ctx, rec, exec, chapter, chapterText, report, mode, recordStage)
	if err != nil {
		return err
	}
	if recordStage {
		_ = rec.StageStart("persist", "persister")
	}
	if err := s.persistChapter(exec.book.ID, chapter, revisedText, model.ChapterStatusRevised); err != nil {
		return s.failRun(rec, "persist", fmt.Errorf("persist chapter: %w", err))
	}
	if recordStage {
		_ = rec.StageSucceed("persist", nil)
	}
	return rec.Complete()
}

func (s *PipelineService) runFullPipeline(
	ctx context.Context,
	rec *run.Recorder,
	runID string,
	exec *pipelineExecution,
	chapter int,
) error {
	currentState, err := s.loadRuntimeState(exec.book.ID)
	if err != nil {
		return s.failRun(rec, "", fmt.Errorf("load state: %w", err))
	}

	intent, _, err := s.planIntent(ctx, rec, exec, chapter, currentState, true)
	if err != nil {
		return err
	}
	if err := s.runtime.SaveIntent(exec.book.ID, intent); err != nil {
		return s.failRun(rec, "plan", fmt.Errorf("save intent: %w", err))
	}

	pkg, ruleStack, chTrace, err := s.composeContext(ctx, rec, exec, chapter, currentState, *intent, true)
	if err != nil {
		return err
	}
	if err := s.persistComposeArtifacts(exec.book.ID, chapter, pkg, ruleStack, chTrace); err != nil {
		return s.failRun(rec, "compose", err)
	}

	chapterText, _, err := s.writeDraft(ctx, rec, exec, chapter, *intent, *pkg, *ruleStack, true)
	if err != nil {
		return err
	}
	nextState, err := s.settleChapter(ctx, rec, exec, chapter, chapterText, currentState, true)
	if err != nil {
		return err
	}
	chapterText, normalized, err := s.normalizeDraft(ctx, rec, exec, chapterText, true)
	if err != nil {
		return err
	}
	if normalized {
		// Re-settle the normalized prose so truth files track the final persisted text.
		// This pass is best-effort because the normalizer is explicitly told not to
		// change plot-critical state; if it times out, keep the already settled state
		// from the original draft rather than failing the entire run.
		refreshedState, settleErr := s.settleChapter(ctx, rec, exec, chapter, chapterText, currentState, false)
		if settleErr == nil {
			nextState = refreshedState
		}
	}

	report, err := s.auditChapter(ctx, rec, exec, chapter, chapterText, previousSummary(currentState, chapter), currentState, true)
	if err != nil {
		return err
	}
	maxAuditRetries := exec.runnerCfg.MaxAuditRetries
	if maxAuditRetries <= 0 {
		maxAuditRetries = 3
	}
	revised := false
	for attempt := 1; report.HasCriticalIssues(); attempt++ {
		chapterText, err = s.reviseChapter(ctx, rec, exec, chapter, chapterText, report, "", true)
		if err != nil {
			return err
		}
		revised = true
		reAudit, err := s.auditChapter(ctx, rec, exec, chapter, chapterText, previousSummary(currentState, chapter), currentState, false)
		if err != nil {
			return err
		}
		report = reAudit
		if report.HasCriticalIssues() && attempt >= maxAuditRetries {
			break
		}
	}
	if revised {
		nextState, err = s.settleChapter(ctx, rec, exec, chapter, chapterText, currentState, false)
		if err != nil {
			return err
		}
	} else {
		_ = rec.StageSkip("revise", "no critical issues detected")
	}
	_ = rec.StageStart("persist", "persister")
	if err := s.saveRuntimeState(exec.book.ID, nextState); err != nil {
		return s.failRun(rec, "persist", fmt.Errorf("save state: %w", err))
	}
	if err := s.persistChapter(exec.book.ID, chapter, chapterText, model.ChapterStatusPendingReview); err != nil {
		return s.failRun(rec, "persist", fmt.Errorf("persist chapter: %w", err))
	}
	if err := s.snapshots.Save(exec.book.ID, &model.ChapterSnapshot{
		Chapter:   chapter,
		CreatedAt: time.Now().UTC(),
		State:     &nextState,
	}); err != nil {
		return s.failRun(rec, "persist", fmt.Errorf("save snapshot: %w", err))
	}
	_ = rec.StageSucceed("persist", nil)
	return rec.Complete()
}

func (s *PipelineService) settleChapter(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	chapterText string,
	currentState model.RuntimeState,
	recordStage bool,
) (model.RuntimeState, error) {
	facts, observeErr := s.observeChapter(ctx, rec, exec, chapter, chapterText, currentState, recordStage)
	if observeErr != nil && recordStage {
		_ = rec.StageFail("observe", observeErr)
		facts = nil
	}
	nextState, err := s.reflectChapter(ctx, rec, exec, chapter, chapterText, facts, currentState, recordStage)
	if err != nil {
		return model.RuntimeState{}, err
	}
	return nextState, nil
}

func (s *PipelineService) planIntent(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	currentState model.RuntimeState,
	recordStage bool,
) (*model.ChapterIntent, *model.TokenUsage, error) {
	if recordStage {
		_ = rec.StageStart("plan", "planner")
	}
	intent, usage, err := exec.agents.Planner.Plan(ctx, agent.PlannerInput{
		Book:         *exec.book,
		Chapter:      chapter,
		State:        currentState,
		OutlineText:  exec.outlineText,
		AgendaConfig: hookAgendaConfig(),
	})
	if err != nil {
		if recordStage {
			return nil, nil, s.failRun(rec, "plan", err)
		}
		return nil, nil, s.failRun(rec, "", err)
	}
	if recordStage {
		_ = rec.RecordTrace(&model.PromptTrace{
			RunID:         recRunID(rec),
			StageName:     "plan",
			Role:          "planner",
			PromptProfile: exec.profiles["planner"],
			ResponseText:  mustJSON(intent),
			Usage:         usage,
		})
		_ = rec.StageSucceed("plan", usage)
	}
	return intent, usage, nil
}

func (s *PipelineService) composeContext(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	currentState model.RuntimeState,
	intent model.ChapterIntent,
	recordStage bool,
) (*model.ContextPackage, *model.RuleStack, *model.ChapterTrace, error) {
	if recordStage {
		_ = rec.StageStart("compose", "composer")
	}
	pkg, ruleStack, chTrace, err := exec.agents.Composer.Compose(ctx, agent.ComposerInput{
		Book:        *exec.book,
		Chapter:     chapter,
		Intent:      intent,
		State:       currentState,
		TokenBudget: agent.RecommendedComposeTokenBudget(lengthSpec(exec.book, exec.runnerCfg).Target),
	})
	if err != nil {
		if recordStage {
			return nil, nil, nil, s.failRun(rec, "compose", err)
		}
		return nil, nil, nil, s.failRun(rec, "", err)
	}
	if recordStage {
		_ = rec.RecordTrace(&model.PromptTrace{
			RunID:         recRunID(rec),
			StageName:     "compose",
			Role:          "composer",
			PromptProfile: exec.profiles["composer"],
			ResponseText:  mustJSON(map[string]any{"context": pkg, "ruleStack": ruleStack, "trace": chTrace}),
		})
		_ = rec.StageSucceed("compose", nil)
	}
	return pkg, ruleStack, chTrace, nil
}

func (s *PipelineService) writeDraft(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	intent model.ChapterIntent,
	pkg model.ContextPackage,
	ruleStack model.RuleStack,
	recordStage bool,
) (string, *model.TokenUsage, error) {
	if recordStage {
		_ = rec.StageStart("write", "writer")
	}
	writerOut, err := exec.agents.Writer.Write(ctx, agent.WriterInput{
		Book:       *exec.book,
		Chapter:    chapter,
		Intent:     intent,
		Context:    pkg,
		RuleStack:  ruleStack,
		LengthSpec: lengthSpec(exec.book, exec.runnerCfg),
	}, func(token string) error {
		rec.BroadcastToken(token)
		return nil
	})
	if err != nil {
		if recordStage {
			return "", nil, s.failRun(rec, "write", err)
		}
		return "", nil, s.failRun(rec, "", err)
	}
	if recordStage {
		_ = rec.RecordTrace(&model.PromptTrace{
			RunID:         recRunID(rec),
			StageName:     "write",
			Role:          "writer",
			PromptProfile: exec.profiles["writer"],
			ResponseText:  writerOut.Content,
			Usage:         writerOut.Usage,
		})
		_ = rec.StageSucceed("write", writerOut.Usage)
	}
	return writerOut.Content, writerOut.Usage, nil
}

func (s *PipelineService) normalizeDraft(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapterText string,
	recordStage bool,
) (string, bool, error) {
	length := lengthSpec(exec.book, exec.runnerCfg)
	currentCount := utf8.RuneCountInString(chapterText)
	if currentCount >= length.Min && currentCount <= length.Max {
		if recordStage {
			_ = rec.StageSkip("normalize", "word count within range")
		}
		return chapterText, false, nil
	}
	if recordStage {
		_ = rec.StageStart("normalize", "normalizer")
	}
	out, err := exec.agents.Normalizer.Normalize(ctx, agent.NormalizerInput{
		ChapterText: chapterText,
		TargetMin:   length.Min,
		TargetMax:   length.Max,
		Language:    string(exec.book.Language),
	})
	if err != nil {
		if recordStage {
			return "", false, s.failRun(rec, "normalize", err)
		}
		return "", false, s.failRun(rec, "", err)
	}
	if recordStage {
		_ = rec.RecordTrace(&model.PromptTrace{
			RunID:         recRunID(rec),
			StageName:     "normalize",
			Role:          "normalizer",
			PromptProfile: exec.profiles["normalizer"],
			ResponseText:  out.Content,
		})
		_ = rec.StageSucceed("normalize", nil)
	}
	return out.Content, out.Action != "unchanged", nil
}

func (s *PipelineService) auditChapter(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	chapterText string,
	prevSummary string,
	currentState model.RuntimeState,
	recordStage bool,
) (*model.AuditReport, error) {
	textCtx, err := s.loadAuditTextContext(exec.book.ID, chapter, currentState)
	if err != nil {
		return nil, s.failRun(rec, "", fmt.Errorf("load audit context: %w", err))
	}
	if recordStage {
		_ = rec.StageStart("audit", "auditor")
	}
	report, usage, err := exec.agents.Auditor.Audit(ctx, agent.AuditorInput{
		Book:                 *exec.book,
		Chapter:              chapter,
		ChapterText:          chapterText,
		PreviousSummary:      prevSummary,
		State:                currentState,
		PreviousChapterText:  textCtx.PreviousChapterText,
		CurrentStateText:     textCtx.CurrentStateText,
		ParticleLedgerText:   textCtx.ParticleLedgerText,
		HooksText:            textCtx.HooksText,
		ChapterSummariesText: textCtx.ChapterSummariesText,
		SubplotBoardText:     textCtx.SubplotBoardText,
		EmotionalArcsText:    textCtx.EmotionalArcsText,
		CharacterMatrixText:  textCtx.CharacterMatrixText,
		StyleGuideText:       textCtx.StyleGuideText,
		StoryBibleText:       textCtx.StoryBibleText,
		VolumeOutlineText:    textCtx.VolumeOutlineText,
		ParentCanonText:      textCtx.ParentCanonText,
		FanficCanonText:      textCtx.FanficCanonText,
	})
	if err != nil {
		if recordStage {
			return nil, s.failRun(rec, "audit", err)
		}
		return nil, s.failRun(rec, "", err)
	}
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID:         recRunID(rec),
		StageName:     "audit",
		Role:          "auditor",
		PromptProfile: exec.profiles["auditor"],
		ResponseText:  mustJSON(report),
		Usage:         usage,
	})
	if recordStage {
		_ = rec.StageSucceed("audit", usage)
	}
	return report, nil
}

func (s *PipelineService) loadAuditTextContext(bookID string, chapter int, currentState model.RuntimeState) (auditTextContext, error) {
	ctx := auditTextContext{
		CurrentStateText:     mustPrettyJSON(currentState.CurrentState),
		ParticleLedgerText:   mustPrettyJSON(currentState.ParticleLedger),
		HooksText:            mustPrettyJSON(currentState.PendingHooks),
		ChapterSummariesText: mustPrettyJSON(currentState.ChapterSummaries),
		SubplotBoardText:     mustPrettyJSON(currentState.SubplotBoard),
		EmotionalArcsText:    mustPrettyJSON(currentState.EmotionalArcs),
		CharacterMatrixText:  mustPrettyJSON(currentState.CharacterMatrix),
	}

	if chapter > 1 {
		previousChapterText, err := s.chapters.GetContent(bookID, chapter-1)
		if err == nil {
			ctx.PreviousChapterText = previousChapterText
		}
	}

	var err error
	if ctx.StyleGuideText, err = s.readOptionalText(filepath.Join(s.dataDir, bookID, "story", "style_guide.md")); err != nil {
		return auditTextContext{}, err
	}
	if ctx.StoryBibleText, err = s.readOptionalText(filepath.Join(s.dataDir, bookID, "story", "story_bible.md")); err != nil {
		return auditTextContext{}, err
	}
	if ctx.VolumeOutlineText, err = s.readOptionalText(filepath.Join(s.dataDir, bookID, "story", "volume_outline.md")); err != nil {
		return auditTextContext{}, err
	}
	if ctx.ParentCanonText, err = s.readOptionalText(filepath.Join(s.dataDir, bookID, "story", "parent_canon.md")); err != nil {
		return auditTextContext{}, err
	}
	if ctx.FanficCanonText, err = s.readOptionalText(filepath.Join(s.dataDir, bookID, "story", "fanfic_canon.md")); err != nil {
		return auditTextContext{}, err
	}

	return ctx, nil
}

func (s *PipelineService) readOptionalText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s *PipelineService) reviseChapter(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	chapterText string,
	report *model.AuditReport,
	mode string,
	recordStage bool,
) (string, error) {
	if recordStage {
		_ = rec.StageStart("revise", "reviser")
	}
	output, err := exec.agents.Reviser.Revise(ctx, agent.ReviserInput{
		Book:        *exec.book,
		Chapter:     chapter,
		ChapterText: chapterText,
		Report:      *report,
		Mode:        NormalizeReviseMode(mode),
		AntiDetect:  exec.runnerCfg.AntiDetect || ReviseModeUsesAntiDetect(mode),
	})
	if err != nil {
		if recordStage {
			return "", s.failRun(rec, "revise", err)
		}
		return "", s.failRun(rec, "", err)
	}
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID:         recRunID(rec),
		StageName:     "revise",
		Role:          "reviser",
		PromptProfile: exec.profiles["reviser"],
		ResponseText:  output.Content,
		Usage:         output.Usage,
	})
	if recordStage {
		_ = rec.StageSucceed("revise", output.Usage)
	}
	return output.Content, nil
}

func (s *PipelineService) observeChapter(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	chapterText string,
	currentState model.RuntimeState,
	recordStage bool,
) ([]model.ObservedFact, error) {
	if recordStage {
		_ = rec.StageStart("observe", "observer")
	}
	output, err := exec.agents.Observer.Observe(ctx, agent.ObserverInput{
		Book:        *exec.book,
		Chapter:     chapter,
		ChapterText: chapterText,
		State:       currentState,
	})
	if err != nil {
		return nil, err
	}
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID:         recRunID(rec),
		StageName:     "observe",
		Role:          "observer",
		PromptProfile: exec.profiles["observer"],
		ResponseText:  mustJSON(output.Facts),
		Usage:         output.Usage,
	})
	if recordStage {
		_ = rec.StageSucceed("observe", output.Usage)
	}
	return output.Facts, nil
}

func (s *PipelineService) reflectChapter(
	ctx context.Context,
	rec *run.Recorder,
	exec *pipelineExecution,
	chapter int,
	chapterText string,
	facts []model.ObservedFact,
	currentState model.RuntimeState,
	recordStage bool,
) (model.RuntimeState, error) {
	textCtx := reflectTextContext(currentState)
	if recordStage {
		_ = rec.StageStart("reflect", "reflector")
	}
	output, err := exec.agents.Reflector.Reflect(ctx, agent.ReflectorInput{
		Book:                 *exec.book,
		Chapter:              chapter,
		ChapterText:          chapterText,
		Facts:                facts,
		State:                currentState,
		CurrentStateText:     textCtx.CurrentStateText,
		HooksText:            textCtx.HooksText,
		ChapterSummariesText: textCtx.ChapterSummariesText,
		SubplotBoardText:     textCtx.SubplotBoardText,
		EmotionalArcsText:    textCtx.EmotionalArcsText,
		CharacterMatrixText:  textCtx.CharacterMatrixText,
		PreviousSummary:      previousSummary(currentState, chapter),
	})
	if err != nil {
		if recordStage {
			return model.RuntimeState{}, s.failRun(rec, "reflect", err)
		}
		return model.RuntimeState{}, s.failRun(rec, "", err)
	}
	nextState, err := state.ApplyRuntimeStateDelta(currentState, output.Delta)
	if err != nil {
		if recordStage {
			return model.RuntimeState{}, s.failRun(rec, "reflect", err)
		}
		return model.RuntimeState{}, s.failRun(rec, "", err)
	}
	if err := state.ValidateRuntimeState(nextState); err != nil {
		if recordStage {
			return model.RuntimeState{}, s.failRun(rec, "reflect", err)
		}
		return model.RuntimeState{}, s.failRun(rec, "", err)
	}
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID:         recRunID(rec),
		StageName:     "reflect",
		Role:          "reflector",
		PromptProfile: exec.profiles["reflector"],
		ResponseText:  mustJSON(output.Delta),
		Usage:         output.Usage,
	})
	if recordStage {
		_ = rec.StageSucceed("reflect", output.Usage)
	}
	return nextState, nil
}

func reflectTextContext(currentState model.RuntimeState) auditTextContext {
	return auditTextContext{
		CurrentStateText:     mustPrettyJSON(currentState.CurrentState),
		HooksText:            mustPrettyJSON(currentState.PendingHooks),
		ChapterSummariesText: mustPrettyJSON(currentState.ChapterSummaries),
		SubplotBoardText:     mustPrettyJSON(currentState.SubplotBoard),
		EmotionalArcsText:    mustPrettyJSON(currentState.EmotionalArcs),
		CharacterMatrixText:  mustPrettyJSON(currentState.CharacterMatrix),
	}
}

func (s *PipelineService) persistComposeArtifacts(
	bookID string,
	chapter int,
	pkg *model.ContextPackage,
	ruleStack *model.RuleStack,
	chTrace *model.ChapterTrace,
) error {
	if err := s.runtime.SaveContext(bookID, pkg); err != nil {
		return fmt.Errorf("save context: %w", err)
	}
	if err := s.runtime.SaveRuleStack(bookID, chapter, ruleStack); err != nil {
		return fmt.Errorf("save rule stack: %w", err)
	}
	if err := s.runtime.SaveTrace(bookID, chTrace); err != nil {
		return fmt.Errorf("save trace: %w", err)
	}
	return nil
}

func (s *PipelineService) persistChapter(bookID string, chapter int, content string, status model.ChapterStatus) error {
	now := time.Now().UTC()
	meta, err := s.chapters.GetMeta(bookID, chapter)
	if err != nil {
		meta = &model.ChapterMeta{
			Number:    chapter,
			Title:     defaultChapterTitle(chapter),
			CreatedAt: now,
		}
	}
	meta.Status = status
	meta.WordCount = utf8.RuneCountInString(content)
	meta.UpdatedAt = now
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = defaultChapterTitle(chapter)
	}
	if err := s.chapters.SaveContent(bookID, chapter, content); err != nil {
		return err
	}
	return s.chapters.SaveMeta(bookID, meta)
}

func (s *PipelineService) loadRuntimeState(bookID string) (model.RuntimeState, error) {
	var st model.RuntimeState
	if err := s.truth.Read(bookID, store.TruthCurrentState, &st.CurrentState); err != nil {
		return st, err
	}
	if err := s.truth.Read(bookID, store.TruthParticleLedger, &st.ParticleLedger); err != nil {
		return st, err
	}
	if err := s.truth.Read(bookID, store.TruthPendingHooks, &st.PendingHooks); err != nil {
		return st, err
	}
	if err := s.truth.Read(bookID, store.TruthChapterSummaries, &st.ChapterSummaries); err != nil {
		return st, err
	}
	if err := s.truth.Read(bookID, store.TruthSubplotBoard, &st.SubplotBoard); err != nil {
		return st, err
	}
	if err := s.truth.Read(bookID, store.TruthEmotionalArcs, &st.EmotionalArcs); err != nil {
		return st, err
	}
	if err := s.truth.Read(bookID, store.TruthCharacterMatrix, &st.CharacterMatrix); err != nil {
		return st, err
	}
	return st, nil
}

func (s *PipelineService) saveRuntimeState(bookID string, st model.RuntimeState) error {
	writes := []struct {
		name store.TruthFileName
		val  any
	}{
		{store.TruthCurrentState, st.CurrentState},
		{store.TruthParticleLedger, st.ParticleLedger},
		{store.TruthPendingHooks, st.PendingHooks},
		{store.TruthChapterSummaries, st.ChapterSummaries},
		{store.TruthSubplotBoard, st.SubplotBoard},
		{store.TruthEmotionalArcs, st.EmotionalArcs},
		{store.TruthCharacterMatrix, st.CharacterMatrix},
	}
	for _, w := range writes {
		if err := s.truth.Write(bookID, w.name, w.val); err != nil {
			return err
		}
	}
	return nil
}

func (s *PipelineService) loadOutlineText(bookID string) string {
	candidates := []string{
		filepath.Join(s.dataDir, bookID, "story", "volume_outline.md"),
		filepath.Join(s.dataDir, bookID, "story", "foundation", "plot_outline.json"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}

func (s *PipelineService) notifyCompletion(
	ctx context.Context,
	bookID string,
	chapter int,
	runID string,
	status model.RunStatus,
	err error,
) {
	eventType := "run.completed"
	payload := map[string]any{}
	if err != nil {
		eventType = "run.failed"
		payload["error"] = err.Error()
	}
	_ = s.notifier.Notify(ctx, notify.Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		BookID:    bookID,
		Chapter:   chapter,
		RunID:     runID,
		Status:    string(status),
		Payload:   payload,
	})
}

func (s *PipelineService) failRun(rec *run.Recorder, stage string, err error) error {
	if stage != "" {
		_ = rec.StageFail(stage, err)
	}
	_ = rec.Fail(err)
	return err
}

func profileNameForAgent(cfg model.ProjectConfig, agentName string) string {
	for _, binding := range cfg.AgentLLMBindings {
		if strings.TrimSpace(binding.Agent) == agentName {
			return strings.TrimSpace(binding.Profile)
		}
	}
	return strings.TrimSpace(cfg.DefaultLLMProfile)
}

func hydrateProjectLLMConfig(cfg *model.ProjectConfig) {
	if cfg == nil {
		return
	}
	cfg.LLM = hydrateLLMConfig(cfg.LLM)
	for i := range cfg.LLMProfiles {
		profile := &cfg.LLMProfiles[i]
		hydrated := hydrateLLMConfig(model.LLMConfig{
			Provider:       profile.Provider,
			Model:          profile.Model,
			BaseURL:        profile.BaseURL,
			APIKey:         profile.APIKey,
			WireAPI:        profile.WireAPI,
			Stream:         profile.Stream,
			Temperature:    profile.Temperature,
			MaxTokens:      profile.MaxTokens,
			ThinkingBudget: profile.ThinkingBudget,
		})
		profile.Provider = hydrated.Provider
		profile.Model = hydrated.Model
		profile.BaseURL = hydrated.BaseURL
		profile.APIKey = hydrated.APIKey
		profile.WireAPI = hydrated.WireAPI
		profile.Stream = hydrated.Stream
		profile.Temperature = hydrated.Temperature
		profile.MaxTokens = hydrated.MaxTokens
		profile.ThinkingBudget = hydrated.ThinkingBudget
	}
	cfg.LLM = model.ResolveDefaultLLMConfig(*cfg)
}

func hydrateLLMConfig(cfg model.LLMConfig) model.LLMConfig {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return cfg
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "anthropic", "claude":
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	default:
		cfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	return cfg
}

func lengthSpec(book *model.BookConfig, cfg pipe.RunnerConfig) agent.LengthSpec {
	target := book.ChapterWordCount
	if target <= 0 {
		target = 3000
	}
	min := cfg.ChapterWordCountMin
	max := cfg.ChapterWordCountMax
	if min <= 0 {
		min = target * 8 / 10
	}
	if max <= 0 {
		max = target * 12 / 10
	}
	return agent.LengthSpec{Min: min, Target: target, Max: max}
}

func previousSummary(st model.RuntimeState, chapter int) string {
	for i := len(st.ChapterSummaries) - 1; i >= 0; i-- {
		row := st.ChapterSummaries[i]
		if row.Chapter < chapter {
			return row.Summary
		}
	}
	return ""
}

func defaultChapterTitle(chapter int) string {
	return fmt.Sprintf("Chapter %d", chapter)
}

func NormalizeReviseMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "", "spot-fix", "polish", "rewrite", "rework", "anti-detect":
		return mode
	default:
		return "spot-fix"
	}
}

func ReviseModeRequiresRewrite(mode string) bool {
	switch NormalizeReviseMode(mode) {
	case "polish", "rewrite", "rework", "anti-detect":
		return true
	default:
		return false
	}
}

func ReviseModeUsesAntiDetect(mode string) bool {
	return NormalizeReviseMode(mode) == "anti-detect"
}

func hookAgendaConfig() hook.AgendaConfig {
	return hook.DefaultAgendaConfig()
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func mustPrettyJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func recRunID(rec *run.Recorder) string {
	type runRecorder interface {
		RunID() string
	}
	if typed, ok := any(rec).(runRecorder); ok {
		return typed.RunID()
	}
	return ""
}
