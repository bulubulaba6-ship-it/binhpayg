package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

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
	var totalCredits float64 // all-time cumulative (from ledger)
	var creditsPurchased float64
	var successCount int64
	var failedCount int64
	var totalTokens int64
	var dailyRequests map[string]int64

	var sessions []middleware.SessionSummary

	var entry *middleware.PostPayUsageEntry
	if e, exists := snapshot[principal]; exists {
		entry = &e
		totalCredits = e.CreditsConsumed
		creditsPurchased = e.CreditsPurchased
		successCount = e.Success
		failedCount = e.Failed
		totalTokens = e.TotalTokens
		sessions = e.Sessions
		dailyRequests = e.DailyRequests
	}

	modelsMap := gin.H{}
	if entry != nil && len(entry.Models) > 0 {
		for modelAlias, count := range entry.Models {
			modelsMap[modelAlias] = gin.H{"total_requests": count}
		}
	} else {
		// Fallback: build from the last 100 sessions (before persistent Models tracking)
		for _, s := range sessions {
			if m, ok := modelsMap[s.Model].(gin.H); ok {
				m["total_requests"] = m["total_requests"].(int) + 1
				modelsMap[s.Model] = m
			} else {
				modelsMap[s.Model] = gin.H{"total_requests": 1}
			}
		}
	}

	var fiveHCredits float64
	fiveHWindowStart := middleware.GetFiveHWindowStartForKey(principal)
	if entry != nil && !fiveHWindowStart.IsZero() && time.Since(fiveHWindowStart) <= 5*time.Hour {
		fiveHCredits = entry.FiveHCredits
	}

	// RPM: sessions in last 30 minutes ÷ 30.
	rpmCutoff := time.Now().Add(-30 * time.Minute)
	var recentRequests int64
	for _, s := range sessions {
		if s.Timestamp.After(rpmCutoff) {
			recentRequests++
		}
	}
	rpm := float64(recentRequests) / 30.0

	if sessions == nil {
		sessions = []middleware.SessionSummary{}
	}

	// Default to unlimited (-1) unless explicitly configured in PostPayBilling
	creditLimit := middleware.GetPostPayCreditLimit(principal)

	// Calculate Tier based on CreditsPurchased
	tier := 1
	if creditsPurchased >= 500_000 {
		tier = 6
	} else if creditsPurchased >= 300_000 {
		tier = 5
	} else if creditsPurchased >= 200_000 {
		tier = 4
	} else if creditsPurchased >= 100_000 {
		tier = 3
	} else if creditsPurchased >= 50_000 {
		tier = 2
	}

	liveCfg := middleware.GetLiveConfig()
	rateLimit5h := 0
	if liveCfg != nil {
		rateLimit5h = liveCfg.DefaultAPIKeyLimit
		if liveCfg.APIKeyLimits != nil {
			if customLimit, ok := liveCfg.APIKeyLimits[principal]; ok {
				rateLimit5h = customLimit
			}
		}
	}

	// Compute window expiry for client display (zero time when no active window).
	var windowExpiresAt interface{}
	if !fiveHWindowStart.IsZero() {
		windowExpiresAt = fiveHWindowStart.Add(5 * time.Hour).UTC().Format(time.RFC3339)
	} else {
		windowExpiresAt = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"usage": gin.H{
			"success_requests": successCount,
			"failed_requests":  failedCount,
			"total_tokens":     totalTokens,
			"rpm":              rpm,
			"models":           modelsMap,
			"daily_requests":   dailyRequests,
		},
		"quota": gin.H{
			"credits_used":       fiveHCredits, // session-derived 5H rolling window (kill-switch input)
			"total_credits_used": totalCredits, // all-time cumulative
			"credits_purchased":  creditsPurchased,
			"tier":               tier,
			"credit_limit":       creditLimit,
			"rate_limit_5h":      rateLimit5h,
			"window_expires_at":  windowExpiresAt, // ISO8601 or null when no active window
			"recent_sessions":    sessions,
		},
		"api_key":             redactKey(strings.TrimSpace(principal)),
		"status":              "active",
		"message":             fmt.Sprintf("Your current API usage is $%.4f (assuming 1,000 credits = $1.00 USD).", totalCredits/1000.0),
		"debug_info":          "finkrouter-postpay-isolated",
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

type DepositRequest struct {
	APIKey    string  `json:"api_key" binding:"required"`
	USDAmount float64 `json:"usd_amount"` // legacy: USD-based deposit (ProcessDeposit)
	VNDAmount float64 `json:"vnd_amount"` // preferred: VND-native deposit (ProcessDepositVND)
	TxnID     string  `json:"txn_id,omitempty"`
}

// PostDeposit handles adding credits to an API key based on cumulative purchase tiers.
func (h *BaseAPIHandler) PostDeposit(c *gin.Context) {
	// 1. Authenticate webhook secret
	liveCfg := middleware.GetLiveConfig()
	if liveCfg == nil || !liveCfg.PostPayBilling.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "post-pay billing not enabled"})
		return
	}

	secret := liveCfg.PostPayBilling.WebhookSecret
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret not configured"})
		return
	}

	authHeader := c.GetHeader("Authorization")
	if authHeader != "Bearer "+secret {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook secret"})
		return
	}

	// 2. Parse request
	var req DepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload: " + err.Error()})
		return
	}

	// 3. Validate: at least one of vnd_amount or usd_amount must be > 0
	if req.VNDAmount <= 0 && req.USDAmount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vnd_amount or usd_amount must be > 0"})
		return
	}

	// 4. Process deposit using the correct function:
	//    - vnd_amount (preferred): use ProcessDepositVND with VND-native tier rates
	//    - usd_amount (legacy): use ProcessDeposit with USD-based tier rates
	var added, newTotal float64
	var tier int
	var isDuplicate bool
	var amountLabel string
	var amountValue float64
	if req.VNDAmount > 0 {
		// payOS native: VND amount → VND-tier cr/VND rates
		added, newTotal, tier, isDuplicate = middleware.ProcessDepositVND(req.APIKey, req.VNDAmount, req.TxnID)
		amountLabel = "vnd_deposited"
		amountValue = req.VNDAmount
	} else {
		// Legacy: USD amount → USD-tier cr/USD rates
		added, newTotal, tier, isDuplicate = middleware.ProcessDeposit(req.APIKey, req.USDAmount, req.TxnID)
		amountLabel = "usd_deposited"
		amountValue = req.USDAmount
	}

	// 5. Force immediate disk save for ledger
	fullLedgerPath := filepath.Join(liveCfg.AuthDir, liveCfg.PostPayBilling.LedgerFile)
	_ = middleware.SavePostPayUsage(fullLedgerPath)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"api_key":          redactKey(req.APIKey),
			amountLabel:        amountValue,
			"credits_added":    added,
			"new_total_bought": newTotal,
			"new_tier":         tier,
			"duplicate_txn":    isDuplicate,
		},
	})
}
