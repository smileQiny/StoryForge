package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"storyforge/internal/app"
	"storyforge/internal/logging"
	"storyforge/internal/model"
	"storyforge/internal/run"
	"storyforge/internal/store"
)

type daemonHandler struct {
	manager *daemonManager
}

type daemonManager struct {
	logger    *slog.Logger
	bookStore *store.BookStore
	chapters  *app.ChaptersService
	pipeline  *app.PipelineService
	runStore  *run.Store
	events    *EventBus
	streams   *run.Broadcaster
	config    *app.ConfigService

	mu          sync.Mutex
	running     bool
	startedAt   *time.Time
	lastPollAt  *time.Time
	tickCount   int
	lastSummary daemonSummary
	cancel      context.CancelFunc
}

type daemonStatus struct {
	Running            bool            `json:"running"`
	StartedAt          *time.Time      `json:"startedAt,omitempty"`
	LastPollAt         *time.Time      `json:"lastPollAt,omitempty"`
	TickCount          int             `json:"tickCount"`
	PollIntervalSec    int             `json:"pollIntervalSec"`
	MaxConcurrentBooks int             `json:"maxConcurrentBooks"`
	Summary            daemonSummary   `json:"summary"`
	Events             []logging.Entry `json:"events"`
	Mode               string          `json:"mode"`
}

type daemonSummary struct {
	BooksTotal    int `json:"booksTotal"`
	BooksActive   int `json:"booksActive"`
	RunsQueued    int `json:"runsQueued"`
	RunsRunning   int `json:"runsRunning"`
	RunsSucceeded int `json:"runsSucceeded"`
	RunsFailed    int `json:"runsFailed"`
	RunsCancelled int `json:"runsCancelled"`
	RunsScheduler int `json:"runsScheduler"`
}

type logsHandler struct{}

func newDaemonHandler(
	logger *slog.Logger,
	bookStore *store.BookStore,
	chapters *app.ChaptersService,
	pipeline *app.PipelineService,
	runStore *run.Store,
	streams *run.Broadcaster,
	events *EventBus,
	config *app.ConfigService,
) *daemonHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &daemonHandler{
		manager: &daemonManager{
			logger:    logger,
			bookStore: bookStore,
			chapters:  chapters,
			pipeline:  pipeline,
			runStore:  runStore,
			events:    events,
			streams:   streams,
			config:    config,
		},
	}
}

func newLogsHandler() *logsHandler {
	return &logsHandler{}
}

func (h *daemonHandler) get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.manager.status())
}

func (h *daemonHandler) start(w http.ResponseWriter, r *http.Request) {
	status, err := h.manager.start()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *daemonHandler) stop(w http.ResponseWriter, r *http.Request) {
	status, err := h.manager.stop()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *daemonHandler) poll(w http.ResponseWriter, r *http.Request) {
	status, err := h.manager.manualPoll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *logsHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": logging.Recent(limit, level),
	})
}

func (m *daemonManager) start() (*daemonStatus, error) {
	m.mu.Lock()
	if m.running {
		status := m.buildStatusLocked()
		m.mu.Unlock()
		return status, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	m.running = true
	m.startedAt = &now
	m.cancel = cancel
	m.tickCount = 0
	m.lastPollAt = nil
	m.lastSummary = daemonSummary{}
	m.mu.Unlock()

	m.logger.Info("daemon started", "component", "daemon")
	if m.events != nil {
		m.events.Publish("daemon:started", map[string]any{})
	}
	go m.loop(ctx)

	if _, err := m.manualPoll(context.Background()); err != nil {
		m.logger.Error("daemon initial poll failed", "component", "daemon", "error", err)
	}
	status := m.status()
	return &status, nil
}

func (m *daemonManager) stop() (*daemonStatus, error) {
	m.mu.Lock()
	if !m.running {
		status := m.buildStatusLocked()
		m.mu.Unlock()
		return status, nil
	}
	cancel := m.cancel
	m.running = false
	m.cancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	m.logger.Info("daemon stopped", "component", "daemon")
	if m.events != nil {
		m.events.Publish("daemon:stopped", map[string]any{})
	}

	status := m.status()
	return &status, nil
}

func (m *daemonManager) manualPoll(ctx context.Context) (*daemonStatus, error) {
	summary, err := m.collectSummary(ctx)
	if err != nil {
		return nil, err
	}
	scheduled, err := m.schedule(ctx, summary)
	if err != nil {
		return nil, err
	}
	summary.RunsQueued += scheduled

	m.mu.Lock()
	now := time.Now().UTC()
	m.lastPollAt = &now
	m.lastSummary = summary
	m.tickCount++
	status := m.buildStatusLocked()
	m.mu.Unlock()

	m.logger.Info(
		"daemon poll completed",
		"component", "daemon",
		"booksActive", summary.BooksActive,
		"runsQueued", summary.RunsQueued,
		"runsRunning", summary.RunsRunning,
		"scheduled", scheduled,
	)
	return status, nil
}

func (m *daemonManager) status() daemonStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *m.buildStatusLocked()
}

func (m *daemonManager) buildStatusLocked() *daemonStatus {
	cfg, _ := m.config.Get()
	maxConcurrent := 1
	if cfg != nil && cfg.MaxConcurrentBooks > 0 {
		maxConcurrent = cfg.MaxConcurrentBooks
	}
	return &daemonStatus{
		Running:            m.running,
		StartedAt:          m.startedAt,
		LastPollAt:         m.lastPollAt,
		TickCount:          m.tickCount,
		PollIntervalSec:    30,
		MaxConcurrentBooks: maxConcurrent,
		Summary:            m.lastSummary,
		Events:             daemonEvents(logging.Recent(50, "")),
		Mode:               "scheduler",
	}
}

func (m *daemonManager) loop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := m.manualPoll(ctx); err != nil {
				m.logger.Error("daemon poll failed", "component", "daemon", "error", err)
			}
		}
	}
}

func (m *daemonManager) collectSummary(ctx context.Context) (daemonSummary, error) {
	_ = ctx

	books, err := m.bookStore.List()
	if err != nil {
		return daemonSummary{}, err
	}

	summary := daemonSummary{BooksTotal: len(books)}
	for _, book := range books {
		if book.Status == model.BookStatusActive {
			summary.BooksActive++
		}

		runs, err := m.runStore.ListByBook(book.ID)
		if err != nil {
			continue
		}
		for _, item := range runs {
			switch item.Status {
			case model.RunStatusQueued:
				summary.RunsQueued++
			case model.RunStatusRunning:
				summary.RunsRunning++
			case model.RunStatusSucceeded:
				summary.RunsSucceeded++
			case model.RunStatusFailed:
				summary.RunsFailed++
			case model.RunStatusCancelled:
				summary.RunsCancelled++
			}
			if item.TriggeredBy == model.RunTriggeredByScheduler {
				summary.RunsScheduler++
			}
		}
	}
	return summary, nil
}

func daemonEvents(entries []logging.Entry) []logging.Entry {
	filtered := make([]logging.Entry, 0, len(entries))
	for _, entry := range entries {
		if component, ok := entry.Attrs["component"].(string); ok && component == "daemon" {
			filtered = append(filtered, entry)
			continue
		}
		if strings.Contains(strings.ToLower(entry.Message), "daemon") {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (m *daemonManager) schedule(ctx context.Context, summary daemonSummary) (int, error) {
	if m.pipeline == nil || m.chapters == nil {
		return 0, nil
	}
	cfg, err := m.config.Get()
	if err != nil {
		return 0, err
	}
	limit := 1
	if cfg != nil && cfg.MaxConcurrentBooks > 0 {
		limit = cfg.MaxConcurrentBooks
	}
	available := limit - summary.RunsQueued - summary.RunsRunning
	if available <= 0 {
		return 0, nil
	}

	books, err := m.bookStore.List()
	if err != nil {
		return 0, err
	}
	scheduled := 0
	for _, book := range books {
		if scheduled >= available {
			break
		}
		if book.Status != model.BookStatusActive {
			continue
		}
		runs, err := m.runStore.ListByBook(book.ID)
		if err != nil || hasInFlightRun(runs) {
			continue
		}

		metas, err := m.chapters.List(book.ID)
		if err != nil {
			continue
		}
		nextChapter := 1
		for _, meta := range metas {
			if meta.Number >= nextChapter {
				nextChapter = meta.Number + 1
			}
		}
		if book.TargetChapters > 0 && nextChapter > book.TargetChapters {
			continue
		}

		runRecord, err := m.pipeline.Trigger(ctx, app.TriggerInput{
			BookID:      book.ID,
			Chapter:     nextChapter,
			Kind:        model.RunKindFullPipeline,
			TriggeredBy: model.RunTriggeredByScheduler,
		})
		if err != nil {
			m.logger.Error("daemon schedule failed", "component", "daemon", "bookId", book.ID, "error", err)
			if m.events != nil {
				m.events.Publish("daemon:error", map[string]any{"bookId": book.ID, "error": err.Error()})
			}
			continue
		}
		scheduled++
		m.logger.Info("daemon scheduled chapter", "component", "daemon", "bookId", book.ID, "chapter", nextChapter, "runId", runRecord.ID)
		m.bridgeSchedulerRun(book.ID, nextChapter, runRecord.ID)
	}
	return scheduled, nil
}

func (m *daemonManager) bridgeSchedulerRun(bookID string, chapter int, runID string) {
	if m.streams == nil {
		return
	}
	ch, cancel := m.streams.Subscribe(runID)
	go func() {
		defer cancel()
		for event := range ch {
			switch event.Type {
			case "complete":
				if m.events != nil {
					m.events.Publish("daemon:chapter", map[string]any{"bookId": bookID, "chapter": chapter, "status": "succeeded", "runId": runID})
				}
				return
			case "error":
				if m.events != nil {
					m.events.Publish("daemon:error", map[string]any{"bookId": bookID, "chapter": chapter, "error": event.Message, "runId": runID})
				}
				return
			}
		}
	}()
}

func hasInFlightRun(runs []*model.Run) bool {
	for _, item := range runs {
		if item.Status == model.RunStatusQueued || item.Status == model.RunStatusRunning {
			return true
		}
	}
	return false
}
