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

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM auth_store").Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("auth_store row count: %d\n", count)

	err = db.QueryRow("SELECT COUNT(*) FROM upstream_keys").Scan(&count)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("upstream_keys row count: %d\n", count)

	rows, err := db.Query("SELECT id, provider, api_key, label, is_active FROM upstream_keys")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var provider, apiKey, label string
		var isActive bool
		if err := rows.Scan(&id, &provider, &apiKey, &label, &isActive); err != nil {
			log.Fatal(err)
		}
		// Mask apiKey
		maskedKey := apiKey
		if len(apiKey) > 8 {
			maskedKey = apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
		}
		fmt.Printf("- ID: %d, Provider: %s, Label: %s, Key: %s, Active: %t\n", id, provider, label, maskedKey, isActive)
	}
}
