package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

type reviewHandler struct {
	svc *app.ReviewService
}

func (h *reviewHandler) approve(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	chapter, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Approve(bookID, chapter); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *reviewHandler) reject(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	chapter, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}

	result, err := h.svc.Reject(r.Context(), bookID, chapter, body.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
