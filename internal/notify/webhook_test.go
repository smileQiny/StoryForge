package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"storyforge/internal/model"
)

type stubConfigLoader struct {
	cfg *model.ProjectConfig
}

func (s stubConfigLoader) Get() (*model.ProjectConfig, error) {
	return s.cfg, nil
}

func TestShouldSend(t *testing.T) {
	if !shouldSend("run.queued", nil) {
		t.Fatal("expected empty filters to allow all events")
	}
	if !shouldSend("run.queued", []string{"run.*"}) {
		t.Fatal("expected wildcard filter to match event prefix")
	}
	if shouldSend("run.queued", []string{"review.*"}) {
		t.Fatal("expected unmatched prefix filter to reject event")
	}
}

func TestWebhookDispatcher_NotifySignsPayload(t *testing.T) {
	var gotSig string
	var gotEvent Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-StoryForge-Signature")
		if err := json.NewDecoder(r.Body).Decode(&gotEvent); err != nil {
			t.Fatalf("decode webhook body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dispatcher := NewWebhookDispatcher(stubConfigLoader{cfg: &model.ProjectConfig{
		Webhooks: []model.WebhookConfig{{
			Enabled:      true,
			URL:          srv.URL,
			Secret:       "secret",
			EventFilters: []string{"run.*"},
			TimeoutMS:    1000,
		}},
	}})

	event := Event{
		Type:      "run.queued",
		Timestamp: time.Now().UTC(),
		BookID:    "book-1",
		RunID:     "run-1",
		Chapter:   3,
	}
	if err := dispatcher.Notify(context.Background(), event); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if gotSig == "" {
		t.Fatal("expected HMAC signature header")
	}
	if gotEvent.Type != "run.queued" || gotEvent.RunID != "run-1" {
		t.Fatalf("unexpected webhook payload: %+v", gotEvent)
	}
}
