package helps

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTranslateImagePayloadIfNeeded_NonDeepSeekPassThrough(t *testing.T) {
	payload := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/img.png"}}]}]}`)
	out := TranslateImagePayloadIfNeeded("gpt-4o", payload)
	if string(out) != string(payload) {
		t.Fatalf("expected pass-through for gpt-4o, got modified payload: %s", out)
	}
}

func TestTranslateImagePayloadIfNeeded_NoMessages(t *testing.T) {
	payload := []byte(`{"model":"deepseek-v3","messages":[]}`)
	out := TranslateImagePayloadIfNeeded("deepseek-v3", payload)
	if string(out) != string(payload) {
		t.Fatalf("expected pass-through for empty messages, got: %s", out)
	}
}

func TestTranslateImagePayloadIfNeeded_TextOnlyMessages(t *testing.T) {
	payload := []byte(`{"model":"deepseek-v3","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	out := TranslateImagePayloadIfNeeded("deepseek-v3", payload)
	if string(out) != string(payload) {
		t.Fatalf("expected pass-through for text-only message, got: %s", out)
	}
}

func TestTranslateImagePayloadIfNeeded_ImageReplacedWithFallback(t *testing.T) {
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":[{"type":"text","text":"describe:"},{"type":"image_url","image_url":{"url":"https://invalid.example-no-such-host.xxx/img.png"}}]}]}`)
	out := TranslateImagePayloadIfNeeded("deepseek-chat", payload)

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nGot: %s", err, out)
	}

	messages, _ := result["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("messages array is empty")
	}
	firstMsg, _ := messages[0].(map[string]any)
	contentArr, _ := firstMsg["content"].([]any)

	if len(contentArr) != 2 {
		t.Fatalf("expected 2 content parts (text + replacement), got %d: %s", len(contentArr), out)
	}

	for i, part := range contentArr {
		p := part.(map[string]any)
		if p["type"] == "image_url" {
			t.Fatalf("image_url content part still present at index %d", i)
		}
	}

	lastPart := contentArr[1].(map[string]any)
	if lastPart["type"] != "text" {
		t.Fatalf("expected last part to be type text, got %v", lastPart["type"])
	}
	if !strings.Contains(lastPart["text"].(string), "[PROXY VISION SYSTEM:") {
		t.Fatalf("expected PROXY VISION SYSTEM prefix in replacement, got: %v", lastPart["text"])
	}
}

func TestTranslateImagePayloadIfNeeded_MixedContentPreservesTextParts(t *testing.T) {
	payload := []byte(`{"model":"deepseek-v3","messages":[{"role":"user","content":[{"type":"text","text":"my question"},{"type":"image_url","image_url":{"url":"https://invalid.nxd.example/img.jpg"}}]}]}`)
	out := TranslateImagePayloadIfNeeded("deepseek-v3", payload)

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nGot: %s", err, out)
	}

	messages := result["messages"].([]any)
	firstMsg := messages[0].(map[string]any)
	contentArr := firstMsg["content"].([]any)

	firstPart := contentArr[0].(map[string]any)
	if firstPart["text"] != "my question" {
		t.Fatalf("expected original text 'my question', got: %v", firstPart["text"])
	}
}

func TestTranslateImagePayloadIfNeeded_CachesIdenticalURL(t *testing.T) {
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://unique-test-cache-url.invalid/img.png"}}]}]}`)
	out1 := TranslateImagePayloadIfNeeded("deepseek-chat", payload)
	out2 := TranslateImagePayloadIfNeeded("deepseek-chat", payload)
	if string(out1) != string(out2) {
		t.Fatalf("cache miss: first=%s, second=%s", out1, out2)
	}
}
