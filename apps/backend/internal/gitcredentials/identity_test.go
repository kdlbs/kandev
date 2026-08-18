package gitcredentials

import "testing"

func TestResolveRepositoryIdentityAcceptsGitHubTransportSpellingsWithoutProviderHost(t *testing.T) {
	for _, remoteURL := range []string{
		"git@github.com:acme/widgets.git",
		"ssh://git@github.com/acme/widgets.git",
		"https://github.com/acme/widgets.git",
	} {
		t.Run(remoteURL, func(t *testing.T) {
			identity, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
				RepositoryID: "repo-1", Provider: "plugin-provider", RemoteURL: remoteURL,
			})
			if err != nil {
				t.Fatalf("ResolveRepositoryIdentity() error = %v", err)
			}
			if identity.Host != "github.com" || identity.Path != "/acme/widgets.git" {
				t.Fatalf("identity = %#v, want github.com /acme/widgets.git", identity)
			}
		})
	}
}

func TestResolveRepositoryIdentityUsesExplicitProviderHostForSSH(t *testing.T) {
	identity, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
		RepositoryID: "repo-1", ProviderHost: "https://ghe.example:8443",
		RemoteURL: "ssh://git@ghe.example:2222/acme/widgets.git",
	})
	if err != nil {
		t.Fatalf("ResolveRepositoryIdentity() error = %v", err)
	}
	if identity.Host != "ghe.example:8443" || identity.Path != "/acme/widgets.git" {
		t.Fatalf("identity = %#v, want ghe.example:8443 /acme/widgets.git", identity)
	}
}

func TestResolveRepositoryIdentityRejectsUntrustedOrConflictingOrigin(t *testing.T) {
	for _, input := range []RepositoryIdentityInput{
		{RepositoryID: "repo-1", RemoteURL: "ssh://git@ghe.example/acme/widgets.git"},
		{RepositoryID: "repo-1", ProviderHost: "https://github.com", RemoteURL: "https://evil.example/acme/widgets.git"},
	} {
		if _, err := ResolveRepositoryIdentity(input); err == nil {
			t.Fatalf("ResolveRepositoryIdentity(%#v) error = nil, want rejection", input)
		}
	}
}
