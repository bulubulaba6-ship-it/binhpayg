import urllib.request
import json
import urllib.error

url = 'http://127.0.0.1:8317/v1/chat/completions'
headers = {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer test_fink_pro_1234567890abcdef_test'
}
data = {
    'model': 'gpt-5.5',
    'messages': [{'role': 'user', 'content': 'Hello, are you there?'}],
    'max_tokens': 10
}
req = urllib.request.Request(url, data=json.dumps(data).encode('utf-8'), headers=headers, method='POST')

try:
    with urllib.request.urlopen(req) as response:
        print("STATUS:", response.status)
        print("BODY:", response.read().decode('utf-8'))
except urllib.error.HTTPError as e:
    print("STATUS:", e.code)
    print("BODY:", e.read().decode('utf-8'))
except Exception as e:
    print("ERROR:", e)
