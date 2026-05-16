package api

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

func pathParam(r *http.Request, key string) string {
	raw := chi.URLParam(r, key)
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}
