package prompt

import (
	"embed"
	"io/fs"
)

//go:embed all:templates
var embeddedTemplates embed.FS

// NewEmbedLoaderFromBuiltin returns a Loader backed by the built-in embedded templates.
func NewEmbedLoaderFromBuiltin() *EmbedLoader {
	sub, err := fs.Sub(embeddedTemplates, "templates")
	if err != nil {
		panic("failed to sub embedded templates: " + err.Error())
	}
	return NewEmbedLoader(sub)
}

// DefaultRegistry builds a Registry pre-populated with all default profiles and
// sections loaded from the embedded templates.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, p := range DefaultProfiles() {
		r.RegisterProfile(p)
	}
	return r
}

// DefaultBuilder creates a Builder using embedded templates and default profiles.
func DefaultBuilder() *Builder {
	return NewBuilder(DefaultRegistry(), NewEmbedLoaderFromBuiltin())
}
