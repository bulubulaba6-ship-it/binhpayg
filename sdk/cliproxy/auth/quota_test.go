package auth

import (
	"context"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestLedger creates a fresh in-process ledger that is NOT registered with
// the global usage plugin chain, so tests are isolated.
func newTestLedger() *memoryExecutionSessionLedger {
	return &memoryExecutionSessionLedger{sessions: make(map[string]*memoryExecutionSession)}
}

func mustBeginSession(t *testing.T, l *memoryExecutionSessionLedger, rec ExecutionSessionRecord) {
	t.Helper()
	if err := l.BeginExecutionSession(context.Background(), rec); err != nil {
		t.Fatalf("BeginExecutionSession: %v", err)
	}
}

func mustFinalizeSession(t *testing.T, l *memoryExecutionSessionLedger, sessionID string, at time.Time) {
	t.Helper()
	if err := l.FinalizeExecutionSession(context.Background(), sessionID, at); err != nil {
		t.Fatalf("FinalizeExecutionSession: %v", err)
	}
}

func usageRecord(sessionID, model string, input, output int64) coreusage.Record {
	return coreusage.Record{
		SessionID: sessionID,
		Model:     model,
		Detail: coreusage.Detail{
			InputTokens:  input,
			OutputTokens: output,
		},
	}
}

// ---------------------------------------------------------------------------
// CalculateTokenCost
// ---------------------------------------------------------------------------

func TestCalculateTokenCost_HaikuPricing(t *testing.T) {
	// haiku: input=0.15, output=0.5 per 1M tokens (default)
	// 1,000,000 input + 1,000,000 output → 0.15 + 0.5 = 0.65 credits
	got := CalculateTokenCost("claude-haiku-4-5", 1_000_000, 1_000_000, 0, 0)
	want := 0.65
	if abs64(got-want) > 1e-9 {
		t.Errorf("haiku cost = %.10f, want %.10f", got, want)
	}
}

func TestCalculateTokenCost_OpusPricing(t *testing.T) {
	// opus: input=1.0, output=3.0 per 1M
	got := CalculateTokenCost("claude-opus-4-7", 1_000_000, 1_000_000, 0, 0)
	want := 4.0
	if abs64(got-want) > 1e-9 {
		t.Errorf("opus cost = %.10f, want %.10f", got, want)
	}
}

func TestCalculateTokenCost_Zero(t *testing.T) {
	got := CalculateTokenCost("claude-haiku-4-5", 0, 0, 0, 0)
	if got != 0 {
		t.Errorf("zero tokens should cost 0, got %f", got)
	}
}

func TestCalculateTokenCost_UnknownModelUsesDefault(t *testing.T) {
	// default: input=0.3, output=1.0 per 1M
	got := CalculateTokenCost("some-unknown-model", 1_000_000, 0, 0, 0)
	want := 0.3
	if abs64(got-want) > 1e-9 {
		t.Errorf("unknown model cost = %.10f, want %.10f", got, want)
	}
}

func TestCalculateTokenCost_ReasoningCountsAsOutput(t *testing.T) {
	// sonnet: output=1.0, reasoning is treated same as output
	// 1M reasoning tokens → 1.0 credit
	got := CalculateTokenCost("claude-sonnet-4-6", 0, 0, 1_000_000, 0)
	want := 1.0
	if abs64(got-want) > 1e-9 {
		t.Errorf("reasoning tokens cost = %.10f, want %.10f", got, want)
	}
}

// ---------------------------------------------------------------------------
// Session lifecycle: BeginExecutionSession
// ---------------------------------------------------------------------------

func TestBeginExecutionSession_EmptyIDReturnsError(t *testing.T) {
	l := newTestLedger()
	err := l.BeginExecutionSession(context.Background(), ExecutionSessionRecord{SessionID: ""})
	if err == nil {
		t.Fatal("expected error for empty session ID, got nil")
	}
}

func TestBeginExecutionSession_CreatesEntry(t *testing.T) {
	l := newTestLedger()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
	})
	if len(l.sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(l.sessions))
	}
}

func TestBeginExecutionSession_IdempotentForUnfinalized(t *testing.T) {
	l := newTestLedger()
	rec := ExecutionSessionRecord{SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5"}
	mustBeginSession(t, l, rec)
	mustBeginSession(t, l, rec) // call again — should not error or create a duplicate
	if len(l.sessions) != 1 {
		t.Fatalf("expected 1 session after duplicate begin, got %d", len(l.sessions))
	}
}

// ---------------------------------------------------------------------------
// Session lifecycle: FinalizeExecutionSession
// ---------------------------------------------------------------------------

func TestFinalizeExecutionSession_EmptyIDReturnsError(t *testing.T) {
	l := newTestLedger()
	if err := l.FinalizeExecutionSession(context.Background(), "", time.Now()); err == nil {
		t.Fatal("expected error for empty session ID")
	}
}

func TestFinalizeExecutionSession_SetsCredits(t *testing.T) {
	l := newTestLedger()
	now := time.Now().UTC()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
	})
	// Tokens arrive via HandleUsage (production flow)
	l.HandleUsage(context.Background(), usageRecord("s1", "claude-haiku-4-5", 1_000_000, 1_000_000))
	mustFinalizeSession(t, l, "s1", now)

	entry := l.sessions["s1"]
	if !entry.finalized {
		t.Fatal("session should be finalized")
	}
	// haiku: 0.15 in + 0.5 out = 0.65 per 1M
	want := 0.65
	if abs64(entry.credits-want) > 1e-9 {
		t.Errorf("finalized credits = %.10f, want %.10f", entry.credits, want)
	}
}

func TestFinalizeExecutionSession_CalculatesDuration(t *testing.T) {
	l := newTestLedger()
	start := time.Now().UTC().Add(-10 * time.Minute)
	finish := start.Add(10 * time.Minute)
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5", StartedAt: start,
	})
	mustFinalizeSession(t, l, "s1", finish)

	if l.sessions["s1"].durationSecs != 600 {
		t.Errorf("duration = %d, want 600", l.sessions["s1"].durationSecs)
	}
}

// ---------------------------------------------------------------------------
// HandleUsage: real-time credit accumulation
// ---------------------------------------------------------------------------

func TestHandleUsage_AccumulatesCreditsForActiveSession(t *testing.T) {
	l := newTestLedger()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
	})

	l.HandleUsage(context.Background(), usageRecord("s1", "claude-haiku-4-5", 500_000, 500_000))

	entry := l.sessions["s1"]
	if entry.record.InputTokens != 500_000 {
		t.Errorf("input tokens = %d, want 500000", entry.record.InputTokens)
	}
	// credits should be > 0 immediately (not waiting for finalize)
	if entry.credits == 0 {
		t.Error("credits should be > 0 after HandleUsage, got 0")
	}
}

func TestHandleUsage_RefreshesUpdatedAt(t *testing.T) {
	l := newTestLedger()
	before := time.Now().UTC()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
		StartedAt: before.Add(-1 * time.Hour), // session started 1h ago
	})

	time.Sleep(2 * time.Millisecond) // ensure wall clock advances
	l.HandleUsage(context.Background(), usageRecord("s1", "claude-haiku-4-5", 100, 100))

	updatedAt := l.sessions["s1"].record.UpdatedAt
	if !updatedAt.After(before) {
		t.Errorf("UpdatedAt (%v) should be after test start (%v)", updatedAt, before)
	}
}

func TestHandleUsage_NoOpForMissingSessionID(t *testing.T) {
	l := newTestLedger()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
	})
	// emit usage with empty session ID — should not panic or affect s1
	l.HandleUsage(context.Background(), usageRecord("", "claude-haiku-4-5", 999_999, 999_999))

	if l.sessions["s1"].record.InputTokens != 0 {
		t.Error("session s1 should not be affected by usage with empty session ID")
	}
}

func TestHandleUsage_NoOpForUnknownSessionID(t *testing.T) {
	l := newTestLedger()
	// No sessions created — should not panic
	l.HandleUsage(context.Background(), usageRecord("ghost-session", "claude-haiku-4-5", 1000, 1000))
}

// ---------------------------------------------------------------------------
// GetExecutionQuotaSummary: window aggregation
// ---------------------------------------------------------------------------

func TestGetExecutionQuotaSummary_Empty(t *testing.T) {
	l := newTestLedger()
	snap, err := l.GetExecutionQuotaSummary(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Sessions != 0 {
		t.Errorf("Sessions = %d, want 0", snap.Sessions)
	}
	if snap.Interval.Credits != 0 {
		t.Errorf("Interval.Credits = %f, want 0", snap.Interval.Credits)
	}
}

func TestGetExecutionQuotaSummary_SingleFinalizedSession_AllWindows(t *testing.T) {
	l := newTestLedger()
	now := time.Now().UTC()

	// Session 30 minutes ago — should appear in 5h, daily, monthly, lifetime
	start := now.Add(-30 * time.Minute)
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
		StartedAt: start,
	})
	// Inject tokens via HandleUsage (production flow)
	l.HandleUsage(context.Background(), usageRecord("s1", "claude-haiku-4-5", 1_000_000, 1_000_000))
	mustFinalizeSession(t, l, "s1", now.Add(-1*time.Minute))

	snap, err := l.GetExecutionQuotaSummary(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.Sessions != 1 {
		t.Errorf("Sessions = %d, want 1", snap.Sessions)
	}
	if snap.ActiveSessions != 0 {
		t.Errorf("ActiveSessions = %d, want 0", snap.ActiveSessions)
	}
	// Credits must be > 0 in all windows
	if snap.Interval.Credits == 0 {
		t.Error("Interval.Credits should be > 0")
	}
	if snap.Daily.Credits == 0 {
		t.Error("Daily.Credits should be > 0")
	}
	if snap.Monthly.Credits == 0 {
		t.Error("Monthly.Credits should be > 0")
	}
	if snap.Lifetime.Credits == 0 {
		t.Error("Lifetime.Credits should be > 0")
	}
	// Backward compatibility aliases
	if snap.CreditsUsed != snap.Interval.Credits {
		t.Errorf("CreditsUsed (%f) != Interval.Credits (%f)", snap.CreditsUsed, snap.Interval.Credits)
	}
	if snap.TotalCreditsUsed != snap.Lifetime.Credits {
		t.Errorf("TotalCreditsUsed (%f) != Lifetime.Credits (%f)", snap.TotalCreditsUsed, snap.Lifetime.Credits)
	}
}

func TestGetExecutionQuotaSummary_OldSession_NotInInterval(t *testing.T) {
	l := newTestLedger()
	now := time.Now().UTC()

	// Session that finished 6h ago — beyond the 5h rolling window
	old := now.Add(-6 * time.Hour)
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "old", Principal: "user-a", Model: "claude-haiku-4-5",
		StartedAt: old,
	})
	// Inject tokens, then finalize with a timestamp from 6h ago
	l.HandleUsage(context.Background(), usageRecord("old", "claude-haiku-4-5", 1_000_000, 1_000_000))
	// Manually backdate UpdatedAt so the session falls outside the 5h window
	l.sessions["old"].record.UpdatedAt = old
	mustFinalizeSession(t, l, "old", old.Add(1*time.Minute))

	snap, _ := l.GetExecutionQuotaSummary(context.Background(), "user-a")

	if snap.Interval.Credits != 0 {
		t.Errorf("Interval.Credits = %f for 6h-old session, want 0", snap.Interval.Credits)
	}
	if snap.Lifetime.Credits == 0 {
		t.Error("Lifetime.Credits should still count old sessions")
	}
}

func TestGetExecutionQuotaSummary_ActiveSession_VisibleInInterval(t *testing.T) {
	l := newTestLedger()

	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "active", Principal: "user-a", Model: "claude-haiku-4-5",
	})
	// Inject usage — this updates UpdatedAt and credits
	l.HandleUsage(context.Background(), usageRecord("active", "claude-haiku-4-5", 1_000_000, 1_000_000))

	snap, _ := l.GetExecutionQuotaSummary(context.Background(), "user-a")

	if snap.ActiveSessions != 1 {
		t.Errorf("ActiveSessions = %d, want 1", snap.ActiveSessions)
	}
	if snap.Interval.Credits == 0 {
		t.Error("active session credits should appear in Interval window")
	}
}

func TestGetExecutionQuotaSummary_OnlyMatchesPrincipal(t *testing.T) {
	l := newTestLedger()
	now := time.Now().UTC()

	// session belonging to user-a (haiku)
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "sa", Principal: "user-a", Model: "claude-haiku-4-5",
	})
	l.HandleUsage(context.Background(), usageRecord("sa", "claude-haiku-4-5", 1_000_000, 1_000_000))
	mustFinalizeSession(t, l, "sa", now)

	// session belonging to user-b (opus — much more expensive)
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "sb", Principal: "user-b", Model: "claude-opus-4-7",
	})
	l.HandleUsage(context.Background(), usageRecord("sb", "claude-opus-4-7", 1_000_000, 1_000_000))
	mustFinalizeSession(t, l, "sb", now)

	snapA, _ := l.GetExecutionQuotaSummary(context.Background(), "user-a")
	snapB, _ := l.GetExecutionQuotaSummary(context.Background(), "user-b")

	// The two snapshots should be independent
	if snapA.Sessions != 1 {
		t.Errorf("user-a sessions = %d, want 1", snapA.Sessions)
	}
	if snapB.Sessions != 1 {
		t.Errorf("user-b sessions = %d, want 1", snapB.Sessions)
	}
	// opus is more expensive than haiku
	if snapB.Lifetime.Credits <= snapA.Lifetime.Credits {
		t.Errorf("user-b (opus) credits (%f) should be > user-a (haiku) credits (%f)",
			snapB.Lifetime.Credits, snapA.Lifetime.Credits)
	}
}

func TestGetExecutionQuotaSummary_RecentSessionsSortedDescending(t *testing.T) {
	l := newTestLedger()
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		start := now.Add(-time.Duration(i+1) * time.Minute)
		mustBeginSession(t, l, ExecutionSessionRecord{
			SessionID: id, Principal: "user-a", Model: "claude-haiku-4-5",
			StartedAt: start, InputTokens: 100, OutputTokens: 100,
		})
		mustFinalizeSession(t, l, id, start.Add(30*time.Second))
	}

	snap, _ := l.GetExecutionQuotaSummary(context.Background(), "user-a")
	if len(snap.RecentSessions) < 2 {
		t.Fatalf("expected ≥2 recent sessions, got %d", len(snap.RecentSessions))
	}
	for i := 1; i < len(snap.RecentSessions); i++ {
		if snap.RecentSessions[i].StartedAt.After(snap.RecentSessions[i-1].StartedAt) {
			t.Errorf("RecentSessions not sorted descending: [%d].StartedAt=%v > [%d].StartedAt=%v",
				i, snap.RecentSessions[i].StartedAt, i-1, snap.RecentSessions[i-1].StartedAt)
		}
	}
}

func TestGetExecutionQuotaSummary_RecentSessionsCappedAt20(t *testing.T) {
	l := newTestLedger()
	now := time.Now().UTC()

	for i := 0; i < 25; i++ {
		id := string(rune('A' + i % 26)) + string(rune('0' + i % 10))
		start := now.Add(-time.Duration(i+1) * time.Minute)
		mustBeginSession(t, l, ExecutionSessionRecord{
			SessionID: id, Principal: "user-a", Model: "claude-haiku-4-5",
			StartedAt: start, InputTokens: 100, OutputTokens: 100,
		})
		mustFinalizeSession(t, l, id, start.Add(30*time.Second))
	}

	snap, _ := l.GetExecutionQuotaSummary(context.Background(), "user-a")
	if len(snap.RecentSessions) > 20 {
		t.Errorf("RecentSessions capped at 20, got %d", len(snap.RecentSessions))
	}
}

func TestGetExecutionQuotaSummary_ThroughputNonNegative(t *testing.T) {
	l := newTestLedger()
	snap, _ := l.GetExecutionQuotaSummary(context.Background(), "nobody")
	if snap.Interval.Throughput < 0 || snap.Daily.Throughput < 0 {
		t.Error("throughput should never be negative")
	}
}

func TestGetExecutionQuotaSummary_WindowExpiresAtIsInFuture(t *testing.T) {
	l := newTestLedger()
	snap, _ := l.GetExecutionQuotaSummary(context.Background(), "user-a")
	if !snap.WindowExpiresAt.After(time.Now().UTC()) {
		t.Errorf("WindowExpiresAt (%v) should be in the future", snap.WindowExpiresAt)
	}
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestConcurrentHandleUsage_DoesNotPanic(t *testing.T) {
	l := newTestLedger()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "concurrent", Principal: "user-a", Model: "claude-haiku-4-5",
	})

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			l.HandleUsage(context.Background(), usageRecord("concurrent", "claude-haiku-4-5", 1000, 1000))
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	// No race or panic = pass
}

func TestConcurrentGetSummary_DoesNotPanic(t *testing.T) {
	l := newTestLedger()
	mustBeginSession(t, l, ExecutionSessionRecord{
		SessionID: "s1", Principal: "user-a", Model: "claude-haiku-4-5",
	})

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			l.GetExecutionQuotaSummary(context.Background(), "user-a") //nolint:errcheck
			done <- struct{}{}
		}()
		go func() {
			l.HandleUsage(context.Background(), usageRecord("s1", "claude-haiku-4-5", 100, 100))
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
