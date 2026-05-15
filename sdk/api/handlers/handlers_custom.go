package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// requestAPIKeyFromContext extracts the authenticated API key from the Gin context.
func requestAPIKeyFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if val, ok := c.Get("userApiKey"); ok {
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func normalizeModelAllowlist(models []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, m := range models {
		m = strings.ToLower(strings.TrimSpace(m))
		if m != "" {
			out[m] = struct{}{}
		}
	}
	return out
}

func modelMatchesAllowlist(model map[string]any, allowlist map[string]struct{}) bool {
	if len(allowlist) == 0 {
		return true // No allowlist means all allowed
	}
	idRaw, ok := model["id"]
	if !ok {
		return false
	}
	idStr, ok := idRaw.(string)
	if !ok {
		return false
	}
	idStr = strings.ToLower(strings.TrimSpace(idStr))
	_, matches := allowlist[idStr]
	return matches
}

// FilterModelsForAPIKeyAllProviders filters models using the union of all per-provider
// allowlists configured for the authenticated API key. This is used by the unified
// /v1/models endpoint which serves models from all providers at once.
func (h *BaseAPIHandler) FilterModelsForAPIKeyAllProviders(c *gin.Context, models []map[string]any) []map[string]any {
	if h == nil || c == nil || len(models) == 0 || h.Cfg == nil || len(h.Cfg.APIKeyModels) == 0 {
		return models
	}
	apiKey := requestAPIKeyFromContext(c)
	if apiKey == "" {
		return models
	}
	perKeyModels, ok := h.Cfg.APIKeyModels[apiKey]
	if !ok || len(perKeyModels) == 0 {
		return models
	}
	// Merge allowed models from all configured providers for this key.
	combined := make(map[string]struct{})
	for _, providerModels := range perKeyModels {
		for id, v := range normalizeModelAllowlist(providerModels) {
			combined[id] = v
		}
	}
	if len(combined) == 0 {
		return models
	}
	filtered := make([]map[string]any, 0, len(models))
	for _, model := range models {
		if modelMatchesAllowlist(model, combined) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

// FilterModelsForAPIKey filters models for a specific provider based on the allowlist configured for the API key.
func (h *BaseAPIHandler) FilterModelsForAPIKey(c *gin.Context, provider string, models []map[string]any) []map[string]any {
	if h == nil || c == nil || len(models) == 0 || h.Cfg == nil || len(h.Cfg.APIKeyModels) == 0 {
		return models
	}
	apiKey := requestAPIKeyFromContext(c)
	if apiKey == "" {
		return models
	}
	perKeyModels, ok := h.Cfg.APIKeyModels[apiKey]
	if !ok || len(perKeyModels) == 0 {
		return models
	}
	providerModels, ok := perKeyModels[provider]
	if !ok || len(providerModels) == 0 {
		return models
	}
	allowlist := normalizeModelAllowlist(providerModels)
	if len(allowlist) == 0 {
		return models
	}
	filtered := make([]map[string]any, 0, len(models))
	for _, model := range models {
		if modelMatchesAllowlist(model, allowlist) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}
