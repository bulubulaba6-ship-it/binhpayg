package main

import (
	"database/sql"
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

    // Exhaust keys
    keys := []string{
        "fink_6db8bcc04240f16a34398b9319d5a1da",
        "fink_54a9733e264f5e61949735a2098cfb70",
        "fink_eb6924786c88368ab2ef52d99d749f65",
        "fink_547a2dd57b7c148f09478206be8f4755",
    }
    for _, k := range keys {
        _, err = db.Exec("INSERT INTO api_key_ledger (api_key, credits_consumed, credits_quota) VALUES ($1, 50000, 100) ON CONFLICT (api_key) DO UPDATE SET credits_consumed = 50000, credits_quota = 100", k)
        if err != nil {
            fmt.Printf("err for %s: %v\n", k, err)
        } else {
            fmt.Printf("exhausted %s\n", k)
        }
    }
}
