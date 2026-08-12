package plugins

import (
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// publicWebhookKeys marks a subset of a ssoRecord's declared webhook keys as
// webhooks[].public: true, standing in for the manifest flag from the
// caller's perspective.
type publicWebhookKeys map[string]bool

func ssoRecord(id string, status string, auth bool, webhooks []string, providers []manifest.AuthProvider) *store.Record {
	return ssoRecordWithPublic(id, status, auth, webhooks, nil, providers)
}

func ssoRecordWithPublic(
	id, status string, auth bool, webhooks []string, public publicWebhookKeys, providers []manifest.AuthProvider,
) *store.Record {
	whs := make([]manifest.Webhook, 0, len(webhooks))
	for _, k := range webhooks {
		whs = append(whs, manifest.Webhook{Key: k, Public: public[k]})
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
	// Valid: active, auth-capable, provider names a declared, public webhook.
	reg.Add(ssoRecordWithPublic("google", StatusActive, true, []string{"initiate", "callback"},
		publicWebhookKeys{"initiate": true, "callback": true},
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))
	// Skipped: provider names an undeclared webhook (button would 404).
	reg.Add(ssoRecord("broken", StatusActive, true, nil,
		[]manifest.AuthProvider{{ID: "x", DisplayName: "X", Initiate: "missing"}}))
	// Skipped: declares providers but lacks the auth capability.
	reg.Add(ssoRecordWithPublic("nocap", StatusActive, false, []string{"initiate"},
		publicWebhookKeys{"initiate": true},
		[]manifest.AuthProvider{{ID: "y", DisplayName: "Y", Initiate: "initiate"}}))
	// Skipped: inactive plugin.
	reg.Add(ssoRecordWithPublic("inactive", store.StatusDisabled, true, []string{"initiate"},
		publicWebhookKeys{"initiate": true},
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

// TestSSOProvidersSkipsNonPublicInitiateWebhook pins AC8: a provider whose
// initiate key is declared but not `public: true` is omitted (the login
// button's very first request has no session or PAT to present, so a
// non-public initiate would 401 immediately), and the same provider is
// included once the manifest marks that key public.
func TestSSOProvidersSkipsNonPublicInitiateWebhook(t *testing.T) {
	reg := NewRegistry()
	reg.Add(ssoRecordWithPublic("google", StatusActive, true, []string{"initiate"}, nil,
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))

	svc := &Service{registry: reg}
	if got := svc.SSOProviders(); len(got) != 0 {
		t.Fatalf("SSOProviders() = %d providers, want 0 (initiate not public): %+v", len(got), got)
	}

	reg2 := NewRegistry()
	reg2.Add(ssoRecordWithPublic("google", StatusActive, true, []string{"initiate"},
		publicWebhookKeys{"initiate": true},
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))
	svc2 := &Service{registry: reg2}
	if got := svc2.SSOProviders(); len(got) != 1 {
		t.Fatalf("SSOProviders() = %d providers, want 1 (initiate public): %+v", len(got), got)
	}
}
