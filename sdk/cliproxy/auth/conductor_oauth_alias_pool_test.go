package auth

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestExecutionModelCandidates_OAuthModelAliasPool(t *testing.T) {
	mgr := NewManager(nil, nil, nil)
	cfg := &config.Config{
		OAuthModelAlias: map[string][]config.OAuthModelAlias{
			"antigravity": {
				{Name: "gemini-3.1-flash-lite-preview", Alias: "claude-opus-4-7"},
				{Name: "gemini-3-flash", Alias: "claude-opus-4-7"},
			},
		},
	}
	mgr.SetConfig(cfg)
	mgr.SetOAuthModelAlias(cfg.OAuthModelAlias)

	auth := &Auth{
		ID:        "test-auth-1",
		Provider:  "antigravity",
		Status:    StatusActive,
		UpdatedAt: time.Now(),
	}

	// First call should return both candidates starting with the primary model
	candidates1 := mgr.executionModelCandidates(auth, "claude-opus-4-7")
	if len(candidates1) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates1))
	}
	if candidates1[0] != "gemini-3.1-flash-lite-preview" {
		t.Errorf("expected primary candidate gemini-3.1-flash-lite-preview, got %s", candidates1[0])
	}
	if candidates1[1] != "gemini-3-flash" {
		t.Errorf("expected fallback candidate gemini-3-flash, got %s", candidates1[1])
	}

	// Because we use round robin rotation, the next call should offset by 1
	candidates2 := mgr.executionModelCandidates(auth, "claude-opus-4-7")
	if len(candidates2) != 2 {
		t.Fatalf("expected 2 candidates on second call, got %d", len(candidates2))
	}
	// Note: offset rotates candidates
	if candidates2[0] != "gemini-3-flash" {
		t.Errorf("expected rotated primary candidate gemini-3-flash, got %s", candidates2[0])
	}
	if candidates2[1] != "gemini-3.1-flash-lite-preview" {
		t.Errorf("expected rotated fallback candidate gemini-3.1-flash-lite-preview, got %s", candidates2[1])
	}

	// Third call should rotate back to the start
	candidates3 := mgr.executionModelCandidates(auth, "claude-opus-4-7")
	if candidates3[0] != "gemini-3.1-flash-lite-preview" {
		t.Errorf("expected rotated primary candidate gemini-3.1-flash-lite-preview, got %s", candidates3[0])
	}

	_ = context.Background() // satisfy compiler if not used
}
