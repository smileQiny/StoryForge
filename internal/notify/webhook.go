package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"storyforge/internal/model"
)

// ConfigLoader is the configuration dependency needed by the webhook dispatcher.
type ConfigLoader interface {
	Get() (*model.ProjectConfig, error)
}

// Event is the normalized outbound webhook payload.
type Event struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	BookID    string         `json:"bookId,omitempty"`
	Chapter   int            `json:"chapter,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	Status    string         `json:"status,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// WebhookDispatcher signs and sends configured webhook events.
type WebhookDispatcher struct {
	loader ConfigLoader
	client *http.Client
}

// NewWebhookDispatcher creates a dispatcher backed by project config.
func NewWebhookDispatcher(loader ConfigLoader) *WebhookDispatcher {
	return &WebhookDispatcher{
		loader: loader,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Notify dispatches an event to all configured webhook subscribers.
func (d *WebhookDispatcher) Notify(ctx context.Context, event Event) error {
	if d == nil || d.loader == nil {
		return nil
	}
	cfg, err := d.loader.Get()
	if err != nil {
		return err
	}
	if len(cfg.Webhooks) == 0 {
		return nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal webhook event: %w", err)
	}

	var firstErr error
	for _, hook := range cfg.Webhooks {
		if !hook.Enabled || !shouldSend(event.Type, hook.EventFilters) {
			continue
		}
		reqCtx := ctx
		cancel := func() {}
		if hook.TimeoutMS > 0 {
			reqCtx, cancel = context.WithTimeout(ctx, time.Duration(hook.TimeoutMS)*time.Millisecond)
		}
		err := d.send(reqCtx, hook, body, event)
		cancel()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *WebhookDispatcher) send(ctx context.Context, hook model.WebhookConfig, body []byte, event Event) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-StoryForge-Event", event.Type)
	req.Header.Set("X-StoryForge-Timestamp", event.Timestamp.UTC().Format(time.RFC3339))
	req.Header.Set("X-StoryForge-Signature", signHMACSHA256(hook.Secret, body))
	for key, value := range hook.Headers {
		req.Header.Set(key, value)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func signHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func shouldSend(eventType string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		if filter == "*" || filter == eventType {
			return true
		}
		if strings.HasSuffix(filter, "*") {
			prefix := strings.TrimSuffix(filter, "*")
			if strings.HasPrefix(eventType, prefix) {
				return true
			}
		}
	}
	return false
}
