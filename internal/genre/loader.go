package genre

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// GenreConfig is the configuration for a single genre.
type GenreConfig struct {
	ID              string   `yaml:"id" json:"id"`
	Name            string   `yaml:"name" json:"name"`
	Language        string   `yaml:"language" json:"language"`
	NumericalSystem bool     `yaml:"numericalSystem" json:"numericalSystem"`
	PowerScaling    bool     `yaml:"powerScaling" json:"powerScaling"`
	EraResearch     bool     `yaml:"eraResearch" json:"eraResearch"`
	FatigueWords    []string `yaml:"fatigueWords" json:"fatigueWords"`
	Rules           []string `yaml:"rules" json:"rules"`
	AuditDimensions []string `yaml:"auditDimensions" json:"auditDimensions"`
}

// AuditDimensionConfig holds genre-level audit dimension settings.
type AuditDimensionConfig struct {
	Required []string `yaml:"required" json:"required"`
	Disabled []string `yaml:"disabled" json:"disabled"`
}

// Registry holds all loaded genre configurations.
type Registry struct {
	genres map[string]*GenreConfig // key: "<language>/<id>"
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{genres: make(map[string]*GenreConfig)}
}

// Get returns a genre config by language and ID.
func (r *Registry) Get(language, id string) (*GenreConfig, bool) {
	g, ok := r.genres[language+"/"+id]
	return g, ok
}

// List returns all genres for a language.
func (r *Registry) List(language string) []*GenreConfig {
	var result []*GenreConfig
	prefix := language + "/"
	for k, v := range r.genres {
		if strings.HasPrefix(k, prefix) {
			result = append(result, v)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListAll returns all genres sorted by language then ID.
func (r *Registry) ListAll() []*GenreConfig {
	result := make([]*GenreConfig, 0, len(r.genres))
	for _, v := range r.genres {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Language == result[j].Language {
			return result[i].ID < result[j].ID
		}
		return result[i].Language < result[j].Language
	})
	return result
}

// LoadFromFS loads all genre YAML files from an fs.FS.
// Expected layout: <language>/<genre-id>.yaml
func LoadFromFS(fsys fs.FS) (*Registry, error) {
	r := NewRegistry()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var cfg GenreConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		// Derive language from directory name
		dir := filepath.Dir(path)
		if dir == "." {
			dir = "unknown"
		}
		cfg.Language = dir

		if cfg.ID == "" {
			cfg.ID = strings.TrimSuffix(filepath.Base(path), ".yaml")
		}

		r.genres[dir+"/"+cfg.ID] = &cfg
		return nil
	})
	return r, err
}
