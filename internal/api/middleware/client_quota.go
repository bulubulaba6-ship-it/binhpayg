package middleware

import (
	"context"
	"net/http"
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

var (
	clientUsageMu sync.RWMutex
	clientUsage   = make(map[string]*ClientUsageEntry)
	globalConfig  *config.Config
)

// SetClientQuotaConfig injects the global config to evaluate limits.
func SetClientQuotaConfig(cfg *config.Config) {
	globalConfig = cfg
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

	var credits float64 = 0
	alias := strings.TrimSpace(record.Alias)
	if globalConfig != nil && globalConfig.ModelPricing != nil {
		if pricing, ok := globalConfig.ModelPricing[alias]; ok {
			credits += float64(record.Detail.InputTokens) * pricing.Input / 1_000_000.0
			credits += float64(record.Detail.OutputTokens) * pricing.Output / 1_000_000.0
			credits += float64(record.Detail.CachedTokens) * pricing.Cache / 1_000_000.0
		}
	}

	success := !record.Failed

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
func ClientQuotaMiddleware(cfg *config.Config) gin.HandlerFunc {
	SetClientQuotaConfig(cfg)
	return func(c *gin.Context) {
		apiKey := ""
		if val, ok := c.Get("userApiKey"); ok {
			if s, ok := val.(string); ok {
				apiKey = strings.TrimSpace(s)
			}
		}

		if apiKey != "" && cfg != nil {
			limit := cfg.DefaultAPIKeyLimit
			if cfg.APIKeyLimits != nil {
				if customLimit, ok := cfg.APIKeyLimits[apiKey]; ok {
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
