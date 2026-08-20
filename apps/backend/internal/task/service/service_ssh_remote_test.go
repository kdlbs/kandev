package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// sshRemoteURLSpellings are the four shapes an SSH checkout leaves in
// repositories.remote_url. Every fixture on this path was HTTPS before #2673,
// so the service had never been driven with the data shape production holds.
func sshRemoteURLSpellings() map[string]string {
	return map[string]string{
		"scp style suffixed":  "git@github.com:acme/widgets.git",
		"scp style bare":      "git@github.com:acme/widgets",
		"ssh scheme suffixed": "ssh://git@github.com/acme/widgets.git",
		"ssh scheme bare":     "ssh://git@github.com/acme/widgets",
	}
}

// Find-or-create must persist an SSH remote as given and, on a second call for
// the same provider identity, adopt that row rather than create a duplicate.
// Resolution keys off the provider columns, so the remote's transport must not
// influence it.
func TestService_FindOrCreateRepositoryPersistsAndAdoptsSSHRemote(t *testing.T) {
	for name, remoteURL := range sshRemoteURLSpellings() {
		t.Run(name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
				t.Fatalf("CreateWorkspace: %v", err)
			}

			created, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
				WorkspaceID: "ws-1", Provider: "github", ProviderHost: "https://github.com",
				ProviderOwner: "acme", ProviderName: "widgets", RemoteURL: remoteURL,
			})
			if err != nil || !wasCreated {
				t.Fatalf("initial FindOrCreateRepository() = %+v, created=%t, err=%v", created, wasCreated, err)
			}
			if created.RemoteURL != remoteURL {
				t.Fatalf("created RemoteURL = %q, want %q", created.RemoteURL, remoteURL)
			}

			// Deliberately the HTTPS spelling of the same repository. Resolution
			// keys off the provider columns, so a later provider-driven request
			// carrying the HTTPS URL must adopt this row rather than create a
			// duplicate. Passing the SSH URL back would also pass against an
			// implementation that matched on exact remote_url.
			resolved, duplicateCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
				WorkspaceID: "ws-1", Provider: "github", ProviderHost: "https://github.com",
				ProviderOwner: "acme", ProviderName: "widgets",
				RemoteURL: "https://github.com/acme/widgets.git",
			})
			if err != nil {
				t.Fatalf("second FindOrCreateRepository() error = %v", err)
			}
			if duplicateCreated || resolved.ID != created.ID {
				t.Fatalf("second call = %q (created=%t), want existing %q", resolved.ID, duplicateCreated, created.ID)
			}
			// The row already had a remote, so the HTTPS spelling must not clobber
			// the stored SSH one: backfill only fills an empty column.
			if resolved.RemoteURL != remoteURL {
				t.Fatalf("resolved RemoteURL = %q, want the stored SSH remote %q", resolved.RemoteURL, remoteURL)
			}

			stored, err := repo.GetRepository(ctx, created.ID)
			if err != nil {
				t.Fatalf("GetRepository: %v", err)
			}
			if stored.RemoteURL != remoteURL {
				t.Fatalf("stored RemoteURL = %q, want %q", stored.RemoteURL, remoteURL)
			}
		})
	}
}

// A row created without a remote is backfilled by a later call that carries
// one. An SSH remote must be accepted by that backfill: this is precisely how
// the production rows behind #2673 acquired their SSH remote_url.
func TestService_FindOrCreateRepositoryBackfillsSSHRemoteOntoExistingRow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1", Name: "acme/widgets", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "acme", ProviderName: "widgets",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.RemoteURL != "" {
		t.Fatalf("precondition: created RemoteURL = %q, want empty", created.RemoteURL)
	}

	resolved, wasCreated, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID: "ws-1", Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "acme", ProviderName: "widgets", RemoteURL: "git@github.com:acme/widgets.git",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if wasCreated || resolved.ID != created.ID {
		t.Fatalf("resolved = %q (created=%t), want existing %q", resolved.ID, wasCreated, created.ID)
	}

	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if stored.RemoteURL != "git@github.com:acme/widgets.git" {
		t.Fatalf("backfilled RemoteURL = %q, want the SSH remote", stored.RemoteURL)
	}
	if stored.ProviderOwner != "acme" || stored.ProviderName != "widgets" {
		t.Fatalf("provider identity after backfill = %q/%q, want acme/widgets", stored.ProviderOwner, stored.ProviderName)
	}
}
