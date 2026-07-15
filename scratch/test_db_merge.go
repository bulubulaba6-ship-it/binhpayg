package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
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

	// Write content to a temp file
	tmpFile, err := ioutil.TempFile("", "config_db_*.yaml")
	if err != nil {
		log.Fatalf("TempFile error: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err = tmpFile.Write([]byte(content)); err != nil {
		log.Fatalf("Write error: %v", err)
	}
	tmpFile.Close()

	// Load the config using LoadConfig
	cfg, err := config.LoadConfigOptional(tmpFile.Name(), false)
	if err != nil {
		log.Fatalf("LoadConfigOptional error: %v", err)
	}

	// Mutate: append a new key
	newKey := "fink_new_test_key_12345"
	cfg.APIKeys = append(cfg.APIKeys, newKey)

	// Save back using SaveConfigPreserveComments
	err = config.SaveConfigPreserveComments(tmpFile.Name(), cfg)
	if err != nil {
		log.Fatalf("SaveConfigPreserveComments error: %v", err)
	}

	// Read and print resulting file content
	res, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		log.Fatalf("ReadFile error: %v", err)
	}
	lines := strings.Split(string(res), "\n")
	limitLines := 45
	if len(lines) < limitLines {
		limitLines = len(lines)
	}
	fmt.Printf("--- Resulting YAML (First %d lines) ---\n%s\n", limitLines, strings.Join(lines[:limitLines], "\n"))
}
