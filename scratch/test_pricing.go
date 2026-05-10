package main

import (
	"fmt"
)

type pricingEntry struct {
	input  float64
	output float64
	cache  float64
}

func CalculateTokenCost(input, output int64, entry pricingEntry) float64 {
	credits := (float64(input)*entry.input + float64(output)*entry.output + float64(0)*entry.cache) / 1_000_000.0
	return credits
}

func main() {
	// Current config:
	// claude-opus-4-7:
	//   input: 450.0
	//   output: 2250.0
	opus := pricingEntry{input: 450.0, output: 2250.0}

	fmt.Println("--- TEST 1: Short Chat ---")
	// 500 tokens in, 500 tokens out
	c1 := CalculateTokenCost(500, 500, opus)
	fmt.Printf("500 in / 500 out: %.5f credits\n", c1)

	fmt.Println("\n--- TEST 2: Medium Chat ---")
	// 2000 tokens in, 1000 tokens out
	c2 := CalculateTokenCost(2000, 1000, opus)
	fmt.Printf("2000 in / 1000 out: %.5f credits\n", c2)

	fmt.Println("\n--- TEST 3: Codebase Context (50k tokens) ---")
	// 50,000 tokens in (large context)
	c3 := CalculateTokenCost(50000, 1000, opus)
	fmt.Printf("50000 in / 1000 out: %.5f credits\n", c3)
	
	fmt.Println("\n--- TEST 4: The 68 Credit Mystery ---")
	// How many tokens for 68 credits?
	// If 1:1 ratio: tokens * (450+2250) / 1M = 68
	tokens := (68.0 * 1_000_000) / (450 + 2250)
	fmt.Printf("To get 68 credits, you need approx %d tokens (input+output)\n", int(tokens))
}
