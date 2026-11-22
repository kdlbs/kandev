package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityCapabilityNegotiation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"capabilities":["antigravity_personal_auth_v1"]}`))
	}))
	defer server.Close()
	c := newClient(server.URL, newTestLogger())
	if err := c.RequireCapability(context.Background(), acpcompat.AntigravityPersonalAuthCapability); err != nil {
		t.Fatalf("capable sidecar rejected: %v", err)
	}

	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"capabilities":["agent-survival.v1"]}`))
	}))
	defer old.Close()
	oldClient := newClient(old.URL, newTestLogger())
	err := oldClient.RequireCapability(context.Background(), acpcompat.AntigravityPersonalAuthCapability)
	if err == nil {
		t.Fatal("older sidecar was accepted")
	}
}
