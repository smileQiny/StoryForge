package api_test

import (
	"net/http"
	"testing"
	"time"

	"storyforge/internal/app"
	"storyforge/internal/model"
)

func TestDaemonLifecycleAndLogs(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodGet, "/api/daemon", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("daemon status: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var status struct {
		Running bool `json:"running"`
	}
	decodeJSON(t, w, &status)
	if status.Running {
		t.Fatal("expected daemon to be stopped initially")
	}

	w = do(t, env.handler, http.MethodPost, "/api/daemon/start", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("daemon start: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	time.Sleep(50 * time.Millisecond)

	w = do(t, env.handler, http.MethodGet, "/api/daemon", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("daemon status after start: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var started struct {
		Running bool `json:"running"`
		Events  []struct {
			Message string `json:"message"`
		} `json:"events"`
	}
	decodeJSON(t, w, &started)
	if !started.Running {
		t.Fatal("expected daemon to be running after start")
	}
	if len(started.Events) == 0 {
		t.Fatal("expected daemon events after start")
	}

	w = do(t, env.handler, http.MethodPost, "/api/daemon/stop", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("daemon stop: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodGet, "/api/logs?limit=20", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("logs: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var logs struct {
		Entries []struct {
			Message string `json:"message"`
		} `json:"entries"`
	}
	decodeJSON(t, w, &logs)
	if len(logs.Entries) == 0 {
		t.Fatal("expected non-empty logs")
	}
}

func TestDaemonManualPollReturnsSummary(t *testing.T) {
	env := newTestEnv(t)

	w := do(t, env.handler, http.MethodPost, "/api/daemon/poll", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("daemon poll: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Summary struct {
			BooksTotal int `json:"booksTotal"`
		} `json:"summary"`
		TickCount int `json:"tickCount"`
	}
	decodeJSON(t, w, &payload)
	if payload.TickCount <= 0 {
		t.Fatalf("expected tick count > 0, got %d", payload.TickCount)
	}
}

func TestDaemonPollSchedulesActiveBookRun(t *testing.T) {
	env := newTestEnv(t)
	configurePipelineLLM(t, env.dir)

	w := do(t, env.handler, http.MethodPost, "/api/books", app.CreateBookInput{
		ID:               "daemon-book",
		Title:            "Daemon Book",
		Genre:            "xuanhuan",
		Language:         model.LanguageZH,
		TargetChapters:   1,
		ChapterWordCount: 3000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create book: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	active := model.BookStatusActive
	w = do(t, env.handler, http.MethodPut, "/api/books/daemon-book", app.UpdateBookInput{
		Status: &active,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("activate book: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = do(t, env.handler, http.MethodPost, "/api/daemon/poll", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("daemon poll: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status struct {
		Mode    string `json:"mode"`
		Summary struct {
			RunsQueued int `json:"runsQueued"`
		} `json:"summary"`
	}
	decodeJSON(t, w, &status)
	if status.Mode != "scheduler" {
		t.Fatalf("expected scheduler mode, got %q", status.Mode)
	}
	if status.Summary.RunsQueued <= 0 {
		t.Fatalf("expected queued scheduler run, got %+v", status.Summary)
	}

	w = do(t, env.handler, http.MethodGet, "/api/books/daemon-book/runs", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list runs: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var runs []model.Run
	decodeJSON(t, w, &runs)
	if len(runs) == 0 {
		t.Fatal("expected daemon-created run")
	}
	runDetail := waitForRunCompletion(t, env.handler, "daemon-book", runs[0].ID, 5*time.Second)
	if runDetail.TriggeredBy != model.RunTriggeredByScheduler {
		t.Fatalf("expected scheduler-triggered run, got %+v", runDetail)
	}
}
