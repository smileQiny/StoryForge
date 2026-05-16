package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

type importHandler struct {
	svc    *app.ImportService
	events *EventBus
}

func (h *importHandler) importChapters(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var body struct {
		Chapters   []app.ImportedChapterInput `json:"chapters"`
		Text       string                     `json:"text"`
		SplitRegex string                     `json:"splitRegex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	input := app.ImportChaptersInput{Chapters: body.Chapters}
	if len(input.Chapters) == 0 && strings.TrimSpace(body.Text) != "" {
		chapters, err := splitChapters(body.Text, body.SplitRegex)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		input.Chapters = chapters
	}
	result, err := h.svc.ImportChapterSummaries(bookID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *importHandler) importStyle(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var body struct {
		app.StyleImportInput
		Text       string `json:"text"`
		SourceName string `json:"sourceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	input := body.StyleImportInput
	if strings.TrimSpace(input.Source) == "" && strings.TrimSpace(body.Text) != "" {
		input.Source = firstNonEmpty(strings.TrimSpace(body.SourceName), "sample")
		input.Summary = firstNonEmpty(strings.TrimSpace(input.Summary), summarizeSource(body.Text))
		input.Notes = firstNonEmpty(strings.TrimSpace(input.Notes), body.Text)
	}
	state, err := h.svc.ImportStyle(bookID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *importHandler) importCanon(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var body struct {
		FromBookID string `json:"fromBookId"`
		app.CanonImportInput
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	var (
		state map[string]any
		err   error
	)
	if strings.TrimSpace(body.FromBookID) != "" {
		state, err = h.svc.ImportCanonFromBook(bookID, body.FromBookID)
	} else {
		state, err = h.svc.ImportCanon(bookID, body.CanonImportInput)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := map[string]any{"ok": true}
	for key, value := range state {
		resp[key] = value
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *importHandler) initFanfic(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var input app.FanficInitInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if h.events != nil {
		h.events.Publish("fanfic:start", map[string]any{"bookId": bookID, "mode": input.Mode})
	}
	result, err := h.svc.InitFanfic(bookID, input)
	if err != nil {
		if h.events != nil {
			h.events.Publish("fanfic:error", map[string]any{"bookId": bookID, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.events != nil {
		h.events.Publish("fanfic:complete", map[string]any{"bookId": bookID, "mode": result.Mode})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *importHandler) getFanfic(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	result, err := h.svc.GetFanfic(bookID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *importHandler) refreshFanfic(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var body struct {
		SourceText string `json:"sourceText"`
		SourceName string `json:"sourceName"`
		app.CanonImportInput
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	input := body.CanonImportInput
	if input.Source == "" && strings.TrimSpace(body.SourceText) != "" {
		input.Source = firstNonEmpty(strings.TrimSpace(body.SourceName), "source")
		input.Title = firstNonEmpty(input.Title, input.BookID)
		input.Summary = summarizeSource(body.SourceText)
		input.Notes = body.SourceText
	}
	if h.events != nil {
		h.events.Publish("fanfic:refresh:start", map[string]any{"bookId": bookID})
	}
	result, err := h.svc.RefreshFanfic(bookID, input)
	if err != nil {
		if h.events != nil {
			h.events.Publish("fanfic:refresh:error", map[string]any{"bookId": bookID, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.events != nil {
		h.events.Publish("fanfic:refresh:complete", map[string]any{"bookId": bookID})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"bookId":       result.BookID,
		"mode":         result.Mode,
		"parentBookId": result.ParentBookID,
		"content":      result.Content,
		"canon":        result.Canon,
		"profile":      result.Profile,
	})
}

func splitChapters(text, pattern string) ([]app.ImportedChapterInput, error) {
	lines := strings.Split(text, "\n")
	regex, err := compileChapterSplitRegex(pattern)
	if err != nil {
		return nil, err
	}

	type match struct {
		title string
		start int
	}
	var matches []match
	for i, line := range lines {
		groups := regex.FindStringSubmatch(line)
		if groups == nil {
			continue
		}
		title := ""
		for _, group := range groups[1:] {
			group = strings.TrimSpace(group)
			if group != "" {
				title = group
				break
			}
		}
		matches = append(matches, match{title: title, start: i})
	}
	if len(matches) == 0 {
		return nil, httpError("no chapters matched split pattern")
	}

	chapters := make([]app.ImportedChapterInput, 0, len(matches))
	for i, item := range matches {
		end := len(lines)
		if i+1 < len(matches) {
			end = matches[i+1].start
		}
		content := strings.TrimSpace(stripTrailingLicense(strings.Join(lines[item.start+1:end], "\n")))
		title := item.title
		if title == "" {
			title = inferFallbackTitle(lines[item.start], i+1)
		}
		chapters = append(chapters, app.ImportedChapterInput{
			Number:  i + 1,
			Title:   title,
			Content: content,
		})
	}
	return chapters, nil
}

func compileChapterSplitRegex(pattern string) (*regexp.Regexp, error) {
	if strings.TrimSpace(pattern) == "" {
		return regexp.Compile(`(?i)^#{0,2}\s*(?:第[零〇○Ｏ０一二三四五六七八九十百千万\d]+(?:章|回)(?:[:：]|\s+)?\s*(.*)|Chapter\s+(?:\d+|[IVXLCDM]+)(?:\.|:|\s+)?\s*(.*))`)
	}
	return regexp.Compile(pattern)
}

func inferFallbackTitle(headingLine string, chapterNumber int) string {
	switch {
	case regexp.MustCompile(`(?i)chapter\s+(?:\d+|[ivxlcdm]+)`).MatchString(headingLine):
		return "Chapter " + strconv.Itoa(chapterNumber)
	case regexp.MustCompile(`第[零一二三四五六七八九十百千万\d]+回`).MatchString(headingLine):
		return "第" + strconv.Itoa(chapterNumber) + "回"
	default:
		return "第" + strconv.Itoa(chapterNumber) + "章"
	}
}

func stripTrailingLicense(content string) string {
	re := regexp.MustCompile(`(?im)^\s*Project Gutenberg(?:™|\(TM\))?.*$`)
	loc := re.FindStringIndex(content)
	if loc == nil {
		return content
	}
	return strings.TrimRight(content[:loc[0]], "\n\r\t ")
}

type httpError string

func (e httpError) Error() string { return string(e) }
