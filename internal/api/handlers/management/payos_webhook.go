package management

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/middleware"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
)

// PayOSWebhookRequest payload structure
type PayOSWebhookRequest struct {
	Code      string                 `json:"code"`
	Desc      string                 `json:"desc"`
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data"`
	Signature string                 `json:"signature"`
}

var (
	webhookDB   *sql.DB
	webhookOnce sync.Once
)

func getWebhookDB() *sql.DB {
	webhookOnce.Do(func() {
		dsn := os.Getenv("PGSTORE_DSN")
		if dsn == "" {
			dsn = os.Getenv("DATABASE_URL")
		}
		if dsn != "" {
			if db, err := sql.Open("pgx", dsn); err == nil {
				// Initialize the schema for api_keys
				_, _ = db.Exec(`
					CREATE TABLE IF NOT EXISTS payment_orders (
						order_code BIGINT PRIMARY KEY,
						email VARCHAR(255) NOT NULL,
						plan VARCHAR(50) NOT NULL,
						amount BIGINT NOT NULL,
						status VARCHAR(20) DEFAULT 'pending',
						created_at TIMESTAMPTZ DEFAULT NOW()
					);
					CREATE TABLE IF NOT EXISTS otp_codes (
						id SERIAL PRIMARY KEY,
						key_hash VARCHAR(64) NOT NULL,
						otp_code VARCHAR(10) NOT NULL,
						expires_at TIMESTAMPTZ NOT NULL,
						created_at TIMESTAMPTZ DEFAULT NOW()
					);
					CREATE TABLE IF NOT EXISTS api_keys (
						id SERIAL PRIMARY KEY,
						key_hash VARCHAR(64) UNIQUE NOT NULL,
						key_prefix VARCHAR(20) NOT NULL,
						email VARCHAR(255),
						plan VARCHAR(50) NOT NULL,
						status VARCHAR(20) DEFAULT 'active',
						order_code VARCHAR(100) UNIQUE NOT NULL,
						created_at TIMESTAMPTZ DEFAULT NOW()
					);
				`)
				// Auto-migrate missing columns for older deployments
				_, _ = db.Exec(`
					ALTER TABLE api_keys 
					ADD COLUMN IF NOT EXISTS key_prefix VARCHAR(20) DEFAULT '',
					ADD COLUMN IF NOT EXISTS email VARCHAR(255),
					ADD COLUMN IF NOT EXISTS plan VARCHAR(50) DEFAULT 'payg',
					ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active',
					ADD COLUMN IF NOT EXISTS order_code VARCHAR(100);
				`)
				webhookDB = db
			}
		}
	})
	return webhookDB
}

// verifyPayOSSignature validates the HMAC-SHA256 signature from payOS.
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
	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(queryString))
	expected := hex.EncodeToString(mac.Sum(nil))

	return expected == signature
}

// PostPayOSWebhook handles the server-to-server callback from payOS.
func (h *Handler) PostPayOSWebhook(c *gin.Context) {
	var req PayOSWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	checksumKey := os.Getenv("PAYOS_WEBHOOK_CHECKSUM_KEY")
	if checksumKey == "" {
		checksumKey = os.Getenv("PAYOS_CHECKSUM_KEY")
	}
	if checksumKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "payos webhook checksum key not configured"})
		return
	}

	// 1. Verify Signature
	if !verifyPayOSSignature(req.Data, req.Signature, checksumKey) {
		log.Warnf("payos webhook signature verification failed for order %v", req.Data["orderCode"])
		c.JSON(http.StatusOK, gin.H{"error": "invalid signature"}) // Return 200 so payOS doesn't retry invalid sigs
		return
	}

	// 2. Extract Fields
	// orderCode arrives as float64 from JSON; format as int64 to avoid scientific notation
	// when comparing against the BIGINT order_code column in Postgres.
	orderCodeFloat, _ := req.Data["orderCode"].(float64)
	orderCode := strconv.FormatInt(int64(orderCodeFloat), 10)
	amountFloat, _ := req.Data["amount"].(float64)
	description := fmt.Sprintf("%v", req.Data["description"])

	// Parse plan from description if possible. E.g., "PRO order", "MAX order", etc.
	tier := "payg"
	displayPlan := "Pay-As-You-Go"
	credits := float64(0)
	limit := 2000
	isSubscription := false
	descLower := strings.ToLower(description)

	if strings.Contains(descLower, "max_20x") || amountFloat >= 1600000 {
		tier = "max"
		displayPlan = "MAX 20x"
		credits = 5600000
		limit = 40000
		isSubscription = true
	} else if strings.Contains(descLower, "max") || amountFloat >= 650000 {
		tier = "max"
		displayPlan = "MAX 5x"
		credits = 1315000
		limit = 10000
		isSubscription = true
	} else if strings.Contains(descLower, "pro") || amountFloat >= 350000 {
		tier = "pro"
		displayPlan = "PRO"
		credits = 245000
		limit = 2000
		isSubscription = true
	} else if strings.Contains(descLower, "day7") {
		// 7-day short-term subscription: 60,000 fixed credits.
		// Detected by description only — amount-based detection is deliberately omitted
		// because a PAYG order for exactly 150,000 VND would be misclassified.
		tier = "payg"
		displayPlan = "7-Day Pass"
		credits = 60000
		limit = 2000
		isSubscription = true
	} else if strings.Contains(descLower, "day1") {
		// 1-day short-term subscription: 8,000 fixed credits.
		// Detected by description only — amount-based detection is deliberately omitted
		// because a PAYG order for exactly 50,000 VND would be misclassified.
		tier = "payg"
		displayPlan = "1-Day Pass"
		credits = 8000
		limit = 2000
		isSubscription = true
	} else {
		tier = "payg"
		displayPlan = "Pay-As-You-Go"
		credits = 0
		limit = 2000
		isSubscription = false
	}

	// We only provision a new key if it's a known plan, otherwise we might deposit to existing.
	// But in this flow, "Buy API Key" always means a new key unless they specify an old key.
	// We'll assume a new key creation for now as per the "shop.web" replacement plan.

	// 3. Generate Key
	prefix := provisionTierPrefix(tier)
	raw := make([]byte, 16)
	rand.Read(raw)
	newKey := prefix + hex.EncodeToString(raw)

	// 4. Save to Postgres FIRST to ensure idempotency and prevent ghost keys
	db := getWebhookDB()
	userEmail := ""
	if db != nil {
		keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(newKey)))
		
		// 4a. Retrieve the email from payment_orders
		row := db.QueryRow("SELECT email FROM payment_orders WHERE order_code = $1", orderCode)
		_ = row.Scan(&userEmail)
		
		// 4b. Insert the API key (check if exists first to avoid unique constraint issues)
		var existingID int
		errCheck := db.QueryRow("SELECT id FROM api_keys WHERE order_code = $1", orderCode).Scan(&existingID)
		if errCheck != nil && errCheck != sql.ErrNoRows {
			log.Errorf("failed to check existing api key: %v", errCheck)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if errCheck == sql.ErrNoRows {
			_, errInsert := db.Exec(`
				INSERT INTO api_keys (name, key_hash, key_prefix, email, plan, status, order_code, created_at)
				VALUES ('PayOS Key', $1, $2, $3, $4, 'active', $5, $6)
			`, keyHash, prefix, userEmail, displayPlan, orderCode, time.Now().Unix())
			if errInsert != nil {
				log.Errorf("failed to insert api key to postgres: %v", errInsert)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
				return
			}
		}

		if errCheck == nil && existingID > 0 {
			log.Infof("payos webhook ignored duplicate order: %v", orderCode)
			c.JSON(http.StatusOK, gin.H{"error": 0, "message": "Duplicate order ignored", "data": nil})
			return
		}
		// 4c. Mark order as paid
		_, _ = db.Exec("UPDATE payment_orders SET status = 'paid' WHERE order_code = $1", orderCode)
	}

	// 5. Commit Key to Config (Ghost keys prevented by DB check above)
	h.mu.Lock()
	h.cfg.APIKeys = append(h.cfg.APIKeys, newKey)

	if h.cfg.APIKeyModels == nil {
		h.cfg.APIKeyModels = make(map[string]map[string][]string)
	}
	h.cfg.APIKeyModels[newKey] = map[string][]string{
		"claude": AllowedClaude,
		"openai": AllowedOpenAI,
	}

	if h.cfg.PostPayBilling.Clients == nil {
		h.cfg.PostPayBilling.Clients = make(map[string]config.PostPayBillingClientCfg)
	}
	h.cfg.PostPayBilling.Clients[newKey] = config.PostPayBillingClientCfg{CreditLimit: 0}

	if h.cfg.APIKeyLimits == nil {
		h.cfg.APIKeyLimits = make(map[string]int)
	}
	h.cfg.APIKeyLimits[newKey] = limit

	if err := config.SaveConfigPreserveComments(h.configFilePath, h.cfg); err != nil {
		log.Errorf("failed to save config on payos webhook: %v", err)
	}
	h.mu.Unlock()

	// 6. Seed Credits to Ledger
	if isSubscription {
		_, _, _ = middleware.ProcessDepositCredits(newKey, credits, orderCode)
	} else {
		_, _, _, _ = middleware.ProcessDepositVND(newKey, amountFloat, orderCode)
	}
	
	liveCfg := middleware.GetLiveConfig()
	if liveCfg != nil && liveCfg.PostPayBilling.LedgerFile != "" {
		fullPath := filepath.Join(liveCfg.AuthDir, liveCfg.PostPayBilling.LedgerFile)
		_ = middleware.SavePostPayUsage(fullPath)
	}

	// 6.5 Send Email
	if userEmail != "" {
		go sendAPIKeyEmail(userEmail, newKey, displayPlan)
	}

	// 7. Respond OK to payOS
	c.JSON(http.StatusOK, gin.H{"error": 0, "message": "Ok", "data": nil})
}

// sendAPIKeyEmail sends the raw API key to the user via Resend
func sendAPIKeyEmail(toEmail, apiKey, plan string) {
	resendKey := os.Getenv("RESEND_API_KEY")
	if resendKey == "" {
		log.Warn("RESEND_API_KEY not set, skipping email delivery")
		return
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "onboarding@resend.dev"
	}

	htmlTemplate := `
<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; border: 1px solid #eaeaec; border-radius: 8px; overflow: hidden; box-shadow: 0 4px 12px rgba(0,0,0,0.05);">
  <div style="background-color: #0d0d0d; color: #eca8d6; padding: 20px; text-align: center;">
    <h1 style="margin: 0; font-size: 24px; font-weight: 600;">FINKROUTER</h1>
    <p style="margin: 5px 0 0; font-size: 14px; color: #888;">Private network proxy access</p>
  </div>
  <div style="padding: 30px; background-color: #ffffff; color: #333;">
    <h2 style="margin-top: 0; color: #111;">Your API Key is Ready</h2>
    <p style="font-size: 16px; line-height: 1.5;">Thank you for your purchase (<strong>%s</strong>). Your payment has been successfully processed.</p>
    <div style="background-color: #f7f7f9; border: 1px dashed #ccc; padding: 20px; text-align: center; border-radius: 6px; margin: 25px 0;">
      <p style="margin: 0; font-size: 14px; color: #666; margin-bottom: 8px; text-transform: uppercase;">Your Secure API Key</p>
      <code style="font-size: 22px; color: #000; font-weight: bold; word-break: break-all;">%s</code>
    </div>
    <p style="font-size: 14px; line-height: 1.5; color: #d9534f; font-weight: bold;">⚠️ Keep this key secure. Do not share it or expose it in public repositories.</p>
    <p style="font-size: 14px; line-height: 1.5; margin-top: 20px;">You can use this key to authenticate with our proxy endpoints. For setup instructions, please refer to our documentation.</p>
  </div>
  <div style="background-color: #f9f9f9; padding: 20px; text-align: center; border-top: 1px solid #eee; color: #777; font-size: 12px;">
    <p style="margin: 0 0 5px;">Need help? Contact our support team at <a href="mailto:support@finkrouter.com" style="color: #eca8d6; text-decoration: none;">support@finkrouter.com</a></p>
    <p style="margin: 0;">© 2026 FinkRouter. All rights reserved.</p>
  </div>
</div>`

	payload := map[string]interface{}{
		"from":    fmt.Sprintf("FinkRouter <%s>", fromEmail),
		"to":      []string{toEmail},
		"subject": "Your FinkRouter API Key",
		"html":    fmt.Sprintf(htmlTemplate, plan, apiKey),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Errorf("failed to create resend req: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("failed to send email via resend: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Errorf("resend returned status: %d", resp.StatusCode)
	} else {
		log.Infof("Successfully sent API key to %s", toEmail)
	}
}
