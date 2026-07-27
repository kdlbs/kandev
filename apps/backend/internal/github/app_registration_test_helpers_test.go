package github

import (
	"context"
	"testing"
)

type manifestConverterFunc func(context.Context, string) (ManifestConversionResult, error)

func (f manifestConverterFunc) Convert(ctx context.Context, code string) (ManifestConversionResult, error) {
	return f(ctx, code)
}

func testManifestConversion(t *testing.T, ownerLogin, ownerType string) ManifestConversionResult {
	t.Helper()
	_, privateKey := testAppPrivateKey(t)
	return ManifestConversionResult{
		AppID: 123, Slug: "kandev-acme", Name: "Kandev Acme",
		Owner:    ManifestConversionOwner{ID: 42, Login: ownerLogin, Type: ownerType},
		ClientID: "Iv1.client", ClientSecret: "generated-client",
		WebhookSecret: "generated-webhook", PrivateKeyPEM: string(privateKey),
		Permissions: map[string]string{
			"actions": "read", "administration": "read", "checks": "read",
			"contents": "write", "issues": "write", "members": "read",
			"metadata": "read", "pull_requests": "write", "statuses": "read",
			"workflows": "write",
		},
		Events: []string{"installation", "installation_repositories", "github_app_authorization"},
	}
}
