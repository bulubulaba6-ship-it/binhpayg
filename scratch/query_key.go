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

	var content string
	err = db.QueryRow("SELECT content FROM auth_store WHERE id = 'ledger'").Scan(&content)
	if err != nil {
		log.Fatalf("query error: %v", err)
	}
	fmt.Printf("Ledger content length: %d\n", len(content))
	fmt.Printf("Ledger content preview:\n%s\n", content[:2000])
}
