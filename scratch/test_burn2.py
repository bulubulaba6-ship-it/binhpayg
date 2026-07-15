import urllib.request
import json
import time

def wait_for_server():
    for _ in range(15):
        try:
            urllib.request.urlopen('http://127.0.0.1:8317/health', timeout=1)
            return True
        except:
            time.sleep(2)
    return False

wait_for_server()
time.sleep(1)

key = 'test_fink_pro_1234567890abcdef_test'

def check_quota():
    req = urllib.request.Request('http://127.0.0.1:8317/v1/billing/quota?key=' + key)
    try:
        with urllib.request.urlopen(req) as res:
            data = json.loads(res.read().decode())
            print(f"QUOTA: limit={data.get('credit_limit')}, consumed={data.get('credits_consumed')}, requests={data.get('total_requests')}")
            return data
    except Exception as e:
        print("QUOTA ERROR:", e)

def burn_tokens():
    print("--- Burning tokens ---")
    data = {'model': 'gpt-5.5', 'messages': [{'role': 'user', 'content': 'Count from 1 to 5'}], 'max_tokens': 100}
    req = urllib.request.Request('http://127.0.0.1:8317/v1/chat/completions', data=json.dumps(data).encode(), headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key}, method='POST')
    try:
        with urllib.request.urlopen(req) as res:
            res_data = json.loads(res.read().decode())
            print(f"Burned! Usage: {res_data.get('usage', {})}")
    except Exception as e:
        print("BURN ERROR:", e)

print("Before burn:")
q1 = check_quota()
burn_tokens()
print("After burn:")
q2 = check_quota()

if q1 and q2:
    print(f"Delta Consumed: {q2.get('credits_consumed', 0) - q1.get('credits_consumed', 0)}")
    print(f"Delta Requests: {q2.get('total_requests', 0) - q1.get('total_requests', 0)}")
