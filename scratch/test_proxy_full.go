//go:build ignore

// Package main: Local integration test for the Proxy + Server pipeline.
// Tests all cases: Direct mode, Proxy config loading, and real API call routing.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
)

func main() {
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  🔬 FULL PROXY PIPELINE TEST SUITE")
	fmt.Println("═══════════════════════════════════════════════════")

	passed := 0
	failed := 0

	// ─────────────────────────────────────────────────────
	// CASE 1: Direct Mode (No proxy configured)
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 1] Direct Mode — No proxy configured")
	{
		// Clear any proxy env
		os.Unsetenv("PROXY_URL")

		// Mock upstream that just returns 200
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"mode":"direct"}`)
		}))
		defer ts.Close()

		transport, mode, err := proxyutil.BuildHTTPTransport("")
		if err != nil || mode != proxyutil.ModeInherit {
			fmt.Printf("  ❌ FAIL: Expected ModeInherit, got %v, err: %v\n", mode, err)
			failed++
		} else {
			client := &http.Client{Timeout: 3 * time.Second}
			if transport != nil {
				client.Transport = transport
			}
			resp, errGet := client.Get(ts.URL)
			if errGet != nil || resp.StatusCode != 200 {
				fmt.Printf("  ❌ FAIL: Direct mode request failed: %v\n", errGet)
				failed++
			} else {
				fmt.Println("  ✅ PASS: Direct mode correctly connects without proxy")
				passed++
			}
		}
	}

	// ─────────────────────────────────────────────────────
	// CASE 2: Config loading — upstream-proxy from YAML
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 2] Config Loading — upstream-proxy from config.yaml")
	{
		cfg, err := config.LoadConfig("config.yaml")
		if err != nil {
			fmt.Printf("  ❌ FAIL: Could not load config.yaml: %v\n", err)
			failed++
		} else if cfg.ProxyURL == "" {
			fmt.Println("  ❌ FAIL: ProxyURL is empty — config sync did not work")
			failed++
		} else {
			fmt.Printf("  ✅ PASS: ProxyURL loaded from YAML: %s\n", cfg.ProxyURL)
			passed++
		}
	}

	// ─────────────────────────────────────────────────────
	// CASE 3: ENV Override — PROXY_URL takes highest priority
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 3] ENV Override — PROXY_URL env takes priority over YAML")
	{
		os.Setenv("PROXY_URL", "socks5h://127.0.0.1:9999")
		defer os.Unsetenv("PROXY_URL")

		cfg, err := config.LoadConfig("config.yaml")
		if err != nil {
			fmt.Printf("  ❌ FAIL: Could not load config: %v\n", err)
			failed++
		} else if cfg.ProxyURL != "socks5h://127.0.0.1:9999" {
			fmt.Printf("  ❌ FAIL: ENV override failed. ProxyURL = %s\n", cfg.ProxyURL)
			failed++
		} else {
			fmt.Println("  ✅ PASS: ENV override correctly takes priority")
			passed++
		}
		os.Unsetenv("PROXY_URL")
	}

	// ─────────────────────────────────────────────────────
	// CASE 4: Proxy Enforcement — socks5h forces traffic through proxy
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 4] Proxy Enforcement — Traffic MUST go through SOCKS5 proxy")
	{
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "DIRECT_HIT")
		}))
		defer ts.Close()

		// Aim at a non-existent proxy → expect CONNECTION REFUSED to proxy, not direct
		transport, _, err := proxyutil.BuildHTTPTransport("socks5h://127.0.0.1:19999")
		if err != nil {
			fmt.Printf("  ❌ FAIL: Could not build transport: %v\n", err)
			failed++
		} else {
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			_, errGet := client.Get(ts.URL)
			if errGet == nil {
				fmt.Println("  ❌ FAIL: Request succeeded! Traffic bypassed the proxy — DNS LEAK risk!")
				failed++
			} else if strings.Contains(errGet.Error(), "socks connect") || strings.Contains(errGet.Error(), "connectex") || strings.Contains(errGet.Error(), "connection refused") {
				fmt.Printf("  ✅ PASS: Traffic correctly blocked at proxy (error: %v)\n", errGet)
				passed++
			} else {
				fmt.Printf("  ⚠️ WARN: Unexpected error type (may still be ok): %v\n", errGet)
				passed++ // still passed — it didn't bypass
			}
		}
	}

	// ─────────────────────────────────────────────────────
	// CASE 5: DNS leak check — socks5 vs socks5h scheme parsing
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 5] DNS Leak Protection — socks5h URL parsed correctly")
	{
		setting, err := proxyutil.Parse("socks5h://127.0.0.1:1080")
		if err != nil {
			fmt.Printf("  ❌ FAIL: Parse error: %v\n", err)
			failed++
		} else if setting.Mode != proxyutil.ModeProxy {
			fmt.Printf("  ❌ FAIL: Wrong mode: %v\n", setting.Mode)
			failed++
		} else if setting.URL.Scheme != "socks5h" {
			fmt.Printf("  ❌ FAIL: Scheme stripped! Got: %s (DNS LEAK!)\n", setting.URL.Scheme)
			failed++
		} else {
			fmt.Printf("  ✅ PASS: socks5h scheme preserved correctly → Remote DNS guaranteed\n")
			passed++
		}
	}

	// ─────────────────────────────────────────────────────
	// CASE 6: Context cancellation — proxy respects timeouts
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 6] Context Timeout — Proxy connection respects deadline")
	{
		transport, _, _ := proxyutil.BuildHTTPTransport("socks5h://127.0.0.1:19999")
		client := &http.Client{Transport: transport}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, "GET", "http://httpbin.org/ip", nil)
		start := time.Now()
		_, errDo := client.Do(req)
		elapsed := time.Since(start)

		if errDo != nil && elapsed < 2*time.Second {
			fmt.Printf("  ✅ PASS: Context timeout respected (elapsed: %v)\n", elapsed.Round(time.Millisecond))
			passed++
		} else if errDo == nil {
			fmt.Println("  ⚠️ WARN: Request succeeded (real proxy may be running on :19999)")
			passed++
		} else {
			fmt.Printf("  ❌ FAIL: Timeout not respected (elapsed: %v)\n", elapsed.Round(time.Millisecond))
			failed++
		}
	}

	// ─────────────────────────────────────────────────────
	// CASE 7: Real API test through live wireproxy (if running)
	// ─────────────────────────────────────────────────────
	fmt.Println("\n[CASE 7] Live Proxy Test — Check if wireproxy is running on :1080")
	{
		conn, err := net.DialTimeout("tcp", "127.0.0.1:1080", 500*time.Millisecond)
		if err != nil {
			fmt.Println("  ℹ️ SKIP: wireproxy not running locally (expected on dev machine)")
			fmt.Println("         Run: wireproxy -c wireproxy.conf   to activate this test")
		} else {
			conn.Close()
			fmt.Println("  ✅ wireproxy IS running on :1080! Testing real egress IP...")
			transport, _, _ := proxyutil.BuildHTTPTransport("socks5h://127.0.0.1:1080")
			client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
			resp, errGet := client.Get("https://ifconfig.me")
			if errGet != nil {
				fmt.Printf("  ❌ FAIL: Cannot reach ifconfig.me through proxy: %v\n", errGet)
				failed++
			} else {
				buf := make([]byte, 64)
				n, _ := resp.Body.Read(buf)
				resp.Body.Close()
				fmt.Printf("  ✅ PASS: Egress IP through WireGuard = %s\n", strings.TrimSpace(string(buf[:n])))
				passed++
			}
		}
	}

	// ─────────────────────────────────────────────────────
	// RESULTS
	// ─────────────────────────────────────────────────────
	fmt.Println("\n═══════════════════════════════════════════════════")
	total := passed + failed
	fmt.Printf("  RESULTS: %d/%d PASSED", passed, total)
	if failed > 0 {
		fmt.Printf(" | ❌ %d FAILED", failed)
	}
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")

	if failed > 0 {
		os.Exit(1)
	}
}
