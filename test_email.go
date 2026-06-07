package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file from the current directory
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("Note: No .env file found or error loading it, using system environment variables.")
	}

	resendKey := os.Getenv("RESEND_API_KEY")
	if resendKey == "" {
		log.Fatal("FATAL: RESEND_API_KEY not set in environment or .env file")
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "send@finkrouter.io.vn" // Updated to match user request
	}

	toEmail := "maiphuocminhtai21032005@gmail.com"
	apiKey := "test_key_12345_finkrouter"
	plan := "PRO (Test)"

	htmlTemplate := `
<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; border: 1px solid #eaeaec; border-radius: 8px; overflow: hidden; box-shadow: 0 4px 12px rgba(0,0,0,0.05);">
  <div style="background-color: #0d0d0d; color: #eca8d6; padding: 20px; text-align: center;">
    <h1 style="margin: 0; font-size: 24px; font-weight: 600;">FINKROUTER TEST</h1>
    <p style="margin: 5px 0 0; font-size: 14px; color: #888;">Private network proxy access</p>
  </div>
  <div style="padding: 30px; background-color: #ffffff; color: #333;">
    <h2 style="margin-top: 0; color: #111;">Your API Key is Ready</h2>
    <p style="font-size: 16px; line-height: 1.5;">Thank you for your purchase (<strong>%s</strong>). Your payment has been successfully processed.</p>
    <div style="background-color: #f7f7f9; border: 1px dashed #ccc; padding: 20px; text-align: center; border-radius: 6px; margin: 25px 0;">
      <p style="margin: 0; font-size: 14px; color: #666; margin-bottom: 8px; text-transform: uppercase;">Your Secure API Key</p>
      <code style="font-size: 22px; color: #000; font-weight: bold; word-break: break-all;">%s</code>
    </div>
  </div>
</div>`

	payload := map[string]interface{}{
		"from":    fmt.Sprintf("FinkRouter <%s>", fromEmail),
		"to":      []string{toEmail},
		"subject": "FinkRouter TEST Delivery",
		"html":    fmt.Sprintf(htmlTemplate, plan, apiKey),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Fatalf("failed to create resend req: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("Sending test email to: %s\n", toEmail)
	fmt.Printf("From: %s\n", fromEmail)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("failed to send email via resend: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	
	fmt.Printf("HTTP Status Code: %d\n", resp.StatusCode)
	fmt.Printf("Response Body: %s\n", string(respBody))
	
	if resp.StatusCode >= 300 {
		log.Fatalf("ERROR: Resend API rejected the request.")
	} else {
		log.Printf("SUCCESS: Email sent successfully! Check the inbox of %s", toEmail)
	}
}
