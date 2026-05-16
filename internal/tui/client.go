package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the existing StoryForge HTTP API. The TUI deliberately uses
// the same API surface as Web Studio so backend behavior stays single-sourced.
type Client struct {
	baseURL string
	http    *http.Client
}

const DefaultAddr = "http://127.0.0.1:8080"

type BookSummary struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Platform         string `json:"platform,omitempty"`
	Genre            string `json:"genre"`
	Status           string `json:"status"`
	Language         string `json:"language"`
	TargetChapters   int    `json:"targetChapters"`
	ChapterWordCount int    `json:"chapterWordCount"`
}

type BookCreateInput struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Genre            string `json:"genre"`
	Brief            string `json:"brief,omitempty"`
	Language         string `json:"language"`
	Platform         string `json:"platform,omitempty"`
	TargetChapters   int    `json:"targetChapters"`
	ChapterWordCount int    `json:"chapterWordCount"`
	FanficMode       string `json:"fanficMode,omitempty"`
	ParentBookID     string `json:"parentBookId,omitempty"`
}

type ChapterSummary struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	WordCount int    `json:"wordCount"`
}

type ChapterDetail struct {
	Meta    ChapterSummary `json:"meta"`
	Content string         `json:"content"`
}

type DaemonStatus struct {
	Running bool `json:"running"`
	Summary struct {
		BooksTotal    int `json:"booksTotal"`
		BooksActive   int `json:"booksActive"`
		RunsQueued    int `json:"runsQueued"`
		RunsRunning   int `json:"runsRunning"`
		RunsSucceeded int `json:"runsSucceeded"`
		RunsFailed    int `json:"runsFailed"`
	} `json:"summary"`
}

type RunAccepted struct {
	RunID             string `json:"runId"`
	Chapter           int    `json:"chapter,omitempty"`
	Mode              string `json:"mode,omitempty"`
	RollbackToChapter int    `json:"rollbackToChapter,omitempty"`
	DeletedFrom       int    `json:"deletedFrom,omitempty"`
}

type ExportSaveResult struct {
	OK       bool   `json:"ok"`
	Path     string `json:"path"`
	Format   string `json:"format"`
	Chapters int    `json:"chapters"`
}

type GenreSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
}

type LogsResponse struct {
	Entries []map[string]any `json:"entries"`
}

func NewClient(rawBaseURL string) (*Client, error) {
	baseURL := strings.TrimSpace(rawBaseURL)
	if baseURL == "" {
		baseURL = DefaultAddr
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid server address %q", rawBaseURL)
	}
	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) ListBooks(ctx context.Context) ([]BookSummary, error) {
	var books []BookSummary
	if err := c.doJSON(ctx, http.MethodGet, "/api/books", nil, &books); err != nil {
		return nil, err
	}
	return books, nil
}

func (c *Client) CreateBook(ctx context.Context, input BookCreateInput) (*BookSummary, error) {
	var book BookSummary
	if err := c.doJSON(ctx, http.MethodPost, "/api/books/create", input, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (c *Client) GetBook(ctx context.Context, bookID string) (*BookSummary, error) {
	var book BookSummary
	if err := c.doJSON(ctx, http.MethodGet, "/api/books/"+url.PathEscape(bookID), nil, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (c *Client) UpdateBook(ctx context.Context, bookID string, input map[string]any) (*BookSummary, error) {
	var book BookSummary
	if err := c.doJSON(ctx, http.MethodPut, "/api/books/"+url.PathEscape(bookID), input, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (c *Client) DeleteBook(ctx context.Context, bookID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/books/"+url.PathEscape(bookID), nil, nil)
}

func (c *Client) ListChapters(ctx context.Context, bookID string) ([]ChapterSummary, error) {
	var chapters []ChapterSummary
	if err := c.doJSON(ctx, http.MethodGet, "/api/books/"+url.PathEscape(bookID)+"/chapters", nil, &chapters); err != nil {
		return nil, err
	}
	return chapters, nil
}

func (c *Client) GetChapter(ctx context.Context, bookID string, chapter int) (*ChapterDetail, error) {
	var detail ChapterDetail
	if err := c.doJSON(ctx, http.MethodGet, bookChapterPath(bookID, chapter), nil, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (c *Client) SaveChapter(ctx context.Context, bookID string, chapter int, content string) error {
	return c.doJSON(ctx, http.MethodPut, bookChapterPath(bookID, chapter), map[string]string{"content": content}, nil)
}

func (c *Client) ApproveChapter(ctx context.Context, bookID string, chapter int) error {
	return c.doJSON(ctx, http.MethodPost, bookChapterPath(bookID, chapter)+"/approve", nil, nil)
}

func (c *Client) RejectChapter(ctx context.Context, bookID string, chapter int, reason string) (map[string]any, error) {
	var result map[string]any
	if err := c.doJSON(ctx, http.MethodPost, bookChapterPath(bookID, chapter)+"/reject", map[string]string{"reason": reason}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) AnalyzeChapter(ctx context.Context, bookID string, chapter int) (map[string]any, error) {
	var result map[string]any
	if err := c.doJSON(ctx, http.MethodPost, bookChapterPath(bookID, chapter)+"/analyze", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Daemon(ctx context.Context) (*DaemonStatus, error) {
	var status DaemonStatus
	if err := c.doJSON(ctx, http.MethodGet, "/api/daemon", nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) PollDaemon(ctx context.Context) (*DaemonStatus, error) {
	var status DaemonStatus
	if err := c.doJSON(ctx, http.MethodPost, "/api/daemon/poll", nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) SetDaemon(ctx context.Context, running bool) (*DaemonStatus, error) {
	action := "stop"
	if running {
		action = "start"
	}
	var status DaemonStatus
	if err := c.doJSON(ctx, http.MethodPost, "/api/daemon/"+action, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) WriteNext(ctx context.Context, bookID string) (*RunAccepted, error) {
	return c.triggerBook(ctx, bookID, "write-next")
}

func (c *Client) DraftNext(ctx context.Context, bookID string) (*RunAccepted, error) {
	return c.triggerBook(ctx, bookID, "draft")
}

func (c *Client) AuditChapter(ctx context.Context, bookID string, chapter int) (*RunAccepted, error) {
	return c.triggerChapter(ctx, bookID, chapter, "audit")
}

func (c *Client) ReviseChapter(ctx context.Context, bookID string, chapter int) (*RunAccepted, error) {
	return c.triggerChapter(ctx, bookID, chapter, "revise")
}

func (c *Client) RewriteChapter(ctx context.Context, bookID string, chapter int) (*RunAccepted, error) {
	return c.triggerChapter(ctx, bookID, chapter, "rewrite")
}

func (c *Client) ListTruth(ctx context.Context, bookID string) (map[string]json.RawMessage, error) {
	var files map[string]json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/api/books/"+url.PathEscape(bookID)+"/truth", nil, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func (c *Client) GetTruthFile(ctx context.Context, bookID, file string) (json.RawMessage, error) {
	var data json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, "/api/books/"+url.PathEscape(bookID)+"/truth/"+url.PathEscape(file), nil, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Client) UpdateTruthFile(ctx context.Context, bookID, file string, data json.RawMessage) error {
	return c.doJSON(ctx, http.MethodPut, "/api/books/"+url.PathEscape(bookID)+"/truth/"+url.PathEscape(file), data, nil)
}

func (c *Client) ExportSave(ctx context.Context, bookID, format string, approvedOnly bool) (*ExportSaveResult, error) {
	var result ExportSaveResult
	body := map[string]any{"format": format, "approvedOnly": approvedOnly}
	if err := c.doJSON(ctx, http.MethodPost, "/api/books/"+url.PathEscape(bookID)+"/export-save", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Project(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/project", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ModelRoutes(ctx context.Context) ([]map[string]any, error) {
	var result []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/config/models", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) TestProfile(ctx context.Context, name string) (map[string]any, error) {
	var result map[string]any
	if err := c.doJSON(ctx, http.MethodPost, "/api/config/profiles/"+url.PathEscape(name)+"/test", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ListGenres(ctx context.Context, language string) ([]GenreSummary, error) {
	path := "/api/genres"
	if strings.TrimSpace(language) != "" {
		path += "?language=" + url.QueryEscape(strings.TrimSpace(language))
	}
	var result []GenreSummary
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Logs(ctx context.Context, limit int, level string) (*LogsResponse, error) {
	values := url.Values{}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	if strings.TrimSpace(level) != "" {
		values.Set("level", strings.TrimSpace(level))
	}
	path := "/api/logs"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var result LogsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Doctor(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/doctor", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) triggerBook(ctx context.Context, bookID, action string) (*RunAccepted, error) {
	var accepted RunAccepted
	if err := c.doJSON(ctx, http.MethodPost, "/api/books/"+url.PathEscape(bookID)+"/"+action, map[string]any{}, &accepted); err != nil {
		return nil, err
	}
	return &accepted, nil
}

func (c *Client) triggerChapter(ctx context.Context, bookID string, chapter int, action string) (*RunAccepted, error) {
	var accepted RunAccepted
	if err := c.doJSON(ctx, http.MethodPost, "/api/books/"+url.PathEscape(bookID)+"/"+action+"/"+fmt.Sprintf("%d", chapter), map[string]any{}, &accepted); err != nil {
		return nil, err
	}
	return &accepted, nil
}

func bookChapterPath(bookID string, chapter int) string {
	return "/api/books/" + url.PathEscape(bookID) + "/chapters/" + fmt.Sprintf("%d", chapter)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("%s", resp.Status)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
