package run

import (
	"sync"
	"time"

	"storyforge/internal/model"
)

// RunEvent is a single event emitted during a pipeline run.
type RunEvent struct {
	RunID     string    `json:"runId"`
	Type      string    `json:"type"` // stage_start/stage_done/token/complete/error/progress
	Stage     string    `json:"stage,omitempty"`
	Message   string    `json:"message,omitempty"`
	Data      any       `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Broadcaster is a pub/sub hub for RunEvents, used to bridge pipeline progress to SSE clients.
type Broadcaster struct {
	mu       sync.RWMutex
	channels map[string][]chan RunEvent // runID → subscriber channels
}

// NewBroadcaster creates a new Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{channels: make(map[string][]chan RunEvent)}
}

// Subscribe registers a new subscriber for a run. Returns the channel and an unsubscribe func.
func (b *Broadcaster) Subscribe(runID string) (<-chan RunEvent, func()) {
	ch := make(chan RunEvent, 64)
	b.mu.Lock()
	b.channels[runID] = append(b.channels[runID], ch)
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		chs := b.channels[runID]
		for i, c := range chs {
			if c == ch {
				b.channels[runID] = append(chs[:i], chs[i+1:]...)
				break
			}
		}
		if len(b.channels[runID]) == 0 {
			delete(b.channels, runID)
		}
		close(ch)
	}
	return ch, cancel
}

// Publish sends an event to all subscribers of a run (non-blocking; slow subscribers are skipped).
func (b *Broadcaster) Publish(runID string, event RunEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.channels[runID] {
		select {
		case ch <- event:
		default:
			// subscriber too slow; skip to avoid blocking the pipeline
		}
	}
}

// Recorder wraps a Run and Broadcaster to provide a convenient recording API for pipeline stages.
type Recorder struct {
	run         *model.Run
	store       *Store
	broadcaster *Broadcaster
}

// NewRecorder creates a Recorder for the given run.
func NewRecorder(run *model.Run, store *Store, broadcaster *Broadcaster) *Recorder {
	return &Recorder{run: run, store: store, broadcaster: broadcaster}
}

// RunID returns the recorder's underlying run ID.
func (r *Recorder) RunID() string {
	if r == nil || r.run == nil {
		return ""
	}
	return r.run.ID
}

// StageStart marks a stage as started and emits an event.
func (r *Recorder) StageStart(stageName, role string) error {
	now := time.Now().UTC()
	r.run.Status = model.RunStatusRunning
	for i, s := range r.run.Stages {
		if s.Name == stageName {
			r.run.Stages[i].Status = model.StageStatusRunning
			r.run.Stages[i].StartedAt = &now
			r.run.Stages[i].Role = role
			break
		}
	}
	r.broadcaster.Publish(r.run.ID, RunEvent{
		RunID: r.run.ID, Type: "stage_start", Stage: stageName, Timestamp: now,
	})
	return r.store.Update(r.run)
}

// StageSucceed marks a stage as succeeded.
func (r *Recorder) StageSucceed(stageName string, usage *model.TokenUsage) error {
	now := time.Now().UTC()
	for i, s := range r.run.Stages {
		if s.Name == stageName {
			r.run.Stages[i].Status = model.StageStatusSucceeded
			r.run.Stages[i].EndedAt = &now
			if usage != nil {
				r.run.Stages[i].Usage = usage
			}
			break
		}
	}
	r.broadcaster.Publish(r.run.ID, RunEvent{
		RunID: r.run.ID, Type: "stage_done", Stage: stageName, Timestamp: now,
	})
	return r.store.Update(r.run)
}

// StageFail marks a stage as failed.
func (r *Recorder) StageFail(stageName string, err error) error {
	now := time.Now().UTC()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	for i, s := range r.run.Stages {
		if s.Name == stageName {
			r.run.Stages[i].Status = model.StageStatusFailed
			r.run.Stages[i].EndedAt = &now
			r.run.Stages[i].Error = msg
			break
		}
	}
	r.broadcaster.Publish(r.run.ID, RunEvent{
		RunID: r.run.ID, Type: "error", Stage: stageName, Message: msg, Timestamp: now,
	})
	return r.store.Update(r.run)
}

// StageSkip marks a stage as skipped.
func (r *Recorder) StageSkip(stageName, reason string) error {
	for i, s := range r.run.Stages {
		if s.Name == stageName {
			r.run.Stages[i].Status = model.StageStatusSkipped
			r.run.Stages[i].Error = reason
			break
		}
	}
	return r.store.Update(r.run)
}

// RecordTrace persists a PromptTrace.
func (r *Recorder) RecordTrace(trace *model.PromptTrace) error {
	return r.store.SaveTrace(trace)
}

// BroadcastToken sends a streaming token event.
func (r *Recorder) BroadcastToken(token string) {
	r.broadcaster.Publish(r.run.ID, RunEvent{
		RunID: r.run.ID, Type: "token", Message: token, Timestamp: time.Now().UTC(),
	})
}

// Complete marks the run as succeeded and emits a complete event.
func (r *Recorder) Complete() error {
	now := time.Now().UTC()
	r.run.Status = model.RunStatusSucceeded
	r.run.EndedAt = &now
	r.broadcaster.Publish(r.run.ID, RunEvent{
		RunID: r.run.ID, Type: "complete", Timestamp: now,
	})
	return r.store.Update(r.run)
}

// Fail marks the run as failed.
func (r *Recorder) Fail(err error) error {
	now := time.Now().UTC()
	r.run.Status = model.RunStatusFailed
	r.run.EndedAt = &now
	if err != nil {
		r.run.Error = err.Error()
	}
	r.broadcaster.Publish(r.run.ID, RunEvent{
		RunID: r.run.ID, Type: "error", Message: r.run.Error, Timestamp: now,
	})
	return r.store.Update(r.run)
}
