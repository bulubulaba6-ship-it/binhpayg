package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	var content string
	err = db.QueryRow("SELECT content FROM config_store WHERE id = 'config'").Scan(&content)
	if err != nil {
		log.Fatalf("Query error: %v", err)
	}

	lines := strings.Split(content, "\n")

	fmt.Println("=== Lines 1-30 ===")
	for i := 0; i < 30 && i < len(lines); i++ {
		fmt.Printf("%d: %s\n", i+1, lines[i])
	}

	fmt.Println("\n=== Lines 170-200 ===")
	for i := 169; i < 199 && i < len(lines); i++ {
		fmt.Printf("%d: %s\n", i+1, lines[i])
	}

	fmt.Println("\n=== Lines 250-280 ===")
	for i := 249; i < 279 && i < len(lines); i++ {
		fmt.Printf("%d: %s\n", i+1, lines[i])
	}

	fmt.Println("\n=== Lines 340-370 ===")
	for i := 339; i < 369 && i < len(lines); i++ {
		fmt.Printf("%d: %s\n", i+1, lines[i])
	}
}
