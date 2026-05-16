package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListBooksUsesHTTPAPI(t *testing.T) {
	var sawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"book-1","title":"Book One","genre":"other","status":"active","language":"zh","targetChapters":10,"chapterWordCount":8000}]`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	books, err := client.ListBooks(context.Background())
	if err != nil {
		t.Fatalf("list books: %v", err)
	}
	if sawPath != "/api/books" {
		t.Fatalf("path = %q, want /api/books", sawPath)
	}
	if len(books) != 1 || books[0].ID != "book-1" {
		t.Fatalf("unexpected books: %+v", books)
	}
}

func TestClientReturnsAPIErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"quota exhausted"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.WriteNext(context.Background(), "book-1")
	if err == nil || err.Error() != "quota exhausted" {
		t.Fatalf("error = %v, want quota exhausted", err)
	}
}

func TestNewClientRejectsInvalidAddress(t *testing.T) {
	if _, err := NewClient("not a url"); err == nil {
		t.Fatal("expected invalid address error")
	}
}

func TestNewClientUsesDefaultAddressWhenEmpty(t *testing.T) {
	client, err := NewClient("")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.baseURL != DefaultAddr {
		t.Fatalf("baseURL = %q, want %q", client.baseURL, DefaultAddr)
	}
}

func TestClientCoversBookChapterAndTruthAPIs(t *testing.T) {
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		seen[key] = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "POST /api/books/create":
			var input BookCreateInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("decode create book input: %v", err)
			}
			if input.ID != "book-1" || input.TargetChapters != 10 || input.ChapterWordCount != 8000 {
				t.Fatalf("unexpected create input: %+v", input)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"book-1","title":"Book One","genre":"short","status":"draft","language":"zh","targetChapters":10,"chapterWordCount":8000}`))
		case "GET /api/books/book-1":
			_, _ = w.Write([]byte(`{"id":"book-1","title":"Book One","genre":"short","status":"draft","language":"zh","targetChapters":10,"chapterWordCount":8000}`))
		case "PUT /api/books/book-1/chapters/1":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode chapter edit: %v", err)
			}
			if body["content"] != "updated chapter" {
				t.Fatalf("content = %q, want updated chapter", body["content"])
			}
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/books/book-1/chapters/1":
			_, _ = w.Write([]byte(`{"meta":{"number":1,"title":"One","status":"draft","wordCount":123},"content":"chapter text"}`))
		case "POST /api/books/book-1/chapters/1/approve":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/books/book-1/chapters/1/reject":
			_, _ = w.Write([]byte(`{"rollbackToChapter":1}`))
		case "POST /api/books/book-1/chapters/1/analyze":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "POST /api/books/book-1/audit/1", "POST /api/books/book-1/revise/1", "POST /api/books/book-1/rewrite/1", "POST /api/books/book-1/draft":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"runId":"run-1","chapter":1}`))
		case "GET /api/books/book-1/truth":
			_, _ = w.Write([]byte(`{"canon.json":{"ok":true}}`))
		case "GET /api/books/book-1/truth/canon.json":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "PUT /api/books/book-1/truth/canon.json":
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode truth update: %v", err)
			}
			if !body["ok"] {
				t.Fatalf("unexpected truth update: %+v", body)
			}
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/books/book-1/export-save":
			_, _ = w.Write([]byte(`{"ok":true,"path":"/tmp/book.txt","format":"txt","chapters":1}`))
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.Background()
	if _, err := client.CreateBook(ctx, BookCreateInput{ID: "book-1", Title: "Book One", Genre: "short", Language: "zh", TargetChapters: 10, ChapterWordCount: 8000}); err != nil {
		t.Fatalf("create book: %v", err)
	}
	if _, err := client.GetBook(ctx, "book-1"); err != nil {
		t.Fatalf("get book: %v", err)
	}
	if detail, err := client.GetChapter(ctx, "book-1", 1); err != nil || detail.Content != "chapter text" {
		t.Fatalf("get chapter = %+v, %v", detail, err)
	}
	if err := client.SaveChapter(ctx, "book-1", 1, "updated chapter"); err != nil {
		t.Fatalf("save chapter: %v", err)
	}
	if err := client.ApproveChapter(ctx, "book-1", 1); err != nil {
		t.Fatalf("approve chapter: %v", err)
	}
	if _, err := client.RejectChapter(ctx, "book-1", 1, "needs rewrite"); err != nil {
		t.Fatalf("reject chapter: %v", err)
	}
	if _, err := client.AnalyzeChapter(ctx, "book-1", 1); err != nil {
		t.Fatalf("analyze chapter: %v", err)
	}
	for name, fn := range map[string]func(context.Context, string, int) (*RunAccepted, error){
		"audit":   client.AuditChapter,
		"revise":  client.ReviseChapter,
		"rewrite": client.RewriteChapter,
	} {
		if _, err := fn(ctx, "book-1", 1); err != nil {
			t.Fatalf("%s chapter: %v", name, err)
		}
	}
	if _, err := client.DraftNext(ctx, "book-1"); err != nil {
		t.Fatalf("draft next: %v", err)
	}
	if _, err := client.ListTruth(ctx, "book-1"); err != nil {
		t.Fatalf("list truth: %v", err)
	}
	if _, err := client.GetTruthFile(ctx, "book-1", "canon.json"); err != nil {
		t.Fatalf("get truth file: %v", err)
	}
	if err := client.UpdateTruthFile(ctx, "book-1", "canon.json", json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatalf("update truth file: %v", err)
	}
	if saved, err := client.ExportSave(ctx, "book-1", "txt", false); err != nil || saved.Path != "/tmp/book.txt" {
		t.Fatalf("export save = %+v, %v", saved, err)
	}
	for _, key := range []string{
		"POST /api/books/create",
		"GET /api/books/book-1",
		"GET /api/books/book-1/chapters/1",
		"PUT /api/books/book-1/chapters/1",
		"POST /api/books/book-1/chapters/1/approve",
		"POST /api/books/book-1/chapters/1/reject",
		"POST /api/books/book-1/chapters/1/analyze",
		"POST /api/books/book-1/audit/1",
		"POST /api/books/book-1/revise/1",
		"POST /api/books/book-1/rewrite/1",
		"POST /api/books/book-1/draft",
		"GET /api/books/book-1/truth",
		"GET /api/books/book-1/truth/canon.json",
		"PUT /api/books/book-1/truth/canon.json",
		"POST /api/books/book-1/export-save",
	} {
		if _, ok := seen[key]; !ok {
			t.Fatalf("missing request %s; seen=%v", key, seen)
		}
	}
}

func TestClientCoversConfigOpsAndCatalogAPIs(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		seen[key] = true
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "GET /api/project":
			_, _ = w.Write([]byte(`{"name":"StoryForge","model":"gpt-5.4","wireApi":"responses"}`))
		case "GET /api/config/models":
			_, _ = w.Write([]byte(`[{"agent":"writer","profile":"default"}]`))
		case "POST /api/config/profiles/default/test":
			_, _ = w.Write([]byte(`{"name":"default","configured":true,"connected":true}`))
		case "GET /api/genres":
			_, _ = w.Write([]byte(`[{"id":"short","name":"Short","language":"zh"}]`))
		case "GET /api/logs":
			_, _ = w.Write([]byte(`{"entries":[{"level":"INFO","message":"ok"}]}`))
		case "GET /api/doctor":
			_, _ = w.Write([]byte(`{"configJson":true,"llmConnected":true}`))
		case "POST /api/daemon/poll":
			_, _ = w.Write([]byte(`{"running":true,"summary":{"booksTotal":1}}`))
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.Background()
	if _, err := client.Project(ctx); err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := client.ModelRoutes(ctx); err != nil {
		t.Fatalf("model routes: %v", err)
	}
	if _, err := client.TestProfile(ctx, "default"); err != nil {
		t.Fatalf("test profile: %v", err)
	}
	if _, err := client.ListGenres(ctx, ""); err != nil {
		t.Fatalf("list genres: %v", err)
	}
	if _, err := client.Logs(ctx, 20, ""); err != nil {
		t.Fatalf("logs: %v", err)
	}
	if _, err := client.Doctor(ctx); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if _, err := client.PollDaemon(ctx); err != nil {
		t.Fatalf("poll daemon: %v", err)
	}
	for _, key := range []string{
		"GET /api/project",
		"GET /api/config/models",
		"POST /api/config/profiles/default/test",
		"GET /api/genres",
		"GET /api/logs",
		"GET /api/doctor",
		"POST /api/daemon/poll",
	} {
		if !seen[key] {
			t.Fatalf("missing request %s", key)
		}
	}
}
