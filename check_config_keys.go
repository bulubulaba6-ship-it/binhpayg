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

	target := "fink_9e90275986313d820ff099aa58b47e73"
	fmt.Printf("Contains target key: %t\n", strings.Contains(content, target))

	// Find where "fink_9e9027598631" appears in config_store content
	lines := strings.Split(content, "\n")
	fmt.Println("Matching lines for '9e9027598631':")
	for i, line := range lines {
		if strings.Contains(line, "9e9027598631") {
			fmt.Printf("Line %d: %s\n", i+1, line)
		}
	}
}
