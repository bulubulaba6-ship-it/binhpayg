package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var content string
	err = db.QueryRow("SELECT content FROM auth_store WHERE id = 'ledger'").Scan(&content)
	if err != nil {
		log.Fatalf("failed to read ledger: %v", err)
	}

	var ledger map[string]interface{}
	if err := json.Unmarshal([]byte(content), &ledger); err != nil {
		log.Fatalf("failed to parse json: %v", err)
	}

	keys := []string{
		"fink_6db8bcc04240f16a34398b9319d5a1da",
		"fink_54a9733e264f5e61949735a2098cfb70",
		"fink_eb6924786c88368ab2ef52d99d749f65",
		"fink_547a2dd57b7c148f09478206be8f4755",
	}

	for _, k := range keys {
		entry, ok := ledger[k].(map[string]interface{})
		if !ok {
			entry = make(map[string]interface{})
			ledger[k] = entry
		}
		// Set credits_consumed to credits_purchased if purchased > 0, else 50k
		purchased := 0.0
		if p, ok := entry["CreditsPurchased"].(float64); ok && p > 0 {
			purchased = p
		}
		if purchased > 0 {
			entry["CreditsConsumed"] = purchased
		} else {
			entry["CreditsConsumed"] = 50000.0
			entry["CreditsPurchased"] = 50000.0
		}
		fmt.Printf("Exhausting %s\n", k)
	}

	newData, err := json.Marshal(ledger)
	if err != nil {
		log.Fatalf("failed to marshal: %v", err)
	}

	_, err = db.Exec("UPDATE auth_store SET content = $1::jsonb, updated_at = NOW() WHERE id = 'ledger'", string(newData))
	if err != nil {
		log.Fatalf("failed to update db: %v", err)
	}
	fmt.Println("DB updated successfully!")
}
