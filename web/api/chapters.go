package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/model"
)

type chaptersHandler struct {
	svc *app.ChaptersService
}

func (h *chaptersHandler) list(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	metas, err := h.svc.List(bookID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if metas == nil {
		metas = []*model.ChapterMeta{}
	}
	writeJSON(w, http.StatusOK, metas)
}

func (h *chaptersHandler) get(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	num, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	meta, err := h.svc.GetMeta(bookID, num)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	content, _ := h.svc.GetContent(bookID, num)

	writeJSON(w, http.StatusOK, map[string]any{
		"meta":    meta,
		"content": content,
	})
}

func (h *chaptersHandler) edit(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	num, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.EditContent(bookID, num, body.Content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseChapterNum(r *http.Request) (int, error) {
	s := chi.URLParam(r, "chapterNum")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid chapter number: %q", s)
	}
	return n, nil
}
