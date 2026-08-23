package plugins

import (
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// webhookAccess assigns access declarations to a test record's webhooks.
type webhookAccess map[string]string

func ssoRecord(id string, status string, auth bool, webhooks []string, providers []manifest.AuthProvider) *store.Record {
	return ssoRecordWithAccess(id, status, auth, webhooks, nil, providers)
}

func ssoRecordWithAccess(
	id, status string, auth bool, webhooks []string, access webhookAccess, providers []manifest.AuthProvider,
) *store.Record {
	whs := make([]manifest.Webhook, 0, len(webhooks))
	for _, k := range webhooks {
		whs = append(whs, manifest.Webhook{Key: k, Access: access[k]})
	}
	return &store.Record{
		Status: status,
		Manifest: manifest.Manifest{
			APIVersion:    manifest.CurrentAPIVersion,
			ID:            id,
			Capabilities:  manifest.Capabilities{Auth: auth},
			Webhooks:      whs,
			AuthProviders: providers,
		},
	}
}

func TestSSOProvidersPreservesLegacyPublicDefault(t *testing.T) {
	record := ssoRecordWithAccess("legacy", StatusActive, true, []string{"initiate"}, nil,
		[]manifest.AuthProvider{{ID: "legacy", DisplayName: "Legacy", Initiate: "initiate"}})
	record.APIVersion = manifest.LegacyAPIVersion
	reg := NewRegistry()
	reg.Add(record)

	svc := &Service{registry: reg}
	if got := svc.SSOProviders(); len(got) != 1 {
		t.Fatalf("SSOProviders() = %d providers, want 1 for v1 public default: %+v", len(got), got)
	}
}

func TestSSOProviders(t *testing.T) {
	reg := NewRegistry()
	// Valid: active, auth-capable, provider names a declared, public webhook.
	reg.Add(ssoRecordWithAccess("google", StatusActive, true, []string{"initiate", "callback"},
		webhookAccess{"initiate": manifest.WebhookAccessPublic, "callback": manifest.WebhookAccessPublic},
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))
	// Skipped: provider names an undeclared webhook (button would 404).
	reg.Add(ssoRecord("broken", StatusActive, true, nil,
		[]manifest.AuthProvider{{ID: "x", DisplayName: "X", Initiate: "missing"}}))
	// Skipped: declares providers but lacks the auth capability.
	reg.Add(ssoRecordWithAccess("nocap", StatusActive, false, []string{"initiate"},
		webhookAccess{"initiate": manifest.WebhookAccessPublic},
		[]manifest.AuthProvider{{ID: "y", DisplayName: "Y", Initiate: "initiate"}}))
	// Skipped: inactive plugin.
	reg.Add(ssoRecordWithAccess("inactive", store.StatusDisabled, true, []string{"initiate"},
		webhookAccess{"initiate": manifest.WebhookAccessPublic},
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
// initiate key is not `access: public` is omitted (the login
// button's very first request has no session or PAT to present, so a
// non-public initiate would 401 immediately), and the same provider is
// included once the manifest marks that key public.
func TestSSOProvidersSkipsNonPublicInitiateWebhook(t *testing.T) {
	reg := NewRegistry()
	reg.Add(ssoRecordWithAccess("google", StatusActive, true, []string{"initiate"}, nil,
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))

	svc := &Service{registry: reg}
	if got := svc.SSOProviders(); len(got) != 0 {
		t.Fatalf("SSOProviders() = %d providers, want 0 (initiate not public): %+v", len(got), got)
	}

	reg2 := NewRegistry()
	reg2.Add(ssoRecordWithAccess("google", StatusActive, true, []string{"initiate"},
		webhookAccess{"initiate": manifest.WebhookAccessPublic},
		[]manifest.AuthProvider{{ID: "google", DisplayName: "Google", Initiate: "initiate"}}))
	svc2 := &Service{registry: reg2}
	if got := svc2.SSOProviders(); len(got) != 1 {
		t.Fatalf("SSOProviders() = %d providers, want 1 (initiate public): %+v", len(got), got)
	}
}
