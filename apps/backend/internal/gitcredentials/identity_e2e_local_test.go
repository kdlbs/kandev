package gitcredentials

import "testing"

func e2eLocalInput(remote string) RepositoryIdentityInput {
	return RepositoryIdentityInput{RepositoryID: "repo-1", Provider: "e2e-local", ProviderOwner: "local", ProviderName: "repo", RemoteURL: remote}
}

func TestE2ELocalRemotesResolveUnderTheE2EProfile(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	for _, remote := range []string{"file:///tmp/work/repo.git", "http://127.0.0.1:8080/local/repo.git", "http://localhost/local/repo", "http://[::1]:9/local/repo.git"} {
		identity, err := ResolveRepositoryIdentity(e2eLocalInput(remote))
		if err != nil {
			t.Fatalf("ResolveRepositoryIdentity(%q) error = %v", remote, err)
		}
		if identity.Owner != "local" || identity.Repository != "repo" {
			t.Fatalf("identity for %q = %+v, want owner local and repository repo", remote, identity)
		}
	}
}

func TestE2ELocalRemotesStayRefusedOutsideTheE2EProfile(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	for _, remote := range []string{"file:///tmp/work/repo.git", "http://127.0.0.1:8080/local/repo.git"} {
		if _, err := ResolveRepositoryIdentity(e2eLocalInput(remote)); err == nil {
			t.Fatalf("ResolveRepositoryIdentity(%q) accepted a local remote outside the e2e profile", remote)
		}
	}
}

func TestE2ELocalExceptionIsLimitedToTheSyntheticProviderAndLoopback(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	github := e2eLocalInput("file:///tmp/work/repo.git")
	github.Provider = "github"
	if _, err := ResolveRepositoryIdentity(github); err == nil {
		t.Fatal("a file remote resolved for a real provider")
	}
	for _, remote := range []string{"http://example.com/local/repo.git", "http://10.0.0.5/local/repo.git", "file://host/tmp/repo.git"} {
		if _, err := ResolveRepositoryIdentity(e2eLocalInput(remote)); err == nil {
			t.Fatalf("ResolveRepositoryIdentity(%q) accepted a non-loopback remote", remote)
		}
	}
}
