package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func checkDSN(name string, dsn string) {
	fmt.Printf("\n=== Checking DB: %s ===\n", name)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Printf("[%s] Open error: %v\n", name, err)
		return
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Printf("[%s] Ping error: %v\n", name, err)
		return
	}

	// 1. List all tables
	rows, err := db.Query(`
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public'
	`)
	if err != nil {
		log.Printf("[%s] Query tables error: %v\n", name, err)
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			tables = append(tables, t)
		}
	}
	fmt.Printf("[%s] Tables: %v\n", name, tables)

	// 2. Search for the key in config_store table
	var configContent string
	err = db.QueryRow("SELECT content FROM config_store WHERE id = 'config'").Scan(&configContent)
	if err != nil {
		log.Printf("[%s] config_store id='config' query error: %v\n", name, err)
	} else {
		containsTarget := stringsContains(configContent, "fink_9e9027598631")
		fmt.Printf("[%s] config_store 'config' contains target key prefix: %t\n", name, containsTarget)

		// Let's print out lines containing "fink" or "api-key"
		for _, line := range stringsSplit(configContent, "\n") {
			if stringsContains(line, "fink") || stringsContains(line, "api-key") {
				fmt.Printf("[%s] Config line: %s\n", name, line)
			}
		}
	}

	// 3. Search in api_keys table if exists
	hasApiKeys := false
	for _, t := range tables {
		if t == "api_keys" {
			hasApiKeys = true
		}
	}
	if hasApiKeys {
		rowsAK, errAK := db.Query("SELECT id, key_hash, key_prefix, email, plan, status FROM api_keys")
		if errAK != nil {
			log.Printf("[%s] Query api_keys error: %v\n", name, errAK)
		} else {
			defer rowsAK.Close()
			fmt.Printf("[%s] api_keys rows:\n", name)
			for rowsAK.Next() {
				var id int
				var keyHash, keyPrefix, email, plan, status sql.NullString
				if err := rowsAK.Scan(&id, &keyHash, &keyPrefix, &email, &plan, &status); err == nil {
					fmt.Printf("  id=%d, hash=%s, prefix=%s, email=%s, plan=%s, status=%s\n",
						id, keyHash.String, keyPrefix.String, email.String, plan.String, status.String)
				}
			}
		}
	}

	// 4. Search in payment_orders table if exists
	hasPaymentOrders := false
	for _, t := range tables {
		if t == "payment_orders" {
			hasPaymentOrders = true
		}
	}
	if hasPaymentOrders {
		rowsPO, errPO := db.Query("SELECT order_code, email, plan, status FROM payment_orders")
		if errPO != nil {
			log.Printf("[%s] Query payment_orders error: %v\n", name, errPO)
		} else {
			defer rowsPO.Close()
			fmt.Printf("[%s] payment_orders rows:\n", name)
			for rowsPO.Next() {
				var orderCode string
				var email, plan, status sql.NullString
				if err := rowsPO.Scan(&orderCode, &email, &plan, &status); err == nil {
					fmt.Printf("  orderCode=%s, email=%s, plan=%s, status=%s\n",
						orderCode, email.String, plan.String, status.String)
				}
			}
		}
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && stringsIndexOf(s, substr) >= 0))
}

func stringsIndexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func stringsSplit(s, sep string) []string {
	var result []string
	if sep == "" {
		for _, r := range s {
			result = append(result, string(r))
		}
		return result
	}
	start := 0
	for {
		idx := stringsIndexOf(s[start:], sep)
		if idx == -1 {
			result = append(result, s[start:])
			break
		}
		result = append(result, s[start:start+idx])
		start += idx + len(sep)
	}
	return result
}

func main() {
	checkDSN("ENV_PGSTORE_DSN (maiphuocanhtai21032005)", "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require")
	checkDSN("ZOK210305_DSN", "postgres://avnadmin:AVNS_gMMqDMXI4MLgS0D6m01@pg-34ab11a6-zok210305-4ca5.g.aivencloud.com:10436/defaultdb?sslmode=require")
}
