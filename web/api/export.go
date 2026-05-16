package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
)

type ExportHandler struct {
	Svc *app.ExportService
}

// NewExportHandler creates an ExportHandler.
func NewExportHandler(svc *app.ExportService) *ExportHandler {
	return &ExportHandler{Svc: svc}
}

func (h *ExportHandler) ExportBook(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "txt"
	}
	approvedOnly := r.URL.Query().Get("approvedOnly") == "true"

	result, err := h.Svc.ExportBook(bookID, format, approvedOnly)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+result.Filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Content)
}

func (h *ExportHandler) SaveBook(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	var body struct {
		Format       string `json:"format"`
		ApprovedOnly bool   `json:"approvedOnly"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	if strings.TrimSpace(body.Format) == "" {
		body.Format = "txt"
	}

	path, chapters, err := h.Svc.SaveBook(bookID, body.Format, body.ApprovedOnly)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"path":     path,
		"format":   strings.ToLower(strings.TrimSpace(body.Format)),
		"chapters": chapters,
	})
}
