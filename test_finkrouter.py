#!/usr/bin/env python3
"""
FinkRouter Local Test — tests all keys against applicable models.
Run: python test_finkrouter.py
"""
import sys, json, time, urllib.request, urllib.error
sys.stdout.reconfigure(encoding="utf-8")

BASE_URL = "https://api.finkrouter.io.vn"
PROMPT   = "Reply with exactly one word: OK"

# key -> list of models to test for that key
KEYS = {
    # max keys (claude only)
    "fink_max_d41d8cd98f00b204e9800998ecf8427e": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
    },
    "fink_max_ad0234829205b9033196ba818f7a872b": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
    },
    "fink_max_1c96b7142d18844ce19e9e2c88bab154": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
        "openai": ["gpt-5.5", "gpt-5.4-mini"],
    },
    # pro keys (claude only)
    "fink_pro_5d41402abc4b2a76b9719d911017c592": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
    },
    "fink_pro_e2fc714c4727ee9395f324cd2e7f331f": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
    },
    # pro keys (claude + openai)
    "fink_pro_a7f3d2e8b1c4f9a0e5d6c3b2a1f0e9d8": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
        "openai": ["gpt-5.5", "gpt-5.4-mini"],
    },
    "fink_pro_e65b9d6c7d02b2939e64ff192520484a": {
        "claude": ["claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"],
        "openai": ["gpt-5.5", "gpt-5.4-mini"],
    },
}

GREEN  = "\033[92m"
RED    = "\033[91m"
YELLOW = "\033[93m"
CYAN   = "\033[96m"
RESET  = "\033[0m"
BOLD   = "\033[1m"

def chat(api_key: str, model: str) -> dict:
    payload = json.dumps({
        "model": model,
        "max_tokens": 16,
        "messages": [{"role": "user", "content": PROMPT}],
        "stream": False,
    }).encode()

    req = urllib.request.Request(
        f"{BASE_URL}/v1/chat/completions",
        data=payload,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        },
        method="POST",
    )

    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = json.loads(resp.read())
            elapsed = time.time() - t0
            text = (body.get("choices") or [{}])[0].get("message", {}).get("content", "").strip()
            return {"ok": True, "text": text[:40], "ms": int(elapsed * 1000), "status": resp.status}
    except urllib.error.HTTPError as e:
        elapsed = time.time() - t0
        body = e.read().decode(errors="replace")[:120]
        return {"ok": False, "text": body, "ms": int(elapsed * 1000), "status": e.code}
    except Exception as ex:
        elapsed = time.time() - t0
        return {"ok": False, "text": str(ex)[:80], "ms": int(elapsed * 1000), "status": 0}


def list_models(api_key: str) -> list:
    req = urllib.request.Request(
        f"{BASE_URL}/v1/models",
        headers={
            "Authorization": f"Bearer {api_key}",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read())
            return [m["id"] for m in body.get("data", [])]
    except Exception:
        return []


def short_key(k: str) -> str:
    parts = k.split("_")
    if len(parts) >= 3:
        tier = "_".join(parts[1:-1]) if len(parts) > 3 else parts[1]
        return f"fink_{tier}_...{k[-8:]}"
    return f"fink_...{k[-8:]}"


print(f"\n{BOLD}{'='*72}{RESET}")
print(f"{BOLD}  FinkRouter Local Test  |  {BASE_URL}{RESET}")
print(f"{BOLD}{'='*72}{RESET}\n")

results = {"pass": 0, "fail": 0, "skip": 0}

for api_key, model_groups in KEYS.items():
    key_label = short_key(api_key)
    print(f"{CYAN}{BOLD}>> {key_label}{RESET}")

    # Verify models endpoint
    available = list_models(api_key)
    if available:
        print(f"  {YELLOW}models/{RESET} {', '.join(available)}")
    else:
        print(f"  {RED}models/ — failed to list{RESET}")

    all_models = []
    for grp in model_groups.values():
        all_models.extend(grp)

    for model in all_models:
        if available and model not in available:
            print(f"  {YELLOW}SKIP{RESET}  {model:<30} (not in key's model list)")
            results["skip"] += 1
            continue

        r = chat(api_key, model)
        if r["ok"]:
            print(f"  {GREEN}PASS{RESET}  {model:<30} {r['ms']:>5}ms  → \"{r['text']}\"")
            results["pass"] += 1
        else:
            print(f"  {RED}FAIL{RESET}  {model:<30} HTTP {r['status']}  {r['text']}")
            results["fail"] += 1

    print()

total = results["pass"] + results["fail"] + results["skip"]
print(f"{BOLD}{'='*72}{RESET}")
print(f"{BOLD}  RESULTS: {GREEN}{results['pass']} PASS{RESET}{BOLD} | {RED}{results['fail']} FAIL{RESET}{BOLD} | {YELLOW}{results['skip']} SKIP{RESET}{BOLD} | {total} TOTAL{RESET}")
print(f"{BOLD}{'='*72}{RESET}\n")
