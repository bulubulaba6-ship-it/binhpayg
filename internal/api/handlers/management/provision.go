package management

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/middleware"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// ProvisionRequest is the payload sent by shop.web after a successful payOS payment.
//
// Tier controls the key prefix:
//   - "payg"  → fink_   (Pay-As-You-Go top-up, no fixed expiry)
//   - "pro"   → fink_pro_  (Pro monthly plan)
//   - "max"   → fink_max_  (Max 5x / Max 20x)
//
// Credits is the EXACT number of credits to add to the key's balance.
// This is the plan's credit allowance, NOT derived from the USD price paid.
// Example: 1-day plan = 4000, 7-day plan = 30000, Pro = 50000.
//
// FiveHLimit is the per-5h rolling-window credit rate limit (api-key-limits).
// The standard volatile kill-switch; resets after 5h.
// Example: 1-day/7-day/Pro = 2000, Max-5x = 10000, Max-20x = 40000.
//
// TxnID is the payOS transaction ID — prevents double-crediting on retried webhooks.
type ProvisionRequest struct {
	Tier       string  `json:"tier"`
	Email      string  `json:"email,omitempty"`
	Credits    float64 `json:"credits"`      // plan credits to deposit (e.g. 8000 for 1-day)
	FiveHLimit int     `json:"five_h_limit"` // per-5h rate limit in credits (e.g. 2000)
	DaysValid  int     `json:"days_valid"`   // number of days until the plan expires (Time-based Kill Switch)
	TxnID      string  `json:"txn_id,omitempty"`
}

// AllowedClaude is the fixed Claude model list shared across all tiers.
var AllowedClaude = []string{
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-opus-4-6",
	"claude-sonnet-5",
	"claude-sonnet-4-6",
	"claude-haiku-4-5",
}

// AllowedOpenAI is the fixed OpenAI/Codex model list shared across all tiers.
var AllowedOpenAI = []string{
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.3-codex-spark",
	"gpt-5.2",
}

// PostProvisionKey generates a new API key server-side and atomically registers it
// across all required config sections in a single persist:
//
//  1. api-keys                   → grants access to the proxy
//  2. api-key-models             → allowed model list for this key
//  3. post-pay-billing.clients   → enrols key in post-pay billing with NO overdraft
//  4. api-key-limits             → per-5h rolling credit rate limit
//
// Then seeds the exact plan credit amount using ProcessDeposit.
//
// Two independent kill-switches protect the balance:
//
//	A) Per-5h rate limit  (api-key-limits[key])
//	   → 429 TooManyRequests when the rolling-5h window exceeds five_h_limit credits.
//	   Resets automatically after 5 hours.
//
//	B) Total balance ceiling  (post-pay ledger: creditsPurchased)
//	   → 402 PaymentRequired when credits_consumed >= credits_purchased.
//	   Only resets when more credits are deposited (top-up or new plan purchase).
//
// Authentication: Bearer <post-pay-billing.webhook-secret> (not management secret).
//
// POST /v1/billing/provision
// Authorization: Bearer <webhook_secret>
//
//	{
//	  "tier":         "pro",     // "payg" | "pro" | "max"
//	  "email":        "user@example.com",
//	  "credits":      245000,   // exact plan credit allowance (total balance ceiling)
//	  "five_h_limit": 2000,     // per-5h rolling rate limit
//	  "days_valid":   30,       // optional time-based kill switch
//	  "txn_id":       "payos_abc123"
//	}
func (h *Handler) PostProvisionKey(c *gin.Context) {
	// 1. Auth via webhook_secret (same as PostDeposit — NOT management secret)
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

	if c.GetHeader("Authorization") != "Bearer "+secret {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook secret"})
		return
	}

	// 2. Parse and validate request
	var req ProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Credits <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credits must be > 0 (plan credit allowance)"})
		return
	}
	if req.FiveHLimit <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "five_h_limit must be > 0 (per-5h rate limit)"})
		return
	}

	// 3. Generate key server-side — prefix encodes the subscription tier.
	// The raw bytes never leave the proxy until the final response JSON.
	prefix := provisionTierPrefix(req.Tier)
	raw := make([]byte, 16)
	if _, errRand := rand.Read(raw); errRand != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
		return
	}
	newKey := prefix + hex.EncodeToString(raw)

	// 4. Register key across all required config sections, then persist once.
	//    persistLocked writes the spool file → fsnotify detects change →
	//    persistConfigAsync() → PersistConfig(ctx) → saved to Postgres (PGSTORE_DSN).
	h.mu.Lock()

	// 4a. api-keys: grant proxy access
	h.cfg.APIKeys = append(h.cfg.APIKeys, newKey)

	// 4b. api-key-models: restrict to the allowed model set
	if h.cfg.APIKeyModels == nil {
		h.cfg.APIKeyModels = make(map[string]map[string][]string)
	}
	h.cfg.APIKeyModels[newKey] = map[string][]string{
		"claude": AllowedClaude,
		"openai": AllowedOpenAI,
	}

	// 4c. post-pay-billing.clients: enrol key with zero overdraft.
	//     effectiveLimit = creditsPurchased + credit-limit(0) = credits only.
	//     Key is blocked the moment credits_consumed >= credits_purchased.
	if h.cfg.PostPayBilling.Clients == nil {
		h.cfg.PostPayBilling.Clients = make(map[string]config.PostPayBillingClientCfg)
	}
	
	clientCfg := config.PostPayBillingClientCfg{
		CreditLimit: 0, // no overdraft — strict enforcement
	}
	if req.DaysValid > 0 {
		clientCfg.ExpiresAt = time.Now().UTC().AddDate(0, 0, req.DaysValid)
	}
	h.cfg.PostPayBilling.Clients[newKey] = clientCfg

	// 4d. api-key-limits: per-5h rolling-window rate limit. per-5h rolling-window rate limit.
	//     Standard volatile kill-switch; resets automatically after 5 hours.
	if h.cfg.APIKeyLimits == nil {
		h.cfg.APIKeyLimits = make(map[string]int)
	}
	h.cfg.APIKeyLimits[newKey] = req.FiveHLimit

	// 4e. Persist everything in one write → auto-synced to Postgres via watcher
	if err := config.SaveConfigPreserveComments(h.configFilePath, h.cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config: " + err.Error()})
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	// 5. Seed the exact plan credit amount directly into the post-pay ledger.
	//    ProcessDepositCredits bypasses any USD/VND conversion — credits are written
	//    exactly as specified, with zero floating-point drift.
	//    This sets creditsPurchased = req.Credits exactly.
	//    The total balance ceiling (Kill Switch B) is then: creditsPurchased = req.Credits.
	addedCredits, newTotal, isDuplicate := middleware.ProcessDepositCredits(
		newKey, req.Credits, req.TxnID,
	)

	// Flush ledger to disk immediately so credits survive a restart
	if liveCfg.PostPayBilling.LedgerFile != "" {
		fullPath := filepath.Join(liveCfg.AuthDir, liveCfg.PostPayBilling.LedgerFile)
		_ = middleware.SavePostPayUsage(fullPath)
	}

	if isDuplicate {
		c.JSON(http.StatusOK, gin.H{
			"status":           "duplicate_txn",
			"api_key":          newKey,
			"tier":             req.Tier,
			"five_h_limit":     req.FiveHLimit,
			"credits_added":    0,
			"new_total_bought": newTotal,
		})
		return
	}

	// 6. Return the plain key exactly once — shop.web must email this to the user.
	c.JSON(http.StatusOK, gin.H{
		"status":           "ok",
		"api_key":          newKey,
		"tier":             req.Tier,
		"five_h_limit":     req.FiveHLimit,
		"credits_added":    addedCredits,
		"new_total_bought": newTotal,
	})
}

// provisionTierPrefix maps a tier name to its fink_ key prefix.
func provisionTierPrefix(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "max":
		return "fink_max_"
	case "pro":
		return "fink_pro_"
	default: // "payg" or unrecognized
		return "fink_"
	}
}
