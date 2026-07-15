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

    rows, err := db.Query("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'payment_orders'")
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    for rows.Next() {
        var colName, dataType string
        if err := rows.Scan(&colName, &dataType); err != nil {
            log.Fatal(err)
        }
        fmt.Printf("- %s (%s)\n", colName, dataType)
    }
}
