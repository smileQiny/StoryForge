package app

import (
	"testing"
	"time"
)

func TestNewBootstrapFoundationContextAllowsSlowLLMReviews(t *testing.T) {
	ctx, cancel := newBootstrapFoundationContext()
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected bootstrap context to carry a deadline")
	}

	remaining := time.Until(deadline)
	if remaining < 3*time.Minute {
		t.Fatalf("expected at least 3 minutes of bootstrap budget, got %s", remaining)
	}
}
