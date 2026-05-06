package main

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

func main() {
	good := `
api-key-models:
  key_one:
    claude:
      - claude-opus-4-7
      - claude-sonnet-4-6
  key_two:
    claude:
      - claude-opus-4-7
`
	// User's bad indentation: "claude:" is indented 6 spaces (under key_two)
	// but the list items "- model" are only 6 spaces (same level as "claude:")
	bad := "api-key-models:\n  key_one:\n    claude:\n      - claude-opus-4-7\n  key_two:\n      claude:\n      - claude-opus-4-7\n"

	type Out struct {
		APIKeyModels map[string]map[string][]string `yaml:"api-key-models"`
	}

	var g Out
	if err := yaml.Unmarshal([]byte(good), &g); err != nil {
		fmt.Println("GOOD parse error:", err)
	} else {
		fmt.Println("GOOD YAML parsed:")
		for k, v := range g.APIKeyModels {
			fmt.Printf("  key=%q providers=%v\n", k, v)
		}
	}

	fmt.Println()

	var b Out
	if err := yaml.Unmarshal([]byte(bad), &b); err != nil {
		fmt.Println("BAD YAML parse error:", err)
	} else {
		fmt.Println("BAD YAML parsed (user's 2nd key with wrong indent):")
		for k, v := range b.APIKeyModels {
			fmt.Printf("  key=%q providers=%v\n", k, v)
		}
		// Check what key_two actually has
		if providers, ok := b.APIKeyModels["key_two"]; ok {
			fmt.Printf("\n  key_two exists: providers=%v\n", providers)
			if models, ok2 := providers["claude"]; ok2 {
				fmt.Printf("  key_two.claude models=%v\n", models)
			} else {
				fmt.Println("  key_two.claude is MISSING — filter will be SKIPPED, all models shown!")
			}
		} else {
			fmt.Println("  key_two is entirely absent from parsed map!")
		}
	}
}
