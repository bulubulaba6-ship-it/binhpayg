import urllib.request
import urllib.error
import json
import sys

def get_quota(test_key):
    quota_url = f"http://127.0.0.1:8317/v1/billing/quota?key={test_key}"
    req = urllib.request.Request(quota_url)
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode("utf-8"))

def send_chat_completion(test_key, model, prompt):
    url = "http://127.0.0.1:8317/v1/chat/completions"
    headers = {
        "Authorization": f"Bearer {test_key}",
        "Content-Type": "application/json"
    }
    payload = {
        "model": model,
        "messages": [
            {"role": "user", "content": prompt}
        ],
        "max_tokens": 20
    }
    req = urllib.request.Request(
        url,
        data=json.dumps(payload).encode("utf-8"),
        headers=headers,
        method="POST"
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read().decode("utf-8"))

def main():
    test_key = "test_fink_pro_1234567890abcdef_test"
    model = "claude-opus-4-7"
    
    print("Fetching initial quota...")
    initial_quota = get_quota(test_key)
    initial_used = initial_quota["quota"]["credits_used"]
    print(f"Initial credits_used: {initial_used}")

    print("\nSending chat completion request...")
    prompt = "Write a one-sentence joke about programming."
    resp_body = send_chat_completion(test_key, model, prompt)
    print("Response received.")
    
    # Extract usage info from response
    usage = resp_body["usage"]
    prompt_tokens = usage["prompt_tokens"]
    completion_tokens = usage["completion_tokens"]
    cached_tokens = usage.get("prompt_tokens_details", {}).get("cached_tokens", 0)
    reasoning_tokens = usage.get("completion_tokens_details", {}).get("reasoning_tokens", 0)
    
    print(f"Usage details:")
    print(f"  Prompt tokens: {prompt_tokens}")
    print(f"  Completion tokens: {completion_tokens}")
    print(f"  Cached tokens: {cached_tokens}")
    print(f"  Reasoning tokens: {reasoning_tokens}")

    # Expected rates for claude-opus-4-7:
    # input: 1400, output: 2800, cache: 140
    input_rate = 1400.0
    output_rate = 2800.0
    cache_rate = 140.0
    
    billable_input = max(0, prompt_tokens - cached_tokens)
    expected_cost = 0.0
    expected_cost += billable_input * input_rate / 1_000_000.0
    expected_cost += completion_tokens * output_rate / 1_000_000.0
    expected_cost += cached_tokens * cache_rate / 1_000_000.0
    expected_cost += reasoning_tokens * output_rate / 1_000_000.0
    
    print(f"Expected billing increment (calculated): {expected_cost} credits")

    print("\nFetching final quota...")
    final_quota = get_quota(test_key)
    final_used = final_quota["quota"]["credits_used"]
    print(f"Final credits_used: {final_used}")

    actual_diff = final_used - initial_used
    print(f"Actual credits_used increment: {actual_diff} credits")

    if abs(actual_diff - expected_cost) < 1e-9:
        print("\nSUCCESS: Quota updated correctly and matched the expected billing amount!")
    else:
        print("\nFAILURE: Quota discrepancy detected!")
        sys.exit(1)

if __name__ == "__main__":
    main()
