package config

import "testing"

func TestAzureDevOpsOAuthConfigEnvironmentBinding(t *testing.T) {
	t.Setenv("KANDEV_AZURE_DEVOPS_OAUTH_CLIENT_ID", "11111111-2222-3333-4444-555555555555")
	t.Setenv("KANDEV_AZURE_DEVOPS_OAUTH_REDIRECT_URL", "https://kandev.example.com/api/v1/azure-devops/oauth/callback")
	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if got := cfg.AzureDevOps.OAuthClientID; got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("client ID = %q", got)
	}
	if got := cfg.AzureDevOps.OAuthRedirectURL; got != "https://kandev.example.com/api/v1/azure-devops/oauth/callback" {
		t.Fatalf("redirect URL = %q", got)
	}
	for _, key := range []string{"azureDevOps.oauthClientId", "azureDevOps.oauthRedirectUrl"} {
		if _, ok := CatalogEntryForKey(key); !ok {
			t.Errorf("catalog is missing %q", key)
		}
	}
}

func TestAzureDevOpsOAuthRedirectURLValidation(t *testing.T) {
	for _, valid := range []string{
		"",
		"http://localhost:38429/api/v1/azure-devops/oauth/callback",
		"https://kandev.example.com/api/v1/azure-devops/oauth/callback",
	} {
		if err := (AzureDevOpsConfig{OAuthRedirectURL: valid}).validate(); err != nil {
			t.Errorf("validate(%q) = %v, want nil", valid, err)
		}
	}
	for _, invalid := range []string{
		"http://kandev.example.com/api/v1/azure-devops/oauth/callback",
		"https://kandev.example.com/callback?next=x",
		"/api/v1/azure-devops/oauth/callback",
	} {
		if err := (AzureDevOpsConfig{OAuthRedirectURL: invalid}).validate(); err == nil {
			t.Errorf("validate(%q) = nil, want error", invalid)
		}
	}
}
