package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const maxEntries = 500

type Entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Raw     string         `json:"raw"`
}

type buffer struct {
	mu      sync.Mutex
	entries []Entry
}

var logBuffer = &buffer{}

type captureWriter struct{}

func (captureWriter) Write(p []byte) (int, error) {
	lines := bytes.Split(p, []byte("\n"))
	for _, line := range lines {
		text := strings.TrimSpace(string(line))
		if text == "" {
			continue
		}
		logBuffer.append(parseEntry(text))
	}
	return len(p), nil
}

func Recent(limit int, level string) []Entry {
	return logBuffer.recent(limit, level)
}

func parseEntry(raw string) Entry {
	entry := Entry{
		Time: time.Now().UTC().Format(time.RFC3339),
		Raw:  raw,
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		entry.Level = "INFO"
		entry.Message = raw
		return entry
	}

	if value, ok := payload["time"].(string); ok && value != "" {
		entry.Time = value
	}
	if value, ok := payload["level"].(string); ok {
		entry.Level = strings.ToUpper(value)
	}
	if value, ok := payload["msg"].(string); ok {
		entry.Message = value
	}
	delete(payload, "time")
	delete(payload, "level")
	delete(payload, "msg")
	if len(payload) > 0 {
		entry.Attrs = payload
	}
	return entry
}

func (b *buffer) append(entry Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = append(b.entries, entry)
	if len(b.entries) > maxEntries {
		b.entries = append([]Entry(nil), b.entries[len(b.entries)-maxEntries:]...)
	}
}

func (b *buffer) recent(limit int, level string) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	if limit > maxEntries {
		limit = maxEntries
	}

	filtered := make([]Entry, 0, limit)
	want := strings.ToUpper(strings.TrimSpace(level))
	for index := len(b.entries) - 1; index >= 0; index-- {
		entry := b.entries[index]
		if want != "" && entry.Level != want {
			continue
		}
		filtered = append(filtered, entry)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}
