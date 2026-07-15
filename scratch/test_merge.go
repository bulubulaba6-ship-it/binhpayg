package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func main() {
	// Create a mock config file content
	mockYaml := `host: ""
port: 8317
api-keys:
  - key1
  - key2
`
	tmpFile, err := ioutil.TempFile("", "config_test_*.yaml")
	if err != nil {
		log.Fatalf("TempFile error: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err = tmpFile.Write([]byte(mockYaml)); err != nil {
		log.Fatalf("Write error: %v", err)
	}
	tmpFile.Close()

	// Load the config
	cfg, err := config.LoadConfigOptional(tmpFile.Name(), false)
	if err != nil {
		log.Fatalf("LoadConfigOptional error: %v", err)
	}

	// Mutate the config in memory
	cfg.APIKeys = append(cfg.APIKeys, "key3")

	// Save back
	err = config.SaveConfigPreserveComments(tmpFile.Name(), cfg)
	if err != nil {
		log.Fatalf("SaveConfigPreserveComments error: %v", err)
	}

	// Read and print the resulting YAML
	res, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		log.Fatalf("ReadFile error: %v", err)
	}
	fmt.Printf("--- Resulting YAML ---\n%s\n", string(res))
}
