package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	internalusage "github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// GetQuota returns the current quota snapshot for the authenticated API key.
func (h *BaseAPIHandler) GetQuota(c *gin.Context) {
	if h == nil || h.AuthManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "quota ledger unavailable"})
		return
	}

	principal := quotaPrincipalFromContext(c)
	if principal == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "api key required"})
		return
	}

	requestCtx := context.Background()
	if c != nil && c.Request != nil {
		requestCtx = c.Request.Context()
	}
	summary, err := h.AuthManager.GetExecutionQuotaSummary(requestCtx, principal)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "quota ledger unavailable"})
		return
	}

	usageStats := h.UsageStats
	if usageStats == nil {
		usageStats = internalusage.GetRequestStatistics()
	}

	c.JSON(http.StatusOK, quotaResponse{
		Quota: summary,
		Usage: buildAPIKeyUsageSummary(usageStats, principal),
	})
}

type quotaResponse struct {
	Quota coreauth.ExecutionQuotaSnapshot `json:"quota"`
	Usage apiKeyUsageSummary              `json:"usage"`
}

type apiKeyUsageSummary struct {
	// APIKey is redacted: only last 4 characters shown (e.g. "****...a1b2").
	APIKey          string                                 `json:"api_key"`
	TotalRequests   int64                                  `json:"total_requests"`
	SuccessRequests int64                                  `json:"success_requests"`
	FailedRequests  int64                                  `json:"failed_requests"`
	TotalTokens     int64                                  `json:"total_tokens"`
	CachedTokens    int64                                  `json:"cached_tokens"`
	ReasoningTokens int64                                  `json:"reasoning_tokens"`
	RPM             float64                                `json:"rpm"`
	TPM             float64                                `json:"tpm"`
	// Models keys are aliased — no real provider model IDs exposed.
	Models map[string]internalusage.ModelSnapshot `json:"models,omitempty"`
}

// redactKey masks all but the last 4 characters of an API key.
func redactKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****..." + key[len(key)-4:]
}

// resolveModelAlias maps internal provider model IDs to user-facing aliases.
// This prevents leaking real Gemini model names and provider architecture.
// Update this map whenever oauth-model-alias in config.yaml changes.
func resolveModelAlias(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case m == "gemini-3.1-flash-lite-preview", m == "gemini-3-flash":
		return "claude-opus-4-7"
	case m == "gemini-3-flash-preview":
		return "claude-opus-4-6"
	case m == "gemini-3.1-flash-lite":
		return "claude-sonnet-4-6"
	case m == "gemini-2.5-flash-lite", m == "gemini-2.5-flash":
		return "claude-haiku-4-5"
	case strings.Contains(m, "gemini"):
		return "claude-sonnet-4-6"
	default:
		return model // already an alias or non-Gemini model
	}
}

// sanitiseDetails strips sensitive fields from per-request detail records.
// Source (Gmail address / credential file path) and AuthIndex (credential hash)
// must never be exposed to API key holders.
func sanitiseDetails(details []internalusage.RequestDetail) []internalusage.RequestDetail {
	out := make([]internalusage.RequestDetail, len(details))
	for i, d := range details {
		out[i] = internalusage.RequestDetail{
			Timestamp: d.Timestamp,
			LatencyMs: d.LatencyMs,
			// Source and AuthIndex intentionally omitted.
			Tokens: d.Tokens,
			Failed: d.Failed,
		}
	}
	return out
}

func buildAPIKeyUsageSummary(stats *internalusage.RequestStatistics, apiKey string) apiKeyUsageSummary {
	summary := apiKeyUsageSummary{APIKey: redactKey(strings.TrimSpace(apiKey))}
	if stats == nil || apiKey == "" {
		return summary
	}

	snapshot := stats.Snapshot()
	apiSnapshot, ok := snapshot.APIs[strings.TrimSpace(apiKey)]
	if !ok {
		return summary
	}

	// Rebuild models map with aliased keys and sanitised details.
	aliasedModels := make(map[string]internalusage.ModelSnapshot, len(apiSnapshot.Models))
	for rawModel, modelSnapshot := range apiSnapshot.Models {
		alias := resolveModelAlias(rawModel)
		existing := aliasedModels[alias]
		existing.TotalRequests += modelSnapshot.TotalRequests
		existing.TotalTokens += modelSnapshot.TotalTokens
		existing.Details = append(existing.Details, sanitiseDetails(modelSnapshot.Details)...)
		aliasedModels[alias] = existing
	}
	summary.Models = aliasedModels

	cutoff := time.Now().UTC().Add(-30 * time.Minute)
	var recentRequests, recentTokens int64

	for _, modelSnapshot := range apiSnapshot.Models {
		for _, detail := range modelSnapshot.Details {
			summary.TotalRequests++
			if detail.Failed {
				summary.FailedRequests++
			} else {
				summary.SuccessRequests++
			}
			if detail.Tokens.CachedTokens > 0 {
				summary.CachedTokens += detail.Tokens.CachedTokens
			}
			if detail.Tokens.ReasoningTokens > 0 {
				summary.ReasoningTokens += detail.Tokens.ReasoningTokens
			}
			totalTokens := detailTotalTokens(detail.Tokens)
			summary.TotalTokens += totalTokens
			if !detail.Timestamp.IsZero() && !detail.Timestamp.Before(cutoff) {
				recentRequests++
				recentTokens += totalTokens
			}
		}
	}

	summary.RPM = float64(recentRequests) / 30.0
	summary.TPM = float64(recentTokens) / 30.0
	return summary
}

func detailTotalTokens(tokens internalusage.TokenStats) int64 {
	totalTokens := tokens.TotalTokens
	if totalTokens == 0 {
		totalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens
	}
	if totalTokens == 0 {
		totalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens + tokens.CachedTokens
	}
	if totalTokens < 0 {
		return 0
	}
	return totalTokens
}

func quotaPrincipalFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, exists := c.Get("apiKey"); exists {
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case fmt.Stringer:
			return strings.TrimSpace(typed.String())
		default:
			return strings.TrimSpace(fmt.Sprintf("%v", typed))
		}
	}
	return ""
}
