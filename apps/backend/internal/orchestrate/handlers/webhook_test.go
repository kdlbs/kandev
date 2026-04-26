package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestVerifySignature_None(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	if !verifySignature("none", "", r, nil) {
		t.Error("mode=none should always pass")
	}
	if !verifySignature("", "", r, nil) {
		t.Error("empty mode should always pass")
	}
}

func TestVerifySignature_Bearer(t *testing.T) {
	secret := "my-token-123"
	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	if !verifySignature("bearer", secret, r, nil) {
		t.Error("valid bearer should pass")
	}

	r.Header.Set("Authorization", "Bearer wrong")
	if verifySignature("bearer", secret, r, nil) {
		t.Error("wrong bearer should fail")
	}
}

func TestVerifySignature_HMAC(t *testing.T) {
	secret := "webhook-secret"
	body := []byte(`{"branch":"main"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	r, _ := http.NewRequest("POST", "/", nil)
	r.Header.Set("X-Signature-256", sig)
	if !verifySignature("hmac_sha256", secret, r, body) {
		t.Error("valid HMAC should pass")
	}

	r.Header.Set("X-Signature-256", "sha256=deadbeef")
	if verifySignature("hmac_sha256", secret, r, body) {
		t.Error("wrong HMAC should fail")
	}
}

func TestVerifySignature_HMAC_MissingHeader(t *testing.T) {
	r, _ := http.NewRequest("POST", "/", nil)
	if verifySignature("hmac_sha256", "secret", r, []byte("body")) {
		t.Error("missing header should fail")
	}
}

func TestParseWebhookPayload(t *testing.T) {
	body := []byte(`{"branch":"release/2.0","count":5}`)
	vars := parseWebhookPayload(body)
	if vars["branch"] != "release/2.0" {
		t.Errorf("branch = %q", vars["branch"])
	}
	if vars["count"] != "5" {
		t.Errorf("count = %q", vars["count"])
	}
}

func TestParseWebhookPayload_Empty(t *testing.T) {
	vars := parseWebhookPayload(nil)
	if len(vars) != 0 {
		t.Errorf("expected empty map, got %v", vars)
	}
}
