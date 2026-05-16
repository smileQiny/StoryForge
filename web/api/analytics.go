package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

type analyticsHandler struct {
	svc *app.AnalyticsService
}

func (h *analyticsHandler) bookOverview(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	resp, err := h.svc.BookOverview(bookID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
