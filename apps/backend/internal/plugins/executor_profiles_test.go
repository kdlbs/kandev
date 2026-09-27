package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
)

func TestPluginExecutorCatalog(t *testing.T) {
	service := NewService(store.NewFSStore(t.TempDir()), NewRegistry(), nil, nil)
	service.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID:           "example-provider",
			Capabilities: manifest.Capabilities{ExecutorProvider: true},
			ExecutorProviders: []manifest.ExecutorProvider{{
				Key: "lambda", DisplayName: "Lambda", Description: "Remote build environment",
				LocalizedMessages: map[string]string{"display_name": "provider.name"},
				ContractVersion:   manifest.CurrentExecutorProviderContractVersion,
				ProfileSchema:     map[string]any{"type": "object", "properties": map[string]any{"region": map[string]any{"type": "string"}}},
				Capabilities:      manifest.ExecutorProviderCapabilities{Terminal: true, Retention: "bounded", MaximumLifetimeSecs: 3600},
			}},
		},
		InstallationID: "installation-1", Status: StatusActive,
	})
	service.SetRuntime(&fakeRuntime{running: map[string]bool{"example-provider": true}})

	providers, err := service.ListExecutorProviders(context.Background())
	if err != nil {
		t.Fatalf("ListExecutorProviders() error = %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("ListExecutorProviders() returned %d entries, want 1", len(providers))
	}
	provider := providers[0]
	if provider.Identity != "plugin:example-provider:lambda" || provider.ExecutorID != models.ExecutorProviderExecutorID(provider.Identity) {
		t.Fatalf("provider identity = %q, executor id = %q", provider.Identity, provider.ExecutorID)
	}
	if !provider.Available || provider.DisplayName != "Lambda" || !provider.Capabilities.Terminal {
		t.Fatalf("provider projection = %+v", provider)
	}
	if provider.LocalizedMessages["display_name"] != "provider.name" {
		t.Fatalf("localized message references = %#v", provider.LocalizedMessages)
	}

}

func TestPluginExecutorProfileSecrets(t *testing.T) {
	secretID := "vault-reference-123"
	stored := map[string]string{
		"region":     "eu-west-1",
		"credential": models.ExecutorProfileSecretReference(secretID),
	}
	config, configured := models.RedactExecutorProfileConfig(stored)
	if config["region"] != "eu-west-1" || len(config) != 1 {
		t.Fatalf("redacted config = %#v, want only non-secret fields", config)
	}
	if !configured["credential"] || len(configured) != 1 {
		t.Fatalf("configured secret projection = %#v, want credential=true", configured)
	}
	if _, ok := config["credential"]; ok {
		t.Fatal("secret reference appeared in ordinary profile config")
	}
	if id, ok := models.ExecutorProfileSecretID(stored["credential"]); !ok || id != secretID {
		t.Fatalf("stored reference decoded as (%q, %v), want %q", id, ok, secretID)
	}
}
