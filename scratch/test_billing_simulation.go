package main

import (
	"fmt"
)

// Simplified Delta Calculation Logic from postgresstore.go
type detail struct {
	InputTokens int64
}

func CalculateDelta(newTokens int64, currentDetail *detail) int64 {
	delta := newTokens - currentDetail.InputTokens
	// Update memory state (as we do in HandleUsage)
	currentDetail.InputTokens = newTokens
	return delta
}

func main() {
	// Pricing: 15.0 per 1M tokens (Opus)
	rate := 15.0
	
	// Memory state for an active session
	sessionMemory := &detail{InputTokens: 0}
	
	totalCreditsInDB := 0.0
	totalTokensInDB := int64(0)

	fmt.Println("--- SIMULATING STREAMING SESSION (GEMINI CUMULATIVE) ---")

	// Update 1: 100 tokens
	d1 := CalculateDelta(100, sessionMemory)
	c1 := (float64(d1) * rate) / 1_000_000.0
	totalCreditsInDB += c1
	totalTokensInDB += d1
	fmt.Printf("Update 1 (100 total): Delta=%d, Credits=+%.5f\n", d1, c1)

	// Update 2: 250 tokens total
	d2 := CalculateDelta(250, sessionMemory)
	c2 := (float64(d2) * rate) / 1_000_000.0
	totalCreditsInDB += c2
	totalTokensInDB += d2
	fmt.Printf("Update 2 (250 total): Delta=%d, Credits=+%.5f\n", d2, c2)

	// Update 3: 500 tokens total (Final)
	d3 := CalculateDelta(500, sessionMemory)
	c3 := (float64(d3) * rate) / 1_000_000.0
	totalCreditsInDB += c3
	totalTokensInDB += d3
	fmt.Printf("Update 3 (500 total): Delta=%d, Credits=+%.5f\n", d3, c3)

	fmt.Println("\n--- FINAL TOTALS IN DATABASE ---")
	fmt.Printf("Total Tokens: %d (Expected: 500)\n", totalTokensInDB)
	fmt.Printf("Total Credits: %.5f (Expected: %.5f)\n", totalCreditsInDB, (500.0 * rate) / 1_000_000.0)

	if totalTokensInDB == 500 {
		fmt.Println("\n✅ SUCCESS: Delta logic prevents double-counting cumulative tokens!")
	} else {
		fmt.Println("\n❌ FAILURE: Math is incorrect.")
	}
}
