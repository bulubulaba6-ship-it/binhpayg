package management

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
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

	// Insert new key
	_, err = db.Exec("INSERT INTO api_keys (key_hash, key_prefix, email, plan, order_code) VALUES ($1, $2, $3, $4, $5)",
		newHash, newPrefix, email, plan, rotationID)
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
		fromEmail = "onboarding@resend.dev"
	}

	bodyStr := fmt.Sprintf(`{"from": "FinkRouter Security <%s>", "to": ["%s"], "subject": "Your FinkRouter Rotation OTP", "html": "<p>You requested to rotate your API key.</p><p>Your verification code is: <strong style='font-size:24px;'>%s</strong></p><p>This code expires in 5 minutes.</p>"}`, fromEmail, toEmail, otp)

	req, _ := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer([]byte(bodyStr)))
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
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
		fromEmail = "onboarding@resend.dev"
	}

	bodyStr := fmt.Sprintf(`{"from": "FinkRouter <%s>", "to": ["%s"], "subject": "Verify your email for FinkRouter purchase", "html": "<p>You're about to purchase a FinkRouter API key.</p><p>Your verification code is: <strong style='font-size:24px;'>%s</strong></p><p>Enter this code in the checkout form. It expires in 5 minutes.</p><p>If you did not request this, you can safely ignore this email.</p>"}`, fromEmail, toEmail, otp)

	req, _ := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer([]byte(bodyStr)))
	req.Header.Set("Authorization", "Bearer "+resendKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
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
