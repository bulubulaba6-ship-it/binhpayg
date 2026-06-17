package management

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type CreatePaymentLinkRequest struct {
	Plan         string `json:"plan"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	CustomAmount int64  `json:"custom_amount"`
	OTP          string `json:"otp"`
}

type PayOSPaymentData struct {
	OrderCode   int64  `json:"orderCode"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	CancelUrl   string `json:"cancelUrl"`
	ReturnUrl   string `json:"returnUrl"`
	Signature   string `json:"signature,omitempty"`
}

type PurchaseOTPRequest struct {
	Email string `json:"email"`
}

// PostSendPurchaseOTP generates an OTP and emails it to the user to verify their email before purchase.
func (h *Handler) PostSendPurchaseOTP(c *gin.Context) {
	var req PurchaseOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
		return
	}

	// Rate limit: max 3 OTP requests per email per hour
	db := getWebhookDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database unavailable"})
		return
	}

	// Hash the email to produce a stable 64-char hex key_hash that always fits VARCHAR(64).
	// Prefix with a fixed byte so purchase OTPs never collide with key rotation OTPs.
	emailHashBytes := sha256.Sum256([]byte("purchase:" + req.Email))
	emailHash := fmt.Sprintf("%x", emailHashBytes)

	var recentCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM otp_codes WHERE key_hash = $1 AND created_at > NOW() - INTERVAL '1 hour'", emailHash).Scan(&recentCount)
	if recentCount >= 3 {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many OTP requests, please wait before trying again"})
		return
	}

	// Clean up expired OTPs for this email
	_, _ = db.Exec("DELETE FROM otp_codes WHERE key_hash = $1 AND expires_at < NOW()", emailHash)

	// Generate 6-digit OTP
	otpCode := generateOTP()
	expiresAt := time.Now().Add(5 * time.Minute)

	// Save OTP
	_, err := db.Exec("INSERT INTO otp_codes (key_hash, otp_code, expires_at) VALUES ($1, $2, $3)", emailHash, otpCode, expiresAt)
	if err != nil {
		log.Errorf("failed to insert otp: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
		return
	}

	// Email OTP
	go sendPurchaseOTPEmail(req.Email, otpCode)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OTP sent to your email"})
}

// PostCreatePaymentLink creates a payment link via payOS and stores the mapping in Postgres
func (h *Handler) PostCreatePaymentLink(c *gin.Context) {
	var req CreatePaymentLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
		return
	}

	plan := strings.ToLower(strings.TrimSpace(req.Plan))
	var amount int64 = 0

	switch plan {
	case "max_20x":
		amount = 1800000
	case "max":
		amount = 650000
	case "pro":
		amount = 350000
	case "day7":
		amount = 150000
	case "day1":
		amount = 50000
	case "payg":
		if req.CustomAmount < 50000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Pay-as-you-go requires a minimum of 50,000 VND"})
			return
		}
		amount = req.CustomAmount
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plan"})
		return
	}

	req.OTP = strings.TrimSpace(req.OTP)
	if req.OTP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OTP is required"})
		return
	}

	// Verify OTP
	db := getWebhookDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not configured"})
		return
	}

	emailHashBytes := sha256.Sum256([]byte("purchase:" + req.Email))
	emailHash := fmt.Sprintf("%x", emailHashBytes)
	var validOTP string
	err := db.QueryRow("SELECT otp_code FROM otp_codes WHERE key_hash = $1 AND expires_at > NOW() ORDER BY created_at DESC LIMIT 1", emailHash).Scan(&validOTP)
	if err != nil || validOTP != req.OTP {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired OTP"})
		return
	}

	// Consume OTP
	_, _ = db.Exec("DELETE FROM otp_codes WHERE key_hash = $1", emailHash)

	clientID := os.Getenv("PAYOS_CLIENT_ID")
	apiKey := os.Getenv("PAYOS_API_KEY")
	checksumKey := os.Getenv("PAYOS_CHECKSUM_KEY")

	if clientID == "" || apiKey == "" || checksumKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment gateway not configured"})
		return
	}

	// Generate unique 15-digit order code (payOS requirement: max 9007199254740991)
	// UnixNano is 19 digits. We can use UnixMicro which is 16 digits, then truncate or take modulo.
	orderCode := time.Now().UnixMicro() % 900000000000000

	// Sanitize optional phone
	req.Phone = strings.TrimSpace(req.Phone)

	// Store in Postgres
	_, err = db.Exec(`
		INSERT INTO payment_orders (order_code, email, plan, amount, phone)
		VALUES ($1, $2, $3, $4, $5)
	`, orderCode, req.Email, plan, amount, req.Phone)
	if err != nil {
		log.Errorf("Failed to insert payment order: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal database error"})
		return
	}

	protocol := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		protocol = "https"
	}
	host := c.Request.Host
	returnUrl := fmt.Sprintf("%s://%s/dashboard?status=PAID", protocol, host)
	cancelUrl := fmt.Sprintf("%s://%s/dashboard?cancel=true", protocol, host)

	// Description format: "plan lastDigits" e.g. "day1 496240"
	// Do NOT include email prefix — it leaks user info and clutters the bank statement.
	orderStr := fmt.Sprintf("%d", orderCode)
	if len(orderStr) > 6 {
		orderStr = orderStr[len(orderStr)-6:]
	}
	desc := fmt.Sprintf("%s %s", plan, orderStr)
	if plan == "payg" {
		desc = fmt.Sprintf("%dk %s", amount/1000, orderStr)
	}
	if len(desc) > 25 {
		desc = desc[:25]
	}

	paymentData := PayOSPaymentData{
		OrderCode:   orderCode,
		Amount:      amount,
		Description: desc,
		CancelUrl:   cancelUrl,
		ReturnUrl:   returnUrl,
	}

	// Create Signature
	// format: amount={amount}&cancelUrl={cancelUrl}&description={description}&orderCode={orderCode}&returnUrl={returnUrl}
	signStr := fmt.Sprintf("amount=%d&cancelUrl=%s&description=%s&orderCode=%d&returnUrl=%s",
		paymentData.Amount, paymentData.CancelUrl, paymentData.Description, paymentData.OrderCode, paymentData.ReturnUrl)

	mac := hmac.New(sha256.New, []byte(checksumKey))
	mac.Write([]byte(signStr))
	paymentData.Signature = hex.EncodeToString(mac.Sum(nil))

	// Call payOS
	reqBody, _ := json.Marshal(paymentData)
	httpReq, _ := http.NewRequest("POST", "https://api-merchant.payos.vn/v2/payment-requests", bytes.NewBuffer(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-client-id", clientID)
	httpReq.Header.Set("x-api-key", apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Errorf("payos API request failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to contact payment gateway"})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var payosResp struct {
		Code string `json:"code"`
		Desc string `json:"desc"`
		Data struct {
			CheckoutUrl string `json:"checkoutUrl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payosResp); err != nil || payosResp.Code != "00" {
		log.Errorf("payos API error: %s, code: %s", payosResp.Desc, payosResp.Code)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment gateway rejected request"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"checkoutUrl": payosResp.Data.CheckoutUrl})
}
