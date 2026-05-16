package prompt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Loader loads prompt template text by section ID and language.
type Loader interface {
	Load(sectionID, language string) (string, error)
}

// EmbedLoader loads templates from an embed.FS (production mode).
type EmbedLoader struct {
	fs fs.FS
}

// NewEmbedLoader creates a Loader backed by an embed.FS.
func NewEmbedLoader(fsys fs.FS) *EmbedLoader {
	return &EmbedLoader{fs: fsys}
}

// Load reads a template from the embedded FS.
// sectionID like "writer/genre_intro" maps to <language>/writer/genre_intro.tmpl
func (l *EmbedLoader) Load(sectionID, language string) (string, error) {
	path := fmt.Sprintf("%s/%s.tmpl", language, sectionID)
	data, err := fs.ReadFile(l.fs, path)
	if err != nil {
		// Try language-agnostic path
		path = fmt.Sprintf("%s.tmpl", sectionID)
		data, err = fs.ReadFile(l.fs, path)
		if err != nil {
			return "", fmt.Errorf("template %q not found for language %q", sectionID, language)
		}
	}
	return string(data), nil
}

// FSLoader loads templates from the filesystem (development hot-reload mode).
type FSLoader struct {
	baseDir string
}

// NewFSLoader creates a Loader that reads from a directory on disk.
func NewFSLoader(baseDir string) *FSLoader {
	return &FSLoader{baseDir: baseDir}
}

// Load reads a template from disk, enabling hot-reload in development.
func (l *FSLoader) Load(sectionID, language string) (string, error) {
	// Try language-specific path first
	path := filepath.Join(l.baseDir, language, filepath.FromSlash(sectionID)+".tmpl")
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	// Fallback to language-agnostic
	path = filepath.Join(l.baseDir, filepath.FromSlash(sectionID)+".tmpl")
	data, err = os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("template %q not found", sectionID)
	}
	return string(data), nil
}

// MultiLoader tries multiple loaders in order, returning the first success.
type MultiLoader struct {
	loaders []Loader
}

// NewMultiLoader creates a MultiLoader.
func NewMultiLoader(loaders ...Loader) *MultiLoader {
	return &MultiLoader{loaders: loaders}
}

func (m *MultiLoader) Load(sectionID, language string) (string, error) {
	var errs []string
	for _, l := range m.loaders {
		text, err := l.Load(sectionID, language)
		if err == nil {
			return text, nil
		}
		errs = append(errs, err.Error())
	}
	return "", fmt.Errorf("template %q not found: %s", sectionID, strings.Join(errs, "; "))
}

// StaticLoader holds templates in memory (for testing and built-in defaults).
type StaticLoader struct {
	templates map[string]string // key: "<language>/<sectionID>" or "<sectionID>"
}

// NewStaticLoader creates a StaticLoader.
func NewStaticLoader(templates map[string]string) *StaticLoader {
	return &StaticLoader{templates: templates}
}

func (s *StaticLoader) Load(sectionID, language string) (string, error) {
	if t, ok := s.templates[language+"/"+sectionID]; ok {
		return t, nil
	}
	if t, ok := s.templates[sectionID]; ok {
		return t, nil
	}
	return "", fmt.Errorf("template %q not found", sectionID)
}
