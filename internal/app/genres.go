package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"storyforge/internal/genre"
)

// GenresService handles genre catalog queries and project-level overrides.
type GenresService struct {
	builtin    *genre.Registry
	projectDir string
}

// NewGenresService creates a GenresService.
func NewGenresService(dataDir string, registry *genre.Registry) *GenresService {
	return &GenresService{
		builtin:    registry,
		projectDir: filepath.Join(dataDir, "genres"),
	}
}

// List returns genres for a specific language, or all genres when language is empty.
func (s *GenresService) List(language string) []*genre.GenreConfig {
	merged, err := s.merged()
	if err != nil {
		return []*genre.GenreConfig{}
	}
	result := make([]*genre.GenreConfig, 0, len(merged))
	for _, cfg := range merged {
		if language != "" && cfg.Language != language {
			continue
		}
		copyCfg := *cfg
		result = append(result, &copyCfg)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Language == result[j].Language {
			return result[i].ID < result[j].ID
		}
		return result[i].Language < result[j].Language
	})
	return result
}

// Get returns one genre by language and id.
func (s *GenresService) Get(language, id string) (*genre.GenreConfig, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("genre id is required")
	}
	merged, err := s.merged()
	if err != nil {
		return nil, err
	}
	if language != "" {
		cfg, ok := merged[genreKey(language, id)]
		if !ok {
			return nil, fmt.Errorf("genre %s/%s not found", language, id)
		}
		copyCfg := *cfg
		return &copyCfg, nil
	}
	for _, cfg := range merged {
		if cfg.ID == id {
			copyCfg := *cfg
			return &copyCfg, nil
		}
	}
	return nil, fmt.Errorf("genre %s not found", id)
}

// Create adds a project-level genre.
func (s *GenresService) Create(cfg *genre.GenreConfig) (*genre.GenreConfig, error) {
	if err := validateGenreConfig(cfg); err != nil {
		return nil, err
	}
	path := s.genrePath(cfg.Language, cfg.ID)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("genre %s/%s already exists", cfg.Language, cfg.ID)
	}
	if err := s.writeProjectGenre(cfg); err != nil {
		return nil, err
	}
	return s.Get(cfg.Language, cfg.ID)
}

// Update upserts a project-level genre.
func (s *GenresService) Update(id string, cfg *genre.GenreConfig) (*genre.GenreConfig, error) {
	if cfg == nil {
		cfg = &genre.GenreConfig{}
	}
	cfg.ID = strings.TrimSpace(firstNonEmpty(cfg.ID, id))
	if err := validateGenreConfig(cfg); err != nil {
		return nil, err
	}
	if err := s.writeProjectGenre(cfg); err != nil {
		return nil, err
	}
	return s.Get(cfg.Language, cfg.ID)
}

// Delete removes a project-level genre override.
func (s *GenresService) Delete(language, id string) error {
	language = normalizeGenreLanguage(language)
	if language == "" {
		return fmt.Errorf("language is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("genre id is required")
	}
	path := s.genrePath(language, id)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("project genre %s/%s not found", language, id)
	}
	return os.Remove(path)
}

// Copy duplicates an existing genre into the project override directory.
func (s *GenresService) Copy(language, id string) (string, error) {
	cfg, err := s.Get(language, id)
	if err != nil {
		return "", err
	}
	if err := s.writeProjectGenre(cfg); err != nil {
		return "", err
	}
	return s.genrePath(cfg.Language, cfg.ID), nil
}

func (s *GenresService) merged() (map[string]*genre.GenreConfig, error) {
	merged := make(map[string]*genre.GenreConfig)
	if s.builtin != nil {
		for _, cfg := range s.builtin.ListAll() {
			copyCfg := *cfg
			merged[genreKey(copyCfg.Language, copyCfg.ID)] = &copyCfg
		}
	}
	project, err := s.projectRegistry()
	if err != nil {
		return nil, err
	}
	for _, cfg := range project.ListAll() {
		copyCfg := *cfg
		merged[genreKey(copyCfg.Language, copyCfg.ID)] = &copyCfg
	}
	return merged, nil
}

func (s *GenresService) projectRegistry() (*genre.Registry, error) {
	if _, err := os.Stat(s.projectDir); os.IsNotExist(err) {
		return genre.NewRegistry(), nil
	}
	return genre.LoadFromFS(os.DirFS(s.projectDir))
}

func (s *GenresService) writeProjectGenre(cfg *genre.GenreConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path := s.genrePath(cfg.Language, cfg.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *GenresService) genrePath(language, id string) string {
	return filepath.Join(s.projectDir, normalizeGenreLanguage(language), id+".yaml")
}

func genreKey(language, id string) string {
	return normalizeGenreLanguage(language) + "/" + strings.TrimSpace(id)
}

func normalizeGenreLanguage(language string) string {
	language = strings.TrimSpace(strings.ToLower(language))
	if language == "" {
		return "zh"
	}
	return language
}

func validateGenreConfig(cfg *genre.GenreConfig) error {
	if cfg == nil {
		return fmt.Errorf("genre config is required")
	}
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Language = normalizeGenreLanguage(cfg.Language)
	if cfg.ID == "" {
		return fmt.Errorf("genre id is required")
	}
	if cfg.Name == "" {
		return fmt.Errorf("genre name is required")
	}
	if strings.Contains(cfg.ID, "..") || strings.ContainsAny(cfg.ID, `/\`) {
		return fmt.Errorf("invalid genre id %q", cfg.ID)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
