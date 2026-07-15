package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/yaml.v3"
)

type Config struct {
	APIKeys []string `yaml:"api-keys"`
}

func main() {
	dsn := "postgres://avnadmin:AVNS_gMMqDMXI4MLgS0D6m01@pg-34ab11a6-zok210305-4ca5.g.aivencloud.com:10436/defaultdb?sslmode=require"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Open error: %v", err)
	}
	defer db.Close()

	var content string
	err = db.QueryRow("SELECT content FROM config_store WHERE id = 'config'").Scan(&content)
	if err != nil {
		log.Fatalf("Query config_store error: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(content), &cfg); err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}

	fmt.Printf("Total API keys in database config: %d\n", len(cfg.APIKeys))
	foundInList := false
	for _, k := range cfg.APIKeys {
		if k == "fink_9e90275986313d820ff099aa58b47e73" {
			foundInList = true
		}
	}
	fmt.Printf("Target key found in api-keys array: %t\n", foundInList)

	// Let's print all of them
	for i, k := range cfg.APIKeys {
		fmt.Printf("Key [%d]: %s\n", i, k)
	}
}
