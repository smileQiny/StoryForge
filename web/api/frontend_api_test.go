package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestFrontendRootAndFallback(t *testing.T) {
	h := newTestHandler(t)

	w := do(t, h, http.MethodGet, "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("root: expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("root: expected html content type, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "StoryForge") {
		t.Fatalf("root: expected index content, got %s", w.Body.String())
	}

	w = do(t, h, http.MethodGet, "/this/path/does/not/exist", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("fallback: expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "StoryForge") {
		t.Fatalf("fallback: expected index content, got %s", w.Body.String())
	}
}
