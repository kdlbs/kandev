package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func setupHandshakeServer(cfg *config.Config) *ControlServer {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return NewControlServer(cfg, nil, log)
}

func TestHandshake_ValidNonce(t *testing.T) {
	cfg := &config.Config{
		AuthToken:      "self-generated-token",
		BootstrapNonce: "test-nonce-123",
	}
	cs := setupHandshakeServer(cfg)

	body, _ := json.Marshal(map[string]string{"nonce": "test-nonce-123"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/handshake", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["token"] != "self-generated-token" {
		t.Fatalf("expected self-generated-token, got %q", resp["token"])
	}
}

func TestHandshake_NonceBurnedAfterUse(t *testing.T) {
	cfg := &config.Config{
		AuthToken:      "token-abc",
		BootstrapNonce: "one-time-nonce",
	}
	cs := setupHandshakeServer(cfg)

	// First handshake succeeds
	body, _ := json.Marshal(map[string]string{"nonce": "one-time-nonce"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/handshake", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("first handshake: expected 200, got %d", w.Code)
	}

	// Second handshake with same nonce fails (burned)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/auth/handshake", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Fatalf("second handshake: expected 403, got %d", w2.Code)
	}
}

func TestHandshake_WrongNonce(t *testing.T) {
	cfg := &config.Config{
		AuthToken:      "token-abc",
		BootstrapNonce: "correct-nonce",
	}
	cs := setupHandshakeServer(cfg)

	body, _ := json.Marshal(map[string]string{"nonce": "wrong-nonce"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/handshake", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHandshake_MissingNonce(t *testing.T) {
	cfg := &config.Config{
		AuthToken:      "token-abc",
		BootstrapNonce: "some-nonce",
	}
	cs := setupHandshakeServer(cfg)

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/handshake", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandshake_ExemptFromAuth(t *testing.T) {
	// When auth is enabled, /auth/handshake should still be accessible without a token
	cfg := &config.Config{
		AuthToken:      "self-generated-token",
		BootstrapNonce: "test-nonce",
	}
	cs := setupHandshakeServer(cfg)

	// First verify that a protected endpoint requires auth
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("GET", "/api/v1/instances", nil)
	cs.router.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for protected endpoint, got %d", w1.Code)
	}

	// But handshake should work without auth
	body, _ := json.Marshal(gin.H{"nonce": "test-nonce"})
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/auth/handshake", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	cs.router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for handshake (exempt), got %d: %s", w2.Code, w2.Body.String())
	}
}
