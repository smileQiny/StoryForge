package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// StudioEvent is a business-level SSE event for the Studio frontend.
type StudioEvent struct {
	Event     string    `json:"event"`
	Data      any       `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EventBus is a lightweight pub/sub hub for Studio business events.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[chan StudioEvent]struct{}
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[chan StudioEvent]struct{})}
}

// Publish broadcasts a Studio event to all subscribers.
func (b *EventBus) Publish(event string, data any) {
	if b == nil {
		return
	}
	entry := StudioEvent{
		Event:     event,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

// Subscribe registers a subscriber to the event bus.
func (b *EventBus) Subscribe() (<-chan StudioEvent, func()) {
	ch := make(chan StudioEvent, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
	}
	return ch, cancel
}

type eventsHandler struct {
	bus *EventBus
}

func (h *eventsHandler) stream(w http.ResponseWriter, r *http.Request) {
	if h.bus == nil {
		writeError(w, http.StatusInternalServerError, "event bus unavailable")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ch, cancel := h.bus.Subscribe()
	defer cancel()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, open := <-ch:
			if !open {
				return
			}
			data, err := json.Marshal(event.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\n", event.Event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, "event: ping\ndata: null\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
