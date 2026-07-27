package plugins

import (
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

func ssoRecord(id string, status string, auth bool, webhooks []string, providers []manifest.AuthProvider) *store.Record {
	whs := make([]manifest.Webhook, 0, len(webhooks))
	for _, k := range webhooks {
		whs = append(whs, manifest.Webhook{Key: k})
	}
	return &store.Record{
		Status: status,
		Manifest: manifest.Manifest{
			ID:            id,
			Capabilities:  manifest.Capabilities{Auth: auth},
			Webhooks:      whs,
			AuthProviders: providers,
		},
	}
}

func TestSSOProviders(t *testing.T) {
	reg := NewRegistry()
	// Valid: active, auth-capable, provider names a declared webhook.
	reg.Add(ssoRecord("google", StatusActive, true, []string{"initiate", "callback"},
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))
	// Skipped: provider names an undeclared webhook (button would 404).
	reg.Add(ssoRecord("broken", StatusActive, true, nil,
		[]manifest.AuthProvider{{ID: "x", DisplayName: "X", Initiate: "missing"}}))
	// Skipped: declares providers but lacks the auth capability.
	reg.Add(ssoRecord("nocap", StatusActive, false, []string{"initiate"},
		[]manifest.AuthProvider{{ID: "y", DisplayName: "Y", Initiate: "initiate"}}))
	// Skipped: inactive plugin.
	reg.Add(ssoRecord("inactive", store.StatusDisabled, true, []string{"initiate"},
		[]manifest.AuthProvider{{ID: "z", DisplayName: "Z", Initiate: "initiate"}}))

	svc := &Service{registry: reg}
	got := svc.SSOProviders()

	if len(got) != 1 {
		t.Fatalf("SSOProviders() = %d providers, want 1: %+v", len(got), got)
	}
	p := got[0]
	if p.ID != "google" || p.DisplayName != "Google" ||
		p.InitiateURL != "/api/plugins/google/webhooks/initiate" {
		t.Fatalf("unexpected provider: %+v", p)
	}
}
