package middleware

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
)

type ClientUsageEntry struct {
	CreditsConsumed float64
	Success         int64
	Failed          int64
	Timestamp       time.Time
}

type SessionSummary struct {
	SessionID       string    `json:"SessionID"`
	Model           string    `json:"Model"`
	InputTokens     int64     `json:"InputTokens"`
	OutputTokens    int64     `json:"OutputTokens"`
	CachedTokens    int64     `json:"CachedTokens"`
	ReasoningTokens int64     `json:"ReasoningTokens"`
	Timestamp       time.Time `json:"StartedAt"`
	CreditsConsumed float64   `json:"CreditsConsumed"` // server-billed amount at time of request
}

type PostPayUsageEntry struct {
	CreditsConsumed  float64            `json:"CreditsConsumed"`
	CreditsPurchased float64            `json:"CreditsPurchased"`
	Success          int64              `json:"Success"`
	Failed           int64              `json:"Failed"`
	Timestamp        time.Time          `json:"Timestamp"`
	TotalTokens      int64              `json:"TotalTokens"`
	Models           map[string]int64   `json:"Models,omitempty"`
	DailyRequests    map[string]int64   `json:"DailyRequests,omitempty"`
	DailyBurn        map[string]float64 `json:"DailyBurn,omitempty"` // credits consumed per calendar day (UTC) — retained for historical stats only
	Sessions         []SessionSummary   `json:"Sessions,omitempty"`
	ProcessedTxns    []string           `json:"ProcessedTxns,omitempty"`

	// 5-hour rolling window burst-pricing fields.
	// FiveHWindowStart marks when the current window opened.
	// FiveHCredits accumulates post-multiplier credits billed within that window.
	// When time.Now() - FiveHWindowStart > 5h the window resets and the
	// multiplier returns to x1.0 for the next request.
	FiveHWindowStart time.Time `json:"FiveHWindowStart,omitempty"`
	FiveHCredits     float64   `json:"FiveHCredits,omitempty"`

	// LastAlertedTier tracks the highest burst tier for which an alert has
	// already been fired in this window, to prevent duplicate webhook POSTs.
	// 0 = no alert sent yet; 4 = ceiling tier already alerted.
	// Resets to 0 whenever FiveHWindowStart is reset.
	LastAlertedTier int `json:"LastAlertedTier,omitempty"`
}

var (
	clientUsageMu sync.RWMutex
	clientUsage   = make(map[string]*ClientUsageEntry)

	postPayUsageMu sync.RWMutex
	postPayUsage   = make(map[string]*PostPayUsageEntry)

	liveCfgMu    sync.RWMutex
	globalConfig *config.Config

	// pgLedgerDB is the optional Postgres connection used to persist the post-pay
	// ledger across Railway redeploys (ephemeral filesystem). When set, every call
	// to SavePostPayUsage also writes to the auth_store table.
	pgLedgerMu sync.RWMutex
	pgLedgerDB *sql.DB

	// alertHTTPClient is a dedicated HTTP client for burst-tier webhook POSTs.
	// Using a separate client (instead of http.DefaultClient) prevents alert
	// goroutines from contending with the global transport pool and makes
	// timeouts fully explicit.
	alertHTTPClient = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    5,
			IdleConnTimeout: 30 * time.Second,
		},
	}
)

// SetClientQuotaConfig injects the live config so the middleware always reads the latest
// credit-limit and billing settings without requiring a server restart.
func SetClientQuotaConfig(cfg *config.Config) {
	liveCfgMu.Lock()
	globalConfig = cfg
	liveCfgMu.Unlock()
}

func init() {
	coreusage.RegisterPlugin(&clientQuotaPlugin{})
}

type clientQuotaPlugin struct{}

// fiveHWindowBurnMultiplier returns the dynamic cost multiplier for a post-pay key
// based on how much of its 5-hour rate limit has been consumed in the current window.
//
// Ratio = FiveHCredits / rateLimit (the key's configured 5H credit cap).
//
//	ratio < 20%  → x1.0  (normal — fresh window)
//	ratio < 40%  → x1.2  (moderate burst)
//	ratio < 60%  → x1.4  (heavy burst)
//	ratio < 80%  → x1.7  (severe burst)
//	ratio ≥ 80%  → random{1.8, 1.9, 2.0} (ceiling — unpredictable to deter gaming)
//
// When the 5H window has fully expired (no write for 5h) the multiplier resets to x1.0.
// This model captures burst abuse (100 requests in 5 minutes) that a daily threshold
// completely misses, while remaining fair to steady, low-frequency heavy users.
func fiveHWindowBurnMultiplier(entry *PostPayUsageEntry, rateLimit float64) float64 {
	if entry == nil || rateLimit <= 0 {
		return 1.0
	}
	// Window fully expired → fresh start.
	if entry.FiveHWindowStart.IsZero() || time.Since(entry.FiveHWindowStart) > 5*time.Hour {
		return 1.0
	}
	ratio := entry.FiveHCredits / rateLimit
	switch {
	case ratio < 0.10:
		return 1.0
	case ratio < 0.30:
		return 1.5
	case ratio < 0.50:
		return 2.0
	case ratio < 0.70:
		return 2.5
	default:
		// Randomised ceiling: unpredictable within [3.0, 4.0] to prevent
		// users from gaming the exact threshold boundary while burning fast.
		opts := [3]float64{3.0, 3.5, 4.0}
		return opts[rand.Intn(3)]
	}
}

func (p *clientQuotaPlugin) HandleUsage(ctx context.Context, record coreusage.Record) {
	apiKey := strings.TrimSpace(record.APIKey)
	if apiKey == "" {
		return
	}

	alias := strings.TrimSpace(record.Alias)
	success := !record.Failed

	// Isolated Post-Pay Billing Logic
	liveCfgMu.RLock()
	liveCfg := globalConfig
	liveCfgMu.RUnlock()
	if liveCfg != nil && liveCfg.PostPayBilling.Enabled {
		if _, isPostPay := liveCfg.PostPayBilling.Clients[apiKey]; isPostPay {
			var credits float64 = 0
			if pricing, ok := liveCfg.PostPayBilling.MarkupRates[alias]; ok {
				billableInput := record.Detail.InputTokens - record.Detail.CachedTokens
				if billableInput < 0 {
					billableInput = 0
				}
				credits += float64(billableInput) * pricing.Input / 1_000_000.0
				credits += float64(record.Detail.OutputTokens) * pricing.Output / 1_000_000.0
				credits += float64(record.Detail.CachedTokens) * pricing.Cache / 1_000_000.0
				// Reasoning tokens billed at output rate (industry standard, same as Anthropic).
				credits += float64(record.Detail.ReasoningTokens) * pricing.Output / 1_000_000.0
			}

			// Resolve this key's 5H rate limit for multiplier calculation.
			rateLimit := float64(liveCfg.DefaultAPIKeyLimit)
			hasCustomLimit := false
			if liveCfg.APIKeyLimits != nil {
				if customLimit, ok := liveCfg.APIKeyLimits[apiKey]; ok {
					rateLimit = float64(customLimit)
					hasCustomLimit = true
				}
			}

			postPayUsageMu.Lock()
			entry, exists := postPayUsage[apiKey]
			if !exists {
				entry = &PostPayUsageEntry{Timestamp: time.Now()}
				postPayUsage[apiKey] = entry
			}

			// PAYG keys get exactly 1.0x multiplier. They have no 5H burst penalty because they pay raw cash.
			if !hasCustomLimit {
				isPAYG := strings.HasPrefix(apiKey, "fink_") && !strings.HasPrefix(apiKey, "fink_max_") && !strings.HasPrefix(apiKey, "fink_pro_") && !strings.HasPrefix(apiKey, "fink_d")
				if isPAYG {
					rateLimit = 0
				}
			}

			// Reset the 5H rolling window when it has fully expired.
			// This guarantees the multiplier returns to x1.0 after a quiet period.
			if entry.FiveHWindowStart.IsZero() || time.Since(entry.FiveHWindowStart) > 5*time.Hour {
				entry.FiveHWindowStart = time.Now()
				entry.FiveHCredits = 0
				entry.LastAlertedTier = 0 // fresh window — reset alert state
			}

			// Apply 5H rolling window burst-rate multiplier BEFORE billing.
			// Multiplier is based on what fraction of the window limit has already
			// been consumed — so the very first request in a fresh window is always x1.0.
			mult := fiveHWindowBurnMultiplier(entry, rateLimit)
			billedCredits := credits * mult

			entry.CreditsConsumed += billedCredits
			entry.TotalTokens += record.Detail.InputTokens + record.Detail.OutputTokens

			// Accumulate 5H window credits (drives next-request multiplier).
			entry.FiveHCredits += billedCredits

			// Also track daily burn for historical stats / admin analytics.
			today := time.Now().UTC().Format("2006-01-02")
			if entry.DailyBurn == nil {
				entry.DailyBurn = make(map[string]float64)
			}
			entry.DailyBurn[today] += billedCredits

			if success {
				entry.Success++
				if entry.Models == nil {
					entry.Models = make(map[string]int64)
				}
				entry.Models[record.Alias]++
				if entry.DailyRequests == nil {
					entry.DailyRequests = make(map[string]int64)
				}
				entry.DailyRequests[today]++
				newSession := SessionSummary{
					SessionID:       record.SessionID,
					Model:           record.Alias,
					InputTokens:     record.Detail.InputTokens,
					OutputTokens:    record.Detail.OutputTokens,
					CachedTokens:    record.Detail.CachedTokens,
					ReasoningTokens: record.Detail.ReasoningTokens,
					Timestamp:       time.Now(),
					CreditsConsumed: billedCredits, // post-multiplier billed amount for dashboard
				}
				entry.Sessions = append(entry.Sessions, SessionSummary{})
				copy(entry.Sessions[1:], entry.Sessions)
				entry.Sessions[0] = newSession
				if len(entry.Sessions) > 100 {
					entry.Sessions = entry.Sessions[:100]
				}
			} else {
				entry.Failed++
			}
			// Check whether we crossed into a new burst tier this request.
			// Tier numbers: 1=20%, 2=40%, 3=60%, 4=80% ceiling.
			// Only fire the alert once per tier per window (LastAlertedTier prevents duplicates).
			if rateLimit > 0 {
				postRatio := entry.FiveHCredits / rateLimit
				newTier := 0
				switch {
				case postRatio >= 0.80:
					newTier = 4
				case postRatio >= 0.60:
					newTier = 3
				case postRatio >= 0.40:
					newTier = 2
				case postRatio >= 0.20:
					newTier = 1
				}
				if newTier > entry.LastAlertedTier {
					entry.LastAlertedTier = newTier
					// Snapshot values under the lock before releasing it.
					alertKey := apiKey
					alertTier := newTier
					alertRatio := postRatio
					alertCredits := entry.FiveHCredits
					alertLimit := rateLimit
					// Read ledger file path and webhook URL from live config — hot-reload aware.
					webhookURL := liveCfg.PostPayBilling.BurstAlertWebhookURL
					ledgerFile := liveCfg.PostPayBilling.LedgerFile
					if webhookURL != "" {
						go fireBurstAlert(webhookURL, alertKey, alertTier, alertRatio, alertCredits, alertLimit)
					}
					// Immediately flush FiveHCredits to disk+Postgres on every tier crossing
					// to close the 60-second periodic-save drift window. Without this, a
					// server crash between saves can restore stale window state from Postgres,
					// giving the user a free quota refund of up to 60s of burst consumption.
					if ledgerFile != "" {
						go func() {
							if err := SavePostPayUsage(ledgerFile); err != nil {
								log.Errorf("burst tier %d: immediate ledger save failed: %v", alertTier, err)
							}
						}()
					}
				}
			}
			postPayUsageMu.Unlock()
			return // Skip standard volatile tracking
		}
	}

	var credits float64 = 0
	liveCfgMu.RLock()
	locCfg := globalConfig
	liveCfgMu.RUnlock()
	if locCfg != nil && locCfg.ModelPricing != nil {
		if pricing, ok := locCfg.ModelPricing[alias]; ok {
			billableInput := record.Detail.InputTokens - record.Detail.CachedTokens
			if billableInput < 0 {
				billableInput = 0
			}
			credits += float64(billableInput) * pricing.Input / 1_000_000.0
			credits += float64(record.Detail.OutputTokens) * pricing.Output / 1_000_000.0
			credits += float64(record.Detail.CachedTokens) * pricing.Cache / 1_000_000.0
		}
	}

	clientUsageMu.Lock()
	defer clientUsageMu.Unlock()
	entry, exists := clientUsage[apiKey]
	now := time.Now()
	// Reset tumbling window every 5 hours
	if !exists || now.Sub(entry.Timestamp) > 5*time.Hour {
		entry = &ClientUsageEntry{Timestamp: now}
		clientUsage[apiKey] = entry
	}
	entry.CreditsConsumed += credits
	if success {
		entry.Success++
	} else {
		entry.Failed++
	}
}

// ClientQuotaMiddleware intercepts requests to enforce APIKeyLimits.
// NOTE: All billing config reads use globalConfig (updated on hot-reload) so that
// changes to credit-limit take effect immediately without a server restart.
func ClientQuotaMiddleware(cfg *config.Config) gin.HandlerFunc {
	SetClientQuotaConfig(cfg)
	return func(c *gin.Context) {
		apiKey := ""
		if val, ok := c.Get("userApiKey"); ok {
			if s, ok := val.(string); ok {
				apiKey = strings.TrimSpace(s)
			}
		}

		if apiKey == "" {
			c.Next()
			return
		}

		// Always read live config from globalConfig so hot-reload takes effect instantly.
		liveCfgMu.RLock()
		liveCfg := globalConfig
		liveCfgMu.RUnlock()

		if liveCfg == nil {
			c.Next()
			return
		}

		// Never block read-only informational endpoints — the user must always
		// be able to check their quota, models list, and billing dashboard,
		// even if they have exceeded the credit limit.
		// Only inference (POST) routes are subject to the kill-switch.
		if c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		// Isolated Post-Pay Kill Switch (reads live config — hot-reload aware)
		if liveCfg.PostPayBilling.Enabled {
			if clientCfg, ok := liveCfg.PostPayBilling.Clients[apiKey]; ok {
				postPayUsageMu.RLock()
				entry, exists := postPayUsage[apiKey]

				var fiveHBilled float64 // post-multiplier credits used in current window
				var creditsConsumed float64
				var creditsPurchased float64
				if exists && entry != nil {
					creditsConsumed = entry.CreditsConsumed
					creditsPurchased = entry.CreditsPurchased
					// Read FiveHCredits directly — this is the single source of truth and
					// includes burst multipliers. Do NOT recompute from raw session tokens:
					// the session-token sum ignores multipliers, creating a gap where a user
					// at x2.0 ceiling can consume 2× more credits than the limit before being blocked.
					// Also check window expiry so stale windows don't block fresh requests.
					if !entry.FiveHWindowStart.IsZero() && time.Since(entry.FiveHWindowStart) <= 5*time.Hour {
						fiveHBilled = entry.FiveHCredits
					}
				}
				postPayUsageMu.RUnlock()

				// 0. Time-based Expiration Check (Kill Switch)
				if !clientCfg.ExpiresAt.IsZero() && time.Now().UTC().After(clientCfg.ExpiresAt.UTC()) {
					c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
						"error": gin.H{
							"message": "subscription_expired: Your API plan has expired. Please renew your subscription.",
							"type":    "subscription_expired",
							"code":    "subscription_expired",
						},
					})
					return
				}

				// 1. Balance Exhaustion (Hard Limit)
				// For post-pay, the effective limit is the sum of payments made plus the allowed line of credit.
				effectiveLimit := creditsPurchased
				if clientCfg.CreditLimit > 0 {
					effectiveLimit += clientCfg.CreditLimit
				}

				if creditsConsumed >= effectiveLimit {
					c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
						"error": gin.H{
							"message": "insufficient_balance: You have exhausted your credit limit. Please deposit funds to continue.",
							"type":    "insufficient_balance",
							"code":    "insufficient_balance",
						},
					})
					return
				}

				// 2. 5-Hour Rate Limit Check
				// Compares post-multiplier window spend (fiveHBilled) against the configured limit
				// so burst pricing actually enforces the cap — not a pre-multiplier shortfall.
				limit := liveCfg.DefaultAPIKeyLimit
				hasCustomLimit := false
				if liveCfg.APIKeyLimits != nil {
					if customLimit, ok := liveCfg.APIKeyLimits[apiKey]; ok {
						limit = customLimit
						hasCustomLimit = true
					}
				}

				// PAYG keys have NO 5h limit by default
				if !hasCustomLimit {
					isPAYG := strings.HasPrefix(apiKey, "fink_") && !strings.HasPrefix(apiKey, "fink_max_") && !strings.HasPrefix(apiKey, "fink_pro_") && !strings.HasPrefix(apiKey, "fink_d")
					if isPAYG {
						limit = 0
					}
				}
				if limit > 0 {
					if fiveHBilled >= float64(limit) {
						// Compute exact seconds until the 5H window resets.
						// This lets clients backoff precisely instead of polling /v1/billing/quota.
						postPayUsageMu.RLock()
						windowEntry := postPayUsage[apiKey]
						postPayUsageMu.RUnlock()
						var retryAfterSecs int
						if windowEntry != nil && !windowEntry.FiveHWindowStart.IsZero() {
							windowEnd := windowEntry.FiveHWindowStart.Add(5 * time.Hour)
							if secs := int(time.Until(windowEnd).Seconds()); secs > 0 {
								retryAfterSecs = secs
							}
						}
						if retryAfterSecs > 0 {
							c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSecs))
						}
						c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
							"error": gin.H{
								"message": "insufficient_quota: You exceeded your 5-hour rate limit. Please wait before continuing.",
								"type":    "insufficient_quota",
								"code":    "insufficient_quota",
							},
						})
						return
					}
				}

				// Note: We deliberately do NOT attach internal billing multipliers
				// or burst-tier ratios to the end-user HTTP response headers.
				// Exposing these details allows clients to reverse-engineer profit margins
				// and game the rate-limit boundaries. This data must remain isolated
				// to the management API endpoints.

				c.Next()
				return // Skip standard volatile kill switch
			}
		}

		// Commercial-mode billing guard: if billing is enabled and this key is NOT
		// enrolled in post-pay-billing.clients, it would silently bypass all credit
		// controls (balance ceiling, expiry, 5H burst limit). Block it immediately
		// so operators are forced to enrol the key in billing before it can be used.
		// This prevents manually-added api-keys from becoming free-tier loopholes.
		if liveCfg.CommercialMode {
			if _, ok := liveCfg.PostPayBilling.Clients[apiKey]; !ok {
				c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
					"error": gin.H{
						"message": "no_billing_account: This API key has no billing configuration. Please contact support to activate your key.",
						"type":    "no_billing_account",
						"code":    "no_billing_account",
					},
				})
				return
			}
		}

		// Standard volatile kill switch (also reads live config)
		limit := liveCfg.DefaultAPIKeyLimit
		if liveCfg.APIKeyLimits != nil {
			if customLimit, ok := liveCfg.APIKeyLimits[apiKey]; ok {
				limit = customLimit
			}
		}

		if limit > 0 {
			clientUsageMu.RLock()
			entry, exists := clientUsage[apiKey]
			clientUsageMu.RUnlock()

			if exists {
				now := time.Now()
				if now.Sub(entry.Timestamp) <= 5*time.Hour {
					if entry.CreditsConsumed >= float64(limit) {
						c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
							"error": gin.H{
								"message": "insufficient_quota: You exceeded your current quota, please check your plan and billing details.",
								"type":    "insufficient_quota",
								"code":    "insufficient_quota",
							},
						})
						return
					}
				}
			}
		}
		c.Next()
	}
}

// GetClientUsageSnapshot returns a copy of the current client token ledger for management UI.
func GetClientUsageSnapshot() map[string]ClientUsageEntry {
	clientUsageMu.RLock()
	defer clientUsageMu.RUnlock()
	out := make(map[string]ClientUsageEntry, len(clientUsage))
	for k, v := range clientUsage {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

// GetPostPaySnapshot returns a copy of the isolated post-pay ledger.
func GetPostPaySnapshot() map[string]PostPayUsageEntry {
	postPayUsageMu.RLock()
	defer postPayUsageMu.RUnlock()
	out := make(map[string]PostPayUsageEntry, len(postPayUsage))
	for k, v := range postPayUsage {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

// GetLiveConfig returns the current live config snapshot (hot-reload aware, thread-safe).
func GetLiveConfig() *config.Config {
	liveCfgMu.RLock()
	defer liveCfgMu.RUnlock()
	return globalConfig
}

// GetPostPayCreditLimit returns the configured credit limit for a given API key.
// Returns -1 if no specific limit is configured.
func GetPostPayCreditLimit(apiKey string) float64 {
	liveCfgMu.RLock()
	cfg := globalConfig
	liveCfgMu.RUnlock()
	if cfg != nil && cfg.PostPayBilling.Enabled {
		if clientCfg, ok := cfg.PostPayBilling.Clients[apiKey]; ok && clientCfg.CreditLimit > 0 {
			return clientCfg.CreditLimit
		}
	}
	return -1
}

// GetCreditsPurchasedForKey returns the total credits purchased for a given key.
func GetCreditsPurchasedForKey(apiKey string) float64 {
	postPayUsageMu.RLock()
	entry := postPayUsage[apiKey]
	postPayUsageMu.RUnlock()
	if entry == nil {
		return 0
	}
	return entry.CreditsPurchased
}

// GetBurnMultiplierForKey returns the current 5H rolling window burst multiplier
// for the given post-pay API key. Looks up the key's rate limit from live config
// so it scales correctly for every plan (1K, 10K, 40K CR limits).
// Returns 1.0 when no active window exists or the window has fully expired.
func GetBurnMultiplierForKey(apiKey string) float64 {
	liveCfgMu.RLock()
	cfg := globalConfig
	liveCfgMu.RUnlock()
	var rateLimit float64
	hasCustomLimit := false
	if cfg != nil {
		rateLimit = float64(cfg.DefaultAPIKeyLimit)
		if cfg.APIKeyLimits != nil {
			if lim, ok := cfg.APIKeyLimits[apiKey]; ok {
				rateLimit = float64(lim)
				hasCustomLimit = true
			}
		}
	}

	postPayUsageMu.RLock()
	entry := postPayUsage[apiKey]
	postPayUsageMu.RUnlock()

	if !hasCustomLimit {
		isPAYG := strings.HasPrefix(apiKey, "fink_") && !strings.HasPrefix(apiKey, "fink_max_") && !strings.HasPrefix(apiKey, "fink_pro_") && !strings.HasPrefix(apiKey, "fink_d")
		if isPAYG {
			rateLimit = 0
		}
	}

	return fiveHWindowBurnMultiplier(entry, rateLimit)
}

// GetFiveHCreditsForKey returns credits consumed in the current 5H rolling window
// for the given key. Returns 0 when no active window exists or it has expired.
func GetFiveHCreditsForKey(apiKey string) float64 {
	postPayUsageMu.RLock()
	entry := postPayUsage[apiKey]
	postPayUsageMu.RUnlock()
	if entry == nil {
		return 0
	}
	if entry.FiveHWindowStart.IsZero() || time.Since(entry.FiveHWindowStart) > 5*time.Hour {
		return 0
	}
	return entry.FiveHCredits
}

// GetFiveHWindowStartForKey returns the start time of the current 5H window.
// Returns zero time when no active window exists.
func GetFiveHWindowStartForKey(apiKey string) time.Time {
	postPayUsageMu.RLock()
	entry := postPayUsage[apiKey]
	postPayUsageMu.RUnlock()
	if entry == nil {
		return time.Time{}
	}
	if entry.FiveHWindowStart.IsZero() || time.Since(entry.FiveHWindowStart) > 5*time.Hour {
		return time.Time{}
	}
	return entry.FiveHWindowStart
}

// GetDailyBurnForKey returns credits consumed today (UTC) for the given key.
// Retained for historical stats and admin analytics dashboards.
func GetDailyBurnForKey(apiKey string) float64 {
	postPayUsageMu.RLock()
	entry := postPayUsage[apiKey]
	postPayUsageMu.RUnlock()
	if entry == nil || entry.DailyBurn == nil {
		return 0
	}
	today := time.Now().UTC().Format("2006-01-02")
	return entry.DailyBurn[today]
}

// LoadPostPayUsage loads the persisted post-pay token ledger from disk.
func LoadPostPayUsage(filePath string) error {
	postPayUsageMu.Lock()
	defer postPayUsageMu.Unlock()
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	err = json.Unmarshal(data, &postPayUsage)
	if err != nil {
		return err
	}

	// Backwards compatibility migration: extrapolate all-time counters from legacy sessions
	for _, entry := range postPayUsage {
		sessionCount := int64(len(entry.Sessions))

		if entry.Models == nil || len(entry.Models) == 0 {
			entry.Models = make(map[string]int64)
			var sessionModels = make(map[string]int64)
			for _, s := range entry.Sessions {
				sessionModels[s.Model]++
			}

			if sessionCount > 0 && entry.Success > sessionCount {
				multiplier := float64(entry.Success) / float64(sessionCount)
				var assigned int64
				var lastModel string
				for model, count := range sessionModels {
					extrapolated := int64(float64(count) * multiplier)
					entry.Models[model] = extrapolated
					assigned += extrapolated
					lastModel = model
				}
				// Fix rounding errors so it perfectly sums to Success count
				if assigned < entry.Success && lastModel != "" {
					entry.Models[lastModel] += (entry.Success - assigned)
				}
			} else {
				for model, count := range sessionModels {
					entry.Models[model] = count
				}
			}
		}

		if entry.DailyRequests == nil || len(entry.DailyRequests) == 0 {
			entry.DailyRequests = make(map[string]int64)
			var sessionDays = make(map[string]int64)
			for _, s := range entry.Sessions {
				dayStr := s.Timestamp.Format("2006-01-02")
				sessionDays[dayStr]++
			}

			if sessionCount > 0 && entry.Success > sessionCount {
				multiplier := float64(entry.Success) / float64(sessionCount)
				var assigned int64
				var lastDay string
				for day, count := range sessionDays {
					extrapolated := int64(float64(count) * multiplier)
					entry.DailyRequests[day] = extrapolated
					assigned += extrapolated
					lastDay = day
				}
				// Fix rounding errors so it perfectly sums to Success count
				if assigned < entry.Success && lastDay != "" {
					entry.DailyRequests[lastDay] += (entry.Success - assigned)
				}
			} else {
				for day, count := range sessionDays {
					entry.DailyRequests[day] = count
				}
			}
		}

		if entry.TotalTokens == 0 {
			var sessionTokens int64
			for _, s := range entry.Sessions {
				sessionTokens += s.InputTokens + s.OutputTokens
			}
			if sessionCount > 0 && entry.Success > sessionCount {
				multiplier := float64(entry.Success) / float64(sessionCount)
				entry.TotalTokens = int64(float64(sessionTokens) * multiplier)
			} else {
				entry.TotalTokens = sessionTokens
			}
		}
	}
	return nil
}

// SetLedgerDB registers a Postgres connection for ledger persistence.
// When set, SavePostPayUsage also writes the ledger to the auth_store table
// so that CreditsPurchased survives Railway redeploys (ephemeral filesystem).
func SetLedgerDB(db *sql.DB) {
	pgLedgerMu.Lock()
	pgLedgerDB = db
	pgLedgerMu.Unlock()
}

// persistLedgerToPostgres upserts the current in-memory ledger into auth_store.
// Called asynchronously after every disk save — failures are logged but not fatal.
func persistLedgerToPostgres(data []byte) {
	pgLedgerMu.RLock()
	db := pgLedgerDB
	pgLedgerMu.RUnlock()
	if db == nil {
		return
	}
	// Use a short context — this is a best-effort background write.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `
		INSERT INTO auth_store (id, content, created_at, updated_at)
		VALUES ('ledger', $1::jsonb, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE
		  SET content    = EXCLUDED.content,
		      updated_at = NOW()
	`, string(data))
	if err != nil {
		log.Errorf("persistLedgerToPostgres: failed to upsert auth_store: %v", err)
	}
}

// LoadLedgerFromPostgres reads the ledger from Postgres auth_store and merges it
// into in-memory state. Designed as a cold-start fallback when the JSON file is
// missing (e.g. after a Railway redeploy with ephemeral FS).
// Existing in-memory entries are preserved; Postgres entries are applied only when
// they carry a higher CreditsPurchased value than what is already loaded.
func LoadLedgerFromPostgres(db *sql.DB) error {
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	row := db.QueryRowContext(ctx, `SELECT content FROM auth_store WHERE id = 'ledger' LIMIT 1`)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil // no ledger snapshot yet — first boot
		}
		return err
	}
	var pgLedger map[string]*PostPayUsageEntry
	if err := json.Unmarshal([]byte(raw), &pgLedger); err != nil {
		return err
	}
	postPayUsageMu.Lock()
	defer postPayUsageMu.Unlock()
	for key, pgEntry := range pgLedger {
		if pgEntry == nil {
			continue
		}
		existing, ok := postPayUsage[key]
		if !ok || existing == nil {
			// Key not in memory — load from Postgres
			postPayUsage[key] = pgEntry
			continue
		}
		// Prefer the higher CreditsPurchased to avoid accidentally reverting top-ups.
		if pgEntry.CreditsPurchased > existing.CreditsPurchased {
			existing.CreditsPurchased = pgEntry.CreditsPurchased
			existing.ProcessedTxns = pgEntry.ProcessedTxns
		}
		// Restore 5H window state from Postgres if in-memory window is stale/empty.
		// This prevents a free window reset when the server redeploys on Railway
		// (ephemeral FS loses the JSON file, but Postgres retains the last snapshot).
		// Only restore when the Postgres window is still active (not yet expired).
		pgWindowAlive := !pgEntry.FiveHWindowStart.IsZero() &&
			time.Since(pgEntry.FiveHWindowStart) <= 5*time.Hour
		memWindowAlive := !existing.FiveHWindowStart.IsZero() &&
			time.Since(existing.FiveHWindowStart) <= 5*time.Hour
		if pgWindowAlive && !memWindowAlive {
			// In-memory window is absent/expired but Postgres has a live one — restore it.
			existing.FiveHWindowStart = pgEntry.FiveHWindowStart
			existing.FiveHCredits = pgEntry.FiveHCredits
			existing.LastAlertedTier = pgEntry.LastAlertedTier
		} else if pgWindowAlive && memWindowAlive {
			// Both windows are alive — prefer the one with more credits consumed
			// (handles dual-writer races during rolling deploys).
			if pgEntry.FiveHCredits > existing.FiveHCredits {
				existing.FiveHWindowStart = pgEntry.FiveHWindowStart
				existing.FiveHCredits = pgEntry.FiveHCredits
				existing.LastAlertedTier = pgEntry.LastAlertedTier
			}
		}
	}
	return nil
}

// fireBurstAlert POSTs a JSON payload to webhookURL notifying operators that
// a post-pay key has crossed a burst tier threshold. It is called in a goroutine
// from HandleUsage — failures are logged but never fatal to the billing path.
//
// Payload shape:
//
//	{
//	  "event":         "burst_tier_crossed",
//	  "api_key":       "fink_max_...",
//	  "tier":          4,
//	  "tier_label":    "CEILING",
//	  "ratio":         0.82,
//	  "five_h_credits": 8200.5,
//	  "limit":         10000,
//	  "fired_at":      "2026-07-06T03:57:00Z"
//	}
//
// maskedKey returns a redacted version of the API key safe to include in
// outbound webhook payloads. Only the last 8 characters are preserved so
// operators can identify the key without exposing the full secret over a
// potentially non-TLS webhook endpoint.
func maskedKey(apiKey string) string {
	const visible = 8
	if len(apiKey) <= visible {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-visible:]
}

func fireBurstAlert(webhookURL, apiKey string, tier int, ratio, fiveHCredits, limit float64) {
	tierLabel := map[int]string{
		1: "MODERATE",
		2: "HEAVY",
		3: "SEVERE",
		4: "CEILING",
	}[tier]
	payload, err := json.Marshal(map[string]any{
		"event":          "burst_tier_crossed",
		"api_key":        maskedKey(apiKey), // masked: prevents key exposure over plain-HTTP webhooks
		"tier":           tier,
		"tier_label":     tierLabel,
		"ratio":          ratio,
		"five_h_credits": fiveHCredits,
		"limit":          limit,
		"fired_at":       time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		log.Errorf("fireBurstAlert: json.Marshal: %v", err)
		return
	}
	// Use dedicated alertHTTPClient — not http.DefaultClient — to keep alert
	// goroutines isolated from the main outbound connection pool.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		log.Errorf("fireBurstAlert: build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := alertHTTPClient.Do(req)
	if err != nil {
		log.Errorf("fireBurstAlert: POST %s: %v", webhookURL, err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Warnf("fireBurstAlert: webhook returned HTTP %d for key %s tier %d",
			resp.StatusCode, maskedKey(apiKey), tier)
	}
}

// SavePostPayUsage saves the current post-pay token ledger to disk atomically
// and asynchronously persists it to Postgres (if SetLedgerDB was called).
func SavePostPayUsage(filePath string) error {
	postPayUsageMu.RLock()
	data, err := json.MarshalIndent(postPayUsage, "", "  ")
	postPayUsageMu.RUnlock()
	if err != nil {
		return err
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		return err
	}

	// Persist to Postgres in the background — non-blocking, best-effort.
	go persistLedgerToPostgres(data)
	return nil
}

// StartPostPayPersister runs a background loop to save post-pay usage periodically.
func StartPostPayPersister(filePath string) {
	go func() {
		for {
			time.Sleep(60 * time.Second)
			_ = SavePostPayUsage(filePath)
		}
	}()
}

// ProcessDeposit adds USD funds to an API key, converting them to credits
// based on the cumulative purchasing tier. It is idempotent if a txnID is provided.
func ProcessDeposit(apiKey string, usdAmount float64, txnID string) (addedCredits float64, newTotalPurchased float64, tier int, isDuplicate bool) {
	postPayUsageMu.Lock()
	defer postPayUsageMu.Unlock()

	entry, exists := postPayUsage[apiKey]
	if !exists {
		entry = &PostPayUsageEntry{Timestamp: time.Now()}
		postPayUsage[apiKey] = entry
	}

	if txnID != "" {
		for _, id := range entry.ProcessedTxns {
			if id == txnID {
				// Determine tier without modifying
				t := 1
				if entry.CreditsPurchased >= 500_000 {
					t = 6
				} else if entry.CreditsPurchased >= 300_000 {
					t = 5
				} else if entry.CreditsPurchased >= 200_000 {
					t = 4
				} else if entry.CreditsPurchased >= 100_000 {
					t = 3
				} else if entry.CreditsPurchased >= 50_000 {
					t = 2
				}
				return 0, entry.CreditsPurchased, t, true
			}
		}
	}

	// Determine current Tier based on CreditsPurchased *BEFORE* this deposit
	var rate float64
	current := entry.CreditsPurchased
	if current < 50_000 {
		rate = 6_000
		tier = 1
	} else if current < 100_000 {
		rate = 6_500
		tier = 2
	} else if current < 200_000 {
		rate = 7_200
		tier = 3
	} else if current < 300_000 {
		rate = 8_000
		tier = 4
	} else if current < 500_000 {
		rate = 9_000
		tier = 5
	} else {
		rate = 10_500
		tier = 6
	}

	addedCredits = usdAmount * rate
	entry.CreditsPurchased += addedCredits
	newTotalPurchased = entry.CreditsPurchased

	if txnID != "" {
		entry.ProcessedTxns = append(entry.ProcessedTxns, txnID)
		if len(entry.ProcessedTxns) > 100 {
			entry.ProcessedTxns = entry.ProcessedTxns[1:] // Keep last 100 txns to prevent unbounded growth
		}
	}

	return addedCredits, newTotalPurchased, tier, false
}

// vndTierEntry describes one PAYG credit conversion tier based on VND amount.
type vndTierEntry struct {
	maxVND     float64 // exclusive upper bound (0 = no upper bound)
	pricePer1k float64 // VND price per 1000 credits
	tier       int
}

// vndTiers defines the Pay-As-You-Go credit conversion tiers based on VND amount.
// Strategic Decoy Pricing: Tier 6 (>2M) is cheaper than Max 20x to act as the ultimate trap.
var vndTiers = []vndTierEntry{
	{maxVND: 50_000, pricePer1k: 15_000, tier: 1},
	{maxVND: 150_000, pricePer1k: 12_000, tier: 2},
	{maxVND: 300_000, pricePer1k: 10_000, tier: 3},
	{maxVND: 600_000, pricePer1k: 8_000, tier: 4},
	{maxVND: 2_000_000, pricePer1k: 3_500, tier: 5},
	{maxVND: 0, pricePer1k: 1_700, tier: 6}, // 0 = no upper bound
}

// vndTierForAmount returns the pricePer1k and tier number for a given VND amount.
func vndTierForAmount(vndAmount float64) (pricePer1k float64, tier int) {
	for _, t := range vndTiers {
		if t.maxVND == 0 || vndAmount <= t.maxVND {
			return t.pricePer1k, t.tier
		}
	}
	// Fallback to best tier
	last := vndTiers[len(vndTiers)-1]
	return last.pricePer1k, last.tier
}

// ProcessDepositVND adds VND funds to an API key, converting them to credits
// using the FinkRouter VND-native pricing tiers. It is idempotent if txnID is provided.
// This is the preferred deposit function for payOS webhook integrations.
//
// Returns: (creditsAdded, newTotalPurchased, tierApplied, isDuplicate).
func ProcessDepositVND(apiKey string, vndAmount float64, txnID string) (addedCredits float64, newTotalPurchased float64, tier int, isDuplicate bool) {
	postPayUsageMu.Lock()
	defer postPayUsageMu.Unlock()

	entry, exists := postPayUsage[apiKey]
	if !exists {
		entry = &PostPayUsageEntry{Timestamp: time.Now()}
		postPayUsage[apiKey] = entry
	}

	// Idempotency check — same txnID from a retried payOS webhook does nothing
	if txnID != "" {
		for _, id := range entry.ProcessedTxns {
			if id == txnID {
				_, t := vndTierForAmount(vndAmount)
				return 0, entry.CreditsPurchased, t, true
			}
		}
	}

	// Apply VND-based tier rate to compute credits
	pricePer1k, tier := vndTierForAmount(vndAmount)
	addedCredits = (vndAmount / pricePer1k) * 1000
	// Round to 3 decimal places to avoid standard floating point issues
	addedCredits = math.Round(addedCredits*1000) / 1000
	entry.CreditsPurchased += addedCredits
	newTotalPurchased = entry.CreditsPurchased

	if txnID != "" {
		entry.ProcessedTxns = append(entry.ProcessedTxns, txnID)
		if len(entry.ProcessedTxns) > 100 {
			entry.ProcessedTxns = entry.ProcessedTxns[1:]
		}
	}

	return addedCredits, newTotalPurchased, tier, false
}

// ProcessDepositCredits adds an exact credit amount directly to an API key's
// creditsPurchased balance — no USD/VND conversion involved.
// Use this for subscription plans where the credit amount is fixed by the plan,
// not derived from a currency amount.
// It is idempotent if txnID is provided.
func ProcessDepositCredits(apiKey string, credits float64, txnID string) (addedCredits float64, newTotalPurchased float64, isDuplicate bool) {
	postPayUsageMu.Lock()
	defer postPayUsageMu.Unlock()

	entry, exists := postPayUsage[apiKey]
	if !exists {
		entry = &PostPayUsageEntry{Timestamp: time.Now()}
		postPayUsage[apiKey] = entry
	}

	// Idempotency check — same txnID from a retried webhook does nothing
	if txnID != "" {
		for _, id := range entry.ProcessedTxns {
			if id == txnID {
				return 0, entry.CreditsPurchased, true
			}
		}
	}

	entry.CreditsPurchased += credits
	newTotalPurchased = entry.CreditsPurchased

	if txnID != "" {
		entry.ProcessedTxns = append(entry.ProcessedTxns, txnID)
		if len(entry.ProcessedTxns) > 100 {
			entry.ProcessedTxns = entry.ProcessedTxns[1:]
		}
	}

	return credits, newTotalPurchased, false
}

// SwapKeyInPostPayMemory transfers post-pay balances and history from an old key to a new key.
func SwapKeyInPostPayMemory(oldKey, newKey string) {
	postPayUsageMu.Lock()
	defer postPayUsageMu.Unlock()

	if entry, exists := postPayUsage[oldKey]; exists {
		postPayUsage[newKey] = entry
		delete(postPayUsage, oldKey)
	}
}

// SwapKeyInConfigMemory replaces the oldKey with the newKey in the live config memory.
func SwapKeyInConfigMemory(oldKey, newKey string) {
	liveCfgMu.Lock()
	defer liveCfgMu.Unlock()

	if globalConfig == nil {
		return
	}

	// Swap in API keys list
	for i, k := range globalConfig.APIKeys {
		if k == oldKey {
			globalConfig.APIKeys[i] = newKey
		}
	}

	// Swap in Model Pricing/Limits map if custom limits exist
	if globalConfig.APIKeyLimits != nil {
		if limit, ok := globalConfig.APIKeyLimits[oldKey]; ok {
			globalConfig.APIKeyLimits[newKey] = limit
			delete(globalConfig.APIKeyLimits, oldKey)
		}
	}

	// Swap in post-pay clients map
	if globalConfig.PostPayBilling.Clients != nil {
		if client, ok := globalConfig.PostPayBilling.Clients[oldKey]; ok {
			globalConfig.PostPayBilling.Clients[newKey] = client
			delete(globalConfig.PostPayBilling.Clients, oldKey)
		}
	}
}
