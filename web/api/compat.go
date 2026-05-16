package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"unicode"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/llm"
	"storyforge/internal/model"
)

type compatHandler struct {
	books        *app.BooksService
	imports      *app.ImportService
	config       *app.ConfigService
	styleAnalyze *app.StyleAnalyzeService
	events       *EventBus
	createStatus *bookCreateTracker
}

type bookCreateTracker struct {
	mu     sync.RWMutex
	status map[string]map[string]any
}

func newBookCreateTracker() *bookCreateTracker {
	return &bookCreateTracker{status: make(map[string]map[string]any)}
}

func (t *bookCreateTracker) Set(bookID string, status map[string]any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.status[bookID] = status
}

func (t *bookCreateTracker) Get(bookID string) (map[string]any, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	status, ok := t.status[bookID]
	if !ok {
		return nil, false
	}
	out := make(map[string]any, len(status))
	for key, value := range status {
		out[key] = value
	}
	return out, true
}

func (t *bookCreateTracker) Delete(bookID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.status, bookID)
}

func (h *compatHandler) createBook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title            string `json:"title"`
		Genre            string `json:"genre"`
		Brief            string `json:"brief"`
		Language         string `json:"language"`
		Platform         string `json:"platform"`
		ChapterWordCount int    `json:"chapterWordCount"`
		TargetChapters   int    `json:"targetChapters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	bookID := slugifyTitle(body.Title)
	if _, err := h.books.Get(bookID); err == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "Book \"" + bookID + "\" already exists"})
		return
	}
	targetChapters := body.TargetChapters
	if targetChapters <= 0 {
		targetChapters = 200
	}
	chapterWordCount := body.ChapterWordCount
	if chapterWordCount <= 0 {
		chapterWordCount = 3000
	}
	h.createStatus.Set(bookID, map[string]any{"status": "creating"})
	if h.events != nil {
		h.events.Publish("book:creating", map[string]any{"bookId": bookID, "title": strings.TrimSpace(body.Title)})
	}

	go func() {
		_, err := h.books.Create(app.CreateBookInput{
			ID:               bookID,
			Title:            strings.TrimSpace(body.Title),
			Genre:            firstNonEmpty(strings.TrimSpace(body.Genre), "xuanhuan"),
			Brief:            strings.TrimSpace(body.Brief),
			Language:         compatLanguage(body.Language),
			Platform:         firstNonEmpty(strings.TrimSpace(body.Platform), "serial-web"),
			TargetChapters:   targetChapters,
			ChapterWordCount: chapterWordCount,
		})
		if err != nil {
			h.createStatus.Set(bookID, map[string]any{"status": "error", "error": err.Error()})
			if h.events != nil {
				h.events.Publish("book:error", map[string]any{"bookId": bookID, "error": err.Error()})
			}
			return
		}
		h.createStatus.Delete(bookID)
		if h.events != nil {
			h.events.Publish("book:created", map[string]any{"bookId": bookID})
		}
	}()

	writeJSON(w, http.StatusOK, map[string]any{"status": "creating", "bookId": bookID})
}

func (h *compatHandler) createBookStatus(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	if status, ok := h.createStatus.Get(bookID); ok {
		writeJSON(w, http.StatusOK, status)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"status": "missing"})
}

func (h *compatHandler) initFanficRoot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title            string `json:"title"`
		SourceText       string `json:"sourceText"`
		SourceName       string `json:"sourceName"`
		Mode             string `json:"mode"`
		Genre            string `json:"genre"`
		Platform         string `json:"platform"`
		TargetChapters   int    `json:"targetChapters"`
		ChapterWordCount int    `json:"chapterWordCount"`
		Language         string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Title) == "" || strings.TrimSpace(body.SourceText) == "" {
		writeError(w, http.StatusBadRequest, "title and sourceText are required")
		return
	}

	bookID := slugifyTitle(body.Title)
	mode := mapCompatFanficMode(body.Mode)
	summary := summarizeSource(body.SourceText)
	if h.events != nil {
		h.events.Publish("fanfic:start", map[string]any{"bookId": bookID, "title": body.Title, "mode": mode})
	}

	_, err := h.books.Create(app.CreateBookInput{
		ID:               bookID,
		Title:            strings.TrimSpace(body.Title),
		Genre:            firstNonEmpty(strings.TrimSpace(body.Genre), "xuanhuan"),
		Language:         compatLanguage(body.Language),
		Platform:         firstNonEmpty(strings.TrimSpace(body.Platform), "serial-web"),
		TargetChapters:   max(body.TargetChapters, 12),
		ChapterWordCount: max(body.ChapterWordCount, 3000),
		Brief:            buildFanficBootstrapBrief(firstNonEmpty(strings.TrimSpace(body.SourceName), strings.TrimSpace(body.Title)), summary, mode),
		FanficMode:       mode,
	})
	if err != nil {
		if h.events != nil {
			h.events.Publish("fanfic:error", map[string]any{"bookId": bookID, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.imports.InitFanfic(bookID, app.FanficInitInput{
		Mode:            mode,
		SourceTitle:     firstNonEmpty(strings.TrimSpace(body.SourceName), strings.TrimSpace(body.Title)),
		SourceSummary:   summary,
		DivergencePoint: defaultFanficDivergence(mode),
		OriginalPremise: "Build an original fanfic storyline from imported source material.",
		Notes:           body.SourceText,
	})
	if err != nil {
		_ = h.books.Delete(bookID)
		if h.events != nil {
			h.events.Publish("fanfic:error", map[string]any{"bookId": bookID, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = h.imports.RefreshFanfic(bookID, app.CanonImportInput{
		Source:  firstNonEmpty(strings.TrimSpace(body.SourceName), "source"),
		Title:   strings.TrimSpace(body.Title),
		Summary: summary,
		Notes:   body.SourceText,
	})
	if h.events != nil {
		h.events.Publish("fanfic:complete", map[string]any{"bookId": bookID, "mode": result.Mode})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bookId": bookID, "mode": result.Mode})
}

func (h *compatHandler) agent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Instruction string `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	body.Instruction = strings.TrimSpace(body.Instruction)
	if body.Instruction == "" {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}
	if h.events != nil {
		h.events.Publish("agent:start", map[string]any{"instruction": body.Instruction})
	}

	cfg, err := h.config.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	router, err := llm.BuildFromConfig(*cfg)
	if err != nil {
		if h.events != nil {
			h.events.Publish("agent:error", map[string]any{"instruction": body.Instruction, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := router.ForAgent("architect").Chat(r.Context(), llm.ChatRequest{
		Model: model.ResolveAgentLLMConfig(*cfg, "architect").Model,
		Messages: []llm.Message{
			{Role: "system", Content: "You are the StoryForge studio agent. Respond concisely and directly."},
			{Role: "user", Content: body.Instruction},
		},
	})
	if err != nil {
		if h.events != nil {
			h.events.Publish("agent:error", map[string]any{"instruction": body.Instruction, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.events != nil {
		h.events.Publish("agent:complete", map[string]any{"instruction": body.Instruction})
	}
	writeJSON(w, http.StatusOK, map[string]any{"response": resp.Content})
}

func compatLanguage(language string) model.Language {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en":
		return model.LanguageEN
	default:
		return model.LanguageZH
	}
}

func mapCompatFanficMode(mode string) model.FanficMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "alternate", "au":
		return model.FanficModeAlternate
	case "continuation", "canon":
		return model.FanficModeContinuation
	case "reverse", "ooc":
		return model.FanficModeReverse
	case "inspired", "cp":
		return model.FanficModeInspired
	default:
		return model.FanficModeInspired
	}
}

func summarizeSource(text string) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) > 240 {
		return string(runes[:240]) + "..."
	}
	return text
}

func defaultFanficDivergence(mode model.FanficMode) string {
	switch mode {
	case model.FanficModeAlternate:
		return "The narrative diverges immediately into an alternate timeline instead of replaying canon."
	case model.FanficModeContinuation:
		return "The story begins after the imported source material and continues with original consequences."
	case model.FanficModeReverse:
		return "The narrative restarts from a reversed or opposing viewpoint and must not retell canon scenes."
	default:
		return "The story uses the imported material only as inspiration and diverges immediately into new plot."
	}
}

func buildFanficBootstrapBrief(sourceName, summary string, mode model.FanficMode) string {
	parts := []string{
		"Build an original fanfic foundation without replaying canon scenes.",
		"Source: " + firstNonEmpty(strings.TrimSpace(sourceName), "imported material"),
		"Divergence point: " + defaultFanficDivergence(mode),
	}
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		parts = append(parts, "Source summary: "+trimmed)
	}
	return strings.Join(parts, " ")
}

func slugifyTitle(title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	var b strings.Builder
	lastDash := false
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.In(r, unicode.Han):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "fanfic-book"
	}
	if len([]rune(out)) > 30 {
		return string([]rune(out)[:30])
	}
	return out
}
