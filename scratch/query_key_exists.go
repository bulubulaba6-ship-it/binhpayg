package main

import (
	"crypto/sha256"
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

	key := "test_fink_pro_1234567890abcdef_test"
	keyHash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))

	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM api_keys WHERE key_hash = $1)", keyHash).Scan(&exists)
	if err != nil {
		log.Fatalf("api_keys query error: %v", err)
	}
	fmt.Printf("Key '%s' (hash: %s) exists in api_keys: %t\n", key, keyHash, exists)
}
