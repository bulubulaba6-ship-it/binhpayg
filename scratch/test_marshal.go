package main

import (
	"fmt"
	"log"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	cfg := &config.Config{}
	cfg.APIKeys = []string{"key1", "key2"}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatalf("Marshal error: %v", err)
	}
	fmt.Printf("--- Marshaled YAML ---\n%s\n", string(data))
}
