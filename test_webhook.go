package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"io"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func verifyPayOSSignature(data map[string]interface{}, checksumKey string) string {
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
		case int:
			strVal = strconv.Itoa(val)
		case int64:
			strVal = strconv.FormatInt(val, 10)
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
	return hex.EncodeToString(mac.Sum(nil))
}

func main() {
	dsn := "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"
	fmt.Println("Connecting to DB...")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Println("DB err:", err)
		return
	}
	fmt.Println("Connected to DB")

	orderCode := time.Now().UnixMicro() % 900000000000000
	email := "maiphuocanhtai21032005@gmail.com"
	plan := "MAX 20x"
	amount := int64(1600000)

	fmt.Println("Inserting order...")
	_, err = db.Exec("INSERT INTO payment_orders (order_code, email, plan, amount, status) VALUES ($1, $2, $3, $4, 'pending')", orderCode, email, plan, amount)
	if err != nil {
		fmt.Println("Insert err:", err)
		return
	}
	fmt.Println("Order inserted")

	data := map[string]interface{}{
		"accountNumber": "123",
		"amount": float64(amount),
		"description": "MAX_20x Test",
		"orderCode": float64(orderCode),
	}
	
	checksumKey := "4b79b69e12f73459af9559fcc188fd3197aa1e8eb72507897c2e7883f3ecdd1f" 
	
	sig := verifyPayOSSignature(data, checksumKey)

	payload := map[string]interface{}{
		"code": "00",
		"desc": "success",
		"data": data,
		"signature": sig,
	}

	for i := 0; i < 10; i++ {
		fmt.Println("Sending webhook...")
		body, _ := json.Marshal(payload)
		resp, err := http.Post("http://localhost:8317/payos-webhook", "application/json", bytes.NewBuffer(body))
		if err != nil {
			fmt.Println("Post err:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		
		fmt.Println("Status:", resp.StatusCode)
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Println("Body:", string(respBody))
		resp.Body.Close()
		
		if resp.StatusCode == 200 {
			break
		}
		time.Sleep(10 * time.Second)
	}
}
