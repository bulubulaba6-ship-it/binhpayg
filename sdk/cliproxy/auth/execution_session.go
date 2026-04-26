package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

// ExecutionSessionRecord captures the mutable metadata tracked for a long-lived
// execution session while it is active.
type ExecutionSessionRecord struct {
	SessionID string
	Principal string
	Provider  string
	Model     string
	AuthID    string
	AuthIndex string
	StartedAt time.Time
	UpdatedAt time.Time
}

// ExecutionQuotaSnapshot summarizes the current session usage for one principal.
type ExecutionQuotaSnapshot struct {
	Sessions        int64     `json:"sessions"`
	ActiveSessions  int64     `json:"active_sessions"`
	CreditsUsed     int64     `json:"credits_used"`
	DurationSeconds int64     `json:"duration_seconds"`
	LastSessionAt   time.Time `json:"last_session_at,omitempty"`
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
	credits      int64
	durationSecs int64
}

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
		l.mu.Unlock()
		return nil
	}
	l.sessions[sessionID] = entry
	l.mu.Unlock()
	return nil
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
	entry.credits = int64(CreditsForDuration(duration))
	entry.record.UpdatedAt = finishedAt
	return nil
}

func (l *memoryExecutionSessionLedger) GetExecutionQuotaSummary(_ context.Context, principal string) (ExecutionQuotaSnapshot, error) {
	if l == nil {
		return ExecutionQuotaSnapshot{}, nil
	}
	principal = strings.TrimSpace(principal)
	l.mu.Lock()
	defer l.mu.Unlock()
	var summary ExecutionQuotaSnapshot
	for _, entry := range l.sessions {
		if entry == nil {
			continue
		}
		if strings.TrimSpace(entry.record.Principal) != principal {
			continue
		}
		summary.Sessions++
		if !entry.finalized {
			summary.ActiveSessions++
		}
		summary.CreditsUsed += entry.credits
		summary.DurationSeconds += entry.durationSecs
		candidate := entry.finishedAt
		if candidate.IsZero() {
			candidate = entry.record.UpdatedAt
		}
		if candidate.IsZero() {
			candidate = entry.record.StartedAt
		}
		if candidate.After(summary.LastSessionAt) {
			summary.LastSessionAt = candidate
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
func CreditsForDuration(duration time.Duration) int {
	if duration < 2*time.Minute {
		return 1
	}
	if duration < 7*time.Minute {
		return 2
	}
	return 3
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
