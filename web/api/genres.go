package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/genre"
)

type genresHandler struct {
	svc *app.GenresService
}

func (h *genresHandler) list(w http.ResponseWriter, r *http.Request) {
	language := r.URL.Query().Get("language")
	writeJSON(w, http.StatusOK, h.svc.List(language))
}

func (h *genresHandler) get(w http.ResponseWriter, r *http.Request) {
	language := chi.URLParam(r, "language")
	genreID := chi.URLParam(r, "genreID")

	cfg, err := h.svc.Get(language, genreID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *genresHandler) getCompat(w http.ResponseWriter, r *http.Request) {
	genreID := chi.URLParam(r, "genreID")
	language := r.URL.Query().Get("language")
	cfg, err := h.svc.Get(language, genreID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *genresHandler) create(w http.ResponseWriter, r *http.Request) {
	var input genre.GenreConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Create(&input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, cfg)
}

func (h *genresHandler) update(w http.ResponseWriter, r *http.Request) {
	genreID := chi.URLParam(r, "genreID")
	var input genre.GenreConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Update(genreID, &input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *genresHandler) delete(w http.ResponseWriter, r *http.Request) {
	genreID := chi.URLParam(r, "genreID")
	language := r.URL.Query().Get("language")
	if err := h.svc.Delete(language, genreID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": genreID, "language": language})
}

func (h *genresHandler) copy(w http.ResponseWriter, r *http.Request) {
	genreID := chi.URLParam(r, "genreID")
	language := r.URL.Query().Get("language")
	path, err := h.svc.Copy(language, genreID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
}
