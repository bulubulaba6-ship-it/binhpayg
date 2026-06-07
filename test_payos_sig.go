package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"encoding/json"
)

func verifyPayOSSignature(data map[string]interface{}, signature, checksumKey string) bool {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := data[k]
		if v == nil {
			continue
		}
		strVal := ""
		switch val := v.(type) {
		case float64:
			strVal = strconv.FormatFloat(val, 'f', -1, 64)
		default:
			strVal = fmt.Sprintf("%v", val)
		}

		if strVal == "null" || strVal == "undefined" || strVal == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, strVal))
	}

	queryString := strings.Join(parts, "&")
	fmt.Println("QueryString:", queryString)
	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(queryString))
	expected := hex.EncodeToString(mac.Sum(nil))
	fmt.Println("Expected Sig:", expected)
	fmt.Println("Actual Sig:  ", signature)
	return expected == signature
}

func main() {
	payload := []byte(`{"code":"00","desc":"success","data":{"accountNumber":"123","amount":2000,"description":"VQ123","orderCode":123},"signature":"test"}`)
	var req struct {
		Data map[string]interface{} `json:"data"`
		Signature string `json:"signature"`
	}
	json.Unmarshal(payload, &req)
	verifyPayOSSignature(req.Data, req.Signature, "my-checksum-key")
}
