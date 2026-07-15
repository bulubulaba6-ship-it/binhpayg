package main

import (
	"fmt"
	"log"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"gopkg.in/yaml.v3"
)

func main() {
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("LoadConfig error: %v", err)
	}

	rendered, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatalf("Marshal error: %v", err)
	}

	fmt.Println("=== Marshaled Config ===")
	fmt.Println(string(rendered))
}
