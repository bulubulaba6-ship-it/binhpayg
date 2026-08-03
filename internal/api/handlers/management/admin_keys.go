package management

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api/middleware"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// AdminKeyInfo is the response shape for a single API key in the admin dashboard.
type AdminKeyInfo struct {
	KeyPrefix        string     `json:"key_prefix"`
	KeyMasked        string     `json:"key_masked"` // e.g. "fink_max_a1b2****c3d4"
	Plan             string     `json:"plan"`
	Status           string     `json:"status"` // "active" | "inactive" | "expired" | "config_error"
	Email            string     `json:"email"`
	CreditsConsumed  float64    `json:"credits_consumed"`
	CreditsPurchased float64    `json:"credits_purchased"`
	CreditsRemaining float64    `json:"credits_remaining"`
	DailyBurnToday   float64    `json:"daily_burn_today"`
	BurnMultiplier   float64    `json:"burn_multiplier"`
	BurnRatio        float64    `json:"burn_ratio"`                    // 0.0–1.0+ fraction of 5H window consumed
	FiveHCredits     float64    `json:"five_h_credits"`                // credits billed in current 5H window
	FiveHWindowStart *time.Time `json:"five_h_window_start,omitempty"` // UTC window open time
	FiveHLimit       int        `json:"five_h_limit"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	IsExpired        bool       `json:"is_expired"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
	TotalRequests    int64      `json:"total_requests"`
	TotalTokens      int64      `json:"total_tokens"`
	// InBilling is false when this key is in api-keys but missing from
	// post-pay-billing.clients. Such keys bypass the credit ceiling entirely
	// and must be added to the billing section or removed.
	InBilling   bool   `json:"in_billing"`
	ConfigError string `json:"config_error,omitempty"`
}

// AdminOrderInfo is the response shape for a payment order.
type AdminOrderInfo struct {
	OrderCode string     `json:"order_code"`
	Email     string     `json:"email"`
	Plan      string     `json:"plan"`
	Amount    int64      `json:"amount_vnd"`
	Phone     string     `json:"phone,omitempty"`
	Status    string     `json:"status"` // "pending" | "paid" | "cancelled"
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// maskKey returns a masked version of the key for safe display in admin UI:
// e.g. "fink_max_a1b2****c3d4" — prefix preserved, middle hidden, last 4 shown.
func maskKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	suffix := key[len(key)-4:]
	for _, p := range []string{"fink_max_", "fink_pro_", "fink_d", "fink_"} {
		if strings.HasPrefix(key, p) {
			rest := key[len(p):]
			if len(rest) >= 8 {
				return p + rest[:4] + "****" + suffix
			}
			return p + "****" + suffix
		}
	}
	return key[:8] + "****" + suffix
}

// planFromKey infers the plan tier from the key prefix.
func planFromKey(key string) string {
	switch {
	case strings.HasPrefix(key, "fink_max_"):
		return "MAX"
	case strings.HasPrefix(key, "fink_pro_"):
		return "PRO"
	case strings.HasPrefix(key, "fink_d"):
		return "DAYPASS"
	default:
		return "PAYG"
	}
}

// GetAdminKeys returns full info for all post-pay API keys.
// It unions post-pay-billing.clients AND api-keys so keys that are in api-keys
// but missing from billing (bypassing the credit ceiling) are always surfaced
// with status "config_error" and in_billing=false.
// GET /v0/management/admin/keys
// Protected by management group middleware.
func (h *Handler) GetAdminKeys(c *gin.Context) {

	cfg := middleware.GetLiveConfig()
	if cfg == nil || !cfg.PostPayBilling.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "post-pay billing not enabled"})
		return
	}

	// Snapshot in-memory ledger
	ledger := middleware.GetPostPaySnapshot()
	// Snapshot live config clients
	clients := cfg.PostPayBilling.Clients
	rateLimits := cfg.APIKeyLimits

	// Attempt to fetch emails from DB
	emailByKey := make(map[string]string)
	createdByKey := make(map[string]*time.Time)
	db := getWebhookDB()
	if db != nil {
		rows, err := db.QueryContext(c.Request.Context(),
			`SELECT key_hash, email, created_at FROM api_keys ORDER BY created_at DESC LIMIT 2000`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var keyHash, email string
				var createdRaw sql.NullString
				if errScan := rows.Scan(&keyHash, &email, &createdRaw); errScan == nil {
					emailByKey[keyHash] = email
					if createdRaw.Valid && createdRaw.String != "" {
						if ts, err := strconv.ParseInt(createdRaw.String, 10, 64); err == nil {
							t := time.Unix(ts, 0)
							createdByKey[keyHash] = &t
						}
					}
				}
			}
		}
	}

	// Build the union of all keys: billing clients + raw api-keys.
	// Keys present in api-keys but absent from billing clients bypass the
	// credit-ceiling kill-switch — expose them with status "config_error"
	// so operators can correct the configuration immediately.
	allKeys := make(map[string]struct{}, len(clients)+len(cfg.APIKeys))
	for key := range clients {
		allKeys[key] = struct{}{}
	}
	for _, key := range cfg.APIKeys {
		allKeys[key] = struct{}{}
	}

	now := time.Now().UTC()
	result := make([]AdminKeyInfo, 0, len(allKeys))

	for key := range allKeys {
		clientCfg, inBilling := clients[key]
		entry := ledger[key]

		keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
		email := emailByKey[keyHash]

		var consumed, purchased float64
		var totalReq, totalTokens int64
		if entry.CreditsConsumed > 0 || entry.CreditsPurchased > 0 {
			consumed = entry.CreditsConsumed
			purchased = entry.CreditsPurchased
			totalReq = entry.Success + entry.Failed
			totalTokens = entry.TotalTokens
		}

		if inBilling && purchased == 0 && clientCfg.CreditLimit > 0 {
			purchased = float64(clientCfg.CreditLimit)
		}

		remaining := purchased - consumed
		if remaining < 0 {
			remaining = 0
		}

		isExpired := false
		var expiresAt *time.Time
		if inBilling && !clientCfg.ExpiresAt.IsZero() {
			t := clientCfg.ExpiresAt
			expiresAt = &t
			if now.After(clientCfg.ExpiresAt) {
				isExpired = true
			}
		}

		var configErr string
		status := "active"
		switch {
		case !inBilling:
			// Key is active on the proxy but bypasses all billing controls.
			status = "config_error"
			configErr = "missing from post-pay-billing.clients — key bypasses credit ceiling"
		case isExpired:
			status = "expired"
		case remaining <= 0 && purchased > 0:
			status = "exhausted"
		}

		fiveHLimit := 0
		hasCustomLimit := false
		if rateLimits != nil {
			if lim, ok := rateLimits[key]; ok {
				fiveHLimit = lim
				hasCustomLimit = true
			}
		}

		if !hasCustomLimit && cfg != nil {
			fiveHLimit = cfg.DefaultAPIKeyLimit
			isPAYG := strings.HasPrefix(key, "fink_") && !strings.HasPrefix(key, "fink_max_") && !strings.HasPrefix(key, "fink_pro_") && !strings.HasPrefix(key, "fink_d")

			// PAYG has no burst threshold (it is strictly 1.0x).
			if isPAYG {
				fiveHLimit = 0
			}
		}

		fiveHCredits := middleware.GetFiveHCreditsForKey(key)
		fiveHWindowStart := middleware.GetFiveHWindowStartForKey(key)

		var fiveHWindowStartPtr *time.Time
		var burnRatio float64
		if !fiveHWindowStart.IsZero() {
			t := fiveHWindowStart
			fiveHWindowStartPtr = &t
			if fiveHLimit > 0 {
				burnRatio = fiveHCredits / float64(fiveHLimit)
			}
		}

		created := createdByKey[keyHash]
		if created == nil && expiresAt != nil {
			// Infer the creation date for older keys that aren't fully mapped in the DB
			// assuming standard 30-day subscriptions for PRO/MAX.
			t := expiresAt.Add(-30 * 24 * time.Hour)
			created = &t
		}

		result = append(result, AdminKeyInfo{
			KeyPrefix:        planFromKey(key),
			KeyMasked:        maskKey(key),
			Plan:             planFromKey(key),
			Status:           status,
			Email:            email,
			CreditsConsumed:  consumed,
			CreditsPurchased: purchased,
			CreditsRemaining: remaining,
			DailyBurnToday:   middleware.GetDailyBurnForKey(key),
			BurnMultiplier:   middleware.GetBurnMultiplierForKey(key),
			BurnRatio:        burnRatio,
			FiveHCredits:     fiveHCredits,
			FiveHWindowStart: fiveHWindowStartPtr,
			FiveHLimit:       fiveHLimit,
			ExpiresAt:        expiresAt,
			IsExpired:        isExpired,
			CreatedAt:        created,
			TotalRequests:    totalReq,
			TotalTokens:      totalTokens,
			InBilling:        inBilling,
			ConfigError:      configErr,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"count": len(result),
		"keys":  result,
	})
}

// GetAdminOrders returns payment orders from the database.
// GET /v0/management/admin/orders?status=pending|paid|all
// Protected by management group middleware.
func (h *Handler) GetAdminOrders(c *gin.Context) {

	db := getWebhookDB()
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database not available"})
		return
	}

	statusFilter := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if statusFilter == "" || statusFilter == "all" {
		statusFilter = ""
	}

	var rows *sql.Rows
	var errQ error
	if statusFilter != "" {
		rows, errQ = db.QueryContext(c.Request.Context(),
			`SELECT order_code, email, plan, amount, COALESCE(phone,''), status, created_at
			 FROM payment_orders WHERE status = $1 ORDER BY created_at DESC LIMIT 500`,
			statusFilter)
	} else {
		rows, errQ = db.QueryContext(c.Request.Context(),
			`SELECT order_code, email, plan, amount, COALESCE(phone,''), status, created_at
			 FROM payment_orders ORDER BY created_at DESC LIMIT 500`)
	}
	if errQ != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db query failed: " + errQ.Error()})
		return
	}
	defer rows.Close()

	orders := make([]AdminOrderInfo, 0)
	for rows.Next() {
		var o AdminOrderInfo
		var createdRaw sql.NullTime
		var orderCode string
		if err := rows.Scan(&orderCode, &o.Email, &o.Plan, &o.Amount, &o.Phone, &o.Status, &createdRaw); err != nil {
			continue
		}
		o.OrderCode = orderCode
		if createdRaw.Valid {
			t := createdRaw.Time
			o.CreatedAt = &t
		}
		orders = append(orders, o)
	}

	pending := 0
	paid := 0
	for _, o := range orders {
		switch o.Status {
		case "pending":
			pending++
		case "paid":
			paid++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total":   len(orders),
		"pending": pending,
		"paid":    paid,
		"orders":  orders,
	})
}

// PostAdminKeyDeactivate sets a key's ExpiresAt to now (effectively immediately blocking it).
// POST /v0/management/admin/keys/deactivate
func (h *Handler) PostAdminKeyDeactivate(c *gin.Context) {
	h.setKeyExpiry(c, time.Now().UTC().Add(-1*time.Second))
}

// PostAdminKeyActivate re-activates a key by clearing its ExpiresAt.
// POST /v0/management/admin/keys/activate
func (h *Handler) PostAdminKeyActivate(c *gin.Context) {
	h.setKeyExpiry(c, time.Time{}) // zero = no expiry
}

// PostAdminKeyExpireNow hard-expires the key immediately.
// POST /v0/management/admin/keys/expire
func (h *Handler) PostAdminKeyExpireNow(c *gin.Context) {
	h.setKeyExpiry(c, time.Now().UTC().Add(-1*time.Second))
}

type keyActionRequest struct {
	Key string `json:"key"`
}

func (h *Handler) setKeyExpiry(c *gin.Context, newExpiry time.Time) {
	var req keyActionRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must have {\"key\": \"<full api key>\"}"})
		return
	}
	key := strings.TrimSpace(req.Key)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cfg.PostPayBilling.Clients == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found in billing config"})
		return
	}

	var targetKey string
	if _, ok := h.cfg.PostPayBilling.Clients[key]; ok {
		targetKey = key
	} else {
		// allow looking up by masked key or hash for admin convenience
		for k := range h.cfg.PostPayBilling.Clients {
			if maskKey(k) == key || fmt.Sprintf("%x", sha256.Sum256([]byte(k))) == key {
				targetKey = k
				break
			}
		}
	}

	if targetKey == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "key not found in billing config"})
		return
	}

	latestCfg, err := config.LoadConfig(h.configFilePath)
	if err == nil {
		clientCfg := latestCfg.PostPayBilling.Clients[targetKey]
		clientCfg.ExpiresAt = newExpiry
		latestCfg.PostPayBilling.Clients[targetKey] = clientCfg

		if err := config.SaveConfigPreserveComments(h.configFilePath, latestCfg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config: " + err.Error()})
			return
		}
		// Apply the updated expiry to the live in-memory config immediately so
		// the kill-switch takes effect on the next request without a redeploy.
		middleware.SetClientQuotaConfig(latestCfg)
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load config: " + err.Error()})
		return
	}

	action := "deactivated"
	if newExpiry.IsZero() {
		action = "activated"
	}

	// Also update DB api_keys.status for persistence
	db := getWebhookDB()
	if db != nil {
		dbStatus := "inactive"
		if newExpiry.IsZero() {
			dbStatus = "active"
		}
		keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(targetKey)))
		_, _ = db.ExecContext(c.Request.Context(),
			`UPDATE api_keys SET status = $1 WHERE key_hash = $2`, dbStatus, keyHash)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"action":  action,
		"key":     maskKey(key),
		"expires": newExpiry,
	})
}

// GetAdminSummary returns a high-level summary for the admin dashboard.
// GET /v0/management/admin/summary
func (h *Handler) GetAdminSummary(c *gin.Context) {
	cfg := middleware.GetLiveConfig()
	if cfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config not available"})
		return
	}

	ledger := middleware.GetPostPaySnapshot()
	clients := cfg.PostPayBilling.Clients

	now := time.Now().UTC()
	var (
		totalKeys      int
		activeKeys     int
		expiredKeys    int
		exhaustedKeys  int
		totalConsumed  float64
		totalPurchased float64
		totalRevenue   float64
	)

	for key, clientCfg := range clients {
		totalKeys++
		entry := ledger[key]
		totalConsumed += entry.CreditsConsumed
		totalPurchased += entry.CreditsPurchased

		isExpired := !clientCfg.ExpiresAt.IsZero() && now.After(clientCfg.ExpiresAt)
		isExhausted := entry.CreditsConsumed >= entry.CreditsPurchased && entry.CreditsPurchased > 0

		switch {
		case isExpired:
			expiredKeys++
		case isExhausted:
			exhaustedKeys++
		default:
			activeKeys++
		}
	}

	// Try to get revenue from DB
	db := getWebhookDB()
	if db != nil {
		row := db.QueryRowContext(c.Request.Context(),
			`SELECT COALESCE(SUM(amount),0) FROM payment_orders WHERE status='paid'`)
		_ = row.Scan(&totalRevenue)
	}

	c.JSON(http.StatusOK, gin.H{
		"total_keys":        totalKeys,
		"active_keys":       activeKeys,
		"expired_keys":      expiredKeys,
		"exhausted_keys":    exhaustedKeys,
		"total_consumed":    totalConsumed,
		"total_purchased":   totalPurchased,
		"total_revenue_vnd": totalRevenue,
		"as_of":             now,
	})
}

// adminKeyRegisterRequest is the payload for PostAdminKeyRegister.
type adminKeyRegisterRequest struct {
	Key     string  `json:"key"`               // full plaintext API key (required)
	Email   string  `json:"email"`             // owner email — shown in admin dashboard
	Credits float64 `json:"credits"`           // credits to seed into the ledger (plan allowance)
	TxnID   string  `json:"txn_id,omitempty"`  // optional idempotency token
}

// PostAdminKeyRegister backfills the DB and ledger for a key that was added manually to
// config.yaml without going through the PayOS/provision flow.
//
// It performs two actions that the manual config edit skips:
//
//  1. DB: inserts a row into api_keys (key_hash, prefix, email, plan, status, order_code)
//     so the key shows up with correct metadata in the admin dashboard.
//  2. Ledger: calls ProcessDepositCredits to set creditsPurchased = credits, which makes
//     balance enforcement and tier calculations correct from day one.
//
// Idempotent: if the DB row already exists (UNIQUE constraint on key_hash) it skips the
// insert and just re-seeds the ledger (also idempotent via txn_id dedup).
//
// POST /v0/management/admin/keys/register
// Protected by management group middleware.
func (h *Handler) PostAdminKeyRegister(c *gin.Context) {
	var req adminKeyRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Key) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body must have {\"key\": \"<full api key>\", \"email\": \"...\", \"credits\": <number>}"})
		return
	}
	key := strings.TrimSpace(req.Key)
	credits := req.Credits

	// Validate key is present in live config — refuse to register unknown keys.
	liveCfg := middleware.GetLiveConfig()
	if liveCfg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config not available"})
		return
	}
	keyInConfig := false
	for _, k := range liveCfg.APIKeys {
		if k == key {
			keyInConfig = true
			break
		}
	}
	if !keyInConfig {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key not found in api-keys config — add it to config first"})
		return
	}
	if _, inBilling := liveCfg.PostPayBilling.Clients[key]; !inBilling {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key not found in post-pay-billing.clients — add it to billing config first"})
		return
	}

	plan := planFromKey(key)
	prefix := ""
	switch {
	case strings.HasPrefix(key, "fink_max_"):
		prefix = "fink_max_"
	case strings.HasPrefix(key, "fink_pro_"):
		prefix = "fink_pro_"
	default:
		prefix = "fink_"
	}

	keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))

	// Step 1: Insert into DB api_keys (idempotent — skip if key_hash already exists).
	dbRegistered := false
	dbSkipped := false
	db := getWebhookDB()
	if db != nil {
		// Use a synthetic order_code for manually registered keys so the UNIQUE constraint is satisfied.
		syntheticOrder := fmt.Sprintf("manual-admin-%s", keyHash[:12])
		_, errInsert := db.ExecContext(c.Request.Context(), `
			INSERT INTO api_keys (name, key_hash, key_prefix, email, plan, status, order_code, created_at)
			VALUES ($1, $2, $3, $4, $5, 'active', $6, $7)
			ON CONFLICT (key_hash) DO NOTHING
		`, plan, keyHash, prefix, req.Email, plan, syntheticOrder, time.Now().Unix())
		if errInsert != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert failed: " + errInsert.Error()})
			return
		}

		// Check whether the row was actually inserted or skipped (ON CONFLICT DO NOTHING).
		var existingID int
		errCheck := db.QueryRowContext(c.Request.Context(),
			`SELECT id FROM api_keys WHERE key_hash = $1`, keyHash).Scan(&existingID)
		if errCheck == nil && existingID > 0 {
			dbRegistered = true
		}
	} else {
		// No DB configured — this is fine, ledger-only registration still works.
		dbSkipped = true
	}

	// Step 2: Seed creditsPurchased in the in-memory ledger via ProcessDepositCredits.
	// Use a deterministic txn_id based on key hash to prevent double-seeding on retries.
	txnID := req.TxnID
	if txnID == "" {
		txnID = fmt.Sprintf("manual-register-%s", keyHash[:16])
	}
	var addedCredits, newTotal float64
	var isDuplicate bool
	if credits > 0 {
		addedCredits, newTotal, isDuplicate = middleware.ProcessDepositCredits(key, credits, txnID)
	} else {
		// credits=0 means caller just wants DB registration without touching the ledger.
		snapshot := middleware.GetPostPaySnapshot()
		if e, ok := snapshot[key]; ok {
			newTotal = e.CreditsPurchased
		}
	}

	// Step 3: Flush ledger to disk + Postgres immediately.
	if liveCfg.PostPayBilling.LedgerFile != "" {
		fullPath := liveCfg.AuthDir + "/" + liveCfg.PostPayBilling.LedgerFile
		_ = middleware.SavePostPayUsage(fullPath)
	}

	result := gin.H{
		"status":          "ok",
		"key":             maskKey(key),
		"plan":            plan,
		"email":           req.Email,
		"credits_seeded":  addedCredits,
		"total_purchased": newTotal,
		"duplicate_txn":   isDuplicate,
		"db_registered":   dbRegistered,
		"db_skipped_no_db": dbSkipped,
	}

	if isDuplicate {
		result["status"] = "duplicate_txn"
		result["note"] = "ledger already seeded with this txn_id — use a different txn_id to force re-seed"
	}

	c.JSON(http.StatusOK, result)
}

// PostAdminKeyGenerate generates a new API key and updates config.yaml directly,
// initializing it with models and billing settings.
func (h *Handler) PostAdminKeyGenerate(c *gin.Context) {
	var req adminKeyRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	plan := strings.ToUpper(strings.TrimSpace(c.Query("plan")))
	if plan == "" {
		plan = "MAX"
	}
	prefix := ""
	switch plan {
	case "PRO":
		prefix = "fink_pro_"
	case "MAX":
		prefix = "fink_max_"
	default:
		prefix = "fink_"
		plan = "PAYG"
	}

	raw := make([]byte, 16)
	if _, errRand := rand.Read(raw); errRand != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
		return
	}
	newKey := prefix + hex.EncodeToString(raw)

	// Update Config
	h.mu.Lock()
	latestCfg, err := config.LoadConfig(h.configFilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load config: " + err.Error()})
		h.mu.Unlock()
		return
	}

	latestCfg.APIKeys = append(latestCfg.APIKeys, newKey)
	if latestCfg.APIKeyModels == nil {
		latestCfg.APIKeyModels = make(map[string]map[string][]string)
	}
	latestCfg.APIKeyModels[newKey] = map[string][]string{
		"claude": AllowedClaude,
		"openai": AllowedOpenAI,
	}

	if latestCfg.PostPayBilling.Clients == nil {
		latestCfg.PostPayBilling.Clients = make(map[string]config.PostPayBillingClientCfg)
	}
	latestCfg.PostPayBilling.Clients[newKey] = config.PostPayBillingClientCfg{
		CreditLimit: 2e6,
	}

	if err := config.SaveConfigPreserveComments(h.configFilePath, latestCfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config: " + err.Error()})
		h.mu.Unlock()
		return
	}
	h.cfg = latestCfg
	h.mu.Unlock()

	keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(newKey)))

	// Insert into Postgres DB
	dbRegistered := false
	dbSkipped := false
	db := getWebhookDB()
	if db != nil {
		syntheticOrder := fmt.Sprintf("manual-gen-%s", keyHash[:12])
		_, errInsert := db.ExecContext(c.Request.Context(), `
			INSERT INTO api_keys (name, key_hash, key_prefix, email, plan, status, order_code, created_at)
			VALUES ($1, $2, $3, $4, $5, 'active', $6, $7)
			ON CONFLICT (key_hash) DO NOTHING
		`, plan, keyHash, prefix, req.Email, plan, syntheticOrder, time.Now().Unix())
		if errInsert == nil {
			var existingID int
			errCheck := db.QueryRowContext(c.Request.Context(),
				`SELECT id FROM api_keys WHERE key_hash = $1`, keyHash).Scan(&existingID)
			if errCheck == nil && existingID > 0 {
				dbRegistered = true
			}
		}
	} else {
		dbSkipped = true
	}

	// Process Credits in Ledger
	txnID := fmt.Sprintf("manual-gen-%s", keyHash[:16])
	var addedCredits, newTotal float64
	var isDuplicate bool
	if req.Credits > 0 {
		addedCredits, newTotal, isDuplicate = middleware.ProcessDepositCredits(newKey, req.Credits, txnID)
	} else {
		snapshot := middleware.GetPostPaySnapshot()
		if e, ok := snapshot[newKey]; ok {
			newTotal = e.CreditsPurchased
		}
	}

	// Flush Ledger
	if latestCfg.PostPayBilling.LedgerFile != "" {
		fullPath := latestCfg.AuthDir + "/" + latestCfg.PostPayBilling.LedgerFile
		_ = middleware.SavePostPayUsage(fullPath)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "ok",
		"key":             newKey,
		"plan":            plan,
		"email":           req.Email,
		"credits_seeded":  addedCredits,
		"total_purchased": newTotal,
		"duplicate_txn":   isDuplicate,
		"db_registered":   dbRegistered,
		"db_skipped_no_db": dbSkipped,
	})
}
