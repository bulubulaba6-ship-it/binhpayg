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
	APIKey          string                                 `json:"api_key"`
	TotalRequests   int64                                  `json:"total_requests"`
	SuccessRequests int64                                  `json:"success_requests"`
	FailedRequests  int64                                  `json:"failed_requests"`
	TotalTokens     int64                                  `json:"total_tokens"`
	CachedTokens    int64                                  `json:"cached_tokens"`
	ReasoningTokens int64                                  `json:"reasoning_tokens"`
	RPM             float64                                `json:"rpm"`
	TPM             float64                                `json:"tpm"`
	Models          map[string]internalusage.ModelSnapshot `json:"models,omitempty"`
}

func buildAPIKeyUsageSummary(stats *internalusage.RequestStatistics, apiKey string) apiKeyUsageSummary {
	summary := apiKeyUsageSummary{APIKey: strings.TrimSpace(apiKey)}
	if stats == nil || summary.APIKey == "" {
		return summary
	}

	snapshot := stats.Snapshot()
	apiSnapshot, ok := snapshot.APIs[summary.APIKey]
	if !ok {
		return summary
	}

	summary.Models = apiSnapshot.Models
	cutoff := time.Now().UTC().Add(-30 * time.Minute)
	var recentRequests int64
	var recentTokens int64

	for _, modelSnapshot := range apiSnapshot.Models {
		for _, detail := range modelSnapshot.Details {
			summary.TotalRequests++
			if detail.Failed {
				summary.FailedRequests++
			} else {
				summary.SuccessRequests++
			}

			cachedTokens := detail.Tokens.CachedTokens
			if cachedTokens < 0 {
				cachedTokens = 0
			}
			reasoningTokens := detail.Tokens.ReasoningTokens
			if reasoningTokens < 0 {
				reasoningTokens = 0
			}
			summary.CachedTokens += cachedTokens
			summary.ReasoningTokens += reasoningTokens
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
