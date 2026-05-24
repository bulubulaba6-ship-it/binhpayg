package helps

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var (
	imageTranslationCache sync.Map
	visionAPIKey          = "fink_pro_sghasffb23fccdac9d65b5769514gash"
	visionEndpoint        = "https://api.aiapigiare.io.vn/v1/chat/completions"
	visionModel           = "claude-3-haiku-20240307"
)

func translateImageToText(imageURL string) string {
	hash := sha256.Sum256([]byte(imageURL))
	hashStr := hex.EncodeToString(hash[:])

	if val, ok := imageTranslationCache.Load(hashStr); ok {
		return val.(string)
	}

	fallback := "[PROXY VISION SYSTEM: The user attached an image, but the vision proxy failed to process it. Please ask the user to describe it.]"

	payload := []byte(`{"model":"","messages":[{"role":"user","content":[{"type":"text","text":"Describe this image in excruciating detail, extract all text, and explain the context. Be concise and accurate."},{"type":"image_url","image_url":{"url":""}}]}],"max_tokens":1000}`)
	payload, _ = sjson.SetBytes(payload, "model", visionModel)
	payload, _ = sjson.SetBytes(payload, "messages.0.content.1.image_url.url", imageURL)

	req, err := http.NewRequest("POST", visionEndpoint, bytes.NewBuffer(payload))
	if err != nil {
		return fallback
	}
	req.Header.Set("Authorization", "Bearer "+visionAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fallback
	}

	content := gjson.GetBytes(body, "choices.0.message.content").String()
	if content == "" {
		return fallback
	}

	result := fmt.Sprintf("[PROXY VISION SYSTEM: The user uploaded an image. Image Analysis: %s]", content)
	imageTranslationCache.Store(hashStr, result)

	return result
}

// TranslateImagePayloadIfNeeded checks if the modelName is a text-only target (e.g. contains deepseek, o1, o3).
// If so, it translates image_url items in the messages of the JSON payload.
func TranslateImagePayloadIfNeeded(modelName string, payload []byte) []byte {
	lowerModel := strings.ToLower(modelName)
	if !strings.Contains(lowerModel, "deepseek") && !strings.Contains(lowerModel, "o1") && !strings.Contains(lowerModel, "o3") {
		return payload
	}

	msgs := gjson.GetBytes(payload, "messages")
	if !msgs.Exists() || !msgs.IsArray() {
		return payload
	}

	updatedJSON := payload
	msgs.ForEach(func(msgIx, msg gjson.Result) bool {
		content := msg.Get("content")
		if content.Exists() && content.IsArray() {
			hasImage := false
			content.ForEach(func(_, part gjson.Result) bool {
				if part.Get("type").String() == "image_url" {
					hasImage = true
				}
				return true
			})

			if hasImage {
				var newContentJSON []byte = []byte("[]")
				content.ForEach(func(_, part gjson.Result) bool {
					if part.Get("type").String() != "image_url" {
						newContentJSON, _ = sjson.SetRawBytes(newContentJSON, "-1", []byte(part.Raw))
					} else {
						imgURL := part.Get("image_url.url").String()
						if imgURL != "" {
							desc := translateImageToText(imgURL)
							txtBlock := []byte(`{"type":"text","text":""}`)
							txtBlock, _ = sjson.SetBytes(txtBlock, "text", desc)
							newContentJSON, _ = sjson.SetRawBytes(newContentJSON, "-1", txtBlock)
						}
					}
					return true
				})

				path := "messages." + msgIx.String() + ".content"
				updatedJSON, _ = sjson.SetRawBytes(updatedJSON, path, newContentJSON)
			}
		}
		return true
	})

	return updatedJSON
}
