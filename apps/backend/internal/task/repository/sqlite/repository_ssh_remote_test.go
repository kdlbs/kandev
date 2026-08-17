package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// sshRemoteURLSpellings are the four shapes a checkout cloned over SSH leaves
// in repositories.remote_url. Rows in this shape have existed in production for
// months; every fixture feeding the credential and materialization paths was
// HTTPS, which is how #2673 shipped. Persistence is where those rows come from,
// so pin that the column survives the round trip verbatim.
func sshRemoteURLSpellings() map[string]string {
	return map[string]string{
		"scp style suffixed":  "git@github.com:acme/widgets.git",
		"scp style bare":      "git@github.com:acme/widgets",
		"ssh scheme suffixed": "ssh://git@github.com/acme/widgets.git",
		"ssh scheme bare":     "ssh://git@github.com/acme/widgets",
	}
}

// An SSH remote_url must survive create -> get -> list unchanged. A rewrite or
// a dropped column here would silently re-point every downstream consumer at a
// different repository, or at none.
func TestRepositorySSHRemoteURL_RoundTrip(t *testing.T) {
	for name, remoteURL := range sshRemoteURLSpellings() {
		t.Run(name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			seedWorkspace(t, repo, "ws-ssh")

			in := &models.Repository{
				ID:            "repo-ssh",
				WorkspaceID:   "ws-ssh",
				Name:          "acme/widgets",
				SourceType:    "provider",
				Provider:      "github",
				ProviderHost:  "https://github.com",
				ProviderOwner: "acme",
				ProviderName:  "widgets",
				RemoteURL:     remoteURL,
				DefaultBranch: "main",
			}
			if err := repo.CreateRepository(ctx, in); err != nil {
				t.Fatalf("create repository: %v", err)
			}

			got, err := repo.GetRepository(ctx, in.ID)
			if err != nil {
				t.Fatalf("get repository: %v", err)
			}
			if got.RemoteURL != remoteURL {
				t.Fatalf("GetRepository RemoteURL = %q, want %q", got.RemoteURL, remoteURL)
			}
			assertSSHRepositoryIdentityIntact(t, got, remoteURL)

			list, err := repo.ListRepositories(ctx, "ws-ssh")
			if err != nil {
				t.Fatalf("list repositories: %v", err)
			}
			if len(list) != 1 {
				t.Fatalf("ListRepositories returned %d rows, want 1", len(list))
			}
			if list[0].RemoteURL != remoteURL {
				t.Fatalf("ListRepositories RemoteURL = %q, want %q", list[0].RemoteURL, remoteURL)
			}
			assertSSHRepositoryIdentityIntact(t, list[0], remoteURL)
		})
	}
}

// The provider columns are the identity a managed credential scope is derived
// from, so they must still be readable alongside an SSH remote. This is the
// exact combination #2673 proved was untested: SSH transport, valid provider
// identity.
func assertSSHRepositoryIdentityIntact(t *testing.T, got *models.Repository, remoteURL string) {
	t.Helper()
	if got.Provider != "github" || got.ProviderOwner != "acme" || got.ProviderName != "widgets" {
		t.Fatalf("provider identity = %q %q/%q, want github acme/widgets (remote %q)",
			got.Provider, got.ProviderOwner, got.ProviderName, remoteURL)
	}
	if got.ProviderHost != "https://github.com" {
		t.Fatalf("provider host = %q, want https://github.com (remote %q)", got.ProviderHost, remoteURL)
	}
	if got.DefaultBranch != "main" {
		t.Fatalf("default branch = %q, want main (remote %q)", got.DefaultBranch, remoteURL)
	}
}

// Updating a repository must not drop or rewrite its SSH remote, and switching
// transport in place must persist. Rows reach the SSH shape both ways: created
// from an SSH checkout, or backfilled onto a row that started HTTPS.
func TestRepositorySSHRemoteURL_UpdateRoundTrip(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-ssh-update")

	in := &models.Repository{
		ID: "repo-ssh-update", WorkspaceID: "ws-ssh-update", Name: "acme/widgets",
		SourceType: "provider", Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://github.com/acme/widgets.git", DefaultBranch: "main",
	}
	if err := repo.CreateRepository(ctx, in); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	in.RemoteURL = "git@github.com:acme/widgets.git"
	if err := repo.UpdateRepository(ctx, in); err != nil {
		t.Fatalf("update repository: %v", err)
	}

	got, err := repo.GetRepository(ctx, in.ID)
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	if got.RemoteURL != "git@github.com:acme/widgets.git" {
		t.Fatalf("RemoteURL after update = %q, want the SSH remote", got.RemoteURL)
	}
	assertSSHRepositoryIdentityIntact(t, got, got.RemoteURL)
}

// Provider-identity lookup keys off the provider columns, never the remote. An
// SSH-remote row must therefore still be found by the same identity as its
// HTTPS twin, which is what lets find-or-create adopt it instead of creating a
// duplicate every launch.
func TestGetRepositoryByProviderIdentityFindsSSHRemoteRow(t *testing.T) {
	for name, remoteURL := range sshRemoteURLSpellings() {
		t.Run(name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			seedWorkspace(t, repo, "ws-ssh-identity")

			in := &models.Repository{
				ID: "repo-ssh-identity", WorkspaceID: "ws-ssh-identity", Name: "acme/widgets",
				SourceType: "provider", Provider: "github", ProviderHost: "https://github.com",
				ProviderOwner: "acme", ProviderName: "widgets", RemoteURL: remoteURL,
			}
			if err := repo.CreateRepository(ctx, in); err != nil {
				t.Fatalf("create repository: %v", err)
			}

			got, err := repo.GetRepositoryByProviderInfo(ctx, "ws-ssh-identity", "github", "https://github.com", "acme", "widgets")
			if err != nil {
				t.Fatalf("GetRepositoryByProviderInfo: %v", err)
			}
			if got == nil {
				t.Fatalf("GetRepositoryByProviderInfo returned no row for SSH remote %q", remoteURL)
			}
			if got.ID != in.ID {
				t.Fatalf("resolved repository = %q, want %q", got.ID, in.ID)
			}
			if got.RemoteURL != remoteURL {
				t.Fatalf("resolved RemoteURL = %q, want %q", got.RemoteURL, remoteURL)
			}
		})
	}
}
