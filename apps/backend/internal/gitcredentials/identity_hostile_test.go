package gitcredentials

import (
	"strings"
	"testing"
)

func TestResolveRepositoryIdentityRejectsUnsafeRemoteAuthorities(t *testing.T) {
	t.Parallel()
	remotes := []string{
		"git@gİthub.com:acme/widgets.git",
		"ssh://git@gİthub.com/acme/widgets.git",
		"https://gİthub.com/acme/widgets.git",
		"git@g%C4%B0thub.com:acme/widgets.git",
		"ssh://git@g%C4%B0thub.com/acme/widgets.git",
		"https://g%C4%B0thub.com/acme/widgets.git",
		"git@g%25C4%25B0thub.com:acme/widgets.git",
		"ssh://git@g%25C4%25B0thub.com/acme/widgets.git",
		"https://g%25C4%25B0thub.com/acme/widgets.git",
	}
	for _, providerHost := range []string{"", "https://github.com"} {
		for _, remoteURL := range remotes {
			t.Run(providerHost+"/"+remoteURL, func(t *testing.T) {
				t.Parallel()
				_, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
					RepositoryID: "repo-1", ProviderHost: providerHost, RemoteURL: remoteURL,
				})
				if err == nil {
					t.Fatal("ResolveRepositoryIdentity() error = nil, want rejection")
				}
				if strings.Contains(err.Error(), remoteURL) || !strings.Contains(err.Error(), "repair its clone URL or provider host") {
					t.Fatalf("error = %q, want generic remediation without hostile remote", err)
				}
			})
		}
	}
}

func TestParseRemoteRejectsUnsafeHTTPSAuthorities(t *testing.T) {
	t.Parallel()
	for _, remoteURL := range []string{
		"https://gİthub.com/acme/widgets.git",
		"https://g%C4%B0thub.com/acme/widgets.git",
		"https://g%25C4%25B0thub.com/acme/widgets.git",
	} {
		t.Run(remoteURL, func(t *testing.T) {
			t.Parallel()
			if _, err := parseRemote(remoteURL); err == nil {
				t.Fatalf("parseRemote(%q) error = nil, want rejection", remoteURL)
			}
		})
	}
}

func TestResolveRepositoryIdentityRejectsUnsafeProviderHost(t *testing.T) {
	t.Parallel()
	for _, providerHost := range []string{
		"gİthub.com",
		"g%C4%B0thub.com",
		"g%25C4%25B0thub.com",
	} {
		t.Run(providerHost, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveRepositoryIdentity(RepositoryIdentityInput{
				RepositoryID: "repo-1", ProviderHost: providerHost,
				RemoteURL: "https://github.com/acme/widgets.git",
			})
			if err == nil {
				t.Fatal("ResolveRepositoryIdentity() error = nil, want rejection")
			}
			if strings.Contains(err.Error(), providerHost) || !strings.Contains(err.Error(), "repair its clone URL or provider host") {
				t.Fatalf("error = %q, want generic remediation without hostile provider host", err)
			}
		})
	}
}

func TestResolveRepositoryIdentityPreservesUnicodeAndEncodedPaths(t *testing.T) {
	t.Parallel()
	for _, remoteURL := range []string{
		"git@github.com:acme/wídgets.git",
		"ssh://git@github.com/acme/wídgets.git",
		"https://github.com/acme/wídgets.git",
		"git@github.com:acme/w%C3%ADdgets.git",
		"ssh://git@github.com/acme/w%C3%ADdgets.git",
		"https://github.com/acme/w%C3%ADdgets.git",
	} {
		t.Run(remoteURL, func(t *testing.T) {
			t.Parallel()
			identity, err := ResolveRepositoryIdentity(RepositoryIdentityInput{RepositoryID: "repo-1", RemoteURL: remoteURL})
			if err != nil {
				t.Fatalf("ResolveRepositoryIdentity() error = %v", err)
			}
			if identity.Path != "/acme/wídgets.git" {
				t.Fatalf("identity.Path = %q, want %q", identity.Path, "/acme/wídgets.git")
			}
		})
	}
}
