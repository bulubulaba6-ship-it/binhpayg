package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("PGSTORE_DSN")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		log.Fatalf("PGSTORE_DSN or DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("sql.Open error: %v", err)
	}
	defer db.Close()

	var content string
	err = db.QueryRowContext(context.Background(), "SELECT content FROM config_store WHERE id = 'config'").Scan(&content)
	if err != nil {
		log.Fatalf("QueryRow error: %v", err)
	}

	lines := strings.Split(content, "\n")
	limit := 80
	if len(lines) < limit {
		limit = len(lines)
	}
	fmt.Println("--- Database Config (First 80 lines) ---")
	fmt.Println(strings.Join(lines[:limit], "\n"))

	// Check specifically if the key is in the api-keys sequence
	hasKey := strings.Contains(content, "fink_9e90275986313d820ff099aa58b47e73")
	fmt.Printf("\nKey fink_9e90275986313d820ff099aa58b47e73 found in raw content: %t\n", hasKey)

	// Check if it appears as a sequence item
	inSeq := false
	inAPIKeysSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "api-keys:") {
			inAPIKeysSection = true
			continue
		}
		if inAPIKeysSection {
			if trimmed == "" || (!strings.HasPrefix(trimmed, "-") && strings.Contains(trimmed, ":")) {
				// exited api-keys section
				inAPIKeysSection = false
				continue
			}
			if strings.Contains(trimmed, "fink_9e90275986313d820ff099aa58b47e73") {
				inSeq = true
			}
		}
	}
	fmt.Printf("Key fink_9e90275986313d820ff099aa58b47e73 found in api-keys sequence: %t\n", inSeq)
}
