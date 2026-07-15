import urllib.request
import json
key = 'test_fink_pro_1234567890abcdef_test'
req = urllib.request.Request('http://127.0.0.1:8317/v1/billing/quota?key=' + key)
with urllib.request.urlopen(req) as res:
    print(res.read().decode())
