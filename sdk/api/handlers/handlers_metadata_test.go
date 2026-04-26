package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"golang.org/x/net/context"
)

type testAPIHandler struct{}

func (testAPIHandler) HandlerType() string { return "openai" }

func (testAPIHandler) Models() []map[string]any { return nil }

func TestRequestExecutionMetadataIncludesExecutionSessionWithoutIdempotencyKey(t *testing.T) {
	ctx := WithExecutionSessionID(context.Background(), "session-1")

	meta := requestExecutionMetadata(ctx)
	if got := meta[coreexecutor.ExecutionSessionMetadataKey]; got != "session-1" {
		t.Fatalf("ExecutionSessionMetadataKey = %v, want %q", got, "session-1")
	}
	if _, ok := meta[idempotencyKeyMetadataKey]; ok {
		t.Fatalf("unexpected idempotency key in metadata: %v", meta[idempotencyKeyMetadataKey])
	}
}

func TestGetContextWithCancelAutoGeneratesExecutionSession(t *testing.T) {
	t.Setenv("GIN_MODE", gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	baseHandler := &BaseAPIHandler{Cfg: &sdkconfig.SDKConfig{}}
	ctx, cancel := baseHandler.GetContextWithCancel(testAPIHandler{}, ginCtx, context.Background())
	defer cancel()

	meta := requestExecutionMetadata(ctx)
	sessionID, ok := meta[coreexecutor.ExecutionSessionMetadataKey].(string)
	if !ok || sessionID == "" {
		t.Fatalf("expected auto-generated execution session id, got %v", meta[coreexecutor.ExecutionSessionMetadataKey])
	}
}
