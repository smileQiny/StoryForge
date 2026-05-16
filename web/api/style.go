package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

type styleHandler struct {
	svc *app.StyleService
}

type styleAnalyzeHandler struct {
	svc *app.StyleAnalyzeService
}

func (h *styleHandler) analyze(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var input app.StyleAnalyzeInput
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	result, err := h.svc.Analyze(bookID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *styleAnalyzeHandler) analyzeGlobal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	result, err := h.svc.AnalyzeText(body.Text)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
