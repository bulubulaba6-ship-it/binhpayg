package management

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestPostPayOSWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Set env vars
	os.Setenv("PAYOS_CHECKSUM_KEY", "4b79b69e12f73459af9559fcc188fd3197aa1e8eb72507897c2e7883f3ecdd1f")
	defer os.Unsetenv("PAYOS_CHECKSUM_KEY")

	// Create test handler
	cfg := &config.Config{
		Host: "127.0.0.1",
		Port: 8080,
	}
	h := NewHandlerWithoutConfigFilePath(cfg, nil)

	// Create router
	r := gin.New()
	r.POST("/payos-webhook", h.PostPayOSWebhook)

	// Create valid payload
	payload := map[string]interface{}{
		"code":    "00",
		"desc":    "success",
		"success": true,
		"data": map[string]interface{}{
			"orderCode":              123,
			"amount":                 3000,
			"description":            "VQRIO123",
			"accountNumber":          "12345678",
			"reference":              "TF230204212323",
			"transactionDateTime":    "2023-02-04 18:25:00",
			"currency":               "VND",
			"paymentLinkId":          "124c33293c43417ab7879e14c8d9eb18",
			"code":                   "00",
			"desc":                   "Thành công",
			"counterAccountBankId":   "",
			"counterAccountBankName": "",
			"counterAccountName":     "",
			"counterAccountNumber":   "",
			"virtualAccountName":     "",
			"virtualAccountNumber":   "",
		},
		"signature": "b43e37465a041ff410a4e23b0543add84b3fead12d9894a22996d776d1e40fa4",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/payos-webhook", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, float64(0), resp["error"])
	assert.Equal(t, "Ok", resp["message"])

	// Assert key was added to config
	assert.Len(t, cfg.APIKeys, 1)
	assert.True(t, len(cfg.APIKeys[0]) > 10)
}
