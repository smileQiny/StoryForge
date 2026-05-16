package storyforge

import (
	"embed"
	"io/fs"
)

// EmbeddedFiles bundles the minimal frontend shell and genre fixtures.
//
//go:embed all:web/frontend/dist all:genres
var EmbeddedFiles embed.FS

func FrontendFS() (fs.FS, error) {
	return fs.Sub(EmbeddedFiles, "web/frontend/dist")
}

func GenresFS() (fs.FS, error) {
	return fs.Sub(EmbeddedFiles, "genres")
}
