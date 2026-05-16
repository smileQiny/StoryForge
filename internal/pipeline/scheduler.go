package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"storyforge/internal/model"
	"storyforge/internal/store"
)

// SchedulerConfig controls the cron scheduler.
type SchedulerConfig struct {
	// MaxConcurrentBooks limits how many books can be written simultaneously.
	MaxConcurrentBooks int
	// PollInterval is how often the scheduler checks for queued runs.
	PollInterval time.Duration
}

// DefaultSchedulerConfig returns sensible defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		MaxConcurrentBooks: 3,
		PollInterval:       30 * time.Second,
	}
}

// Scheduler is a daemon that picks up queued runs and executes them.
type Scheduler struct {
	runner    *Runner
	bookStore *store.BookStore
	cfg       SchedulerConfig
	sem       chan struct{}
	logger    *slog.Logger
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewScheduler creates a Scheduler.
func NewScheduler(runner *Runner, bookStore *store.BookStore, cfg SchedulerConfig, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		runner:    runner,
		bookStore: bookStore,
		cfg:       cfg,
		sem:       make(chan struct{}, cfg.MaxConcurrentBooks),
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start launches the scheduler loop in the background.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.loop(ctx)
}

// Stop signals the scheduler to stop and waits for in-flight runs to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// poll scans all books for queued runs and dispatches them.
func (s *Scheduler) poll(ctx context.Context) {
	books, err := s.bookStore.List()
	if err != nil {
		s.logger.Error("scheduler: list books", "err", err)
		return
	}

	for _, book := range books {
		if book.Status != model.BookStatusActive {
			continue
		}
		runs, err := s.runner.stores.Run.ListByBook(book.ID)
		if err != nil {
			continue
		}
		for _, r := range runs {
			if r.Status != model.RunStatusQueued {
				continue
			}
			s.dispatch(ctx, *book, r)
		}
	}
}

// dispatch acquires the semaphore and runs the pipeline in a goroutine.
func (s *Scheduler) dispatch(ctx context.Context, book model.BookConfig, r *model.Run) {
	// Non-blocking semaphore acquire
	select {
	case s.sem <- struct{}{}:
	default:
		// At capacity — skip this run for now
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }()

		s.logger.Info("scheduler: starting run", "runId", r.ID, "bookId", book.ID, "chapter", r.Chapter)
		err := s.runner.Run(ctx, RunInput{
			Book:    book,
			Chapter: r.Chapter,
			RunID:   r.ID,
		})
		if err != nil {
			s.logger.Error("scheduler: run failed", "runId", r.ID, "err", err)
		} else {
			s.logger.Info("scheduler: run complete", "runId", r.ID)
		}
	}()
}
