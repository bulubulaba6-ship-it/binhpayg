package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
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
}

type PostPayUsageEntry struct {
	CreditsConsumed float64          `json:"CreditsConsumed"`
	CreditsPurchased float64         `json:"CreditsPurchased"`
	Success         int64            `json:"Success"`
	Failed          int64            `json:"Failed"`
	Timestamp       time.Time        `json:"Timestamp"`
	TotalTokens     int64            `json:"TotalTokens"`
	Models          map[string]int64 `json:"Models,omitempty"`
	DailyRequests   map[string]int64 `json:"DailyRequests,omitempty"`
	Sessions        []SessionSummary `json:"Sessions,omitempty"`
	ProcessedTxns   []string         `json:"ProcessedTxns,omitempty"`
}

var (
	clientUsageMu sync.RWMutex
	clientUsage   = make(map[string]*ClientUsageEntry)

	postPayUsageMu sync.RWMutex
	postPayUsage   = make(map[string]*PostPayUsageEntry)

	liveCfgMu    sync.RWMutex
	globalConfig *config.Config
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
				credits += float64(record.Detail.InputTokens) * pricing.Input / 1_000_000.0
				credits += float64(record.Detail.OutputTokens) * pricing.Output / 1_000_000.0
				credits += float64(record.Detail.CachedTokens) * pricing.Cache / 1_000_000.0
			}

			postPayUsageMu.Lock()
			entry, exists := postPayUsage[apiKey]
			if !exists {
				entry = &PostPayUsageEntry{Timestamp: time.Now()}
				postPayUsage[apiKey] = entry
			}
			entry.CreditsConsumed += credits
			entry.TotalTokens += record.Detail.InputTokens + record.Detail.OutputTokens
			if success {
				entry.Success++
				if entry.Models == nil {
					entry.Models = make(map[string]int64)
				}
				entry.Models[record.Alias]++
				if entry.DailyRequests == nil {
					entry.DailyRequests = make(map[string]int64)
				}
				entry.DailyRequests[time.Now().Format("2006-01-02")]++
				entry.Sessions = append([]SessionSummary{{
					SessionID:       record.SessionID,
					Model:           record.Alias,
					InputTokens:     record.Detail.InputTokens,
					OutputTokens:    record.Detail.OutputTokens,
					CachedTokens:    record.Detail.CachedTokens,
					ReasoningTokens: record.Detail.ReasoningTokens,
					Timestamp:       time.Now(),
				}}, entry.Sessions...)
				if len(entry.Sessions) > 100 {
					entry.Sessions = entry.Sessions[:100]
				}
			} else {
				entry.Failed++
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
			credits += float64(record.Detail.InputTokens) * pricing.Input / 1_000_000.0
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
				
				var fiveHCredits float64
				var creditsConsumed float64
				var creditsPurchased float64
				if exists && entry != nil {
					creditsConsumed = entry.CreditsConsumed
					creditsPurchased = entry.CreditsPurchased
					fiveHCutoff := time.Now().Add(-5 * time.Hour)
					for _, s := range entry.Sessions {
						if s.Timestamp.After(fiveHCutoff) {
							if pricing, ok := liveCfg.ModelPricing[s.Model]; ok {
								fiveHCredits += float64(s.InputTokens) * pricing.Input / 1_000_000.0
								fiveHCredits += float64(s.OutputTokens) * pricing.Output / 1_000_000.0
								fiveHCredits += float64(s.CachedTokens) * pricing.Cache / 1_000_000.0
							}
						}
					}
				}
				postPayUsageMu.RUnlock()
				
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
				c.Next()
				return // Skip standard volatile kill switch
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

// SavePostPayUsage saves the current post-pay token ledger to disk atomically.
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
	return os.Rename(tmpPath, filePath)
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
				if entry.CreditsPurchased >= 500_000 { t = 6 } else if entry.CreditsPurchased >= 300_000 { t = 5 } else if entry.CreditsPurchased >= 200_000 { t = 4 } else if entry.CreditsPurchased >= 100_000 { t = 3 } else if entry.CreditsPurchased >= 50_000 { t = 2 }
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
