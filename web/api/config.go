package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"storyforge/internal/app"
	"storyforge/internal/model"
)

type configHandler struct {
	svc *app.ConfigService
}

type studioProjectView struct {
	Name             string  `json:"name,omitempty"`
	Language         string  `json:"language,omitempty"`
	LanguageExplicit bool    `json:"languageExplicit"`
	Model            string  `json:"model,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	BaseURL          string  `json:"baseUrl,omitempty"`
	WireAPI          string  `json:"wireApi,omitempty"`
	Stream           bool    `json:"stream,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
	MaxTokens        int     `json:"maxTokens,omitempty"`
	ThinkingBudget   int     `json:"thinkingBudget,omitempty"`
}

func (h *configHandler) get(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *configHandler) getProject(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, studioProjectView{
		Name:             cfg.Name,
		Language:         cfg.Language,
		LanguageExplicit: projectLanguageExplicit(h.svc),
		Model:            cfg.LLM.Model,
		Provider:         cfg.LLM.Provider,
		BaseURL:          cfg.LLM.BaseURL,
		WireAPI:          cfg.LLM.WireAPI,
		Stream:           cfg.LLM.Stream,
		Temperature:      cfg.LLM.Temperature,
		MaxTokens:        cfg.LLM.MaxTokens,
		ThinkingBudget:   cfg.LLM.ThinkingBudget,
	})
}

func (h *configHandler) update(w http.ResponseWriter, r *http.Request) {
	var input model.ProjectConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Update(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *configHandler) updateProject(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v, ok := body["name"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.Name = strings.TrimSpace(v)
	}
	if v, ok := body["language"].(string); ok {
		cfg.Language = strings.TrimSpace(v)
	}
	if v, ok := numberValue(body["temperature"]); ok {
		cfg.LLM.Temperature = v
	}
	if v, ok := intValue(body["maxTokens"]); ok {
		cfg.LLM.MaxTokens = v
	}
	if v, ok := intValue(body["thinkingBudget"]); ok {
		cfg.LLM.ThinkingBudget = v
	}
	if v, ok := body["stream"].(bool); ok {
		cfg.LLM.Stream = v
	}
	if v, ok := body["provider"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.LLM.Provider = strings.TrimSpace(v)
	}
	if v, ok := body["model"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.LLM.Model = strings.TrimSpace(v)
	}
	if v, ok := body["baseUrl"].(string); ok {
		cfg.LLM.BaseURL = strings.TrimSpace(v)
	}
	if v, ok := body["wireApi"].(string); ok {
		cfg.LLM.WireAPI = strings.TrimSpace(v)
	}
	applyProjectLLMCompatEdits(cfg)
	updated, err := h.svc.Update(*cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, studioProjectView{
		Name:             updated.Name,
		Language:         updated.Language,
		LanguageExplicit: projectLanguageExplicit(h.svc),
		Model:            updated.LLM.Model,
		Provider:         updated.LLM.Provider,
		BaseURL:          updated.LLM.BaseURL,
		WireAPI:          updated.LLM.WireAPI,
		Stream:           updated.LLM.Stream,
		Temperature:      updated.LLM.Temperature,
		MaxTokens:        updated.LLM.MaxTokens,
		ThinkingBudget:   updated.LLM.ThinkingBudget,
	})
}

func (h *configHandler) getModels(w http.ResponseWriter, r *http.Request) {
	routes, err := h.svc.GetModelRoutes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, routes)
}

func (h *configHandler) updateModel(w http.ResponseWriter, r *http.Request) {
	agent := chi.URLParam(r, "agent")
	var input model.AgentLLMBinding
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	routes, err := h.svc.SetAgentModel(agent, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, routes)
}

func (h *configHandler) testProfile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(chi.URLParam(r, "name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "profile name is required")
		return
	}

	var cfg model.ProjectConfig
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		model.NormalizeLLMProfiles(&cfg)
	} else {
		current, err := h.svc.Get()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if current == nil {
			writeError(w, http.StatusNotFound, "config not found")
			return
		}
		cfg = *current
	}

	profile, ok := model.FindLLMProfile(cfg, name)
	if !ok {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	check := buildDoctorProfileCheck(r.Context(), cfg, profileForTestProbe(profile))
	writeJSON(w, http.StatusOK, check)
}

func (h *configHandler) setLanguage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg.Language = strings.TrimSpace(body.Language)
	updated, err := h.svc.Update(*cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "language": updated.Language})
}

func (h *configHandler) getModelOverrides(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overrides": projectOverridesFromConfig(*cfg)})
}

func (h *configHandler) updateModelOverrides(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Overrides any `json:"overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	overrides, err := normalizeOverrides(body.Overrides)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	replaceProjectOverrides(cfg, overrides)
	if _, err := h.svc.Update(*cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func projectLanguageExplicit(svc *app.ConfigService) bool {
	data, err := os.ReadFile(svc.Path())
	if err != nil {
		return false
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, ok := raw["language"]
	return ok
}

func overridesToMap(overrides []model.AgentModelOverride) map[string]map[string]any {
	out := make(map[string]map[string]any, len(overrides))
	for _, item := range overrides {
		if strings.TrimSpace(item.Agent) == "" {
			continue
		}
		out[item.Agent] = map[string]any{
			"provider":       item.Provider,
			"model":          item.Model,
			"baseUrl":        item.BaseURL,
			"apiKey":         item.APIKey,
			"wireApi":        item.WireAPI,
			"thinkingBudget": item.ThinkingBudget,
		}
	}
	return out
}

func projectOverridesFromConfig(cfg model.ProjectConfig) map[string]map[string]any {
	if len(cfg.AgentLLMBindings) == 0 {
		return map[string]map[string]any{}
	}
	out := map[string]map[string]any{}
	for _, binding := range cfg.AgentLLMBindings {
		profileName := strings.TrimSpace(binding.Profile)
		if profileName == "" || profileName == strings.TrimSpace(cfg.DefaultLLMProfile) {
			continue
		}
		profile, ok := model.FindLLMProfile(cfg, profileName)
		if !ok {
			continue
		}
		out[binding.Agent] = map[string]any{
			"provider":       profile.Provider,
			"model":          profile.Model,
			"baseUrl":        profile.BaseURL,
			"apiKey":         profile.APIKey,
			"wireApi":        profile.WireAPI,
			"thinkingBudget": profile.ThinkingBudget,
		}
	}
	return out
}

func normalizeOverrides(raw any) ([]model.AgentModelOverride, error) {
	switch val := raw.(type) {
	case nil:
		return nil, nil
	case []any:
		data, err := json.Marshal(val)
		if err != nil {
			return nil, err
		}
		var overrides []model.AgentModelOverride
		if err := json.Unmarshal(data, &overrides); err != nil {
			return nil, err
		}
		return overrides, nil
	case map[string]any:
		overrides := make([]model.AgentModelOverride, 0, len(val))
		for agent, rawCfg := range val {
			cfg, _ := rawCfg.(map[string]any)
			override := model.AgentModelOverride{
				Agent:          agent,
				Provider:       strings.TrimSpace(stringAny(cfg["provider"])),
				Model:          strings.TrimSpace(stringAny(cfg["model"])),
				BaseURL:        strings.TrimSpace(stringAny(cfg["baseUrl"])),
				APIKey:         strings.TrimSpace(stringAny(cfg["apiKey"])),
				WireAPI:        strings.TrimSpace(stringAny(cfg["wireApi"])),
				ThinkingBudget: intAny(cfg["thinkingBudget"]),
			}
			if override.Model == "" {
				continue
			}
			overrides = append(overrides, override)
		}
		return overrides, nil
	default:
		return nil, &model.ValidationError{Field: "overrides", Message: "must be an object or array"}
	}
}

func replaceProjectOverrides(cfg *model.ProjectConfig, overrides []model.AgentModelOverride) {
	if cfg == nil {
		return
	}
	overrideByAgent := make(map[string]model.AgentModelOverride, len(overrides))
	for _, override := range overrides {
		agent := strings.TrimSpace(override.Agent)
		if agent == "" || strings.TrimSpace(override.Model) == "" {
			continue
		}
		override.Agent = agent
		overrideByAgent[agent] = override
	}

	candidateProfiles := map[string]struct{}{}
	for _, binding := range cfg.AgentLLMBindings {
		candidateProfiles[binding.Agent+"-profile"] = struct{}{}
	}
	for agent := range overrideByAgent {
		candidateProfiles[agent+"-profile"] = struct{}{}
	}

	filteredProfiles := make([]model.LLMProfile, 0, len(cfg.LLMProfiles))
	for _, profile := range cfg.LLMProfiles {
		if _, remove := candidateProfiles[strings.TrimSpace(profile.Name)]; remove {
			continue
		}
		filteredProfiles = append(filteredProfiles, profile)
	}

	for agent, override := range overrideByAgent {
		base := cfg.LLM
		if strings.TrimSpace(override.Provider) != "" {
			base.Provider = override.Provider
		}
		base.Model = override.Model
		if strings.TrimSpace(override.BaseURL) != "" {
			base.BaseURL = override.BaseURL
		}
		if strings.TrimSpace(override.APIKey) != "" {
			base.APIKey = override.APIKey
		}
		if strings.TrimSpace(override.WireAPI) != "" {
			base.WireAPI = override.WireAPI
		}
		if override.ThinkingBudget > 0 {
			base.ThinkingBudget = override.ThinkingBudget
		}
		filteredProfiles = append(filteredProfiles, model.LLMProfile{
			Name:           agent + "-profile",
			Language:       cfg.Language,
			Provider:       base.Provider,
			Model:          base.Model,
			BaseURL:        base.BaseURL,
			APIKey:         base.APIKey,
			WireAPI:        base.WireAPI,
			Stream:         base.Stream,
			Temperature:    base.Temperature,
			MaxTokens:      base.MaxTokens,
			ThinkingBudget: base.ThinkingBudget,
		})
	}
	cfg.LLMProfiles = filteredProfiles

	bindings := make([]model.AgentLLMBinding, 0, len(cfg.AgentLLMBindings)+len(overrideByAgent))
	seen := map[string]struct{}{}
	for _, binding := range cfg.AgentLLMBindings {
		agent := strings.TrimSpace(binding.Agent)
		if agent == "" {
			continue
		}
		if _, ok := overrideByAgent[agent]; ok {
			bindings = append(bindings, model.AgentLLMBinding{Agent: agent, Profile: agent + "-profile"})
		} else if strings.TrimSpace(binding.Profile) != agent+"-profile" {
			bindings = append(bindings, binding)
		}
		seen[agent] = struct{}{}
	}
	for agent := range overrideByAgent {
		if _, ok := seen[agent]; ok {
			continue
		}
		bindings = append(bindings, model.AgentLLMBinding{Agent: agent, Profile: agent + "-profile"})
	}
	cfg.AgentLLMBindings = bindings
	cfg.LLM.AgentOverrides = nil
}

func stringAny(v any) string {
	s, _ := v.(string)
	return s
}

func numberValue(v any) (float64, bool) {
	n, ok := v.(float64)
	return n, ok
}

func intValue(v any) (int, bool) {
	n, ok := numberValue(v)
	return int(n), ok
}

func intAny(v any) int {
	n, _ := intValue(v)
	return n
}

func applyProjectLLMCompatEdits(cfg *model.ProjectConfig) {
	if cfg == nil {
		return
	}
	profileName := strings.TrimSpace(cfg.DefaultLLMProfile)
	if profileName == "" && len(cfg.LLMProfiles) > 0 {
		profileName = cfg.LLMProfiles[0].Name
	}
	for i := range cfg.LLMProfiles {
		if strings.TrimSpace(cfg.LLMProfiles[i].Name) != profileName {
			continue
		}
		cfg.LLMProfiles[i].Provider = cfg.LLM.Provider
		cfg.LLMProfiles[i].Model = cfg.LLM.Model
		cfg.LLMProfiles[i].BaseURL = cfg.LLM.BaseURL
		cfg.LLMProfiles[i].APIKey = cfg.LLM.APIKey
		cfg.LLMProfiles[i].WireAPI = cfg.LLM.WireAPI
		cfg.LLMProfiles[i].Stream = cfg.LLM.Stream
		cfg.LLMProfiles[i].Temperature = cfg.LLM.Temperature
		cfg.LLMProfiles[i].MaxTokens = cfg.LLM.MaxTokens
		cfg.LLMProfiles[i].ThinkingBudget = cfg.LLM.ThinkingBudget
		return
	}
	if profileName != "" {
		cfg.LLMProfiles = append(cfg.LLMProfiles, model.LLMProfile{
			Name:           profileName,
			Language:       cfg.Language,
			Provider:       cfg.LLM.Provider,
			Model:          cfg.LLM.Model,
			BaseURL:        cfg.LLM.BaseURL,
			APIKey:         cfg.LLM.APIKey,
			WireAPI:        cfg.LLM.WireAPI,
			Stream:         cfg.LLM.Stream,
			Temperature:    cfg.LLM.Temperature,
			MaxTokens:      cfg.LLM.MaxTokens,
			ThinkingBudget: cfg.LLM.ThinkingBudget,
		})
	}
}

func (h *configHandler) getNotify(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	channels := cfg.Webhooks
	if channels == nil {
		channels = []model.WebhookConfig{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func (h *configHandler) updateNotify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channels []model.WebhookConfig `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cfg, err := h.svc.Get()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg.Webhooks = body.Channels
	if _, err := h.svc.Update(*cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
