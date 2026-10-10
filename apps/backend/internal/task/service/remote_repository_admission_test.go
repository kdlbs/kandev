package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.5
func TestResolveRepositoryRef_RemoteSelectionUnscopedAdmission(t *testing.T) {
	for _, first := range []string{"deleted local", "scoped provider"} {
		for _, eligible := range []bool{false, true} {
			t.Run(first+map[bool]string{false: "/create", true: "/reuse"}[eligible], func(t *testing.T) {
				svc, store := remoteResolutionFixture(t)
				ctx := context.Background()
				if first == "deleted local" {
					seedRemoteResolutionRepo(t, store, "deleted", sourceTypeLocal, true)
				}
				foreign := seedRemoteResolutionRepo(t, store, "scoped", sourceTypeProvider, false)
				foreign.ProviderScope, foreign.ProviderRepoID = "instance-a", "42"
				if err := store.UpdateRepository(ctx, foreign); err != nil {
					t.Fatal(err)
				}
				before, err := store.GetRepository(ctx, foreign.ID)
				if err != nil {
					t.Fatal(err)
				}
				if eligible {
					seedRemoteResolutionRepo(t, store, "unscoped", sourceTypeProvider, false)
				}
				var previous string
				for attempt := 0; attempt < 2; attempt++ {
					id, _, created, resolveErr := svc.ResolveRepositoryRef(ctx, "ws-remote", remoteResolutionInput())
					if resolveErr != nil || id == foreign.ID || created != (!eligible && attempt == 0) {
						t.Fatalf("unscoped selection=%s created=%v error=%v", id, created, resolveErr)
					}
					if eligible && id != "unscoped" || attempt > 0 && id != previous {
						t.Fatalf("unscoped selection did not converge: %s, previous=%s", id, previous)
					}
					previous = id
				}
				after, err := store.GetRepository(ctx, foreign.ID)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("unscoped selection mutated scoped row: %+v, %v", after, err)
				}
			})
		}
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.4
// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.5
func TestResolveRepositoryRef_RemoteSelectionRejectsChangedOrigin(t *testing.T) {
	for _, origin := range []string{"https://github.com/other/widgets.git", "https://foreign.example/acme/widgets.git", "", "invalid"} {
		for _, position := range []string{"first", "fallback", "replacement"} {
			t.Run(origin+"/"+position, func(t *testing.T) {
				svc, store := remoteResolutionFixture(t)
				ctx := context.Background()
				if position == "fallback" {
					seedRemoteResolutionRepo(t, store, "deleted", sourceTypeLocal, true)
				}
				local := seedRemoteResolutionRepo(t, store, "local", sourceTypeLocal, false)
				before, err := store.GetRepository(ctx, local.ID)
				if err != nil {
					t.Fatal(err)
				}
				if position == "replacement" {
					if err := os.RemoveAll(local.LocalPath); err != nil {
						t.Fatal(err)
					}
					initRealGitRepo(t, local.LocalPath)
				}
				writeRemoteSelectionOrigin(t, local.LocalPath, origin)
				rowsBefore, err := store.ListRepositories(ctx, "ws-remote")
				if err != nil {
					t.Fatal(err)
				}
				_, _, _, err = svc.ResolveRepositoryRef(ctx, "ws-remote", remoteResolutionInput())
				if !errors.Is(err, ErrInvalidRepositorySettings) {
					t.Fatalf("changed origin accepted: %v", err)
				}
				after, err := store.GetRepository(ctx, local.ID)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("origin validation mutated saved row: %+v, %v", after, err)
				}
				rowsAfter, err := store.ListRepositories(ctx, "ws-remote")
				if err != nil || !reflect.DeepEqual(rowsBefore, rowsAfter) {
					t.Fatalf("origin validation changed repository registrations: %v", err)
				}
				id, _, created, err := svc.ResolveRepositoryRef(ctx, "ws-remote", TaskRepositoryInput{RepositoryID: local.ID})
				if err != nil || created || id != local.ID {
					t.Fatalf("origin validation changed explicit ID selection: %s, %v", id, err)
				}
			})
		}
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.4
func TestResolveRepositoryRef_RemoteSelectionEquivalentOrigins(t *testing.T) {
	for _, pair := range []struct {
		input  TaskRepositoryInput
		origin string
	}{
		{remoteResolutionInput(), "git@github.com:acme/widgets.git"},
		{remoteResolutionInput(), "ssh://git@github.com:2222/acme/widgets.git"},
		{TaskRepositoryInput{RemoteURL: "git@github.com:acme/widgets.git"}, "https://github.com/acme/widgets"},
		{TaskRepositoryInput{RemoteURL: "https://gitlab.com/acme/sub/widgets.git"}, "git@gitlab.com:acme/sub/widgets.git"},
		{TaskRepositoryInput{RemoteURL: "https://gitlab.example.test/acme/widgets.git", Provider: "gitlab", TrustedRemote: true}, "git@gitlab.example.test:acme/widgets.git"},
		{TaskRepositoryInput{RemoteURL: "https://dev.azure.com/acme/Project/_git/widgets"}, "git@ssh.dev.azure.com:v3/acme/Project/widgets"},
		{TaskRepositoryInput{RemoteURL: "git@ssh.dev.azure.com:v3/acme/Project/widgets"}, "https://dev.azure.com/acme/Project/_git/widgets"},
		{TaskRepositoryInput{RemoteURL: "https://forge.example.test/scm/acme/widgets.git", Provider: "forge", ProviderHost: "https://forge.example.test", ProviderScope: "instance-a", ProviderRepoID: "42", ProviderOwner: "acme", ProviderName: "widgets", TrustedProviderDescriptor: true}, "git@forge.example.test:scm/acme/widgets.git"},
	} {
		t.Run(pair.origin, func(t *testing.T) {
			svc, store := remoteResolutionFixture(t)
			ctx := context.Background()
			id, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-remote", pair.input)
			if err != nil {
				t.Fatal(err)
			}
			local, err := store.GetRepository(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			local.SourceType, local.LocalPath = sourceTypeLocal, filepath.Join(t.TempDir(), "checkout")
			initRealGitRepo(t, local.LocalPath)
			writeRemoteSelectionOrigin(t, local.LocalPath, pair.origin)
			if err := store.UpdateRepository(ctx, local); err != nil {
				t.Fatal(err)
			}
			selected, _, created, err := svc.ResolveRepositoryRef(ctx, "ws-remote", pair.input)
			if err != nil || created || selected != id {
				t.Fatalf("equivalent origin rejected: %s created=%v error=%v", selected, created, err)
			}
		})
	}
}

func writeRemoteSelectionOrigin(t *testing.T, path, origin string) {
	t.Helper()
	config := "[core]\n\trepositoryformatversion = 0\n[remote \"origin\"]\n\turl = " + origin + "\n"
	if err := os.WriteFile(filepath.Join(path, ".git", "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}
