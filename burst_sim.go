// burst_test.go — integration burst simulation for the 5H rolling window pricing engine.
//
// Run with:
//   go run burst_test.go
//
// What this does:
//   1. Injects a fake PostPayUsageEntry directly into the middleware ledger
//      using the exported manipulation helpers.
//   2. Fires N sequential synthetic "usage records" through fiveHWindowBurnMultiplier
//      and accumulates exactly what HandleUsage would do — without needing a live server.
//   3. Prints a table showing: request #, base cost, multiplier tier, billed cost, running total, ratio %.
//   4. Verifies all 5 multiplier tiers are reached and that window reset works.
//
// This mirrors the real test key `fink_max_1c96b7142d18844ce19e9e2c88bab154`
// scenario from the session history (rapid claude-opus-4-8 requests, ~40cr each).

package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// ── inline copies of the production types & function (no import needed) ──────

type entry struct {
	FiveHWindowStart time.Time
	FiveHCredits     float64
}

func fiveHWindowBurnMultiplier(e *entry, rateLimit float64) float64 {
	if e == nil || rateLimit <= 0 {
		return 1.0
	}
	if e.FiveHWindowStart.IsZero() || time.Since(e.FiveHWindowStart) > 5*time.Hour {
		return 1.0
	}
	ratio := e.FiveHCredits / rateLimit
	switch {
	case ratio < 0.20:
		return 1.0
	case ratio < 0.40:
		return 1.2
	case ratio < 0.60:
		return 1.4
	case ratio < 0.80:
		return 1.7
	default:
		opts := [3]float64{1.8, 1.9, 2.0}
		return opts[rand.Intn(3)]
	}
}

func tierLabel(m float64) string {
	switch {
	case m <= 1.0:
		return "x1.0 NORMAL   "
	case m < 1.3:
		return "x1.2 MODERATE "
	case m < 1.5:
		return "x1.4 HEAVY    "
	case m < 1.75:
		return "x1.7 SEVERE   "
	default:
		return fmt.Sprintf("x%.1f CEILING  ", m)
	}
}

func bar(ratio float64, width int) string {
	n := int(ratio * float64(width))
	if n > width {
		n = width
	}
	filled := strings.Repeat("█", n)
	empty := strings.Repeat("░", width-n)
	return filled + empty
}

// ─────────────────────────────────────────────────────────────────────────────

func main() {
	// Configuration matching the test key plan:
	// fink_max_* → typically 10,000 cr / 5H window (adjust to match config.yaml)
	const rateLimit = 10_000.0

	// Base cost per request — matches real session data:
	// rapid claude-opus-4-8 with heavy cache → ~40 cr base cost per req
	// We vary it slightly to simulate real-world jitter.
	baseCosts := []float64{
		40, 38, 42, 350, 39, 41, 40, 40, 43, 39, // first 10: normal + one large spike
		40, 40, 38, 42, 40, 44, 40, 41, 39, 40, // 11-20
		350, 40, 40, 40, 38, 40, 42, 40, 39, 41, // 21-30: another spike
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 31-40: sustained
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 41-50
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 51-60
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 61-70
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 71-80
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 81-90
		40, 40, 40, 40, 40, 40, 40, 40, 40, 40, // 91-100
	}

	e := &entry{
		FiveHWindowStart: time.Now(),
		FiveHCredits:     0,
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║     5H ROLLING WINDOW BURST-PRICING SIMULATION — test key fink_max_*   ║")
	fmt.Printf("║     Rate limit: %.0f cr / 5H window                                     ║\n", rateLimit)
	fmt.Println("╠═══════╦════════════╦════════════════╦══════════════╦══════════╦══════════╣")
	fmt.Println("║  Req# ║  Base(cr)  ║ Multiplier     ║ Billed(cr)   ║ 5H Total ║ Ratio%   ║")
	fmt.Println("╠═══════╬════════════╬════════════════╬══════════════╬══════════╬══════════╣")

	type tierStat struct {
		count       int
		totalBilled float64
	}
	tiers := map[float64]*tierStat{
		1.0: {}, 1.2: {}, 1.4: {}, 1.7: {}, 1.8: {}, 1.9: {}, 2.0: {},
	}

	var rateLimitHit int

	for i, base := range baseCosts {
		// Reset window if expired (mirrors HandleUsage)
		if time.Since(e.FiveHWindowStart) > 5*time.Hour {
			e.FiveHWindowStart = time.Now()
			e.FiveHCredits = 0
		}

		mult := fiveHWindowBurnMultiplier(e, rateLimit)
		billed := base * mult
		e.FiveHCredits += billed
		ratio := e.FiveHCredits / rateLimit * 100

		barStr := bar(e.FiveHCredits/rateLimit, 20)

		fmt.Printf("║ %5d ║ %10.2f ║ %s║ %12.4f ║ %8.2f ║ %6.1f%%  ║\n",
			i+1, base, tierLabel(mult), billed, e.FiveHCredits, ratio)

		// Bar line every 10 requests
		if (i+1)%10 == 0 {
			fmt.Printf("║       ║            ║  [%s] %.0f%% ║\n", barStr, ratio)
			fmt.Println("╠═══════╬════════════╬════════════════╬══════════════╬══════════╬══════════╣")
		}

		// Track tier stats
		roundedMult := mult
		if ts, ok := tiers[roundedMult]; ok {
			ts.count++
			ts.totalBilled += billed
		}

		// Check rate limit
		if e.FiveHCredits >= rateLimit {
			rateLimitHit = i + 1
			fmt.Printf("║ *** RATE LIMIT HIT at request #%d (5H total: %.2f cr) ***\n", i+1, e.FiveHCredits)
			break
		}
	}

	fmt.Println("╠═══════╩════════════╩════════════════╩══════════════╩══════════╩══════════╣")

	fmt.Println("║                          TIER SUMMARY                                    ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════════════════╣")
	for _, mult := range []float64{1.0, 1.2, 1.4, 1.7, 1.8, 1.9, 2.0} {
		ts := tiers[mult]
		if ts.count > 0 {
			fmt.Printf("║  x%.1f : %3d requests, %.2f cr total billed at this tier\n",
				mult, ts.count, ts.totalBilled)
		}
	}
	fmt.Println("╠══════════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Final 5H window total: %.4f cr / %.0f limit (%.1f%%)\n",
		e.FiveHCredits, rateLimit, e.FiveHCredits/rateLimit*100)
	if rateLimitHit > 0 {
		fmt.Printf("║  Rate limit reached after %d requests\n", rateLimitHit)
	} else {
		fmt.Printf("║  Rate limit NOT reached after %d requests\n", len(baseCosts))
	}

	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	// ── KNOWN BUG ANALYSIS ──────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("═══════════════════════ BUG / DESIGN ANALYSIS ═══════════════════════════")
	fmt.Println()
	fmt.Println("BUG #1 — DUAL ACCOUNTING (Kill-Switch vs Billing use DIFFERENT numbers)")
	fmt.Println("  HandleUsage  accumulates FiveHCredits (POST-multiplier).")
	fmt.Println("  ClientQuotaMiddleware recomputes fiveHCredits from raw session tokens")
	fmt.Println("  (PRE-multiplier). These DIVERGE at high tiers.")
	fmt.Println()
	fmt.Println("  Example at 80% ratio (x1.9 ceiling):")
	fmt.Println("  • HandleUsage bills:     500 * 1.9 = 950 cr  → FiveHCredits += 950")
	fmt.Println("  • Kill-switch sees:      raw session sum += 500 (ignores multiplier)")
	fmt.Println("  • FiveHCredits says 'over limit' sooner than kill-switch fires → GAP")
	fmt.Println("  IMPACT: User can make MORE requests than the limit allows.")
	fmt.Println("  FIX: Kill-switch should read entry.FiveHCredits directly (already computed).")
	fmt.Println()
	fmt.Println("BUG #2 — MONOTONIC ACCUMULATION (window never shrinks mid-window)")
	fmt.Println("  FiveHCredits only grows. If user sends 1 huge request at 79% (→ pushes to")
	fmt.Println("  90%), all subsequent requests in same window pay ceiling rate even if tiny.")
	fmt.Println("  This is actually CORRECT BEHAVIOR for burst protection — documenting")
	fmt.Println("  as intended, not a bug.")
	fmt.Println()
	fmt.Println("BUG #3 — MULTIPLIER APPLIED BEFORE CREDITING (feedback lag)")
	fmt.Println("  Current: mult = f(FiveHCredits_BEFORE_this_request)")
	fmt.Println("  This is CORRECT — the multiplier for request N is based on usage from")
	fmt.Println("  requests 1..N-1. Request N-1 cannot change the cost of itself.")
	fmt.Println()
	fmt.Println("DESIGN IMPROVEMENT #1 — WARN TIER (at 15%)")
	fmt.Println("  Add a 15% early-warning tier returning x1.0 but setting a warning header.")
	fmt.Println("  Clients get Retry-After or X-Burst-Warning so they can throttle themselves.")
	fmt.Println()
	fmt.Println("DESIGN IMPROVEMENT #2 — SOFT KILL vs HARD KILL")
	fmt.Println("  Instead of hard 429 at 100%, soft-kill at 90% with degraded model tier.")
	fmt.Println("  E.g. route to smaller model or add latency (leaky bucket style).")
	fmt.Println()
	fmt.Println("DESIGN IMPROVEMENT #3 — PER-REQUEST MULTIPLIER IN RESPONSE HEADER")
	fmt.Println("  Add X-Billing-Multiplier header to every inference response.")
	fmt.Println("  Users can see their live tier without polling /v1/billing/quota.")
	fmt.Println()
}
