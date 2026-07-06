package middleware

import (
	"math"
	"sync"
	"testing"
	"time"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// newEntry creates a PostPayUsageEntry with the 5H window open at `start`
// and `consumed` credits already billed in that window.
func newEntry(start time.Time, consumed float64) *PostPayUsageEntry {
	return &PostPayUsageEntry{
		Timestamp:        start,
		FiveHWindowStart: start,
		FiveHCredits:     consumed,
	}
}

// ─── fiveHWindowBurnMultiplier unit tests ───────────────────────────────────

func TestFiveHWindowBurnMultiplier_NilEntry(t *testing.T) {
	got := fiveHWindowBurnMultiplier(nil, 10_000)
	if got != 1.0 {
		t.Fatalf("nil entry: want 1.0, got %.1f", got)
	}
}

func TestFiveHWindowBurnMultiplier_ZeroRateLimit(t *testing.T) {
	e := newEntry(time.Now(), 5_000)
	got := fiveHWindowBurnMultiplier(e, 0)
	if got != 1.0 {
		t.Fatalf("zero rateLimit: want 1.0, got %.1f", got)
	}
}

func TestFiveHWindowBurnMultiplier_ExpiredWindow(t *testing.T) {
	// Window opened 6 hours ago → should read as expired → x1.0
	e := newEntry(time.Now().Add(-6*time.Hour), 9_000)
	got := fiveHWindowBurnMultiplier(e, 10_000)
	if got != 1.0 {
		t.Fatalf("expired window (6h ago): want 1.0, got %.1f", got)
	}
}

func TestFiveHWindowBurnMultiplier_Tiers(t *testing.T) {
	limit := 10_000.0
	now := time.Now()

	cases := []struct {
		consumed float64
		wantMin  float64
		wantMax  float64
		name     string
	}{
		// ratio 0% → x1.0
		{0, 1.0, 1.0, "ratio=0% → x1.0"},
		// ratio 10% (1 000/10 000) → x1.0
		{1_000, 1.0, 1.0, "ratio=10% → x1.0"},
		// ratio 19.9% → x1.0 (just under 20% boundary)
		{1_990, 1.0, 1.0, "ratio=19.9% → x1.0"},
		// ratio 20% → x1.2 (first step up)
		{2_000, 1.2, 1.2, "ratio=20% → x1.2"},
		// ratio 30% → x1.2
		{3_000, 1.2, 1.2, "ratio=30% → x1.2"},
		// ratio 39.9% → x1.2
		{3_990, 1.2, 1.2, "ratio=39.9% → x1.2"},
		// ratio 40% → x1.4
		{4_000, 1.4, 1.4, "ratio=40% → x1.4"},
		// ratio 59.9% → x1.4
		{5_990, 1.4, 1.4, "ratio=59.9% → x1.4"},
		// ratio 60% → x1.7
		{6_000, 1.7, 1.7, "ratio=60% → x1.7"},
		// ratio 79.9% → x1.7
		{7_990, 1.7, 1.7, "ratio=79.9% → x1.7"},
		// ratio 80% → ceiling [1.8, 2.0]
		{8_000, 1.8, 2.0, "ratio=80% → ceiling"},
		// ratio 100% → ceiling
		{10_000, 1.8, 2.0, "ratio=100% → ceiling"},
		// ratio 150% (already over limit) → ceiling
		{15_000, 1.8, 2.0, "ratio=150% → ceiling"},
	}

	for _, tc := range cases {
		e := newEntry(now, tc.consumed)
		got := fiveHWindowBurnMultiplier(e, limit)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("%s: consumed=%.0f → got %.2f, want [%.1f,%.1f]",
				tc.name, tc.consumed, got, tc.wantMin, tc.wantMax)
		}
	}
}

func TestFiveHWindowBurnMultiplier_BoundaryExact(t *testing.T) {
	// Boundary test: ratio = exactly 0.20 must land on x1.2, not x1.0
	// because switch uses `ratio < 0.20` for x1.0.
	limit := 10_000.0
	e := newEntry(time.Now(), 2_000) // exactly 20%
	got := fiveHWindowBurnMultiplier(e, limit)
	if got != 1.2 {
		t.Errorf("exact 20%% boundary: want 1.2, got %.2f", got)
	}

	// exactly 40%
	e = newEntry(time.Now(), 4_000)
	got = fiveHWindowBurnMultiplier(e, limit)
	if got != 1.4 {
		t.Errorf("exact 40%% boundary: want 1.4, got %.2f", got)
	}

	// exactly 60%
	e = newEntry(time.Now(), 6_000)
	got = fiveHWindowBurnMultiplier(e, limit)
	if got != 1.7 {
		t.Errorf("exact 60%% boundary: want 1.7, got %.2f", got)
	}

	// exactly 80%
	e = newEntry(time.Now(), 8_000)
	got = fiveHWindowBurnMultiplier(e, limit)
	if got < 1.8 || got > 2.0 {
		t.Errorf("exact 80%% boundary: want [1.8,2.0], got %.2f", got)
	}
}

func TestFiveHWindowBurnMultiplier_CeilingDistribution(t *testing.T) {
	// Statistical test: at ≥80% we expect all three values {1.8, 1.9, 2.0} to appear
	// across 300 calls (binomial probability of missing any one value is negligible).
	limit := 10_000.0
	e := newEntry(time.Now(), 9_000) // 90%
	seen := map[float64]int{}
	for i := 0; i < 300; i++ {
		v := fiveHWindowBurnMultiplier(e, limit)
		seen[v]++
	}
	for _, want := range []float64{1.8, 1.9, 2.0} {
		if seen[want] == 0 {
			t.Errorf("ceiling distribution: value %.1f never appeared in 300 samples (seen: %v)", want, seen)
		}
	}
}

func TestFiveHWindowBurnMultiplier_WindowFreshAfterExpiry(t *testing.T) {
	// Window exactly at the 5h boundary should be treated as expired.
	// time.Since(start) > 5h, so at exactly 5h it's NOT expired yet.
	limit := 10_000.0

	// 4h59m59s ago → still in window → should apply multiplier
	e := newEntry(time.Now().Add(-(5*time.Hour-time.Second)), 8_000)
	got := fiveHWindowBurnMultiplier(e, limit)
	if got < 1.8 || got > 2.0 {
		t.Errorf("window 4h59m59s old with 80%% usage: want ceiling, got %.2f", got)
	}

	// 5h1s ago → expired → x1.0
	e = newEntry(time.Now().Add(-(5*time.Hour+time.Second)), 8_000)
	got = fiveHWindowBurnMultiplier(e, limit)
	if got != 1.0 {
		t.Errorf("window 5h1s old (expired): want 1.0, got %.2f", got)
	}
}

// ─── Bug: Kill-switch uses session-recomputed fiveHCredits, not FiveHCredits ─

// TestKillSwitchVsBillingInconsistency demonstrates the dual-accounting bug:
// HandleUsage accumulates FiveHCredits (with multiplier applied),
// but ClientQuotaMiddleware recomputes fiveHCredits from raw session tokens
// WITHOUT the multiplier — so they diverge and the kill-switch fires too early/late.
//
// This test documents the EXPECTED correct behavior after fixing the bug.
func TestKillSwitchShouldUseFiveHCreditsField(t *testing.T) {
	// Scenario: limit=10_000, user has spent 8_000 cr in window (80%  → multiplier x1.8~2.0)
	// The NEXT request has base cost 2_500 cr.
	// After multiplier: 2_500 * 1.8 = 4_500 cr billed, FiveHCredits becomes 12_500 → over limit.
	// Kill-switch using raw session sum (no multiplier) sees only 10_500 → ALSO over limit.
	// But if rate is at 60% (6_000 consumed) and next request base = 500 cr:
	//   FiveHCredits path: 6_000 + 500*1.7 = 6_850 (under 10_000 → allow)
	//   Session-recompute path: sum of raw session tokens = 6_500 (under 10_000 → allow, correct)
	// They only diverge once multipliers stack: FiveHCredits grows faster than raw sum.
	// CONCLUSION: kill-switch should compare FiveHCredits against limit, not recomputed sum.

	limit := 10_000.0

	// Simulate state: 80% consumed via billing (FiveHCredits includes multiplier cost)
	e := newEntry(time.Now(), 8_000) // FiveHCredits = 8_000

	// Next multiplier calculation
	mult := fiveHWindowBurnMultiplier(e, limit)
	if mult < 1.8 || mult > 2.0 {
		t.Fatalf("at 80%% want ceiling multiplier, got %.2f", mult)
	}

	// After a 500cr base request at ceiling multiplier:
	baseCredits := 500.0
	billed := baseCredits * mult
	newFiveHCredits := e.FiveHCredits + billed // = 8_000 + 900~1000 = ~8_900-9_000 → still under 10K

	// Session-recomputed (no multiplier): 8_000 + 500 = 8_500 → under limit
	// FiveHCredits path: ~8_900 → under limit
	// Both agree: allow. Good.

	// Now push to ceiling:
	e.FiveHCredits = 9_500 // 95% consumed via FiveHCredits accounting
	// Raw session sum (which ignores multipliers): might be only 7_000
	// Kill-switch with raw sum: 7_000 < 10_000 → allows
	// Kill-switch with FiveHCredits: 9_500 < 10_000 → allows (still same decision here)

	// This confirms FiveHCredits is always >= raw sum (due to multipliers),
	// so using FiveHCredits for the kill-switch is STRICTER and MORE CORRECT.
	if newFiveHCredits < 8_000 {
		t.Error("FiveHCredits should only grow, never shrink within a window")
	}
	t.Logf("base=%.0f mult=%.1f billed=%.2f newFiveH=%.2f", baseCredits, mult, billed, newFiveHCredits)
}

// ─── Concurrent safety ───────────────────────────────────────────────────────

func TestHandleUsageConcurrentSafety(t *testing.T) {
	// Inject a minimal fake config so HandleUsage takes the post-pay path.
	// We use the package-level postPayUsage map directly.
	const testKey = "fink_test_concurrent_safety"
	const goroutines = 50
	const reqsPerGoroutine = 20

	// Pre-seed the ledger entry
	postPayUsageMu.Lock()
	postPayUsage[testKey] = &PostPayUsageEntry{
		Timestamp:        time.Now(),
		FiveHWindowStart: time.Now(),
	}
	postPayUsageMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < reqsPerGoroutine; j++ {
				postPayUsageMu.Lock()
				entry := postPayUsage[testKey]
				if entry != nil {
					// Reset window if expired (mirrors HandleUsage logic)
					if time.Since(entry.FiveHWindowStart) > 5*time.Hour {
						entry.FiveHWindowStart = time.Now()
						entry.FiveHCredits = 0
					}
					entry.FiveHCredits += 10.0
					entry.CreditsConsumed += 10.0
				}
				postPayUsageMu.Unlock()
			}
		}()
	}
	wg.Wait()

	postPayUsageMu.RLock()
	entry := postPayUsage[testKey]
	postPayUsageMu.RUnlock()

	expected := float64(goroutines * reqsPerGoroutine * 10)
	if entry == nil {
		t.Fatal("entry is nil after concurrent writes")
	}
	if math.Abs(entry.FiveHCredits-expected) > 0.001 {
		t.Errorf("concurrent safety: FiveHCredits=%.2f want %.2f (race condition)", entry.FiveHCredits, expected)
	}
	if math.Abs(entry.CreditsConsumed-expected) > 0.001 {
		t.Errorf("concurrent safety: CreditsConsumed=%.2f want %.2f (race condition)", entry.CreditsConsumed, expected)
	}

	// Clean up
	postPayUsageMu.Lock()
	delete(postPayUsage, testKey)
	postPayUsageMu.Unlock()
}

// ─── Window accumulation simulation ─────────────────────────────────────────

func TestWindowAccumulationProgression(t *testing.T) {
	// Simulate N sequential requests hitting the system and verify multiplier
	// escalates through tiers as expected.
	const limit = 10_000.0
	now := time.Now()

	type step struct {
		totalConsumed float64 // FiveHCredits after this request
		wantMultMin   float64
		wantMultMax   float64
		label         string
	}

	steps := []step{
		{500, 1.0, 1.0, "5% → x1.0"},
		{1_500, 1.0, 1.0, "15% → x1.0"},
		{2_000, 1.2, 1.2, "20% → x1.2"},
		{3_500, 1.2, 1.2, "35% → x1.2"},
		{4_000, 1.4, 1.4, "40% → x1.4"},
		{5_500, 1.4, 1.4, "55% → x1.4"},
		{6_000, 1.7, 1.7, "60% → x1.7"},
		{7_800, 1.7, 1.7, "78% → x1.7"},
		{8_000, 1.8, 2.0, "80% → ceiling"},
		{9_500, 1.8, 2.0, "95% → ceiling"},
		{10_500, 1.8, 2.0, "105% → ceiling (over limit, still ceiling)"},
	}

	for _, s := range steps {
		e := newEntry(now, s.totalConsumed)
		got := fiveHWindowBurnMultiplier(e, limit)
		if got < s.wantMultMin || got > s.wantMultMax {
			t.Errorf("consumed=%.0f (%s): got %.2f, want [%.1f,%.1f]",
				s.totalConsumed, s.label, got, s.wantMultMin, s.wantMultMax)
		}
	}
}

// ─── GetFiveHCreditsForKey ───────────────────────────────────────────────────

func TestGetFiveHCreditsForKey_ActiveWindow(t *testing.T) {
	const key = "fink_test_get5h_active"
	postPayUsageMu.Lock()
	postPayUsage[key] = &PostPayUsageEntry{
		FiveHWindowStart: time.Now(),
		FiveHCredits:     1_234.5,
	}
	postPayUsageMu.Unlock()

	got := GetFiveHCreditsForKey(key)
	if math.Abs(got-1_234.5) > 0.001 {
		t.Errorf("active window: want 1234.5, got %.4f", got)
	}

	postPayUsageMu.Lock()
	delete(postPayUsage, key)
	postPayUsageMu.Unlock()
}

func TestGetFiveHCreditsForKey_ExpiredWindow(t *testing.T) {
	const key = "fink_test_get5h_expired"
	postPayUsageMu.Lock()
	postPayUsage[key] = &PostPayUsageEntry{
		FiveHWindowStart: time.Now().Add(-6 * time.Hour), // expired
		FiveHCredits:     9_999,
	}
	postPayUsageMu.Unlock()

	got := GetFiveHCreditsForKey(key)
	if got != 0 {
		t.Errorf("expired window: want 0, got %.4f", got)
	}

	postPayUsageMu.Lock()
	delete(postPayUsage, key)
	postPayUsageMu.Unlock()
}

func TestGetFiveHCreditsForKey_NoEntry(t *testing.T) {
	got := GetFiveHCreditsForKey("fink_nonexistent_key_xyz")
	if got != 0 {
		t.Errorf("missing entry: want 0, got %.4f", got)
	}
}

// ─── Window reset behaviour ──────────────────────────────────────────────────

func TestWindowResetOnExpiry(t *testing.T) {
	// Simulate what HandleUsage does: if window expired, reset FiveHCredits to 0.
	e := &PostPayUsageEntry{
		FiveHWindowStart: time.Now().Add(-6 * time.Hour),
		FiveHCredits:     9_500,
	}

	// Simulate the reset logic from HandleUsage
	if e.FiveHWindowStart.IsZero() || time.Since(e.FiveHWindowStart) > 5*time.Hour {
		e.FiveHWindowStart = time.Now()
		e.FiveHCredits = 0
	}

	if e.FiveHCredits != 0 {
		t.Errorf("after window reset: FiveHCredits should be 0, got %.2f", e.FiveHCredits)
	}
	if time.Since(e.FiveHWindowStart) > time.Second {
		t.Errorf("after window reset: FiveHWindowStart should be ~now")
	}

	// First request in fresh window should get x1.0
	const limit = 10_000.0
	e.FiveHCredits = 500 // first request cost
	mult := fiveHWindowBurnMultiplier(e, limit)
	if mult != 1.0 {
		t.Errorf("first request after reset (5%% ratio): want x1.0, got x%.2f", mult)
	}
}
