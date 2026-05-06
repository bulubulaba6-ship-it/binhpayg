package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

// ExecutionSessionRecord captures the mutable metadata tracked for a long-lived
// execution session while it is active.
type ExecutionSessionRecord struct {
	SessionID string
	Principal string
	Provider  string
	Model     string
	AuthID    string
	AuthIndex       string
	StartedAt       time.Time
	UpdatedAt       time.Time
	InputTokens     int64
	OutputTokens    int64
	ReasoningTokens int64
	CachedTokens    int64
}

// ExecutionSessionSummary captures a finalized session and its cost for the quota dashboard.
// Token fields are read directly from the embedded ExecutionSessionRecord.
type ExecutionSessionSummary struct {
	ExecutionSessionRecord
	Credits         float64 `json:"credits"`
	DurationSeconds int64   `json:"duration_seconds"`
}

// ExecutionSessionContextKey is used to propagate an execution session ID via context
// for one-shot API requests.
type ExecutionSessionContextKey struct{}

// ExecutionQuotaSnapshot summarizes the current session usage for one principal.
type ExecutionQuotaSnapshot struct {
	Sessions        int64     `json:"sessions"`
	ActiveSessions  int64     `json:"active_sessions"`
	CreditsUsed      float64   `json:"credits_used"`
	TotalCreditsUsed float64   `json:"total_credits_used"`
	CreditLimit     int64     `json:"credit_limit"` // 1,000 Credits = $1 USD
	TotalTokens     int64     `json:"total_tokens"`
	DurationSeconds int64                     `json:"duration_seconds"`
	LastSessionAt   time.Time                 `json:"last_session_at,omitempty"`
	WindowExpiresAt time.Time                 `json:"window_expires_at,omitempty"`
	RecentSessions  []ExecutionSessionSummary `json:"recent_sessions,omitempty"`
}

// ExecutionSessionLedger stores session lifecycle events durably.
type ExecutionSessionLedger interface {
	BeginExecutionSession(ctx context.Context, record ExecutionSessionRecord) error
	FinalizeExecutionSession(ctx context.Context, sessionID string, finishedAt time.Time) error
	GetExecutionQuotaSummary(ctx context.Context, principal string) (ExecutionQuotaSnapshot, error)
}

// memoryExecutionSessionLedger provides a process-local fallback ledger.
type memoryExecutionSessionLedger struct {
	mu       sync.Mutex
	sessions map[string]*memoryExecutionSession
}

type memoryExecutionSession struct {
	record       ExecutionSessionRecord
	finishedAt   time.Time
	finalized    bool
	credits      float64
	durationSecs int64
}

func newMemoryExecutionSessionLedger() *memoryExecutionSessionLedger {
	ledger := &memoryExecutionSessionLedger{sessions: make(map[string]*memoryExecutionSession)}
	usage.RegisterPlugin(ledger)
	return ledger
}

func (l *memoryExecutionSessionLedger) BeginExecutionSession(_ context.Context, record ExecutionSessionRecord) error {
	if l == nil {
		return nil
	}
	sessionID := strings.TrimSpace(record.SessionID)
	if sessionID == "" {
		return fmt.Errorf("execution session id is empty")
	}
	startedAt := record.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	updatedAt := record.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = startedAt
	}
	entry := &memoryExecutionSession{
		record: ExecutionSessionRecord{
			SessionID: sessionID,
			Principal: strings.TrimSpace(record.Principal),
			Provider:  strings.TrimSpace(record.Provider),
			Model:     strings.TrimSpace(record.Model),
			AuthID:    strings.TrimSpace(record.AuthID),
			AuthIndex: strings.TrimSpace(record.AuthIndex),
			StartedAt: startedAt,
			UpdatedAt: updatedAt,
		},
	}
	l.mu.Lock()
	if l.sessions == nil {
		l.sessions = make(map[string]*memoryExecutionSession)
	}
	if existing, ok := l.sessions[sessionID]; ok && existing != nil {
		if existing.finalized {
			l.mu.Unlock()
			return nil
		}
		if existing.record.StartedAt.IsZero() || startedAt.Before(existing.record.StartedAt) {
			existing.record.StartedAt = startedAt
		}
		if updatedAt.After(existing.record.UpdatedAt) {
			existing.record.UpdatedAt = updatedAt
		}
		if entry.record.Principal != "" {
			existing.record.Principal = entry.record.Principal
		}
		if entry.record.Provider != "" {
			existing.record.Provider = entry.record.Provider
		}
		if entry.record.Model != "" {
			existing.record.Model = entry.record.Model
		}
		if entry.record.AuthID != "" {
			existing.record.AuthID = entry.record.AuthID
		}
		if entry.record.AuthIndex != "" {
			existing.record.AuthIndex = entry.record.AuthIndex
		}
		// Preserve tokens
		existing.record.InputTokens += entry.record.InputTokens
		existing.record.OutputTokens += entry.record.OutputTokens
		existing.record.ReasoningTokens += entry.record.ReasoningTokens
		existing.record.CachedTokens += entry.record.CachedTokens
		l.mu.Unlock()
		return nil
	}
	l.sessions[sessionID] = entry
	l.mu.Unlock()
	return nil
}

// HandleUsage intercepts usage records and accumulates token counts for the active session.
func (l *memoryExecutionSessionLedger) HandleUsage(ctx context.Context, record usage.Record) {
	if l == nil {
		return
	}
	sessionID := strings.TrimSpace(record.SessionID)
	if sessionID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.sessions[sessionID]
	if !ok || entry == nil {
		return
	}
	entry.record.InputTokens += record.Detail.InputTokens
	entry.record.OutputTokens += record.Detail.OutputTokens
	entry.record.ReasoningTokens += record.Detail.ReasoningTokens
	entry.record.CachedTokens += record.Detail.CachedTokens
}

func (l *memoryExecutionSessionLedger) FinalizeExecutionSession(_ context.Context, sessionID string, finishedAt time.Time) error {
	if l == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("execution session id is empty")
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.sessions[sessionID]
	if !ok || entry == nil {
		entry = &memoryExecutionSession{record: ExecutionSessionRecord{SessionID: sessionID, StartedAt: finishedAt, UpdatedAt: finishedAt}}
		l.sessions[sessionID] = entry
	}
	if entry.finalized {
		return nil
	}
	startedAt := entry.record.StartedAt
	if startedAt.IsZero() || finishedAt.Before(startedAt) {
		startedAt = finishedAt
		entry.record.StartedAt = startedAt
	}
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}
	entry.finishedAt = finishedAt
	entry.finalized = true
	entry.durationSecs = int64(duration / time.Second)
	entry.credits = CalculateTokenCost(entry.record.Model, entry.record.InputTokens, entry.record.OutputTokens, entry.record.ReasoningTokens, entry.record.CachedTokens)
	entry.record.UpdatedAt = finishedAt
	return nil
}

func (l *memoryExecutionSessionLedger) GetExecutionQuotaSummary(_ context.Context, principal string) (ExecutionQuotaSnapshot, error) {
	if l == nil {
		return ExecutionQuotaSnapshot{}, nil
	}
	principal = strings.TrimSpace(principal)
	
	now := time.Now().UTC()
	rollingCutoff := now.Add(-5 * time.Hour)
	
	l.mu.Lock()
	defer l.mu.Unlock()
	
	var summary ExecutionQuotaSnapshot
	summary.CreditLimit = 100 // Fallback limit, typically overridden by the API handler
	
	// Step 1: Find the first session in the current 5-hour block
	var windowStart time.Time
	for _, entry := range l.sessions {
		if entry == nil || strings.TrimSpace(entry.record.Principal) != principal {
			continue
		}
		
		candidate := entry.finishedAt
		if candidate.IsZero() { candidate = entry.record.UpdatedAt }
		if candidate.IsZero() { candidate = entry.record.StartedAt }
		
		if !candidate.Before(rollingCutoff) {
			if windowStart.IsZero() || candidate.Before(windowStart) {
				windowStart = candidate
			}
		}
	}
	
	// If there's no session in the last 5h, the window starts now
	if windowStart.IsZero() {
		windowStart = now
	}
	
	windowExpiresAt := windowStart.Add(5 * time.Hour)
	summary.WindowExpiresAt = windowExpiresAt
	
	// Step 2: Aggregate credits within this specific window
	for _, entry := range l.sessions {
		if entry == nil || strings.TrimSpace(entry.record.Principal) != principal {
			continue
		}
		
		candidate := entry.finishedAt
		if candidate.IsZero() { candidate = entry.record.UpdatedAt }
		if candidate.IsZero() { candidate = entry.record.StartedAt }
		
		summary.Sessions++
		if !entry.finalized {
			summary.ActiveSessions++
		}
		
		summary.TotalTokens += entry.record.InputTokens + entry.record.OutputTokens + entry.record.ReasoningTokens + entry.record.CachedTokens
		
		// Only aggregate credits if the session occurred within the determined window
		if !candidate.Before(windowStart) && candidate.Before(windowExpiresAt) {
			summary.CreditsUsed += entry.credits
			summary.DurationSeconds += entry.durationSecs
		}
		
		if candidate.After(summary.LastSessionAt) {
			summary.LastSessionAt = candidate
		}

		if !candidate.Before(windowStart) && candidate.Before(windowExpiresAt) {
			summary.RecentSessions = append(summary.RecentSessions, ExecutionSessionSummary{
				ExecutionSessionRecord: entry.record,
				Credits:                entry.credits,
				DurationSeconds:        entry.durationSecs,
			})
		}
	}
	
	// Sort by StartedAt descending
	if len(summary.RecentSessions) > 0 {
		for i := 0; i < len(summary.RecentSessions)-1; i++ {
			for j := i + 1; j < len(summary.RecentSessions); j++ {
				if summary.RecentSessions[j].StartedAt.After(summary.RecentSessions[i].StartedAt) {
					summary.RecentSessions[i], summary.RecentSessions[j] = summary.RecentSessions[j], summary.RecentSessions[i]
				}
			}
		}
		if len(summary.RecentSessions) > 20 {
			summary.RecentSessions = summary.RecentSessions[:20]
		}
	}
	
	return summary, nil
}

func apiKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ginCtx, ok := ctx.Value("gin").(*gin.Context)
	if !ok || ginCtx == nil {
		return ""
	}
	if v, exists := ginCtx.Get("apiKey"); exists {
		switch value := v.(type) {
		case string:
			return strings.TrimSpace(value)
		case fmt.Stringer:
			return strings.TrimSpace(value.String())
		default:
			return strings.TrimSpace(fmt.Sprintf("%v", value))
		}
	}
	return ""
}

// CreditsForDuration returns the credit bucket for a completed session.
func CreditsForDuration(d time.Duration) int {
	return 0 // Deprecated: use CalculateTokenCost instead
}

// CalculateTokenCost evaluates the cost in credits based on model alias pricing.
// Credit scale: 1,000 Credits = $1 USD  →  1 Credit = $0.001 USD
//
// Pricing table (USD per 1M tokens, mirrors real Anthropic/OpenAI rates):
//   claude-opus-*       → $15.00 in / $75.00 out / $3.75 cache
//   claude-sonnet-*     →  $3.00 in / $15.00 out / $0.30 cache
//   claude-haiku-*      →  $0.25 in /  $1.25 out / $0.03 cache
//   gpt-4o/gpt-4-turbo  →  $5.00 in / $15.00 out / $2.50 cache
//   gpt-4               → $30.00 in / $60.00 out / $15.00 cache
//   o1                  → $15.00 in / $60.00 out / $7.50 cache
//   gemini-1.5-pro      →  $3.50 in / $10.50 out / $0.88 cache
//   gemini-1.5-flash    →  $0.35 in /  $1.05 out / $0.08 cache
//   default (unknown)   →  $0.50 in /  $1.50 out / $0.20 cache
// pricingEntry holds virtual credit rates for a model (credits per 1M tokens).
type pricingEntry struct {
	input  float64
	output float64
	cache  float64
}

// globalPricing stores configurable per-model pricing, set at startup via SetModelPricing.
var (
	globalPricingMu      sync.RWMutex
	globalPricingMap     map[string]pricingEntry
	globalPricingLoaded  bool
)

// SetModelPricing registers per-model virtual credit rates from the application config.
// It should be called once at server startup after config is loaded.
// Each entry is [input, output, cache] credits per 1,000,000 tokens.
// Model names are matched case-insensitively (exact alias first, then keyword fallback).
func SetModelPricing(pricing map[string][3]float64) {
	globalPricingMu.Lock()
	defer globalPricingMu.Unlock()
	m := make(map[string]pricingEntry, len(pricing))
	for k, v := range pricing {
		m[strings.ToLower(strings.TrimSpace(k))] = pricingEntry{
			input:  v[0],
			output: v[1],
			cache:  v[2],
		}
	}
	globalPricingMap = m
	globalPricingLoaded = len(m) > 0
}

// defaultPricingFor returns built-in low-cost virtual credit rates as a fallback.
// Used only when model-pricing is not set in config.yaml.
// 1 credit ≈ 1,000 tokens (haiku rate), appropriate for a free Gemini CLI service.
func defaultPricingFor(modelLower string) pricingEntry {
	switch {
	case strings.Contains(modelLower, "opus"):
		return pricingEntry{input: 1.0, output: 3.0, cache: 0.10}
	case strings.Contains(modelLower, "sonnet"):
		return pricingEntry{input: 0.3, output: 1.0, cache: 0.03}
	case strings.Contains(modelLower, "haiku"):
		return pricingEntry{input: 0.15, output: 0.5, cache: 0.015}
	case strings.Contains(modelLower, "gpt-4o"), strings.Contains(modelLower, "gpt-4-turbo"):
		return pricingEntry{input: 0.5, output: 1.5, cache: 0.15}
	case strings.Contains(modelLower, "gpt-4"):
		return pricingEntry{input: 2.0, output: 4.0, cache: 1.0}
	case strings.Contains(modelLower, "o1"):
		return pricingEntry{input: 1.0, output: 4.0, cache: 0.5}
	case strings.Contains(modelLower, "gemini-1.5-pro"):
		return pricingEntry{input: 0.35, output: 1.05, cache: 0.09}
	case strings.Contains(modelLower, "gemini-1.5-flash"):
		return pricingEntry{input: 0.035, output: 0.1, cache: 0.008}
	default:
		return pricingEntry{input: 0.3, output: 1.0, cache: 0.03}
	}
}

// CalculateTokenCost returns the virtual credit cost for a request.
// It checks configurable model-pricing from config.yaml (exact match, then keyword fallback).
// Credits are quota units, NOT real USD — 1,000 credits ≈ $1 USD informational equivalent.
func CalculateTokenCost(model string, input, output, reasoning, cache int64) float64 {
	modelLower := strings.ToLower(strings.TrimSpace(model))

	var entry pricingEntry
	globalPricingMu.RLock()
	loaded := globalPricingLoaded
	if loaded {
		// Exact match first (e.g. "claude-opus-4-7")
		if e, ok := globalPricingMap[modelLower]; ok {
			entry = e
		} else {
			// Keyword fallback within configured keys (e.g. model contains "opus")
			for k, v := range globalPricingMap {
				if strings.Contains(modelLower, k) || strings.Contains(k, modelLower) {
					entry = v
					break
				}
			}
			if entry == (pricingEntry{}) {
				entry = defaultPricingFor(modelLower)
			}
		}
	}
	globalPricingMu.RUnlock()

	if !loaded {
		entry = defaultPricingFor(modelLower)
	}

	credits := (float64(input)*entry.input + float64(output+reasoning)*entry.output + float64(cache)*entry.cache) / 1_000_000.0
	return credits
}

func sessionIDFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata[cliproxyexecutor.ExecutionSessionMetadataKey]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}
