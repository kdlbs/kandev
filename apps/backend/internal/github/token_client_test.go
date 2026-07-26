package github

import "testing"

func TestTokenClient_PreservesPrincipalMetadata(t *testing.T) {
	principal := TokenPrincipal{
		Kind:           TokenCredentialInstallation,
		PrincipalID:    "installation:42",
		Login:          "acme",
		InstallationID: 42,
	}
	client := NewTokenClient("token", principal)
	if got := client.Principal(); got != principal {
		t.Fatalf("Principal() = %+v, want %+v", got, principal)
	}
}

func TestTokenClient_PATCompatibility(t *testing.T) {
	client := NewPATClient("pat")
	if got := client.Principal().Kind; got != TokenCredentialPAT {
		t.Fatalf("PAT principal kind = %q, want %q", got, TokenCredentialPAT)
	}
	var _ Client = client
	var _ GraphQLExecutor = client
}

func TestTokenClient_AppUserConstructor(t *testing.T) {
	client := NewAppUserTokenClient("ghu_token", 123, "octocat")
	principal := client.Principal()
	if principal.Kind != TokenCredentialUser || principal.PrincipalID != "user:123" || principal.Login != "octocat" {
		t.Fatalf("App user principal = %+v", principal)
	}
}
