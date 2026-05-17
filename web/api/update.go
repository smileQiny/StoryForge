package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"storyforge/internal/update"
)

type updateHandler struct {
	currentVersion string
}

func newUpdateHandler(currentVersion string) *updateHandler {
	if strings.TrimSpace(currentVersion) == "" {
		currentVersion = "dev"
	}
	return &updateHandler{currentVersion: currentVersion}
}

func (h *updateHandler) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version": h.currentVersion,
	})
}

func (h *updateHandler) check(w http.ResponseWriter, r *http.Request) {
	result, err := update.NewService(h.currentVersion).Check(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *updateHandler) install(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version string `json:"version"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	result, err := update.NewService(h.currentVersion).Install(r.Context(), strings.TrimSpace(body.Version))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
