package acpcompat

import (
	"strings"
	"testing"
	"time"
)

func TestAntigravityACPMessageEnvelope(t *testing.T) {
	env := ParseAntigravityACPMessage(-32000, "Internal error", map[string]any{
		"status":     "RESOURCE_EXHAUSTED",
		"reason":     "QUOTA_EXHAUSTED",
		"httpStatus": float64(429),
		"message":    "You have reached your quota.",
	})
	if env.RPCCode != -32000 {
		t.Errorf("rpc code = %d, want -32000", env.RPCCode)
	}
	if env.HTTPStatus != 429 || env.Status != "RESOURCE_EXHAUSTED" || env.Reason != "QUOTA_EXHAUSTED" {
		t.Errorf("structured fields = (%d, %q, %q)", env.HTTPStatus, env.Status, env.Reason)
	}
	if env.Message != "You have reached your quota." {
		t.Errorf("message = %q", env.Message)
	}
	if !env.QuotaExhausted {
		t.Error("QuotaExhausted = false, want true for QUOTA_EXHAUSTED")
	}
	if env.ResetHint != nil || env.ResetDelay != 0 {
		t.Errorf("reset evidence = (%v, %v), want unknown", env.ResetHint, env.ResetDelay)
	}
}

func TestAntigravityACPMessageParsesNestedEnvelope(t *testing.T) {
	env := ParseAntigravityACPMessage(-32000, "Internal error", map[string]any{
		"error": `Error 503, Message: Service unavailable, Status: UNAVAILABLE`,
	})
	if env.HTTPStatus != 503 || env.Status != "UNAVAILABLE" {
		t.Fatalf("nested envelope = (%d, %q), want (503, UNAVAILABLE)", env.HTTPStatus, env.Status)
	}
	if env.Message != "Service unavailable" {
		t.Fatalf("message = %q, want the nested provider message", env.Message)
	}
}

func TestAntigravityNativeErrorPayload(t *testing.T) {
	payload := []byte("Error 429, Message: Quota exhausted for this project., Status: RESOURCE_EXHAUSTED\n" +
		"reason:QUOTA_EXHAUSTED\n" +
		"quotaResetTimeStamp:2026-11-03T14:30:00Z\n" +
		"quotaResetDelay:2h0m0s\n" +
		"goroutine 1 [running]:\nmain.main()\n")
	env, ok := ParseAntigravityNativeErrorPayload(payload)
	if !ok {
		t.Fatal("native payload was not parsed")
	}
	if env.HTTPStatus != 429 || env.Status != "RESOURCE_EXHAUSTED" || env.Reason != "QUOTA_EXHAUSTED" {
		t.Fatalf("fields = (%d, %q, %q)", env.HTTPStatus, env.Status, env.Reason)
	}
	if env.Message != "Quota exhausted for this project." {
		t.Fatalf("message = %q", env.Message)
	}
	if env.ResetHint == nil || !env.ResetHint.Equal(time.Date(2026, 11, 3, 14, 30, 0, 0, time.UTC)) {
		t.Fatalf("reset hint = %v, want the provider timestamp", env.ResetHint)
	}
	if env.ResetDelay != 2*time.Hour {
		t.Fatalf("reset delay = %v, want 2h", env.ResetDelay)
	}
	if !env.QuotaExhausted {
		t.Error("QuotaExhausted = false")
	}
}

func TestAntigravityNativeErrorPayloadJSON(t *testing.T) {
	env, ok := ParseAntigravityNativeErrorPayload([]byte(
		`{"status":"RESOURCE_EXHAUSTED","reason":"QUOTA_EXHAUSTED","message":"Quota exhausted."}`,
	))
	if !ok {
		t.Fatal("JSON native payload was not parsed")
	}
	if env.Status != "RESOURCE_EXHAUSTED" || env.Reason != "QUOTA_EXHAUSTED" {
		t.Fatalf("fields = (%q, %q)", env.Status, env.Reason)
	}
	if env.Message != "Quota exhausted." {
		t.Fatalf("message = %q", env.Message)
	}
	if !env.QuotaExhausted {
		t.Error("QuotaExhausted = false")
	}
}

func TestAntigravityNativeErrorPayloadWithoutReset(t *testing.T) {
	env, ok := ParseAntigravityNativeErrorPayload([]byte(
		"Error 429, Message: Too many requests., Status: RESOURCE_EXHAUSTED\n",
	))
	if !ok {
		t.Fatal("native payload was not parsed")
	}
	if env.ResetHint != nil || env.ResetDelay != 0 {
		t.Fatalf("reset evidence = (%v, %v), want unknown", env.ResetHint, env.ResetDelay)
	}
	if env.QuotaExhausted {
		t.Error("QuotaExhausted = true without quota evidence")
	}
}

func TestAntigravityNativeErrorPayloadRejectsEmptyAndBoundsText(t *testing.T) {
	if _, ok := ParseAntigravityNativeErrorPayload(nil); ok {
		t.Fatal("empty payload parsed as evidence")
	}
	if _, ok := ParseAntigravityNativeErrorPayload([]byte(" \n\t ")); ok {
		t.Fatal("blank payload parsed as evidence")
	}

	oversized := []byte(strings.Repeat("x", AntigravityMaxErrorPayloadBytes+4096))
	env, ok := ParseAntigravityNativeErrorPayload(oversized)
	if !ok {
		t.Fatal("oversized payload was not parsed")
	}
	if len(env.Message) > AntigravityMaxErrorMessageBytes {
		t.Fatalf("message length = %d, want <= %d", len(env.Message), AntigravityMaxErrorMessageBytes)
	}
}

func TestAntigravityNativeErrorPayloadUnknownData(t *testing.T) {
	env, ok := ParseAntigravityNativeErrorPayload([]byte("something unexpected happened"))
	if !ok {
		t.Fatal("unknown payload was not parsed")
	}
	if env.Message != "something unexpected happened" {
		t.Fatalf("message = %q, want the bounded native text", env.Message)
	}
	if env.HTTPStatus != 0 || env.Status != "" || env.Reason != "" {
		t.Fatalf("unknown payload invented structured fields: %+v", env)
	}
}

func TestAntigravityInvalidResetTimestampStaysUnknown(t *testing.T) {
	env, ok := ParseAntigravityNativeErrorPayload([]byte(
		"Error 429, Message: Quota exhausted., Status: RESOURCE_EXHAUSTED\n" +
			"reason:QUOTA_EXHAUSTED\n" +
			"quotaResetTimeStamp:not-a-time\n",
	))
	if !ok {
		t.Fatal("native payload was not parsed")
	}
	if env.ResetHint != nil {
		t.Fatalf("reset hint = %v, want unknown for an invalid timestamp", env.ResetHint)
	}
}
