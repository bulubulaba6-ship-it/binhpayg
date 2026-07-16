package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	keys := []string{
		"fink_6db8bcc04240f16a34398b9319d5a1da",
		"fink_54a9733e264f5e61949735a2098cfb70",
		"fink_eb6924786c88368ab2ef52d99d749f65",
		"fink_547a2dd57b7c148f09478206be8f4755",
		"fink_max_cfacc7472333305910a0176c82e951d8",
		"test_fink_pro_1234567890abcdef_test",
	}

	client := &http.Client{Timeout: 15 * time.Second}

	for _, k := range keys {
		fmt.Printf("========================================\n")
		fmt.Printf("Testing Key: %s\n", k)
		fmt.Printf("========================================\n")

		// 1. Test /v1/models
		reqM, _ := http.NewRequest("GET", "https://api.finkrouter.io.vn/v1/models", nil)
		reqM.Header.Set("Authorization", "Bearer "+k)
		respM, err := client.Do(reqM)
		if err != nil {
			fmt.Printf("[GET /v1/models] ERROR: %v\n", err)
		} else {
			bodyM, _ := io.ReadAll(respM.Body)
			respM.Body.Close()
			fmt.Printf("[GET /v1/models] Status: %d %s | Body: %s\n", respM.StatusCode, respM.Status, string(bodyM))
		}

		// 2. Test /v1/chat/completions (with gpt-5.4-mini)
		payload := map[string]interface{}{
			"model": "gpt-5.4-mini",
			"messages": []map[string]string{
				{"role": "user", "content": "hi"},
			},
			"max_tokens": 5,
		}
		pBytes, _ := json.Marshal(payload)
		reqC, _ := http.NewRequest("POST", "https://api.finkrouter.io.vn/v1/chat/completions", bytes.NewReader(pBytes))
		reqC.Header.Set("Authorization", "Bearer "+k)
		reqC.Header.Set("Content-Type", "application/json")
		respC, err := client.Do(reqC)
		if err != nil {
			fmt.Printf("[POST /v1/chat/completions (gpt-5.4-mini)] ERROR: %v\n", err)
		} else {
			bodyC, _ := io.ReadAll(respC.Body)
			respC.Body.Close()
			fmt.Printf("[POST /v1/chat/completions (gpt-5.4-mini)] Status: %d %s | Body: %s\n\n", respC.StatusCode, respC.Status, string(bodyC))
		}
	}
}
