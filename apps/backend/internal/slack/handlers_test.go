package slack

import (
	"encoding/json"
	"errors"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestWsFail_MapsAPIErrorCodes locks the wire-level error codes wsFail
// produces for each input variant. wsFail mirrors writeClientError's HTTP
// mapping so WS clients see the same distinct errors (bad-request,
// unauthorized, forbidden, not-found, internal) instead of the previous
// "everything → INTERNAL_ERROR" collapse, and a regression here would put
// the WS path silently out of sync with the HTTP path again.
func TestWsFail_MapsAPIErrorCodes(t *testing.T) {
	msg := &ws.Message{ID: "1", Action: ws.ActionSlackConfigTest}

	cases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"ErrNotConfigured → SLACK_NOT_CONFIGURED", ErrNotConfigured, errCodeSlackNotConfigured},
		{"APIError 400 → BAD_REQUEST", &APIError{StatusCode: 400, Message: "bad"}, ws.ErrorCodeBadRequest},
		{"APIError 401 → UNAUTHORIZED", &APIError{StatusCode: 401, Message: "unauth"}, ws.ErrorCodeUnauthorized},
		{"APIError 403 → FORBIDDEN", &APIError{StatusCode: 403, Message: "forbidden"}, ws.ErrorCodeForbidden},
		{"APIError 404 → NOT_FOUND", &APIError{StatusCode: 404, Message: "notfound"}, ws.ErrorCodeNotFound},
		{"APIError 500 → INTERNAL_ERROR", &APIError{StatusCode: 500, Message: "server"}, ws.ErrorCodeInternalError},
		{"plain error → INTERNAL_ERROR", errors.New("transient"), ws.ErrorCodeInternalError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, err := wsFail(msg, c.err)
			if err != nil {
				t.Fatalf("wsFail returned error: %v", err)
			}
			if resp == nil {
				t.Fatal("wsFail returned nil response")
			}
			var payload ws.ErrorPayload
			if jsonErr := json.Unmarshal(resp.Payload, &payload); jsonErr != nil {
				t.Fatalf("decode payload: %v (raw=%s)", jsonErr, string(resp.Payload))
			}
			if payload.Code != c.wantCode {
				t.Errorf("code mismatch: got %q want %q", payload.Code, c.wantCode)
			}
			if payload.Message == "" {
				t.Error("expected non-empty message in error payload")
			}
		})
	}
}
