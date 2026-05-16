package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/store"
)

type truthHandler struct {
	svc *app.TruthService
}

func (h *truthHandler) getAll(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	view, err := h.svc.GetAll(bookID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h *truthHandler) getFile(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	fileName := store.TruthFileName(chi.URLParam(r, "file"))
	if !isValidTruthFile(fileName) {
		writeError(w, http.StatusBadRequest, "unknown truth file: "+string(fileName))
		return
	}
	data, err := h.svc.GetFile(bookID, fileName)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *truthHandler) updateFile(w http.ResponseWriter, r *http.Request) {
	bookID := chi.URLParam(r, "bookID")
	fileName := store.TruthFileName(chi.URLParam(r, "file"))
	if !isValidTruthFile(fileName) {
		writeError(w, http.StatusBadRequest, "unknown truth file: "+string(fileName))
		return
	}
	raw, err := decodeTruthUpdatePayload(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.UpdateFile(bookID, fileName, raw); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isValidTruthFile(name store.TruthFileName) bool {
	for _, f := range store.AllTruthFiles {
		if f == name {
			return true
		}
	}
	return false
}

func decodeTruthUpdatePayload(body io.Reader) (json.RawMessage, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty body")
	}
	if compat := unwrapTruthContentPayload(raw); compat != nil {
		return compat, nil
	}
	return json.RawMessage(raw), nil
}

func unwrapTruthContentPayload(raw []byte) json.RawMessage {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload) != 1 {
		return nil
	}
	content, ok := payload["content"]
	if !ok {
		return nil
	}
	var text string
	if err := json.Unmarshal(content, &text); err != nil {
		return nil
	}
	inner := bytes.TrimSpace([]byte(text))
	if !json.Valid(inner) {
		return nil
	}
	return json.RawMessage(inner)
}
