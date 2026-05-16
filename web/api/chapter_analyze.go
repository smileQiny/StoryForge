package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

type chapterAnalyzeHandler struct {
	svc *app.ChapterAnalyzeService
}

func (h *chapterAnalyzeHandler) analyze(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeError(w, http.StatusInternalServerError, "chapter analyze service is not configured")
		return
	}

	bookID := chi.URLParam(r, "bookID")
	chapter, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.Analyze(r.Context(), bookID, chapter)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
