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
	SessionID       string
	Principal       string
	Provider        string
	Model           string
	AuthID          string
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

// WindowSnapshot summarizes usage metrics for a specific time horizon.
type WindowSnapshot struct {
	Credits    float64 `json:"credits"`
	Requests   int64   `json:"requests"`
	Tokens     int64   `json:"tokens"`
	Duration   int64   `json:"duration_seconds"`
	Throughput float64 `json:"throughput_tpm"` // Tokens Per Minute (TPM) in this window
}

// ExecutionQuotaSnapshot summarizes the current session usage for one principal.
type ExecutionQuotaSnapshot struct {
	Sessions       int64 `json:"sessions"`
	ActiveSessions int64 `json:"active_sessions"`

	// CreditsUsed and TotalCreditsUsed are preserved for backward compatibility
	// (mapped to Interval and Lifetime respectively).
	CreditsUsed      float64 `json:"credits_used"`
	TotalCreditsUsed float64 `json:"total_credits_used"`

	CreditLimit     int64     `json:"credit_limit"` // 1,000 Credits = $1 USD
	LastSessionAt   time.Time `json:"last_session_at,omitempty"`
	WindowExpiresAt time.Time `json:"window_expires_at,omitempty"`

	// Window-based aggregates
	Interval WindowSnapshot `json:"interval"` // 5-hour rolling window
	Daily    WindowSnapshot `json:"daily"`    // 24-hour window
	Monthly  WindowSnapshot `json:"monthly"`  // 30-day window
	Lifetime WindowSnapshot `json:"lifetime"` // All-time window

	RecentSessions []ExecutionSessionSummary `json:"recent_sessions,omitempty"`
}

// ExecutionSessionLedger stores session lifecycle events durably.
type ExecutionSessionLedger interface {
	BeginExecutionSession(ctx context.Context, record ExecutionSessionRecord) error
	FinalizeExecutionSession(ctx context.Context, sessionID string, finishedAt time.Time) error
	GetExecutionQuotaSummary(ctx context.Context, principal string) (ExecutionQuotaSnapshot, error)
	HandleUsage(ctx context.Context, record usage.Record)
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

// NewMemoryExecutionSessionLedger creates a standalone in-memory ledger and
// registers it directly with the global usage dispatcher. Use this when you
// are not going through Manager.SetExecutionSessionLedger.
func NewMemoryExecutionSessionLedger() ExecutionSessionLedger {
	ledger := newMemoryExecutionSessionLedger()
	usage.RegisterPlugin(ledger)
	return ledger
}

// newMemoryExecutionSessionLedger creates the ledger without registering it
// as a usage plugin. Registration is handled by Manager.SetExecutionSessionLedger
// so that the correct (possibly overridden) instance receives usage events.
func newMemoryExecutionSessionLedger() *memoryExecutionSessionLedger {
	return &memoryExecutionSessionLedger{sessions: make(map[string]*memoryExecutionSession)}
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
	// Refresh UpdatedAt so the session is visible in rolling window queries.
	entry.record.UpdatedAt = time.Now().UTC()

	// Recalculate credits even for in-progress sessions to ensure dashboard visibility.
	entry.credits = CalculateTokenCost(entry.record.Model, entry.record.InputTokens, entry.record.OutputTokens, entry.record.ReasoningTokens, entry.record.CachedTokens)
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
	rolling5h := now.Add(-5 * time.Hour)
	rolling1d := now.Add(-24 * time.Hour)
	rolling30d := now.Add(-30 * 24 * time.Hour)

	l.mu.Lock()
	defer l.mu.Unlock()

	var summary ExecutionQuotaSnapshot
	summary.CreditLimit = 100                        // Fallback limit
	summary.WindowExpiresAt = now.Add(5 * time.Hour) // window always extends from now

	for _, entry := range l.sessions {
		if entry == nil || strings.TrimSpace(entry.record.Principal) != principal {
			continue
		}

		candidate := entry.finishedAt
		if candidate.IsZero() {
			candidate = entry.record.UpdatedAt
		}
		if candidate.IsZero() {
			candidate = entry.record.StartedAt
		}

		tokens := entry.record.InputTokens + entry.record.OutputTokens + entry.record.ReasoningTokens + entry.record.CachedTokens

		summary.Sessions++
		if !entry.finalized {
			summary.ActiveSessions++
		}

		// Aggregate Lifetime stats
		summary.Lifetime.Requests++
		summary.Lifetime.Credits += entry.credits
		summary.Lifetime.Tokens += tokens
		summary.Lifetime.Duration += entry.durationSecs

		// Aggregate Monthly stats
		if !candidate.Before(rolling30d) {
			summary.Monthly.Requests++
			summary.Monthly.Credits += entry.credits
			summary.Monthly.Tokens += tokens
			summary.Monthly.Duration += entry.durationSecs
		}

		// Aggregate Daily stats
		if !candidate.Before(rolling1d) {
			summary.Daily.Requests++
			summary.Daily.Credits += entry.credits
			summary.Daily.Tokens += tokens
			summary.Daily.Duration += entry.durationSecs
		}

		// Aggregate 5h Interval stats (simple rolling window from now-5h)
		if !candidate.Before(rolling5h) {
			summary.Interval.Requests++
			summary.Interval.Credits += entry.credits
			summary.Interval.Tokens += tokens
			summary.Interval.Duration += entry.durationSecs

			summary.RecentSessions = append(summary.RecentSessions, ExecutionSessionSummary{
				ExecutionSessionRecord: entry.record,
				Credits:                entry.credits,
				DurationSeconds:        entry.durationSecs,
			})
		}

		if candidate.After(summary.LastSessionAt) {
			summary.LastSessionAt = candidate
		}
	}

	// Calculate Throughput (TPM) for each window
	// Interval: 5h (300 min)
	summary.Interval.Throughput = float64(summary.Interval.Tokens) / 300.0
	// Daily: 24h (1440 min)
	summary.Daily.Throughput = float64(summary.Daily.Tokens) / 1440.0
	// Monthly: 30d (43200 min)
	summary.Monthly.Throughput = float64(summary.Monthly.Tokens) / 43200.0
	// Lifetime: since start (estimated as 30 days if no data)
	summary.Lifetime.Throughput = float64(summary.Lifetime.Tokens) / 43200.0

	// Backward compatibility mappings
	summary.CreditsUsed = summary.Interval.Credits
	summary.TotalCreditsUsed = summary.Lifetime.Credits

	// Sort recent sessions by StartedAt descending
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
//
//	claude-opus-*       → $15.00 in / $75.00 out / $3.75 cache
//	claude-sonnet-*     →  $3.00 in / $15.00 out / $0.30 cache
//	claude-haiku-*      →  $0.25 in /  $1.25 out / $0.03 cache
//	gpt-4o/gpt-4-turbo  →  $5.00 in / $15.00 out / $2.50 cache
//	gpt-4               → $30.00 in / $60.00 out / $15.00 cache
//	o1                  → $15.00 in / $60.00 out / $7.50 cache
//	gemini-1.5-pro      →  $3.50 in / $10.50 out / $0.88 cache
//	gemini-1.5-flash    →  $0.35 in /  $1.05 out / $0.08 cache
//	default (unknown)   →  $0.50 in /  $1.50 out / $0.20 cache
//
// pricingEntry holds virtual credit rates for a model (credits per 1M tokens).
type pricingEntry struct {
	input  float64
	output float64
	cache  float64
}

// globalPricing stores configurable per-model pricing, set at startup via SetModelPricing.
var (
	globalPricingMu     sync.RWMutex
	globalPricingMap    map[string]pricingEntry
	globalPricingLoaded bool
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
