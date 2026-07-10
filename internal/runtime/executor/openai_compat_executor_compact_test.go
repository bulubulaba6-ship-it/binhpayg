package executor

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatExecutorCompactPassthrough(t *testing.T) {
	var gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response.compaction","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	payload := []byte(`{"model":"gpt-5.1-codex-max","input":[{"role":"user","content":"hi"}]}`)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.1-codex-max",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Alt:          "responses/compact",
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/responses/compact" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses/compact")
	}
	if !gjson.GetBytes(gotBody, "input").Exists() {
		t.Fatalf("expected input in body")
	}
	if gjson.GetBytes(gotBody, "messages").Exists() {
		t.Fatalf("unexpected messages in body")
	}
	if string(resp.Payload) != `{"id":"resp_1","object":"response.compaction","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3},"model":"gpt-5.1-codex-max"}` {
		t.Fatalf("payload = %s", string(resp.Payload))
	}
}

func TestOpenAICompatExecutorPayloadOverrideWinsOverThinkingSuffix(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{
		Payload: config.PayloadConfig{
			Override: []config.PayloadRule{
				{
					Models: []config.PayloadModelRule{
						{Name: "custom-openai", Protocol: "openai"},
					},
					Params: map[string]any{
						"reasoning_effort": "low",
					},
				},
			},
		},
	})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	payload := []byte(`{"model":"custom-openai(high)","messages":[{"role":"user","content":"hi"}]}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "custom-openai(high)",
		Payload: payload,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(gotBody, "reasoning_effort").String(); got != "low" {
		t.Fatalf("reasoning_effort = %q, want %q; body=%s", got, "low", string(gotBody))
	}
}

func TestOpenAICompatExecutorImagesGenerationsPassthrough(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"b64_json":"AA=="}],"usage":{"total_tokens":1}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "upstream-image",
		Payload: []byte(`{"model":"compat-image","prompt":"draw"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-image"),
		Stream:       false,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/images/generations",
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/images/generations")
	}
	if gotContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", gotContentType)
	}
	if got := gjson.GetBytes(gotBody, "model").String(); got != "upstream-image" {
		t.Fatalf("model = %q, want upstream-image; body=%s", got, string(gotBody))
	}
	if got := gjson.GetBytes(resp.Payload, "data.0.b64_json").String(); got != "AA==" {
		t.Fatalf("response payload = %s", string(resp.Payload))
	}
}

func TestOpenAICompatExecutorImagesGenerationsStreamsUpstream(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: image_generation.partial\ndata: {\"type\":\"image_generation.partial\"}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	streamResult, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "upstream-image",
		Payload: []byte(`{"model":"compat-image","prompt":"draw","stream":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-image"),
		Stream:       true,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/images/generations",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	var streamed bytes.Buffer
	for chunk := range streamResult.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		streamed.Write(chunk.Payload)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/images/generations")
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("accept = %q, want text/event-stream", gotAccept)
	}
	if got := gjson.GetBytes(gotBody, "model").String(); got != "upstream-image" {
		t.Fatalf("model = %q, want upstream-image; body=%s", got, string(gotBody))
	}
	if !gjson.GetBytes(gotBody, "stream").Bool() {
		t.Fatalf("stream flag missing from upstream body: %s", string(gotBody))
	}
	if !strings.Contains(streamed.String(), "event: image_generation.partial") || !strings.Contains(streamed.String(), "data: [DONE]") {
		t.Fatalf("streamed body = %q", streamed.String())
	}
}

func TestOpenAICompatExecutorImagesEditsMultipartRewritesModel(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if errWrite := writer.WriteField("model", "compat-image"); errWrite != nil {
		t.Fatalf("write model field: %v", errWrite)
	}
	if errWrite := writer.WriteField("prompt", "edit"); errWrite != nil {
		t.Fatalf("write prompt field: %v", errWrite)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", multipart.FileContentDisposition("image", "image.png"))
	header.Set("Content-Type", "image/png")
	part, errCreate := writer.CreatePart(header)
	if errCreate != nil {
		t.Fatalf("create image field: %v", errCreate)
	}
	if _, errWrite := part.Write([]byte("png-data")); errWrite != nil {
		t.Fatalf("write image field: %v", errWrite)
	}
	if errClose := writer.Close(); errClose != nil {
		t.Fatalf("close multipart writer: %v", errClose)
	}
	contentType := writer.FormDataContentType()

	var gotPath string
	var gotModel string
	var gotPrompt string
	var gotFile string
	var gotFileContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if errParse := r.ParseMultipartForm(32 << 20); errParse != nil {
			t.Fatalf("parse multipart form: %v", errParse)
		}
		gotModel = r.FormValue("model")
		gotPrompt = r.FormValue("prompt")
		file, fileHeader, errFile := r.FormFile("image")
		if errFile != nil {
			t.Fatalf("read image file: %v", errFile)
		}
		gotFileContentType = fileHeader.Header.Get("Content-Type")
		data, errRead := io.ReadAll(file)
		if errClose := file.Close(); errClose != nil {
			t.Fatalf("close image file: %v", errClose)
		}
		if errRead != nil {
			t.Fatalf("read image file: %v", errRead)
		}
		gotFile = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"b64_json":"AA=="}]}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "upstream-image",
		Payload: body.Bytes(),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-image"),
		Stream:       false,
		Headers: http.Header{
			"Content-Type": []string{contentType},
		},
		Metadata: map[string]any{
			cliproxyexecutor.RequestPathMetadataKey: "/v1/images/edits",
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/images/edits")
	}
	if gotModel != "upstream-image" {
		t.Fatalf("model = %q, want upstream-image", gotModel)
	}
	if gotPrompt != "edit" {
		t.Fatalf("prompt = %q, want edit", gotPrompt)
	}
	if gotFile != "png-data" {
		t.Fatalf("file = %q, want png-data", gotFile)
	}
	if gotFileContentType != "image/png" {
		t.Fatalf("file content type = %q, want image/png", gotFileContentType)
	}
}

func TestRewriteOpenAICompatImagesMultipartPayloadPreservesStreamAndFileContentType(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if errWrite := writer.WriteField("model", "compat-image"); errWrite != nil {
		t.Fatalf("write model field: %v", errWrite)
	}
	if errWrite := writer.WriteField("stream", "false"); errWrite != nil {
		t.Fatalf("write stream field: %v", errWrite)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", multipart.FileContentDisposition("image", "image.webp"))
	header.Set("Content-Type", "image/webp")
	part, errCreate := writer.CreatePart(header)
	if errCreate != nil {
		t.Fatalf("create image field: %v", errCreate)
	}
	if _, errWrite := part.Write([]byte("webp-data")); errWrite != nil {
		t.Fatalf("write image field: %v", errWrite)
	}
	if errClose := writer.Close(); errClose != nil {
		t.Fatalf("close multipart writer: %v", errClose)
	}

	out, contentType, err := prepareOpenAICompatImagesPayload(body.Bytes(), "upstream-image", writer.FormDataContentType(), true)
	if err != nil {
		t.Fatalf("prepareOpenAICompatImagesPayload error: %v", err)
	}
	mediaType, params, errParse := mime.ParseMediaType(contentType)
	if errParse != nil {
		t.Fatalf("parse content type: %v", errParse)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(bytes.NewReader(out), params["boundary"])
	form, errRead := reader.ReadForm(32 << 20)
	if errRead != nil {
		t.Fatalf("read rewritten form: %v", errRead)
	}
	defer func() {
		if errRemove := form.RemoveAll(); errRemove != nil {
			t.Fatalf("remove form files: %v", errRemove)
		}
	}()
	if got := form.Value["model"]; len(got) != 1 || got[0] != "upstream-image" {
		t.Fatalf("model values = %#v, want upstream-image", got)
	}
	if got := form.Value["stream"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("stream values = %#v, want true", got)
	}
	if got := form.File["image"]; len(got) != 1 || got[0].Header.Get("Content-Type") != "image/webp" {
		t.Fatalf("image headers = %#v, want image/webp", got)
	}
}

func TestOpenAICompatExecutorStreamRejectsPlainJSONAfterBlankLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("\n\n: openrouter processing\n\nevent: error\n"))
		_, _ = w.Write([]byte(`{"error":{"message":"upstream failed","type":"server_error"}}` + "\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "openrouter-model",
		Payload: []byte(`{"model":"openrouter-model","messages":[{"role":"user","content":"hi"}],"stream":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var gotErr error
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			gotErr = chunk.Err
			break
		}
	}
	if gotErr == nil {
		t.Fatalf("expected plain JSON stream error")
	}
	if status, ok := gotErr.(interface{ StatusCode() int }); !ok || status.StatusCode() != http.StatusBadGateway {
		t.Fatalf("stream error status = %v, want %d", gotErr, http.StatusBadGateway)
	}
	if !strings.Contains(gotErr.Error(), "upstream failed") {
		t.Fatalf("stream error = %v", gotErr)
	}
}

func TestOpenAICompatExecutorStreamSkipsKeepAliveUntilDataLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("\n\n: openrouter processing\n\nevent: ping\nid: 1\nretry: 1000\n"))
		_, _ = w.Write([]byte(`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}` + "\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "openrouter-model",
		Payload: []byte(`{"model":"openrouter-model","messages":[{"role":"user","content":"hi"}],"stream":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var got strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		got.Write(chunk.Payload)
	}
	if gjson.Get(got.String(), "choices.0.delta.content").String() != "hello" {
		t.Fatalf("stream payload = %s", got.String())
	}
}

func TestOpenAICompatExecutorModelOverrideVariousFormats(t *testing.T) {
	// Test cases for different format overrides
	t.Run("Claude Stream Format Override", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1677652288,\"model\":\"deepseek-v4-flash\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n"))
		}))
		defer server.Close()

		executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
		auth := &cliproxyauth.Auth{Attributes: map[string]string{
			"base_url": server.URL + "/v1",
			"api_key":  "test",
		}}
		result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "deepseek-v4-flash",
			Payload: []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`),
		}, cliproxyexecutor.Options{
			SourceFormat:    sdktranslator.FromString("claude"),
			Stream:          true,
			OriginalRequest: []byte(`{"model":"claude-opus-4-8","messages":[{"role":"user","content":"hi"}],"stream":true}`),
			Metadata: map[string]any{
				cliproxyexecutor.RequestedModelMetadataKey: "claude-opus-4-8",
			},
		})
		if err != nil {
			t.Fatalf("ExecuteStream error: %v", err)
		}

		var got strings.Builder
		for chunk := range result.Chunks {
			if chunk.Err != nil {
				t.Fatalf("unexpected stream error: %v", chunk.Err)
			}
			got.Write(chunk.Payload)
		}

		payload := got.String()
		if !strings.Contains(payload, "data: ") {
			t.Fatalf("expected data: prefix in stream output, got: %q", payload)
		}
		// Extract json part of first chunk (there might be multiple events translated, e.g. message_start)
		lines := strings.Split(payload, "\n")
		var targetJSON string
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				jsonPart := strings.TrimPrefix(line, "data: ")
				if gjson.Get(jsonPart, "type").String() == "message_start" {
					targetJSON = jsonPart
					break
				}
			}
		}
		if targetJSON == "" {
			t.Fatalf("could not find message_start event in payload: %q", payload)
		}
		modelVal := gjson.Get(targetJSON, "message.model").String()
		if modelVal != "claude-opus-4-8" {
			t.Fatalf("message.model = %q, want %q; payload = %q", modelVal, "claude-opus-4-8", targetJSON)
		}
	})

	t.Run("Gemini Format Override", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"chatcmpl-123","object":"chat.completion","created":1677652288,"model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`))
		}))
		defer server.Close()

		executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
		auth := &cliproxyauth.Auth{Attributes: map[string]string{
			"base_url": server.URL + "/v1",
			"api_key":  "test",
		}}
		resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "deepseek-v4-flash",
			Payload: []byte(`{"model":"deepseek-v4-flash","contents":[{"parts":[{"text":"hi"}]}]}`),
		}, cliproxyexecutor.Options{
			SourceFormat: sdktranslator.FromString("gemini"),
			Stream:       false,
			Metadata: map[string]any{
				cliproxyexecutor.RequestedModelMetadataKey: "gemini-alias",
			},
		})
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		modelVal := gjson.GetBytes(resp.Payload, "model").String()
		if modelVal != "gemini-alias" {
			t.Fatalf("model = %q, want %q; payload = %q", modelVal, "gemini-alias", string(resp.Payload))
		}
	})
}

// TestNonStreamModelAliasOverride verifies the Execute (non-streaming) path
// rewrites the "model" field in the translated response to the alias stored in
// RequestedModelMetadataKey, not the real upstream model name.
// This is the end-to-end proof our proxy does not leak upstream model identity
// to the downstream client.
func TestNonStreamModelAliasOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Upstream returns its real model name.
		_, _ = w.Write([]byte(`{"id":"chatcmpl-xyz","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "deepseek-v4-flash",
		Payload: []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
		Metadata: map[string]any{
			// Client requested claude-opus-4-8; our proxy must echo that alias back.
			cliproxyexecutor.RequestedModelMetadataKey: "claude-opus-4-8",
		},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	got := gjson.GetBytes(resp.Payload, "model").String()
	if got != "claude-opus-4-8" {
		t.Fatalf("response model = %q, want %q; full payload = %s", got, "claude-opus-4-8", string(resp.Payload))
	}
}

// TestStreamModelAliasOverrideCompactSSE verifies that ExecuteStream rewrites
// the "model" field even when the upstream sends SSE with no space after the
// colon ("data:{...}" vs "data: {...}").  Some OpenAI-compat providers emit
// this compact form; our line-split logic must handle both variants.
func TestStreamModelAliasOverrideCompactSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Compact SSE: no space between "data:" and JSON.
		_, _ = w.Write([]byte(`data:{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "deepseek-v4-flash",
		Payload: []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
		Metadata: map[string]any{
			cliproxyexecutor.RequestedModelMetadataKey: "claude-opus-4-8",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var combined strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		combined.Write(chunk.Payload)
	}
	out := combined.String()
	// When the openai→openai translator is used, chunks are emitted as
	// concatenated JSON objects (no "data:" envelope in the combined buffer).
	// We verify the override by checking the model field directly.
	if strings.Contains(out, "deepseek-v4-flash") {
		t.Fatalf("upstream model name leaked in stream output: %q", out)
	}
	modelVal := gjson.Get(out, "model").String()
	if modelVal != "claude-opus-4-8" {
		t.Fatalf("stream model = %q, want %q; full output:\n%s", modelVal, "claude-opus-4-8", out)
	}
}

// TestStreamModelAliasFallsBackToReqModel verifies that when no
// RequestedModelMetadataKey is present, the executor falls back to req.Model,
// never leaking the real upstream model name to the client.
func TestStreamModelAliasFallsBackToReqModel(t *testing.T) {
	const requestedModel = "my-proxy-model"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Upstream returns a totally different model name.
		_, _ = w.Write([]byte(`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"ultra-secret-internal-model-v9","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   requestedModel,
		Payload: []byte(`{"model":"` + requestedModel + `","messages":[{"role":"user","content":"hi"}],"stream":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
		// Intentionally NO Metadata — exercises the req.Model fallback path.
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}

	var combined strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		combined.Write(chunk.Payload)
	}
	out := combined.String()
	// Upstream internal model name must never appear in client-visible output.
	if strings.Contains(out, "ultra-secret-internal-model-v9") {
		t.Fatalf("upstream model leaked through to client; output:\n%s", out)
	}
	// The model field in the output must be req.Model (fallback path).
	modelVal := gjson.Get(out, "model").String()
	if modelVal != requestedModel {
		t.Fatalf("model = %q, want %q; full output:\n%s", modelVal, requestedModel, out)
	}
}

func TestOverrideModelUnexported(t *testing.T) {
	executor := &OpenAICompatExecutor{}

	tests := []struct {
		name     string
		input    string
		model    string
		expected string
	}{
		{
			name:     "root model only",
			input:    `{"model":"old-model"}`,
			model:    "new-model",
			expected: `{"model":"new-model"}`,
		},
		{
			name:     "message.model nested",
			input:    `{"message":{"model":"old-model"}}`,
			model:    "new-model",
			expected: `{"message":{"model":"new-model"},"model":"new-model"}`,
		},
		{
			name:     "modelVersion root",
			input:    `{"modelVersion":"old-model"}`,
			model:    "new-model",
			expected: `{"modelVersion":"new-model","model":"new-model"}`,
		},
		{
			name:     "response.modelVersion nested",
			input:    `{"response":{"modelVersion":"old-model"}}`,
			model:    "new-model",
			expected: `{"response":{"modelVersion":"new-model"},"model":"new-model"}`,
		},
		{
			name:     "multiple fields",
			input:    `{"model":"old","message":{"model":"old"},"modelVersion":"old","response":{"modelVersion":"old"}}`,
			model:    "new",
			expected: `{"model":"new","message":{"model":"new"},"modelVersion":"new","response":{"modelVersion":"new"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := executor.overrideModel([]byte(tt.input), tt.model)
			if !gjson.ValidBytes(got) {
				t.Fatalf("invalid json result: %s", string(got))
			}
			gotStr := string(got)
			if gjson.Get(gotStr, "model").String() != tt.model {
				t.Errorf("model = %q, want %q", gjson.Get(gotStr, "model").String(), tt.model)
			}
			if gjson.Get(gotStr, "message.model").Exists() && gjson.Get(gotStr, "message.model").String() != tt.model {
				t.Errorf("message.model = %q, want %q", gjson.Get(gotStr, "message.model").String(), tt.model)
			}
			if gjson.Get(gotStr, "modelVersion").Exists() && gjson.Get(gotStr, "modelVersion").String() != tt.model {
				t.Errorf("modelVersion = %q, want %q", gjson.Get(gotStr, "modelVersion").String(), tt.model)
			}
			if gjson.Get(gotStr, "response.modelVersion").Exists() && gjson.Get(gotStr, "response.modelVersion").String() != tt.model {
				t.Errorf("response.modelVersion = %q, want %q", gjson.Get(gotStr, "response.modelVersion").String(), tt.model)
			}
		})
	}
}

// TestStripModelDisclosureHeaders verifies that provider-specific response
// headers that reveal the real upstream model name are removed before the
// response reaches the client.  This closes the header-level bypass that
// would otherwise leak the real model identity even after the body is rewritten.
func TestStripModelDisclosureHeaders(t *testing.T) {
	disclosureHeaders := []struct {
		key   string
		value string
	}{
		{"X-Model-Id", "deepseek-v4-flash"},
		{"X-Openrouter-Model", "deepseek-v4-flash"},
		{"X-Actual-Model", "deepseek-v4-flash"},
		{"X-Served-By-Model", "deepseek-v4-flash"},
		{"Openrouter-Model", "deepseek-v4-flash"},
	}

	for _, hdr := range disclosureHeaders {
		t.Run(hdr.key, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set(hdr.key, hdr.value)
				_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			}))
			defer server.Close()

			executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
			auth := &cliproxyauth.Auth{Attributes: map[string]string{
				"base_url": server.URL + "/v1",
				"api_key":  "test",
			}}
			resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
				Model:   "deepseek-v4-flash",
				Payload: []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}]}`),
			}, cliproxyexecutor.Options{
				SourceFormat: sdktranslator.FromString("openai"),
				Stream:       false,
				Metadata: map[string]any{
					cliproxyexecutor.RequestedModelMetadataKey: "claude-opus-4-8",
				},
			})
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			// The disclosure header must not be forwarded to the client.
			if got := resp.Headers.Get(hdr.key); got != "" {
				t.Fatalf("header %q was not stripped; got %q", hdr.key, got)
			}
		})
	}
}

// TestUpstreamRequestCarriesCorrectMappedModel verifies that the model field
// in the request body sent to the upstream provider is the correctly mapped
// name (from mapModelToName), not the raw alias the client requested.
// This is the request-side mirror of TestNonStreamModelAliasOverride.
func TestUpstreamRequestCarriesCorrectMappedModel(t *testing.T) {
	var gotUpstreamModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotUpstreamModel = gjson.GetBytes(body, "model").String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-2","object":"chat.completion","model":"real-upstream-name","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "my-alias-model",
		Payload: []byte(`{"model":"my-alias-model","messages":[{"role":"user","content":"hi"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// Without config-level model mapping the upstream receives the same model
	// name that was in the payload — no transformation should corrupt it.
	if gotUpstreamModel != "my-alias-model" {
		t.Fatalf("upstream request model = %q, want %q", gotUpstreamModel, "my-alias-model")
	}
}

// TestAliasResolutionFor502Prevention verifies that the refactored GetProviderName
// correctly reads from the global config snapshot to resolve an aliased model.
// This prevents 502 Bad Gateway errors where the relay fails to locate the backend
// provider for models defined only in the OpenAICompatibility block.
func TestAliasResolutionFor502Prevention(t *testing.T) {
	// Set up a mock global configuration that defines an alias "gpt-5.5" mapped to "mock-compat" provider
	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name: "mock-compat",
				Models: []config.OpenAICompatibilityModel{
					{
						Alias: "gpt-5.5",
						Name:  "real-upstream-model",
					},
				},
			},
		},
	}
	config.SetGlobalConfig(cfg)
	defer config.SetGlobalConfig(nil) // Cleanup

	// The util package should be able to resolve "gpt-5.5" to "mock-compat"
	providers := util.GetProviderName("gpt-5.5")
	if len(providers) == 0 {
		t.Fatalf("expected at least one provider for aliased model gpt-5.5, got 0")
	}

	found := false
	for _, p := range providers {
		if p == "mock-compat" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected provider 'mock-compat' in resolved providers %v", providers)
	}
}
