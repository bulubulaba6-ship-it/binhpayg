package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("PGSTORE_DSN")
	if dsn == "" {
		fmt.Println("No DSN")
		return
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("Err open:", err)
		return
	}

	// Create table
	db.Exec("CREATE TABLE IF NOT EXISTS test_bigint (id BIGINT, val VARCHAR(10))")
	db.Exec("INSERT INTO test_bigint (id, val) VALUES (123456789, 'hello')")

	// Query with string
	var val string
	err = db.QueryRow("SELECT val FROM test_bigint WHERE id = $1", "123456789").Scan(&val)
	if err != nil {
		fmt.Println("Err query string:", err)
	} else {
		fmt.Println("Query string success:", val)
	}

	// Query with int64
	err = db.QueryRow("SELECT val FROM test_bigint WHERE id = $1", int64(123456789)).Scan(&val)
	if err != nil {
		fmt.Println("Err query int64:", err)
	} else {
		fmt.Println("Query int64 success:", val)
	}
	db.Exec("DROP TABLE test_bigint")
}
