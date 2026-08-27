package gitcredentials

import (
	"strings"
	"testing"
)

func TestResolveRepositoryIdentityAcceptsGitHubTransportSpellingsWithoutProviderHost(t *testing.T) {
	for _, provider := range []string{"", "plugin-provider", "kandev-plugin-tags"} {
		for _, remoteURL := range []string{
			"git@github.com:acme/widgets.git",
			"ssh://git@github.com/acme/widgets.git",
			"https://github.com/acme/widgets.git",
		} {
			t.Run(provider+"/"+remoteURL, func(t *testing.T) {
				identity, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
					RepositoryID: "repo-1", Provider: provider, RemoteURL: remoteURL,
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
}

func TestResolveRepositoryIdentityUsesExplicitProviderHostForHTTPS(t *testing.T) {
	identity, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
		RepositoryID: "repo-1", Provider: "acme-forge", ProviderHost: "https://forge.example:9443",
		RemoteURL: "https://forge.example:9443/acme/widgets.git",
	})
	if err != nil {
		t.Fatalf("ResolveRepositoryIdentity() error = %v", err)
	}
	if identity.Host != "forge.example:9443" || identity.Path != "/acme/widgets.git" {
		t.Fatalf("identity = %#v, want forge.example:9443 /acme/widgets.git", identity)
	}
}

// A custom host with no ProviderHost metadata must fail closed even when
// Provider names a would-be resolver and owner/name are populated: only an
// explicit provider host - or the well-known public github.com - is a
// trusted origin.
func TestResolveRepositoryIdentityRequiresExplicitProviderHostForNonGitHubProvider(t *testing.T) {
	_, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
		RepositoryID: "repo-1", Provider: "acme-forge", ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://forge.example/acme/widgets.git",
	})
	if err == nil {
		t.Fatal("ResolveRepositoryIdentity() error = nil, want rejection of an untrusted custom host")
	}
}

// A GitHub row whose persisted owner/name conflict with its remote's actual
// path must be rejected even though the remote itself resolves to a trusted
// github.com origin - populated provider identity fields must not silently
// win over a mismatched remote.
func TestResolveRepositoryIdentityRejectsGitHubOwnerNameConflictWithRemotePath(t *testing.T) {
	_, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
		RepositoryID: "repo-1", Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://github.com/acme/other-repo.git",
	})
	if err == nil {
		t.Fatal("ResolveRepositoryIdentity() error = nil, want rejection of a provider/remote path conflict")
	}
}

// Credential-bearing and query/fragment-bearing URLs must be rejected, and
// the rejection must never echo the credential-bearing URL back - only the
// safe repository ID and a generic remediation.
func TestResolveRepositoryIdentityRejectsCredentialBearingAndMalformedURLsWithoutLeakingSecrets(t *testing.T) {
	for name, remoteURL := range map[string]string{
		"https userinfo":     "https://oauth2:sh_supersecret@github.com/acme/widgets.git",
		"https query":        "https://github.com/acme/widgets.git?token=sh_supersecret",
		"https fragment":     "https://github.com/acme/widgets.git#sh_supersecret",
		"ssh password":       "ssh://git:sh_supersecret@github.com/acme/widgets.git",
		"ssh query":          "ssh://git@github.com/acme/widgets.git?token=sh_supersecret",
		"ssh fragment":       "ssh://git@github.com/acme/widgets.git#sh_supersecret",
		"scp-style query":    "git@github.com:acme/widgets.git?token=sh_supersecret",
		"scp-style fragment": "git@github.com:acme/widgets.git#sh_supersecret",
		"malformed":          "not-a-valid-url",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveRepositoryIdentity(RepositoryIdentityInput{RepositoryID: "repo-1", RemoteURL: remoteURL})
			if err == nil {
				t.Fatalf("ResolveRepositoryIdentity(%q) error = nil, want rejection", remoteURL)
			}
			if strings.Contains(err.Error(), "sh_supersecret") {
				t.Fatalf("error leaked the credential-bearing URL: %v", err)
			}
			if strings.Contains(err.Error(), remoteURL) {
				t.Fatalf("error leaked the rejected remote: %v", err)
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
