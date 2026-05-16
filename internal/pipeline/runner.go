// Package pipeline implements the main writing pipeline and scheduler.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"storyforge/internal/agent"
	"storyforge/internal/hook"
	"storyforge/internal/model"
	"storyforge/internal/run"
	"storyforge/internal/state"
	"storyforge/internal/store"
)

// Agents bundles all agent instances needed by the runner.
type Agents struct {
	Planner    *agent.Planner
	Composer   *agent.Composer
	Writer     *agent.Writer
	Observer   *agent.Observer
	Reflector  *agent.Reflector
	Normalizer *agent.Normalizer
	Auditor    *agent.Auditor
	Reviser    *agent.Reviser
	Radar      *agent.Radar
}

// Stores bundles all persistence stores needed by the runner.
type Stores struct {
	Truth    *store.TruthStore
	Chapter  *store.ChapterStore
	Runtime  *store.RuntimeStore
	Snapshot *store.SnapshotStore
	Run      *run.Store
}

// RunnerConfig holds tunable parameters for the pipeline runner.
type RunnerConfig struct {
	// MaxAuditRetries is the maximum number of audit-revise cycles before giving up.
	MaxAuditRetries int
	// ChapterWordCountMin/Max override book defaults when non-zero.
	ChapterWordCountMin int
	ChapterWordCountMax int
	// SkipRadar disables the Radar scan.
	SkipRadar bool
	// AntiDetect enables anti-AI-detection mode in the Reviser.
	AntiDetect bool
	// GovernanceMode selects the input governance mode ("v2" or "legacy").
	GovernanceMode string
}

// DefaultRunnerConfig returns sensible defaults.
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		MaxAuditRetries: 3,
		GovernanceMode:  "v2",
	}
}

// Runner executes the full writing pipeline for a single chapter.
type Runner struct {
	agents Agents
	stores Stores
	cfg    RunnerConfig
	bc     *run.Broadcaster
}

// NewRunner creates a Runner.
func NewRunner(agents Agents, stores Stores, cfg RunnerConfig, bc *run.Broadcaster) *Runner {
	return &Runner{agents: agents, stores: stores, cfg: cfg, bc: bc}
}

// RunInput is the input to a full pipeline run.
type RunInput struct {
	Book        model.BookConfig
	Chapter     int
	RunID       string
	OutlineText string
}

// Run executes plan → compose → write → observe → reflect → normalize → audit → revise → persist.
func (r *Runner) Run(ctx context.Context, input RunInput) error {
	// Load the run record
	runRecord, err := r.stores.Run.Get(input.Book.ID, input.RunID)
	if err != nil {
		return fmt.Errorf("load run: %w", err)
	}
	rec := run.NewRecorder(runRecord, r.stores.Run, r.bc)

	// Load current state
	currentState, err := r.loadState(input.Book.ID)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Apply governance mode
	currentState = applyGovernance(currentState, r.cfg.GovernanceMode)

	// ── Stage: plan ──────────────────────────────────────────────────────────
	_ = rec.StageStart("plan", "planner")
	intent, planUsage, err := r.agents.Planner.Plan(ctx, agent.PlannerInput{
		Book:         input.Book,
		Chapter:      input.Chapter,
		State:        currentState,
		OutlineText:  input.OutlineText,
		AgendaConfig: hook.DefaultAgendaConfig(),
	})
	if err != nil {
		_ = rec.StageFail("plan", err)
		return rec.Fail(err)
	}
	_ = rec.StageSucceed("plan", planUsage)
	_ = r.stores.Runtime.SaveIntent(input.Book.ID, intent)
	_ = rec.RecordTrace(&model.PromptTrace{
		RunID: input.RunID, StageName: "plan", Role: "planner",
	})

	// ── Stage: compose ───────────────────────────────────────────────────────
	_ = rec.StageStart("compose", "composer")
	lengthSpec := r.lengthSpec(input.Book)
	pkg, ruleStack, chTrace, err := r.agents.Composer.Compose(ctx, agent.ComposerInput{
		Book:        input.Book,
		Chapter:     input.Chapter,
		Intent:      *intent,
		State:       currentState,
		TokenBudget: agent.RecommendedComposeTokenBudget(lengthSpec.Target),
	})
	if err != nil {
		_ = rec.StageFail("compose", err)
		return rec.Fail(err)
	}
	_ = rec.StageSucceed("compose", nil)
	_ = r.stores.Runtime.SaveContext(input.Book.ID, pkg)
	_ = r.stores.Runtime.SaveRuleStack(input.Book.ID, input.Chapter, ruleStack)
	_ = r.stores.Runtime.SaveTrace(input.Book.ID, chTrace)

	// ── Stage: write ─────────────────────────────────────────────────────────
	_ = rec.StageStart("write", "writer")
	writerOut, err := r.agents.Writer.Write(ctx, agent.WriterInput{
		Book:       input.Book,
		Chapter:    input.Chapter,
		Intent:     *intent,
		Context:    *pkg,
		RuleStack:  *ruleStack,
		LengthSpec: lengthSpec,
	}, func(token string) error {
		rec.BroadcastToken(token)
		return nil
	})
	if err != nil {
		_ = rec.StageFail("write", err)
		return rec.Fail(err)
	}
	chapterText := writerOut.Content
	_ = rec.StageSucceed("write", writerOut.Usage)

	// ── Stage: observe ───────────────────────────────────────────────────────
	_ = rec.StageStart("observe", "observer")
	observerOut, err := r.agents.Observer.Observe(ctx, agent.ObserverInput{
		Book:        input.Book,
		Chapter:     input.Chapter,
		ChapterText: chapterText,
		State:       currentState,
	})
	if err != nil {
		// Non-critical: log and continue
		_ = rec.StageFail("observe", err)
	} else {
		_ = rec.StageSucceed("observe", observerOut.Usage)
	}

	// ── Stage: reflect ───────────────────────────────────────────────────────
	_ = rec.StageStart("reflect", "reflector")
	var facts []model.ObservedFact
	if observerOut != nil {
		facts = observerOut.Facts
	}
	reflectorOut, err := r.agents.Reflector.Reflect(ctx, agent.ReflectorInput{
		Book:        input.Book,
		Chapter:     input.Chapter,
		ChapterText: chapterText,
		Facts:       facts,
		State:       currentState,
	})
	if err != nil {
		_ = rec.StageFail("reflect", err)
		return rec.Fail(fmt.Errorf("reflect: %w", err))
	}
	_ = rec.StageSucceed("reflect", reflectorOut.Usage)

	// Apply delta and validate
	nextState, err := state.ApplyRuntimeStateDelta(currentState, reflectorOut.Delta)
	if err != nil {
		_ = rec.StageFail("reflect", err)
		return rec.Fail(fmt.Errorf("apply delta: %w", err))
	}
	if err := state.ValidateRuntimeState(nextState); err != nil {
		_ = rec.StageFail("reflect", err)
		return rec.Fail(fmt.Errorf("validate state: %w", err))
	}

	// ── Stage: normalize ─────────────────────────────────────────────────────
	if writerOut.NeedsNormalize {
		_ = rec.StageStart("normalize", "normalizer")
		normOut, err := r.agents.Normalizer.Normalize(ctx, agent.NormalizerInput{
			ChapterText: chapterText,
			TargetMin:   lengthSpec.Min,
			TargetMax:   lengthSpec.Max,
			Language:    string(input.Book.Language),
		})
		if err == nil && normOut.Action != "unchanged" {
			chapterText = normOut.Content
			nextState, err = r.settleChapter(ctx, input, rec, chapterText, currentState, false)
			if err != nil {
				return err
			}
		}
		_ = rec.StageSucceed("normalize", nil)
	} else {
		_ = rec.StageSkip("normalize", "word count within range")
	}

	// ── Stage: audit + revise loop ───────────────────────────────────────────
	prevSummary := previousSummary(currentState, input.Chapter)
	chapterText, revised, err := r.auditReviseLoop(ctx, input, rec, chapterText, prevSummary, nextState)
	if err != nil {
		return err
	}
	if revised {
		nextState, err = r.settleChapter(ctx, input, rec, chapterText, currentState, false)
		if err != nil {
			return err
		}
	}

	// ── Stage: persist ───────────────────────────────────────────────────────
	_ = rec.StageStart("persist", "persister")
	if err := r.saveState(input.Book.ID, nextState); err != nil {
		_ = rec.StageFail("persist", err)
		return rec.Fail(fmt.Errorf("save state: %w", err))
	}
	if err := r.stores.Chapter.SaveContent(input.Book.ID, input.Chapter, chapterText); err != nil {
		_ = rec.StageFail("persist", err)
		return rec.Fail(fmt.Errorf("save chapter: %w", err))
	}
	if r.stores.Snapshot != nil {
		if err := r.stores.Snapshot.Save(input.Book.ID, &model.ChapterSnapshot{
			Chapter:   input.Chapter,
			CreatedAt: time.Now().UTC(),
			State:     &nextState,
		}); err != nil {
			_ = rec.StageFail("persist", err)
			return rec.Fail(fmt.Errorf("save snapshot: %w", err))
		}
	}
	_ = rec.StageSucceed("persist", nil)

	// ── Radar scan (non-critical) ─────────────────────────────────────────────
	if r.agents.Radar != nil {
		_, _ = r.agents.Radar.Scan(ctx, agent.RadarInput{
			ChapterText: chapterText,
			Language:    string(input.Book.Language),
			Skip:        r.cfg.SkipRadar,
		})
	}

	return rec.Complete()
}

// auditReviseLoop runs the audit-revise cycle up to MaxAuditRetries times.
// It returns the final chapter text (possibly revised).
func (r *Runner) auditReviseLoop(
	ctx context.Context,
	input RunInput,
	rec *run.Recorder,
	chapterText, prevSummary string,
	st model.RuntimeState,
) (string, bool, error) {
	maxRetries := r.cfg.MaxAuditRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	revised := false

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Audit
		stageName := fmt.Sprintf("audit")
		if attempt > 0 {
			stageName = fmt.Sprintf("audit-%d", attempt+1)
		}
		_ = rec.StageStart(stageName, "auditor")
		report, auditUsage, err := r.agents.Auditor.Audit(ctx, agent.AuditorInput{
			Book:            input.Book,
			Chapter:         input.Chapter,
			ChapterText:     chapterText,
			PreviousSummary: prevSummary,
			State:           st,
		})
		if err != nil {
			_ = rec.StageFail(stageName, err)
			return chapterText, revised, rec.Fail(fmt.Errorf("audit: %w", err))
		}
		_ = rec.StageSucceed(stageName, auditUsage)

		// Save audit trace
		auditJSON, _ := json.Marshal(report)
		_ = rec.RecordTrace(&model.PromptTrace{
			RunID: input.RunID, StageName: stageName, Role: "auditor",
			ResponseText: string(auditJSON), Usage: auditUsage,
		})

		if report.Passed || !report.HasCriticalIssues() {
			break
		}

		// Revise
		reviseStageName := fmt.Sprintf("revise")
		if attempt > 0 {
			reviseStageName = fmt.Sprintf("revise-%d", attempt+1)
		}
		_ = rec.StageStart(reviseStageName, "reviser")
		revOut, err := r.agents.Reviser.Revise(ctx, agent.ReviserInput{
			Book:        input.Book,
			Chapter:     input.Chapter,
			ChapterText: chapterText,
			Report:      *report,
			AntiDetect:  r.cfg.AntiDetect,
		})
		if err != nil {
			_ = rec.StageFail(reviseStageName, err)
			return chapterText, revised, rec.Fail(fmt.Errorf("revise: %w", err))
		}
		chapterText = revOut.Content
		revised = true
		_ = rec.StageSucceed(reviseStageName, revOut.Usage)
	}

	return chapterText, revised, nil
}

func (r *Runner) settleChapter(
	ctx context.Context,
	input RunInput,
	rec *run.Recorder,
	chapterText string,
	currentState model.RuntimeState,
	recordStage bool,
) (model.RuntimeState, error) {
	if recordStage {
		_ = rec.StageStart("observe", "observer")
	}
	observerOut, err := r.agents.Observer.Observe(ctx, agent.ObserverInput{
		Book:        input.Book,
		Chapter:     input.Chapter,
		ChapterText: chapterText,
		State:       currentState,
	})
	if err != nil {
		if recordStage {
			_ = rec.StageFail("observe", err)
		}
		observerOut = nil
	} else if recordStage {
		_ = rec.StageSucceed("observe", observerOut.Usage)
	}

	if recordStage {
		_ = rec.StageStart("reflect", "reflector")
	}
	var facts []model.ObservedFact
	if observerOut != nil {
		facts = observerOut.Facts
	}
	reflectorOut, err := r.agents.Reflector.Reflect(ctx, agent.ReflectorInput{
		Book:        input.Book,
		Chapter:     input.Chapter,
		ChapterText: chapterText,
		Facts:       facts,
		State:       currentState,
	})
	if err != nil {
		if recordStage {
			_ = rec.StageFail("reflect", err)
		}
		return model.RuntimeState{}, rec.Fail(fmt.Errorf("reflect: %w", err))
	}
	if recordStage {
		_ = rec.StageSucceed("reflect", reflectorOut.Usage)
	}

	nextState, err := state.ApplyRuntimeStateDelta(currentState, reflectorOut.Delta)
	if err != nil {
		if recordStage {
			_ = rec.StageFail("reflect", err)
		}
		return model.RuntimeState{}, rec.Fail(fmt.Errorf("apply delta: %w", err))
	}
	if err := state.ValidateRuntimeState(nextState); err != nil {
		if recordStage {
			_ = rec.StageFail("reflect", err)
		}
		return model.RuntimeState{}, rec.Fail(fmt.Errorf("validate state: %w", err))
	}
	return nextState, nil
}

// loadState reads the RuntimeState from the 7 truth files.
func (r *Runner) loadState(bookID string) (model.RuntimeState, error) {
	var s model.RuntimeState

	if err := r.stores.Truth.Read(bookID, store.TruthCurrentState, &s.CurrentState); err != nil {
		return s, err
	}
	if err := r.stores.Truth.Read(bookID, store.TruthParticleLedger, &s.ParticleLedger); err != nil {
		return s, err
	}
	if err := r.stores.Truth.Read(bookID, store.TruthPendingHooks, &s.PendingHooks); err != nil {
		return s, err
	}
	if err := r.stores.Truth.Read(bookID, store.TruthChapterSummaries, &s.ChapterSummaries); err != nil {
		return s, err
	}
	if err := r.stores.Truth.Read(bookID, store.TruthSubplotBoard, &s.SubplotBoard); err != nil {
		return s, err
	}
	if err := r.stores.Truth.Read(bookID, store.TruthEmotionalArcs, &s.EmotionalArcs); err != nil {
		return s, err
	}
	if err := r.stores.Truth.Read(bookID, store.TruthCharacterMatrix, &s.CharacterMatrix); err != nil {
		return s, err
	}

	return s, nil
}

// saveState writes the RuntimeState to the 7 truth files.
func (r *Runner) saveState(bookID string, s model.RuntimeState) error {
	writes := []struct {
		name store.TruthFileName
		val  any
	}{
		{store.TruthCurrentState, s.CurrentState},
		{store.TruthParticleLedger, s.ParticleLedger},
		{store.TruthPendingHooks, s.PendingHooks},
		{store.TruthChapterSummaries, s.ChapterSummaries},
		{store.TruthSubplotBoard, s.SubplotBoard},
		{store.TruthEmotionalArcs, s.EmotionalArcs},
		{store.TruthCharacterMatrix, s.CharacterMatrix},
	}
	for _, w := range writes {
		if err := r.stores.Truth.Write(bookID, w.name, w.val); err != nil {
			return fmt.Errorf("write %s: %w", w.name, err)
		}
	}
	return nil
}

func (r *Runner) lengthSpec(book model.BookConfig) agent.LengthSpec {
	target := book.ChapterWordCount
	if target <= 0 {
		target = 3000
	}
	min := r.cfg.ChapterWordCountMin
	max := r.cfg.ChapterWordCountMax
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

// applyGovernance applies input governance mode transformations.
func applyGovernance(st model.RuntimeState, mode string) model.RuntimeState {
	// v2 mode: no-op for now (hook admission is handled by the Planner/Reflector)
	// legacy mode: same
	_ = mode
	return st
}

// nowPtr returns a pointer to the current UTC time.
func nowPtr() *time.Time {
	t := time.Now().UTC()
	return &t
}
