package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/yaml.v3"
)

type Config struct {
	PostPayBilling struct {
		Clients map[string]map[string]interface{} `yaml:"clients"`
	} `yaml:"post-pay-billing"`
}

func main() {
	dsn := "postgres://avnadmin:AVNS_s4t4FBCqjI9lXtwYTpT@pg-13bba9b2-maiphuocanhtai21032005-bd89.j.aivencloud.com:20938/defaultdb?sslmode=require"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	var content string
	err = db.QueryRow("SELECT content FROM config_store WHERE id = 'config'").Scan(&content)
	if err != nil {
		log.Fatalf("query error: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		log.Fatalf("yaml parse error: %v", err)
	}

	for k, v := range cfg.PostPayBilling.Clients {
		fmt.Printf("Client: %s\n", k)
		for pk, pv := range v {
			fmt.Printf("  %s: %v\n", pk, pv)
		}
	}
}
