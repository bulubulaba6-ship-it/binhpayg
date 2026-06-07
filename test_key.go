package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	url := "http://localhost:8317/v1/chat/completions"
	apiKey := "fink_max_1c96b7142d18844ce19e9e2c88bab154"

	payload := map[string]interface{}{
		"model": "claude-opus-4-8",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, are you working?"},
		},
		"max_tokens": 50,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("Status:", resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println("Response:", string(respBody))
}
