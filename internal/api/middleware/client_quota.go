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
	Success         int64            `json:"Success"`
	Failed          int64            `json:"Failed"`
	Timestamp       time.Time        `json:"Timestamp"`
	Sessions        []SessionSummary `json:"Sessions,omitempty"`
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
			if success {
				entry.Success++
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
				if clientCfg.CreditLimit > 0 {
					postPayUsageMu.RLock()
					entry, exists := postPayUsage[apiKey]
					postPayUsageMu.RUnlock()
					if exists && entry.CreditsConsumed >= clientCfg.CreditLimit {
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

// GetPostPayCreditLimit returns the configured credit limit for a given API key.
// Returns -1 if no specific limit is configured.
func GetPostPayCreditLimit(apiKey string) float64 {
	if globalConfig != nil && globalConfig.PostPayBilling.Enabled {
		if clientCfg, ok := globalConfig.PostPayBilling.Clients[apiKey]; ok && clientCfg.CreditLimit > 0 {
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
	return json.Unmarshal(data, &postPayUsage)
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
