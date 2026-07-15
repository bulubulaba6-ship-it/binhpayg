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
    req = urllib.request.Request('http://127.0.0.1:8317/management/quota', headers={'Authorization': 'Bearer ' + key})
    try:
        with urllib.request.urlopen(req) as res:
            data = json.loads(res.read().decode())
            print(f"QUOTA: consumed={data['total_consumed']} / limit={data['total_limit']}, reqs={data['total_requests']}")
            return data
    except Exception as e:
        print("QUOTA ERROR:", e)

def burn_tokens():
    print("--- Burning tokens ---")
    data = {'model': 'gpt-5.5', 'messages': [{'role': 'user', 'content': 'Hello! Please talk for exactly 10 tokens.'}], 'max_tokens': 10}
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
    print(f"Delta Consumed: {q2['total_consumed'] - q1['total_consumed']}")
    print(f"Delta Requests: {q2['total_requests'] - q1['total_requests']}")
