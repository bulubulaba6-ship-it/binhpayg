"""
burn_test_rapid.py — FinkRouter billing stress tester.

Tests:
  1. Basic burst (50 concurrent requests) — confirms rate limiting fires
  2. Retry-After header validation — confirms header is present on 429
  3. Tier escalation probe — sends incremental requests and logs each
     response status to observe when rate limit kicks in

Usage:
    python scratch/burn_test_rapid.py [--mode basic|retry|probe|all]
"""
import urllib.request
import urllib.error
import json
import concurrent.futures
import time
import argparse

URL = "http://127.0.0.1:8317/v1/chat/completions"
API_KEY = "fink_max_3d6be4e0c6f57d845c49b853b246053a"

_BODY = json.dumps({
    "model": "claude-haiku-4-5",
    "messages": [{"role": "user", "content": "Hello, how are you? Explain quantum mechanics in 200 words."}],
    "max_tokens": 100,
}).encode("utf-8")


def _make_req(i, extra_headers=None):
    req = urllib.request.Request(URL, data=_BODY, method="POST")
    req.add_header("Authorization", f"Bearer {API_KEY}")
    req.add_header("Content-Type", "application/json")
    if extra_headers:
        for k, v in extra_headers.items():
            req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            resp.read()
            return i, resp.status, None, dict(resp.headers)
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        return i, e.code, body, dict(e.headers)
    except Exception as e:
        return i, 0, str(e), {}


# ─── TEST 1: Basic burst ──────────────────────────────────────────────────────

def run_basic_burst(total=50, workers=10):
    print(f"\n[TEST 1] Basic Burst — {total} reqs × {workers} threads on {URL}")
    start = time.time()
    with concurrent.futures.ThreadPoolExecutor(max_workers=workers) as ex:
        results = list(ex.map(lambda i: _make_req(i), range(total)))

    ok = rate_limited = errors = 0
    for i, status, body, _ in results:
        tag = "SUCCESS" if status == 200 else (f"RATE-LIMITED({status})" if status == 429 else f"ERROR({status})")
        print(f"  Req {i:02d}: {tag}")
        if status == 200:
            ok += 1
        elif status == 429:
            rate_limited += 1
        else:
            errors += 1

    elapsed = time.time() - start
    print(f"\n  ✓ {ok} success  |  ⚛ {rate_limited} rate-limited  |  ✗ {errors} errors  |  {elapsed:.2f}s")
    return ok, rate_limited, errors


# ─── TEST 2: Retry-After header validation ────────────────────────────────────

def run_retry_after_check(n=20):
    """Send n requests and verify that any 429 response includes Retry-After."""
    print(f"\n[TEST 2] Retry-After header check — {n} sequential requests")
    found_429 = False
    retry_after_present = False

    for i in range(n):
        _, status, body, headers = _make_req(i)
        if status == 429:
            found_429 = True
            ra = headers.get("Retry-After", headers.get("retry-after", ""))
            retry_after_present = bool(ra)
            try:
                secs = int(ra)
                print(f"  Req {i:02d}: 429 ✓  Retry-After={secs}s  (window expires in ~{secs//60}m{secs%60}s)")
            except (ValueError, TypeError):
                print(f"  Req {i:02d}: 429 ✗  Retry-After header missing or invalid: {ra!r}")
            break
        else:
            print(f"  Req {i:02d}: {status}")

    if not found_429:
        print("  ⚠ No 429 encountered — consider lowering the window limit or running after TEST 1.")
        return False
    print(f"  Result: Retry-After present = {retry_after_present}")
    return retry_after_present


# ─── TEST 3: Tier escalation probe ────────────────────────────────────────────

def run_tier_probe(max_reqs=30, delay_s=0.5):
    """
    Send requests one by one with a small delay and observe status codes.
    Useful for confirming the rate limit window resets after 5 hours and
    the first batch of requests in a fresh window always gets 200.
    """
    print(f"\n[TEST 3] Tier Escalation Probe — up to {max_reqs} reqs, {delay_s}s apart")
    results = []
    for i in range(max_reqs):
        _, status, body, headers = _make_req(i)
        ra = headers.get("Retry-After", headers.get("retry-after", ""))
        tag = "✓ 200" if status == 200 else (f"⚛ 429 (Retry-After={ra}s)" if status == 429 else f"✗ {status}")
        print(f"  Req {i:02d}: {tag}")
        results.append(status)
        if status == 429:
            print(f"    ↳ Rate limit hit at request #{i}. Stopping probe.")
            break
        time.sleep(delay_s)

    ok_count = results.count(200)
    print(f"\n  {ok_count}/{len(results)} succeeded before rate limit.")
    return results


# ─── Main ─────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="FinkRouter burn tester")
    parser.add_argument(
        "--mode",
        choices=["basic", "retry", "probe", "all"],
        default="basic",
        help="Which test to run (default: basic)"
    )
    args = parser.parse_args()

    if args.mode in ("basic", "all"):
        run_basic_burst()

    if args.mode in ("retry", "all"):
        run_retry_after_check()

    if args.mode in ("probe", "all"):
        run_tier_probe()

    print("\nDone.")


if __name__ == "__main__":
    main()
