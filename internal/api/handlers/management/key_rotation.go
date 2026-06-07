package management

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/middleware"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
)

type RotateRequest struct {
	WorkspaceKey string `json:"workspaceKey"`
}

type VerifyRotationRequest struct {
	WorkspaceKey string `json:"workspaceKey"`
	OTP          string `json:"otp"`
}

// PostRequestRotation generates an OTP and emails it to the user.
// Rate limited: max 3 OTP requests per key per hour to prevent spam/brute-force.
func (h *Handler) PostRequestRotation(c *gin.Context) {
	var req RotateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	if req.WorkspaceKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceKey is required"})
		return
	}

	keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.WorkspaceKey)))
	db := getWebhookDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database unavailable"})
		return
	}

	// Find the email and check if key is active
	var email string
	err := db.QueryRow("SELECT email FROM api_keys WHERE key_hash = $1 AND status = 'active'", keyHash).Scan(&email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked key"})
		return
	}

	// Rate limit: max 3 OTP requests per key per hour
	var recentCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM otp_codes WHERE key_hash = $1 AND created_at > NOW() - INTERVAL '1 hour'", keyHash).Scan(&recentCount)
	if recentCount >= 3 {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many OTP requests, please wait before trying again"})
		return
	}

	// Clean up expired OTPs for this key (housekeeping)
	_, _ = db.Exec("DELETE FROM otp_codes WHERE key_hash = $1 AND expires_at < NOW()", keyHash)

	// Generate 6-digit OTP
	otpCode := generateOTP()
	expiresAt := time.Now().Add(5 * time.Minute)

	// Save OTP
	_, err = db.Exec("INSERT INTO otp_codes (key_hash, otp_code, expires_at) VALUES ($1, $2, $3)", keyHash, otpCode, expiresAt)
	if err != nil {
		log.Errorf("failed to insert otp: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Email OTP
	go sendOTPEmail(email, otpCode)

	// Return masked email hint so user knows where to check
	maskedEmail := maskEmail(email)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OTP sent to your email", "emailHint": maskedEmail})
}

// PostVerifyRotation verifies the OTP and performs the actual rotation
func (h *Handler) PostVerifyRotation(c *gin.Context) {
	var req VerifyRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.WorkspaceKey)))
	db := getWebhookDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database unavailable"})
		return
	}

	// Verify OTP
	var otpID int
	var expiresAt time.Time
	err := db.QueryRow("SELECT id, expires_at FROM otp_codes WHERE key_hash = $1 AND otp_code = $2 ORDER BY id DESC LIMIT 1", keyHash, req.OTP).Scan(&otpID, &expiresAt)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid otp code"})
		return
	}

	if time.Now().After(expiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "otp expired"})
		return
	}

	// Burn the OTP — single-use only, prevents replay attacks
	_, _ = db.Exec("DELETE FROM otp_codes WHERE id = $1", otpID)

	// Get original key info
	// order_code is VARCHAR(100) — it can hold both numeric IDs ("880645...") and
	// rotation IDs ("ROTATE-{hex}"). Scan as string to avoid int64 parse failure
	// on rotated keys, which would falsely return "key no longer active".
	var email, plan, orderCodeStr string
	err = db.QueryRow("SELECT email, plan, order_code FROM api_keys WHERE key_hash = $1 AND status = 'active'", keyHash).Scan(&email, &plan, &orderCodeStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "key no longer active"})
		return
	}

	// Mark old key as revoked
	_, err = db.Exec("UPDATE api_keys SET status = 'revoked' WHERE key_hash = $1", keyHash)
	if err != nil {
		log.Errorf("failed to revoke old key: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Generate new key — use provisionTierPrefix to ensure the rotated key
	// has the same prefix format as the original (e.g. payg→"fink_", pro→"fink_pro_").
	randomBytes := make([]byte, 16)
	_, _ = rand.Read(randomBytes)
	newPrefix := provisionTierPrefix(plan)
	newKey := newPrefix + fmt.Sprintf("%x", randomBytes)
	newHash := fmt.Sprintf("%x", sha256.Sum256([]byte(newKey)))

	// Generate a unique rotation order code so it doesn't conflict with the original order_code UNIQUE constraint
	rotationID := fmt.Sprintf("ROTATE-%x", randomBytes)

	// Insert new key — include "name" to satisfy the legacy NOT NULL constraint on the production DB
	_, err = db.Exec("INSERT INTO api_keys (name, key_hash, key_prefix, email, plan, order_code) VALUES ($1, $2, $3, $4, $5, $6)",
		plan, newHash, newPrefix, email, plan, rotationID)
	if err != nil {
		log.Errorf("failed to insert new key: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Migrate Config and Ledger files + live memory swap (old key → new key)
	if err = h.migrateKeyInFiles(req.WorkspaceKey, newKey); err != nil {
		log.Errorf("failed to migrate key in files: %v", err)
		// Non-fatal: config file write failed but memory is already swapped.
		// The new key works for the current server lifetime but may revert on restart.
	}

	// Also register the new key's model access + rate limit into h.cfg memory
	// (SwapKeyInConfigMemory only moves the key string in APIKeys list, it does NOT
	// copy over the api-key-models or api-key-limits entries, so we must do that here)
	h.mu.Lock()
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
	// Copy old key's credit limit to new key, then remove old entry
	if oldCfg, ok := h.cfg.PostPayBilling.Clients[req.WorkspaceKey]; ok {
		h.cfg.PostPayBilling.Clients[newKey] = oldCfg
		delete(h.cfg.PostPayBilling.Clients, req.WorkspaceKey)
	} else {
		h.cfg.PostPayBilling.Clients[newKey] = config.PostPayBillingClientCfg{CreditLimit: 0}
	}
	if h.cfg.APIKeyLimits != nil {
		if oldLimit, ok := h.cfg.APIKeyLimits[req.WorkspaceKey]; ok {
			h.cfg.APIKeyLimits[newKey] = oldLimit
			delete(h.cfg.APIKeyLimits, req.WorkspaceKey)
		}
	}
	_ = config.SaveConfigPreserveComments(h.configFilePath, h.cfg)
	h.mu.Unlock()

	// Email the new key
	go sendAPIKeyEmail(email, newKey, plan)

	c.JSON(http.StatusOK, gin.H{"success": true, "newKey": newKey})
}

// maskEmail hides most of an email address for security display hints, e.g. "m***@gmail.com"
func maskEmail(email string) string {
	at := -1
	for i, c := range email {
		if c == '@' {
			at = i
			break
		}
	}
	if at <= 1 {
		return "***@" + email[at+1:]
	}
	return string(email[0]) + "***@" + email[at+1:]
}

func generateOTP() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	num := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	return fmt.Sprintf("%06d", num%1000000)
}

func sendOTPEmail(toEmail, otp string) {
	resendKey := os.Getenv("RESEND_API_KEY")
	if resendKey == "" {
		log.Warn("RESEND_API_KEY not set, skipping OTP email")
		return
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "send@finkrouter.io.vn"
	}

	htmlTemplate := `
<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; border: 1px solid #eaeaec; border-radius: 8px; overflow: hidden; box-shadow: 0 4px 12px rgba(0,0,0,0.05);">
  <div style="background-color: #0d0d0d; color: #eca8d6; padding: 20px; text-align: center;">
    <h1 style="margin: 0; font-size: 24px; font-weight: 600;">FINKROUTER</h1>
    <p style="margin: 5px 0 0; font-size: 14px; color: #888;">Private network proxy access</p>
  </div>
  <div style="padding: 30px; background-color: #ffffff; color: #333;">
    <h2 style="margin-top: 0; color: #111;">Key Rotation Verification</h2>
    <p style="font-size: 16px; line-height: 1.5;">You have requested to rotate your FinkRouter API key. Please use the verification code below to authorize this action.</p>
    <div style="background-color: #f7f7f9; border: 1px dashed #ccc; padding: 20px; text-align: center; border-radius: 6px; margin: 25px 0;">
      <p style="margin: 0; font-size: 14px; color: #666; margin-bottom: 8px; text-transform: uppercase;">Verification Code</p>
      <code style="font-size: 32px; color: #000; font-weight: bold; letter-spacing: 4px;">%s</code>
    </div>
    <p style="font-size: 14px; line-height: 1.5; color: #d9534f; font-weight: bold;">⚠️ Warning: Rotating your key will permanently revoke your old key.</p>
    <p style="font-size: 14px; line-height: 1.5; color: #666;">This code will expire in 5 minutes. If you did not request this, your account may be compromised. Please contact support immediately.</p>
  </div>
  <div style="background-color: #f9f9f9; padding: 20px; text-align: center; border-top: 1px solid #eee; color: #777; font-size: 12px;">
    <p style="margin: 0 0 5px;">Need help? Contact our support team at <a href="mailto:support@finkrouter.com" style="color: #eca8d6; text-decoration: none;">support@finkrouter.com</a></p>
    <p style="margin: 0;">© 2026 FinkRouter. All rights reserved.</p>
  </div>
</div>`

	payload := map[string]interface{}{
		"from":    fmt.Sprintf("FinkRouter Security <%s>", fromEmail),
		"to":      []string{toEmail},
		"subject": "Your FinkRouter Rotation OTP",
		"html":    fmt.Sprintf(htmlTemplate, otp),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Errorf("sendOTPEmail: failed to create request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("sendOTPEmail: failed to send: %v", err)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Errorf("sendOTPEmail: resend returned status %d for %s: %s", resp.StatusCode, toEmail, string(respBody))
	} else {
		log.Infof("sendOTPEmail: OTP sent to %s", toEmail)
	}
}

// sendPurchaseOTPEmail sends an OTP to verify an email address before purchase.
func sendPurchaseOTPEmail(toEmail, otp string) {
	resendKey := os.Getenv("RESEND_API_KEY")
	if resendKey == "" {
		log.Warn("RESEND_API_KEY not set, skipping purchase OTP email")
		return
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "send@finkrouter.io.vn"
	}

	htmlTemplate := `
<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; border: 1px solid #eaeaec; border-radius: 8px; overflow: hidden; box-shadow: 0 4px 12px rgba(0,0,0,0.05);">
  <div style="background-color: #0d0d0d; color: #eca8d6; padding: 20px; text-align: center;">
    <h1 style="margin: 0; font-size: 24px; font-weight: 600;">FINKROUTER</h1>
    <p style="margin: 5px 0 0; font-size: 14px; color: #888;">Private network proxy access</p>
  </div>
  <div style="padding: 30px; background-color: #ffffff; color: #333;">
    <h2 style="margin-top: 0; color: #111;">Verify Your Email</h2>
    <p style="font-size: 16px; line-height: 1.5;">You are attempting to make a purchase on FinkRouter. Please use the verification code below to proceed.</p>
    <div style="background-color: #f7f7f9; border: 1px dashed #ccc; padding: 20px; text-align: center; border-radius: 6px; margin: 25px 0;">
      <p style="margin: 0; font-size: 14px; color: #666; margin-bottom: 8px; text-transform: uppercase;">Verification Code</p>
      <code style="font-size: 32px; color: #000; font-weight: bold; letter-spacing: 4px;">%s</code>
    </div>
    <p style="font-size: 14px; line-height: 1.5; color: #666;">This code will expire in 5 minutes. If you did not request this, please ignore this email safely.</p>
  </div>
  <div style="background-color: #f9f9f9; padding: 20px; text-align: center; border-top: 1px solid #eee; color: #777; font-size: 12px;">
    <p style="margin: 0 0 5px;">Need help? Contact our support team at <a href="mailto:support@finkrouter.com" style="color: #eca8d6; text-decoration: none;">support@finkrouter.com</a></p>
    <p style="margin: 0;">© 2026 FinkRouter. All rights reserved.</p>
  </div>
</div>`

	payload := map[string]interface{}{
		"from":    fmt.Sprintf("FinkRouter <%s>", fromEmail),
		"to":      []string{toEmail},
		"subject": "Verify your email for FinkRouter purchase",
		"html":    fmt.Sprintf(htmlTemplate, otp),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Errorf("sendPurchaseOTPEmail: failed to create request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("sendPurchaseOTPEmail: failed to send: %v", err)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Errorf("sendPurchaseOTPEmail: resend returned status %d for %s: %s", resp.StatusCode, toEmail, string(respBody))
	} else {
		log.Infof("sendPurchaseOTPEmail: OTP sent to %s", toEmail)
	}
}

func (h *Handler) migrateKeyInFiles(oldKey, newKey string) error {
	liveCfg := middleware.GetLiveConfig()
	if liveCfg == nil {
		return fmt.Errorf("live config is nil")
	}
	cfgFile := h.configFilePath

	// 1. Rewrite config file
	cfgData, err := os.ReadFile(cfgFile)
	if err == nil {
		cfgData = bytes.ReplaceAll(cfgData, []byte(oldKey), []byte(newKey))
		_ = os.WriteFile(cfgFile, cfgData, 0644)
	}

	// 2. Rewrite ledger file
	if liveCfg.PostPayBilling.LedgerFile != "" {
		ledgerPath := filepath.Join(liveCfg.AuthDir, liveCfg.PostPayBilling.LedgerFile)
		ledgerData, err := os.ReadFile(ledgerPath)
		if err == nil {
			ledgerData = bytes.ReplaceAll(ledgerData, []byte(oldKey), []byte(newKey))
			_ = os.WriteFile(ledgerPath, ledgerData, 0644)
			// Trigger reload of postpay by re-saving it or letting persister catch up.
			// SavePostPayUsage marshals memory to disk, so we must mutate memory first!
		}
	}

	// Modify memory config directly so we don't wait for file watcher
	middleware.SwapKeyInConfigMemory(oldKey, newKey)
	middleware.SwapKeyInPostPayMemory(oldKey, newKey)

	return nil
}
