package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

// DetectHandler exposes chapter AI-trace detection.
type DetectHandler struct {
	svc *app.DetectService
}

// NewDetectHandler creates a DetectHandler.
func NewDetectHandler(svc *app.DetectService) *DetectHandler {
	return &DetectHandler{svc: svc}
}

func (h *DetectHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	chapter, err := parseDetectChapter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.AnalyzeChapter(bookID, chapter)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DetectHandler) AnalyzePath(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	chapter, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.svc.AnalyzeChapter(bookID, chapter)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DetectHandler) analyze(w http.ResponseWriter, r *http.Request) {
	h.Analyze(w, r)
}

func (h *DetectHandler) AnalyzeAll(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	results, err := h.svc.AnalyzeAll(bookID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookId":  bookID,
		"results": results,
	})
}

func (h *DetectHandler) Stats(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	stats, err := h.svc.Stats(bookID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func parseDetectChapter(r *http.Request) (int, error) {
	if raw := r.URL.Query().Get("chapter"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return 0, errors.New("invalid chapter query parameter")
		}
		return n, nil
	}

	var body struct {
		Chapter int `json:"chapter"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
	}
	if body.Chapter <= 0 {
		return 0, errors.New("chapter is required")
	}
	return body.Chapter, nil
}
