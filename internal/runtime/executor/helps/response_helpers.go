package helps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

)

type statusError struct {
	code    int
	message string
}

func (e *statusError) Error() string { return e.message }
func (e *statusError) StatusCode() int { return e.code }

// ValidateUpstreamResponse ensures that an upstream HTTP response with status 200
// actually contains a valid, non-empty payload. If the payload is empty or malformed,
// it returns a retryable 502 Bad Gateway error.
func ValidateUpstreamResponse(ctx context.Context, statusCode int, headers http.Header, body []byte) error {
	if statusCode < 200 || statusCode >= 300 {
		return &statusError{
			code:    statusCode,
			message: string(body),
		}
	}

	if statusCode != http.StatusOK {
		return nil
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return &statusError{
			code:    http.StatusBadGateway,
			message: "upstream returned an empty response (HTTP 200) - possible gateway interception",
		}
	}

	contentType := strings.ToLower(headers.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") {
		var decoded any
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			return &statusError{
				code:    http.StatusBadGateway,
				message: fmt.Sprintf("upstream returned malformed JSON (HTTP 200): %v", err),
			}
		}
	}

	return nil
}
