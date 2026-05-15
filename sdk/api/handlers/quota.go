package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
