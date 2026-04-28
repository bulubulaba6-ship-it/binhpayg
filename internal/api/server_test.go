package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gin "github.com/gin-gonic/gin"
	proxyconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	internalregistry "github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	internalusage "github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v6/sdk/access"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

type staticAccessProvider struct {
	principal string
}

func (p *staticAccessProvider) Identifier() string { return "static" }

func (p *staticAccessProvider) Authenticate(context.Context, *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	return &sdkaccess.Result{Provider: "static", Principal: p.principal}, nil
}

type recordingQuotaLedger struct {
	summary          auth.ExecutionQuotaSnapshot
	lastPrincipal    string
	lastRequestedCtx context.Context
}

func (l *recordingQuotaLedger) BeginExecutionSession(context.Context, auth.ExecutionSessionRecord) error {
	return nil
}

func (l *recordingQuotaLedger) FinalizeExecutionSession(context.Context, string, time.Time) error {
	return nil
}

func (l *recordingQuotaLedger) GetExecutionQuotaSummary(ctx context.Context, principal string) (auth.ExecutionQuotaSnapshot, error) {
	l.lastRequestedCtx = ctx
	l.lastPrincipal = principal
	return l.summary, nil
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	authDir := filepath.Join(tmpDir, "auth")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("failed to create auth dir: %v", err)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: sdkconfig.SDKConfig{
			APIKeys: []string{"test-key"},
		},
		Port:                   0,
		AuthDir:                authDir,
		Debug:                  true,
		LoggingToFile:          false,
		UsageStatisticsEnabled: false,
	}

	authManager := auth.NewManager(nil, nil, nil)
	accessManager := sdkaccess.NewManager()

	configPath := filepath.Join(tmpDir, "config.yaml")
	return NewServer(cfg, authManager, accessManager, configPath)
}

func TestHealthz(t *testing.T) {
	server := newTestServer(t)

	t.Run("GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var resp struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v; body=%s", err, rr.Body.String())
		}
		if resp.Status != "ok" {
			t.Fatalf("unexpected response status: got %q want %q", resp.Status, "ok")
		}
	})

	t.Run("HEAD", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
		rr := httptest.NewRecorder()
		server.engine.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("expected empty body for HEAD request, got %q", rr.Body.String())
		}
	})
}

func TestAmpProviderModelRoutes(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		wantStatus   int
		wantContains string
	}{
		{
			name:         "openai root models",
			path:         "/api/provider/openai/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "groq root models",
			path:         "/api/provider/groq/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "openai models",
			path:         "/api/provider/openai/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"object":"list"`,
		},
		{
			name:         "anthropic models",
			path:         "/api/provider/anthropic/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"data"`,
		},
		{
			name:         "google models v1",
			path:         "/api/provider/google/v1/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
		{
			name:         "google models v1beta",
			path:         "/api/provider/google/v1beta/models",
			wantStatus:   http.StatusOK,
			wantContains: `"models"`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := newTestServer(t)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer test-key")

			rr := httptest.NewRecorder()
			server.engine.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("unexpected status code for %s: got %d want %d; body=%s", tc.path, rr.Code, tc.wantStatus, rr.Body.String())
			}
			if body := rr.Body.String(); !strings.Contains(body, tc.wantContains) {
				t.Fatalf("response body for %s missing %q: %s", tc.path, tc.wantContains, body)
			}
		})
	}
}

func TestModelsRouteHonorsAPIKeyAllowlist(t *testing.T) {
	server := newTestServer(t)
	server.accessManager.SetProviders([]sdkaccess.Provider{&staticAccessProvider{principal: "fink_pro_8c9e9ffb23fccdac9d65b57695145f13"}})
	server.handlers.Cfg.APIKeyModels = map[string]map[string][]string{
		"fink_pro_8c9e9ffb23fccdac9d65b57695145f13": {
			"claude": {
				"claude-opus-4-7",
				"claude-sonnet-4-6",
				"claude-haiku-4-5",
			},
		},
	}

	registry := internalregistry.GetGlobalRegistry()
	clientID := "test-allowlist-client"
	registry.RegisterClient(clientID, "claude", []*internalregistry.ModelInfo{
		{ID: "claude-opus-4-7", Object: "model", Created: 1776297600, OwnedBy: "anthropic", Type: "claude", DisplayName: "Claude Opus 4.7"},
		{ID: "claude-sonnet-4-6", Object: "model", Created: 1771372800, OwnedBy: "anthropic", Type: "claude", DisplayName: "Claude Sonnet 4.6"},
		{ID: "claude-haiku-4-5", Object: "model", Created: 1765939200, OwnedBy: "anthropic", Type: "claude", DisplayName: "Claude Haiku 4.5"},
	})
	defer registry.UnregisterClient(clientID)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer fink_pro_8c9e9ffb23fccdac9d65b57695145f13")
	req.Header.Set("User-Agent", "claude-cli/1.0")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse models response: %v; body=%s", err, rr.Body.String())
	}
	if len(resp.Data) != 3 {
		t.Fatalf("unexpected model count: got %d want %d; body=%s", len(resp.Data), 3, rr.Body.String())
	}
	want := map[string]struct{}{
		"claude-opus-4-7":   {},
		"claude-sonnet-4-6": {},
		"claude-haiku-4-5":  {},
	}
	for _, model := range resp.Data {
		id, _ := model["id"].(string)
		if _, ok := want[id]; !ok {
			t.Fatalf("unexpected model id %q in filtered response: %s", id, rr.Body.String())
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing expected model IDs: %v; body=%s", want, rr.Body.String())
	}
}

func TestQuotaRoute(t *testing.T) {
	server := newTestServer(t)
	server.accessManager.SetProviders([]sdkaccess.Provider{&staticAccessProvider{principal: "quota-key"}})
	server.handlers.UsageStats = internalusage.NewRequestStatistics()
	now := time.Now().UTC()
	server.handlers.UsageStats.Record(nil, coreusage.Record{
		APIKey:      "quota-key",
		Provider:    "openai",
		Model:       "gpt-4o",
		RequestedAt: now.Add(-5 * time.Minute),
		Detail: coreusage.Detail{
			InputTokens:     10,
			OutputTokens:    5,
			ReasoningTokens: 2,
			CachedTokens:    3,
			TotalTokens:     20,
		},
	})
	server.handlers.UsageStats.Record(nil, coreusage.Record{
		APIKey:      "quota-key",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		RequestedAt: now.Add(-1 * time.Minute),
		Failed:      true,
		Detail: coreusage.Detail{
			InputTokens:  4,
			OutputTokens: 2,
			TotalTokens:  7,
		},
	})

	ledger := &recordingQuotaLedger{
		summary: auth.ExecutionQuotaSnapshot{
			Sessions:        4,
			ActiveSessions:  1,
			CreditsUsed:     9,
			DurationSeconds: 420,
		},
	}
	server.handlers.AuthManager.SetExecutionSessionLedger(ledger)

	req := httptest.NewRequest(http.MethodGet, "/v1/quota", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if ledger.lastPrincipal != "quota-key" {
		t.Fatalf("quota summary queried for %q, want %q", ledger.lastPrincipal, "quota-key")
	}

	var resp struct {
		Quota auth.ExecutionQuotaSnapshot `json:"quota"`
		Usage struct {
			APIKey          string  `json:"api_key"`
			TotalRequests   int64   `json:"total_requests"`
			SuccessRequests int64   `json:"success_requests"`
			FailedRequests  int64   `json:"failed_requests"`
			TotalTokens     int64   `json:"total_tokens"`
			CachedTokens    int64   `json:"cached_tokens"`
			ReasoningTokens int64   `json:"reasoning_tokens"`
			RPM             float64 `json:"rpm"`
			TPM             float64 `json:"tpm"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse quota response: %v; body=%s", err, rr.Body.String())
	}
	if resp.Quota.CreditsUsed != 9 || resp.Quota.ActiveSessions != 1 || resp.Quota.Sessions != 4 {
		t.Fatalf("unexpected quota payload: %+v", resp.Quota)
	}
	if resp.Usage.APIKey != "quota-key" {
		t.Fatalf("usage summary queried for %q, want %q", resp.Usage.APIKey, "quota-key")
	}
	if resp.Usage.TotalRequests != 2 || resp.Usage.SuccessRequests != 1 || resp.Usage.FailedRequests != 1 {
		t.Fatalf("unexpected usage request counts: %+v", resp.Usage)
	}
	if resp.Usage.TotalTokens != 27 || resp.Usage.CachedTokens != 3 || resp.Usage.ReasoningTokens != 2 {
		t.Fatalf("unexpected usage token totals: %+v", resp.Usage)
	}
	if resp.Usage.RPM <= 0 || resp.Usage.TPM <= 0 {
		t.Fatalf("expected positive usage rates, got %+v", resp.Usage)
	}
}

func TestRootRouteUsesAIAPIGiaReBranding(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp struct {
		Message   string   `json:"message"`
		Endpoints []string `json:"endpoints"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse root response: %v; body=%s", err, rr.Body.String())
	}
	if resp.Message != "AIAPIGiaRe" {
		t.Fatalf("unexpected root brand message: got %q want %q", resp.Message, "AIAPIGiaRe")
	}
	if len(resp.Endpoints) != 4 {
		t.Fatalf("unexpected root endpoints: %+v", resp.Endpoints)
	}
	if resp.Endpoints[0] != "POST /v1/chat/completions" || resp.Endpoints[3] != "GET /v1/quota" {
		t.Fatalf("unexpected root endpoint list: %+v", resp.Endpoints)
	}
}

func TestQuotaViewerRoute(t *testing.T) {
	server := newTestServer(t)

	redirectReq := httptest.NewRequest(http.MethodGet, "/quota-check", nil)
	redirectRR := httptest.NewRecorder()
	server.engine.ServeHTTP(redirectRR, redirectReq)

	if redirectRR.Code != http.StatusTemporaryRedirect {
		t.Fatalf("unexpected redirect status code: got %d want %d; body=%s", redirectRR.Code, http.StatusTemporaryRedirect, redirectRR.Body.String())
	}
	if got := redirectRR.Header().Get("Location"); !strings.Contains(got, "/quota-check?") || !strings.Contains(got, "nocache=") {
		t.Fatalf("quota viewer redirect missing cache-busting location: %q", got)
	}
	if got := redirectRR.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") || !strings.Contains(got, "no-cache") {
		t.Fatalf("quota viewer page missing no-cache headers: %q", got)
	}
	if got := redirectRR.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("quota viewer page missing pragma header: %q", got)
	}
	if got := redirectRR.Header().Get("Expires"); got != "0" {
		t.Fatalf("quota viewer page missing expires header: %q", got)
	}
	if got := redirectRR.Header().Get("Clear-Site-Data"); got != `"cache"` {
		t.Fatalf("quota viewer page missing clear-site-data header: %q", got)
	}

	redirectURL := redirectRR.Header().Get("Location")
	if redirectURL == "" {
		t.Fatal("quota viewer redirect missing location header")
	}

	req := httptest.NewRequest(http.MethodGet, redirectURL, nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") || !strings.Contains(got, "no-cache") {
		t.Fatalf("quota viewer page missing no-cache headers: %q", got)
	}
	if got := rr.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("quota viewer page missing pragma header: %q", got)
	}
	if got := rr.Header().Get("Expires"); got != "0" {
		t.Fatalf("quota viewer page missing expires header: %q", got)
	}
	if got := rr.Header().Get("Clear-Site-Data"); got != `"cache"` {
		t.Fatalf("quota viewer page missing clear-site-data header: %q", got)
	}
	if !strings.Contains(rr.Body.String(), "const summary = extractUsage(source);") {
		t.Fatalf("quota viewer page missing usage payload extraction fix: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "AiApiGiaRe API Key Quota Viewer") {
		t.Fatalf("quota viewer page missing expected branded title: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "AiApiGiaRe") {
		t.Fatalf("quota viewer page missing renamed brand text: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "API base URL") {
		t.Fatalf("quota viewer page still exposes a base URL field: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Usage charts") {
		t.Fatalf("quota viewer page missing usage charts section: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Credits snapshot") {
		t.Fatalf("quota viewer page missing credits snapshot card: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "chartTotalCostValue") {
		t.Fatalf("quota viewer page missing total cost chart card: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Total Cost") {
		t.Fatalf("quota viewer page missing total cost card label: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "state.quota && state.quota.credits_used") {
		t.Fatalf("quota viewer page missing credits-used binding: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Track credits used for the selected range.") || !strings.Contains(rr.Body.String(), "Credits used in the current snapshot.") {
		t.Fatalf("quota viewer page missing credits-based cost overview copy: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Raw response") {
		t.Fatalf("quota viewer page still exposes raw response UI: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Total requests") || !strings.Contains(rr.Body.String(), "Token mix") {
		t.Fatalf("quota viewer page missing chart cards: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "Success Requests") || strings.Contains(rr.Body.String(), "Failed Requests") || strings.Contains(rr.Body.String(), "0 success / 0 failed") {
		t.Fatalf("quota viewer page still exposes request breakdown copy: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Usage Statistics") || !strings.Contains(rr.Body.String(), "Time Range") {
		t.Fatalf("quota viewer page missing usage statistics controls: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Export") || !strings.Contains(rr.Body.String(), "Import") || !strings.Contains(rr.Body.String(), "Refresh") {
		t.Fatalf("quota viewer page missing dashboard actions: %s", rr.Body.String())
	}
	if usageChartsIndex := strings.Index(rr.Body.String(), "Usage charts"); usageChartsIndex == -1 || strings.Index(rr.Body.String(), "Token Type Breakdown") == -1 || usageChartsIndex > strings.Index(rr.Body.String(), "Token Type Breakdown") {
		t.Fatalf("quota viewer page did not swap usage charts before token breakdown: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Token Type Breakdown") || !strings.Contains(rr.Body.String(), "Cost Overview") || !strings.Contains(rr.Body.String(), "Request Trends") || !strings.Contains(rr.Body.String(), "Token Usage Trends") {
		t.Fatalf("quota viewer page missing analytics sections: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Total Requests") {
		t.Fatalf("quota viewer page missing usage metric labels: %s", rr.Body.String())
	}
}

func TestManagementControlPanelRouteRewritesBranding(t *testing.T) {
	server := newTestServer(t)

	filePath := filepath.Join(filepath.Dir(server.configFilePath), "static", "management.html")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatalf("failed to create management asset directory: %v", err)
	}
	fixture := `<!doctype html><html><head><title>CLI Proxy API Management Center</title><link rel="icon" type="image/svg+xml" href="data:image/svg+xml,abc"></head><body>CLI Proxy API Management Center CLI Proxy API CPAMC CLIProxyAPI const Td="data:image/jpeg;base64,abc" h.jsxs("div",{className:yi.brandContent,children:[h.jsx("span",{className:yi.brandWord,children:"CLI"}),h.jsx("span",{className:yi.brandWord,children:"PROXY"}),h.jsx("span",{className:yi.brandWord,children:"API"})]})</body></html>`
	if err := os.WriteFile(filePath, []byte(fixture), 0o600); err != nil {
		t.Fatalf("failed to write management asset fixture: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/management.html", nil)
	rr := httptest.NewRecorder()
	server.engine.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") || !strings.Contains(got, "no-cache") {
		t.Fatalf("management page missing no-cache headers: %q", got)
	}
	if got := rr.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("management page missing pragma header: %q", got)
	}
	if got := rr.Header().Get("Expires"); got != "0" {
		t.Fatalf("management page missing expires header: %q", got)
	}
	if got := rr.Header().Get("Clear-Site-Data"); got != `"cache"` {
		t.Fatalf("management page missing clear-site-data header: %q", got)
	}
	if !strings.Contains(rr.Body.String(), "AIAPIGiaRe Management Center") {
		t.Fatalf("management page missing rewritten brand title: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "AIAPIGiaRe") {
		t.Fatalf("management page missing rewritten brand text: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "data:image/png;base64,") || strings.Count(rr.Body.String(), "data:image/png;base64,") < 2 {
		t.Fatalf("management page missing PNG icon rewrite: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "CLI Proxy API") || strings.Contains(rr.Body.String(), "CLIProxyAPI") || strings.Contains(rr.Body.String(), "CLIPROXYAPI") || strings.Contains(rr.Body.String(), "CPAMC") || strings.Contains(rr.Body.String(), "data:image/svg+xml,abc") || strings.Contains(rr.Body.String(), "data:image/jpeg;base64,abc") || strings.Contains(rr.Body.String(), `className:yi.brandContent,children:[h.jsx("span",{className:yi.brandWord,children:"CLI"}),h.jsx("span",{className:yi.brandWord,children:"PROXY"}),h.jsx("span",{className:yi.brandWord,children:"API"})]}`) {
		t.Fatalf("management page still exposes legacy branding: %s", rr.Body.String())
	}
}

func TestDefaultRequestLoggerFactory_UsesResolvedLogDirectory(t *testing.T) {
	t.Setenv("WRITABLE_PATH", "")
	t.Setenv("writable_path", "")

	originalWD, errGetwd := os.Getwd()
	if errGetwd != nil {
		t.Fatalf("failed to get current working directory: %v", errGetwd)
	}

	tmpDir := t.TempDir()
	if errChdir := os.Chdir(tmpDir); errChdir != nil {
		t.Fatalf("failed to switch working directory: %v", errChdir)
	}
	defer func() {
		if errChdirBack := os.Chdir(originalWD); errChdirBack != nil {
			t.Fatalf("failed to restore working directory: %v", errChdirBack)
		}
	}()

	// Force ResolveLogDirectory to fallback to auth-dir/logs by making ./logs not a writable directory.
	if errWriteFile := os.WriteFile(filepath.Join(tmpDir, "logs"), []byte("not-a-directory"), 0o644); errWriteFile != nil {
		t.Fatalf("failed to create blocking logs file: %v", errWriteFile)
	}

	configDir := filepath.Join(tmpDir, "config")
	if errMkdirConfig := os.MkdirAll(configDir, 0o755); errMkdirConfig != nil {
		t.Fatalf("failed to create config dir: %v", errMkdirConfig)
	}
	configPath := filepath.Join(configDir, "config.yaml")

	authDir := filepath.Join(tmpDir, "auth")
	if errMkdirAuth := os.MkdirAll(authDir, 0o700); errMkdirAuth != nil {
		t.Fatalf("failed to create auth dir: %v", errMkdirAuth)
	}

	cfg := &proxyconfig.Config{
		SDKConfig: proxyconfig.SDKConfig{
			RequestLog: false,
		},
		AuthDir:           authDir,
		ErrorLogsMaxFiles: 10,
	}

	logger := defaultRequestLoggerFactory(cfg, configPath)
	fileLogger, ok := logger.(*internallogging.FileRequestLogger)
	if !ok {
		t.Fatalf("expected *FileRequestLogger, got %T", logger)
	}

	errLog := fileLogger.LogRequestWithOptions(
		"/v1/chat/completions",
		http.MethodPost,
		map[string][]string{"Content-Type": []string{"application/json"}},
		[]byte(`{"input":"hello"}`),
		http.StatusBadGateway,
		map[string][]string{"Content-Type": []string{"application/json"}},
		[]byte(`{"error":"upstream failure"}`),
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		"issue-1711",
		time.Now(),
		time.Now(),
	)
	if errLog != nil {
		t.Fatalf("failed to write forced error request log: %v", errLog)
	}

	authLogsDir := filepath.Join(authDir, "logs")
	authEntries, errReadAuthDir := os.ReadDir(authLogsDir)
	if errReadAuthDir != nil {
		t.Fatalf("failed to read auth logs dir %s: %v", authLogsDir, errReadAuthDir)
	}
	foundErrorLogInAuthDir := false
	for _, entry := range authEntries {
		if strings.HasPrefix(entry.Name(), "error-") && strings.HasSuffix(entry.Name(), ".log") {
			foundErrorLogInAuthDir = true
			break
		}
	}
	if !foundErrorLogInAuthDir {
		t.Fatalf("expected forced error log in auth fallback dir %s, got entries: %+v", authLogsDir, authEntries)
	}

	configLogsDir := filepath.Join(configDir, "logs")
	configEntries, errReadConfigDir := os.ReadDir(configLogsDir)
	if errReadConfigDir != nil && !os.IsNotExist(errReadConfigDir) {
		t.Fatalf("failed to inspect config logs dir %s: %v", configLogsDir, errReadConfigDir)
	}
	for _, entry := range configEntries {
		if strings.HasPrefix(entry.Name(), "error-") && strings.HasSuffix(entry.Name(), ".log") {
			t.Fatalf("unexpected forced error log in config dir %s", configLogsDir)
		}
	}
}
