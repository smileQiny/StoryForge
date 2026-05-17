package api

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	storyforge "storyforge"
	"storyforge/internal/app"
	"storyforge/internal/genre"
	"storyforge/internal/notify"
	"storyforge/internal/run"
	"storyforge/internal/state"
	"storyforge/internal/store"
)

var BuildVersion = "dev"

// Services bundles all application services needed by the API handlers.
type Services struct {
	BookStore      *store.BookStore
	Books          *app.BooksService
	Chapters       *app.ChaptersService
	Truth          *app.TruthService
	Pipeline       *app.PipelineService
	ChapterAnalyze *app.ChapterAnalyzeService
	Review         *app.ReviewService
	Import         *app.ImportService
	Style          *app.StyleService
	StyleAnalyze   *app.StyleAnalyzeService
	Export         *app.ExportService
	Detect         *app.DetectService
	Config         *app.ConfigService
	Genres         *app.GenresService
	Analytics      *app.AnalyticsService
	RunStore       *run.Store
	Broadcaster    *run.Broadcaster
	Events         *EventBus
	CreateTrack    *bookCreateTracker
}

// NewDefaultServices wires up all services using the given data directory.
func NewDefaultServices(dataDir string, genresFS fs.FS) (*Services, error) {
	bookStore := store.NewBookStore(dataDir)
	chapterStore := store.NewChapterStore(dataDir)
	truthStore := store.NewTruthStore(dataDir)
	runtimeStore := store.NewRuntimeStore(dataDir)
	snapshotStore := store.NewSnapshotStore(dataDir)
	fileLock := store.NewFileLock()
	runStore := run.NewStore(dataDir)
	broadcaster := run.NewBroadcaster()
	eventBus := NewEventBus()
	memoryDB, err := state.OpenMemoryDB(filepath.Join(dataDir, "memory.db"))
	if err != nil {
		return nil, err
	}
	configService := app.NewConfigService(dataDir)
	webhookDispatcher := notify.NewWebhookDispatcher(configService)
	genreRegistry, err := genre.LoadFromFS(genresFS)
	if err != nil {
		return nil, err
	}
	pipelineSvc := app.NewPipelineService(
		dataDir,
		bookStore,
		chapterStore,
		truthStore,
		runtimeStore,
		snapshotStore,
		configService,
		runStore,
		broadcaster,
		webhookDispatcher,
	)

	return &Services{
		BookStore:      bookStore,
		Books:          app.NewBooksService(dataDir, bookStore, truthStore, configService, fileLock, slog.Default()),
		Chapters:       app.NewChaptersService(chapterStore, bookStore),
		Truth:          app.NewTruthService(truthStore, bookStore),
		Pipeline:       pipelineSvc,
		ChapterAnalyze: app.NewChapterAnalyzeService(pipelineSvc),
		Review:         app.NewReviewService(dataDir, bookStore, chapterStore, truthStore, runtimeStore, snapshotStore, runStore, memoryDB, fileLock),
		Import:         app.NewImportService(dataDir, bookStore, chapterStore, truthStore),
		Style:          app.NewStyleService(bookStore, chapterStore, truthStore),
		StyleAnalyze:   app.NewStyleAnalyzeService(bookStore, chapterStore),
		Export:         app.NewExportService(dataDir, bookStore, chapterStore),
		Detect:         app.NewDetectService(bookStore, chapterStore),
		Config:         configService,
		Genres:         app.NewGenresService(dataDir, genreRegistry),
		Analytics:      app.NewAnalyticsService(bookStore, chapterStore, runStore),
		RunStore:       runStore,
		Broadcaster:    broadcaster,
		Events:         eventBus,
		CreateTrack:    newBookCreateTracker(),
	}, nil
}

func NewHandler(logger *slog.Logger) (http.Handler, error) {
	return NewHandlerWithServices(logger, nil, "")
}

// NewHandlerWithServices creates the HTTP handler with explicit services (for testing).
func NewHandlerWithServices(logger *slog.Logger, svc *Services, dataDir string) (http.Handler, error) {
	frontendFS, err := storyforge.FrontendFS()
	if err != nil {
		return nil, err
	}

	genresFS, err := storyforge.GenresFS()
	if err != nil {
		return nil, err
	}

	if svc == nil {
		if dataDir == "" {
			dataDir = "./data"
		}
		svc, err = NewDefaultServices(dataDir, genresFS)
		if err != nil {
			return nil, err
		}
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if repoRoot, err = filepath.Abs(repoRoot); err != nil {
		return nil, err
	}
	if dataDir, err = filepath.Abs(dataDir); err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	router.Get("/api/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		entries, err := fs.ReadDir(genresFS, ".")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read genres")
			return
		}
		type bootstrap struct {
			Status     string   `json:"status"`
			GenreRoots []string `json:"genreRoots"`
		}
		roots := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				roots = append(roots, entry.Name())
			}
		}
		writeJSON(w, http.StatusOK, bootstrap{Status: "ok", GenreRoots: roots})
	})

	compatH := &compatHandler{
		books:        svc.Books,
		imports:      svc.Import,
		config:       svc.Config,
		styleAnalyze: svc.StyleAnalyze,
		events:       svc.Events,
		createStatus: svc.CreateTrack,
	}
	styleAnalyzeH := &styleAnalyzeHandler{svc: svc.StyleAnalyze}
	router.Post("/api/agent", compatH.agent)
	router.Post("/api/fanfic/init", compatH.initFanficRoot)
	router.Post("/api/style/analyze", styleAnalyzeH.analyzeGlobal)

	// Books
	booksHandler := &booksHandler{svc: svc.Books}
	router.Post("/api/books/create", compatH.createBook)
	router.Route("/api/books", func(r chi.Router) {
		r.Get("/", booksHandler.list)
		r.Post("/", booksHandler.create)
		r.Route("/{bookID}", func(r chi.Router) {
			r.Get("/", booksHandler.get)
			r.Put("/", booksHandler.update)
			r.Delete("/", booksHandler.delete)
			r.Get("/create-status", compatH.createBookStatus)

			// Chapters
			chaptersH := &chaptersHandler{svc: svc.Chapters}
			chapterAnalyzeH := &chapterAnalyzeHandler{svc: svc.ChapterAnalyze}
			r.Get("/chapters", chaptersH.list)
			r.Get("/chapters/{chapterNum}", chaptersH.get)
			r.Put("/chapters/{chapterNum}", chaptersH.edit)
			r.Post("/chapters/{chapterNum}/analyze", chapterAnalyzeH.analyze)

			// Truth files
			truthH := &truthHandler{svc: svc.Truth}
			r.Get("/truth", truthH.getAll)
			r.Get("/truth/{file}", truthH.getFile)
			r.Put("/truth/{file}", truthH.updateFile)

			// Review
			reviewH := &reviewHandler{svc: svc.Review}
			r.Post("/chapters/{chapterNum}/approve", reviewH.approve)
			r.Post("/chapters/{chapterNum}/reject", reviewH.reject)
			r.Post("/chapters/{chapterNum}/review/approve", reviewH.approve)
			r.Post("/chapters/{chapterNum}/review/reject", reviewH.reject)

			// Import
			importH := &importHandler{svc: svc.Import, events: svc.Events}
			r.Post("/import/chapters", importH.importChapters)
			r.Post("/import/style", importH.importStyle)
			r.Post("/import/canon", importH.importCanon)
			r.Post("/style/import", importH.importStyle)
			r.Post("/fanfic/init", importH.initFanfic)
			r.Get("/fanfic", importH.getFanfic)
			r.Post("/fanfic/refresh", importH.refreshFanfic)

			// Style
			styleH := &styleHandler{svc: svc.Style}
			r.Get("/style/analyze", styleH.analyze)
			r.Post("/style/analyze", styleH.analyze)

			// Export
			exportH := NewExportHandler(svc.Export)
			r.Get("/export", exportH.ExportBook)
			r.Post("/export-save", exportH.SaveBook)

			// Detect
			detectH := NewDetectHandler(svc.Detect)
			r.Get("/detect", detectH.Analyze)
			r.Post("/detect", detectH.Analyze)
			r.Post("/detect/{chapterNum}", detectH.AnalyzePath)
			r.Post("/detect-all", detectH.AnalyzeAll)
			r.Get("/detect/stats", detectH.Stats)

			// Pipeline triggers
			pipelineH := &pipelineHandler{svc: svc.Pipeline, review: svc.Review, chapters: svc.Chapters, broadcaster: svc.Broadcaster, events: svc.Events}
			r.Post("/write", pipelineH.triggerWrite)
			r.Post("/write-next", pipelineH.triggerWriteNext)
			r.Post("/plan", pipelineH.triggerPlan)
			r.Post("/compose", pipelineH.triggerCompose)
			r.Post("/draft", pipelineH.triggerDraft)
			r.Post("/audit/{chapterNum}", pipelineH.triggerAudit)
			r.Post("/revise/{chapterNum}", pipelineH.triggerRevise)
			r.Post("/rewrite/{chapterNum}", pipelineH.rewrite)

			// Runs for this book
			r.Get("/runs", pipelineH.listRuns)
		})
	})

	// Runs (global)
	runsH := &runsHandler{svc: svc.Pipeline, broadcaster: svc.Broadcaster}
	router.Route("/api/runs/{runID}", func(r chi.Router) {
		r.Get("/", runsH.get)
		r.Get("/events", runsH.events)
		r.Get("/traces", runsH.traces)
	})

	eventsH := &eventsHandler{bus: svc.Events}
	router.Get("/api/events", eventsH.stream)

	// Project config
	configH := &configHandler{svc: svc.Config}
	router.Route("/api/project", func(r chi.Router) {
		r.Get("/", configH.getProject)
		r.Put("/", configH.updateProject)
		r.Post("/language", configH.setLanguage)
		r.Get("/model-overrides", configH.getModelOverrides)
		r.Put("/model-overrides", configH.updateModelOverrides)
		r.Get("/notify", configH.getNotify)
		r.Put("/notify", configH.updateNotify)
	})
	router.Route("/api/config", func(r chi.Router) {
		r.Get("/", configH.get)
		r.Put("/", configH.update)
		r.Get("/models", configH.getModels)
		r.Put("/models/{agent}", configH.updateModel)
		r.Post("/profiles/{name}/test", configH.testProfile)
	})

	// Genres
	genresH := &genresHandler{svc: svc.Genres}
	router.Get("/api/genres", genresH.list)
	router.Get("/api/genres/{language}/{genreID}", genresH.get)
	router.Get("/api/genres/{genreID}", genresH.getCompat)
	router.Post("/api/genres/create", genresH.create)
	router.Put("/api/genres/{genreID}", genresH.update)
	router.Delete("/api/genres/{genreID}", genresH.delete)
	router.Post("/api/genres/{genreID}/copy", genresH.copy)

	// Analytics
	analyticsH := &analyticsHandler{svc: svc.Analytics}
	router.Get("/api/books/{bookID}/analytics", analyticsH.bookOverview)
	router.Get("/api/analytics/books/{bookID}/overview", analyticsH.bookOverview)

	// Ops shell
	opsH := newOpsShellHandler(repoRoot, dataDir)
	router.Get("/api/ops/shell", opsH.describe)
	router.Post("/api/ops/shell", opsH.run)
	router.Get("/api/ops/terminal/ws", opsH.terminalWS)

	daemonH := newDaemonHandler(logger, svc.BookStore, svc.Chapters, svc.Pipeline, svc.RunStore, svc.Broadcaster, svc.Events, svc.Config)
	router.Get("/api/daemon", daemonH.get)
	router.Post("/api/daemon/start", daemonH.start)
	router.Post("/api/daemon/stop", daemonH.stop)
	router.Post("/api/daemon/poll", daemonH.poll)

	logsH := newLogsHandler()
	router.Get("/api/logs", logsH.list)

	radarDoctorH := newRadarDoctorHandler(logger, repoRoot, dataDir, svc.Books, svc.Chapters, svc.Config)
	router.Post("/api/radar/scan", radarDoctorH.scan)
	router.Get("/api/doctor", radarDoctorH.doctor)

	updateH := newUpdateHandler(BuildVersion)
	router.Get("/api/version", updateH.version)
	router.Get("/api/update/check", updateH.check)
	router.Post("/api/update/install", updateH.install)

	router.Handle("/*", spaHandler(frontendFS, logger))

	return router, nil
}

func spaHandler(frontendFS fs.FS, logger *slog.Logger) http.Handler {
	fileServer := http.FileServer(http.FS(frontendFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}
		if _, err := fs.Stat(frontendFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		if cleanPath != "index.html" {
			logger.Debug("frontend asset not found, falling back to index", "path", cleanPath)
		}
		index, err := fs.ReadFile(frontendFS, "index.html")
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
