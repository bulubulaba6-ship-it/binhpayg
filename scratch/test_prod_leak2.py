import urllib.request
import json
import urllib.error

url = 'https://api.finkrouter.io.vn/v1/chat/completions'
headers = {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer test_fink_pro_1234567890abcdef_test',
    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    'Accept': '*/*'
}

def check_leak(name, is_stream):
    print(f"--- Testing {name} (stream={is_stream}) on api.finkrouter.io.vn ---")
    data = {
        'model': 'gpt-5.5',
        'messages': [{'role': 'user', 'content': 'Say the word hello. Do not output anything else.'}],
        'max_tokens': 10,
        'stream': is_stream
    }
    req = urllib.request.Request(url, data=json.dumps(data).encode('utf-8'), headers=headers, method='POST')
    try:
        with urllib.request.urlopen(req) as response:
            print("STATUS:", response.status)
            print("HEADERS:")
            for k, v in response.getheaders():
                print(f"  {k}: {v}")
            
            print("BODY:")
            leak_found = False
            for line in response:
                decoded = line.decode('utf-8').strip()
                if decoded:
                    print(decoded)
                    if 'deepseek' in decoded.lower() or 'anthropic' in decoded.lower():
                        print("  >>> LEAK IN BODY CHUNK!")
                        leak_found = True
            if not leak_found:
                print("  => No backend string found in body.")
    except urllib.error.HTTPError as e:
        print("HTTP ERROR:", e.code, e.read().decode('utf-8'))
    except Exception as e:
        print("ERROR:", e)
    print("\n")

check_leak("Non-Stream Request", False)
check_leak("Stream Request", True)
