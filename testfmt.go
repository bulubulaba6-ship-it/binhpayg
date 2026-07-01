package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
	resendKey := "re_A6k5djfB_3DH2T3Dsqnb8Z9Ar3n15tbR5"
	toEmail := "maiphuocanhtai21032005@gmail.com"
	plan := "pro"
	apiKey := "sk-fink-test-12345"

	bodyStr := fmt.Sprintf(`{"from": "FinkRouter <onboarding@resend.dev>", "to": ["%s"], "subject": "Your FinkRouter API Key", "html": "<p>Thank you for your purchase (%s)!</p><p>Your API Key is: <strong>%s</strong></p><p>Keep this key secure and do not share it.</p>"}`, toEmail, plan, apiKey)

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer([]byte(bodyStr)))
	if err != nil {
		log.Fatalf("failed to create resend req: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("failed to send email via resend: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Response Status: %d", resp.StatusCode)
	log.Printf("Response Body: %s", string(body))
}
