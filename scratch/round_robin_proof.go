package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

func main() {
	fmt.Println("=== STRESS TEST & MATHEMATICAL VERIFICATION OF ROUND-ROBIN ===")
	fmt.Println("Simulating 100 Google Accounts (50 Antigravity, 50 Gemini CLI)")

	var auths []*auth.Auth
	parentProjectCounts := make(map[string]int)

	// 1. Generate 50 Flat Antigravity Accounts
	for i := 1; i <= 50; i++ {
		id := fmt.Sprintf("antigravity_account_%02d", i)
		auths = append(auths, &auth.Auth{
			ID:       id,
			Provider: "antigravity",
			Status:   auth.StatusActive,
		})
		parentProjectCounts[id] = 1
	}

	// 2. Generate 50 Gemini CLI Accounts (with variable projects per account)
	// Some accounts have 1 project, some have 5, some have 10.
	for i := 1; i <= 50; i++ {
		parentEmail := fmt.Sprintf("user_%02d@gmail.com", i)
		numProjects := rand.Intn(5) + 1 // 1 to 5 projects per account
		parentProjectCounts[parentEmail] = numProjects
		
		for p := 1; p <= numProjects; p++ {
			id := fmt.Sprintf("%s_project_%d", parentEmail, p)
			auths = append(auths, &auth.Auth{
				ID:       id,
				Provider: "gemini-cli",
				Status:   auth.StatusActive,
				Attributes: map[string]string{
					"gemini_virtual_parent": parentEmail,
				},
			})
		}
	}

	fmt.Printf("Total Auth Entries (Credentials) Loaded: %d\n\n", len(auths))

	ctx := context.Background()
	opts := cliproxyexecutor.Options{}

	// Test A: Antigravity (Flat Round Robin)
	fmt.Println("--- TEST A: Antigravity Load Distribution (10,000 requests) ---")
	testLoadDistribution("antigravity", 10000, auths, ctx, opts)

	// Test B: Gemini CLI (Two-Level Grouped Round Robin)
	fmt.Println("\n--- TEST B: Gemini CLI Load Distribution (50,000 requests) ---")
	testLoadDistribution("gemini-cli", 50000, auths, ctx, opts)
}

func testLoadDistribution(provider string, numRequests int, auths []*auth.Auth, ctx context.Context, opts cliproxyexecutor.Options) {
	selector := &auth.RoundRobinSelector{}
	
	// Filter auths down to the provider to simulate getAvailableAuths behavior inside Pick
	var providerAuths []*auth.Auth
	for _, a := range auths {
		if a.Provider == provider {
			providerAuths = append(providerAuths, a)
		}
	}

	hitCounts := make(map[string]int)
	parentHits := make(map[string]int)

	for i := 0; i < numRequests; i++ {
		picked, err := selector.Pick(ctx, provider, "gemini-3.1-flash-lite-preview", opts, providerAuths)
		if err != nil {
			panic(err)
		}
		
		hitCounts[picked.ID]++
		
		parent := picked.ID
		if p, ok := picked.Attributes["gemini_virtual_parent"]; ok && p != "" {
			parent = p
		}
		parentHits[parent]++
	}

	// Analyze parent distribution
	var minHits, maxHits int
	minHits = numRequests + 1
	var keys []string
	for p := range parentHits {
		keys = append(keys, p)
	}
	sort.Strings(keys)

	for _, p := range keys {
		hits := parentHits[p]
		if hits < minHits {
			minHits = hits
		}
		if hits > maxHits {
			maxHits = hits
		}
	}

	variance := maxHits - minHits
	avg := numRequests / len(parentHits)

	fmt.Printf("Total Unique Parent Accounts: %d\n", len(parentHits))
	fmt.Printf("Average requests per parent: %d\n", avg)
	fmt.Printf("Min hits on a parent: %d\n", minHits)
	fmt.Printf("Max hits on a parent: %d\n", maxHits)
	fmt.Printf("Variance between highest and lowest hit parent: %d\n", variance)
	if variance <= 1 {
		fmt.Println("Result: PERFECT MATHEMATICAL DISTRIBUTION ACROSS PARENT ACCOUNTS.")
	} else {
		fmt.Println("Result: IMBALANCED DISTRIBUTION DETECTED!")
	}

	if provider == "gemini-cli" {
		fmt.Println("\nDeep Dive: Project-level distribution for the first 3 parent accounts:")
		for i := 1; i <= 3; i++ {
			parentEmail := fmt.Sprintf("user_%02d@gmail.com", i)
			fmt.Printf("  %s (Total hits: %d)\n", parentEmail, parentHits[parentEmail])
			
			// Find all projects for this parent
			var projects []string
			for id, hits := range hitCounts {
				if strings.HasPrefix(id, parentEmail+"_project") {
					projects = append(projects, fmt.Sprintf("    -> %s: %d hits", id, hits))
				}
			}
			sort.Strings(projects)
			for _, pstr := range projects {
				fmt.Println(pstr)
			}
		}
	}
}
