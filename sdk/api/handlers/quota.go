package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/middleware"
)

// GetQuota returns the current quota snapshot for the authenticated API key.
//
// NOTE: This endpoint is temporarily simplified pending a port to the v7
// redisqueue-based usage architecture. The internal/usage package was removed
// by upstream in v7. Full quota tracking will be restored once the migration
// to internal/redisqueue is complete.
func (h *BaseAPIHandler) GetQuota(c *gin.Context) {
	principal := quotaPrincipalFromContext(c)
	if principal == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "api key required"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_key":    redactKey(strings.TrimSpace(principal)),
		"status":     "quota_tracking_migrating",
		"message":    "Quota tracking is being migrated to the v7 redisqueue architecture. Check back soon.",
		"debug_info": "antigravity-v1-verified",
	})
}

// GetPostPayQuota returns the rich JSON ledger required by the custom isolated React dashboard.
// It explicitly reads from the isolated postPayUsage map.
func (h *BaseAPIHandler) GetPostPayQuota(c *gin.Context) {
	principal := quotaPrincipalFromContext(c)
	if principal == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "api key required"})
		return
	}

	snapshot := middleware.GetPostPaySnapshot()
	var creditsConsumed float64
	var successCount int64
	var failedCount int64
	var totalTokens int64

	modelsMap := gin.H{}
	var sessions []middleware.SessionSummary

	if entry, exists := snapshot[principal]; exists {
		creditsConsumed = entry.CreditsConsumed
		successCount = entry.Success
		failedCount = entry.Failed
		sessions = entry.Sessions

		for _, s := range entry.Sessions {
			totalTokens += s.InputTokens + s.OutputTokens + s.CachedTokens
			if m, ok := modelsMap[s.Model].(gin.H); ok {
				m["total_requests"] = m["total_requests"].(int) + 1
				modelsMap[s.Model] = m
			} else {
				modelsMap[s.Model] = gin.H{"total_requests": 1}
			}
		}
	}

	if sessions == nil {
		sessions = []middleware.SessionSummary{}
	}
	
	// Default to unlimited (-1) unless explicitly configured in PostPayBilling
	creditLimit := middleware.GetPostPayCreditLimit(principal)

	c.JSON(http.StatusOK, gin.H{
		"usage": gin.H{
			"success_requests": successCount,
			"failed_requests":  failedCount,
			"total_tokens":     totalTokens,
			"rpm":              0.0,
			"models":           modelsMap,
		},
		"quota": gin.H{
			"credits_used":       creditsConsumed,
			"total_credits_used": creditsConsumed,
			"credit_limit":       creditLimit,
			"window_expires_at":  "0001-01-01T00:00:00Z",
			"recent_sessions":    sessions,
		},
		"api_key":    redactKey(strings.TrimSpace(principal)),
		"status":     "active",
		"message":    fmt.Sprintf("Your current API usage is $%.4f (assuming 1,000 credits = $1.00 USD).", creditsConsumed/1000.0),
		"debug_info": "aiapigiare-postpay-isolated",
	})
}

// TrendPoint is a single data point for time-series charts.
type TrendPoint struct {
	Timestamp string `json:"t"`
	Requests  int64  `json:"r"`
	Tokens    int64  `json:"tok"`
}

// redactKey masks all but the last 4 characters of an API key.
func redactKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****..." + key[len(key)-4:]
}

// resolveModelAlias maps internal provider model IDs to user-facing aliases.
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
		return model
	}
}

func quotaPrincipalFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	
	// AuthMiddleware in server.go sets "userApiKey"
	val, exists := c.Get("userApiKey")
	if !exists {
		val, exists = c.Get("apiKey") // Fallback
	}
	
	if exists {
		switch typed := val.(type) {
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
