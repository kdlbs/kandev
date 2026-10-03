package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.1
// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.3
func TestResolveRepositoryRef_RemoteSelectionSkipsDeletedLocalCheckout(t *testing.T) {
	svc, _, store := createTestService(t)
	ctx := context.Background()
	if err := store.CreateWorkspace(ctx, &models.Workspace{ID: "ws-remote", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "checkout")
	makeRepo(t, path)
	local, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-remote", Name: "acme/widgets", SourceType: sourceTypeLocal, LocalPath: path,
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets", DefaultBranch: "main",
		SetupScript: "local-only setup",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.GetRepository(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldTask := &models.Task{ID: "existing-local-task", WorkspaceID: "ws-remote", Title: "Local work"}
	if err := store.CreateTask(ctx, oldTask); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTaskRepository(ctx, &models.TaskRepository{ID: "original-link", TaskID: oldTask.ID, RepositoryID: local.ID, BaseBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	id, branch, created, err := svc.ResolveRepositoryRef(ctx, "ws-remote", TaskRepositoryInput{
		RemoteURL: "https://github.com/acme/widgets.git", BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	managed, err := store.GetRepository(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !created || id == local.ID || managed.SourceType != sourceTypeProvider || managed.LocalPath != "" {
		t.Fatalf("remote selected ID=%s created=%v source=%s path=%s; want a separate pathless provider registration", id, created, managed.SourceType, managed.LocalPath)
	}
	if branch != "main" || managed.RemoteURL != "https://github.com/acme/widgets.git" || managed.SetupScript != "" {
		t.Fatalf("unexpected managed selection: branch=%s repository=%+v", branch, managed)
	}
	after, err := store.GetRepository(ctx, local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("remote fallback changed the original local registration")
	}
	links, err := store.ListTaskRepositories(ctx, oldTask.ID)
	if err != nil || len(links) != 1 || links[0].RepositoryID != local.ID {
		t.Fatalf("remote fallback changed existing task associations: %+v, %v", links, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original checkout was recreated: %v", err)
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.4
func TestResolveRepositoryRef_RemoteSelectionReusesValidLocal(t *testing.T) {
	svc, store := remoteResolutionFixture(t)
	local := seedRemoteResolutionRepo(t, store, "local", sourceTypeLocal, false)
	sentinel := filepath.Join(local.LocalPath, "untracked.txt")
	if err := os.WriteFile(sentinel, []byte("local work"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, _, created, err := svc.ResolveRepositoryRef(context.Background(), "ws-remote", remoteResolutionInput())
	if err != nil || created || id != local.ID {
		t.Fatalf("usable local resolved to %s created=%v err=%v", id, created, err)
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "local work" {
		t.Fatalf("remote selection changed existing local work: %q, %v", content, err)
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.2
// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.4
func TestResolveRepositoryRef_RemoteSelectionCandidates(t *testing.T) {
	for _, candidate := range []string{"live local", "two live locals", "pathless provider", "deleted provider", "all deleted"} {
		t.Run(candidate, func(t *testing.T) {
			svc, store := remoteResolutionFixture(t)
			seedRemoteResolutionRepo(t, store, "first", sourceTypeLocal, true)
			second := seedRemoteResolutionRepo(t, store, "second", sourceTypeLocal, true)
			var wanted *models.Repository
			if candidate != "all deleted" {
				source := sourceTypeProvider
				if candidate == "live local" || candidate == "two live locals" {
					source = sourceTypeLocal
				}
				wanted = seedRemoteResolutionRepo(t, store, "eligible", source, candidate == "deleted provider")
				if candidate == "two live locals" {
					seedRemoteResolutionRepo(t, store, "later", sourceTypeLocal, false)
				}
			}
			for attempt := 0; attempt < 3; attempt++ {
				id, _, created, err := svc.ResolveRepositoryRef(context.Background(), "ws-remote", remoteResolutionInput())
				if err != nil {
					t.Fatal(err)
				}
				if wanted == nil {
					if !created || id == second.ID {
						t.Fatalf("first fallback = %s created=%v", id, created)
					}
					wanted, err = store.GetRepository(context.Background(), id)
					if err != nil {
						t.Fatal(err)
					}
				} else if created || id != wanted.ID {
					t.Fatalf("attempt %d = %s created=%v, want existing %s", attempt, id, created, wanted.ID)
				}
			}
		})
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.2
func TestResolveRepositoryRef_RemoteSelectionConcurrentFallback(t *testing.T) {
	svc, store := remoteResolutionFixture(t)
	seedRemoteResolutionRepo(t, store, "deleted", sourceTypeLocal, true)
	const callers = 8
	type result struct {
		id      string
		created bool
		err     error
	}
	results := make(chan result, callers)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range callers {
		workers.Go(func() {
			<-start
			id, _, created, err := svc.ResolveRepositoryRef(context.Background(), "ws-remote", remoteResolutionInput())
			results <- result{id, created, err}
		})
	}
	close(start)
	workers.Wait()
	close(results)
	var winner string
	createdCount := 0
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if winner == "" {
			winner = got.id
		}
		if got.id != winner {
			t.Fatalf("different winners: %s and %s", winner, got.id)
		}
		if got.created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created %d fallback registrations, want 1", createdCount)
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.3
func TestResolveRepositoryRef_RemoteSelectionPreservesExplicitLocal(t *testing.T) {
	svc, store := remoteResolutionFixture(t)
	local := seedRemoteResolutionRepo(t, store, "local", sourceTypeLocal, true)
	for _, input := range []TaskRepositoryInput{{RepositoryID: local.ID}, {LocalPath: local.LocalPath}} {
		id, _, created, err := svc.ResolveRepositoryRef(context.Background(), "ws-remote", input)
		if err != nil || created || id != local.ID {
			t.Fatalf("explicit local resolved to %s created=%v err=%v", id, created, err)
		}
	}
	resolved, created, err := svc.FindOrCreateRepository(context.Background(), &FindOrCreateRepositoryRequest{
		WorkspaceID: "ws-remote", Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
	})
	if err != nil || created || resolved.ID != local.ID {
		t.Fatalf("metadata lookup changed: %+v created=%v err=%v", resolved, created, err)
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.4
func TestResolveRepositoryRef_RemoteSelectionRejectsInvalidLocal(t *testing.T) {
	for _, invalid := range []string{"missing git metadata", "symlink replacement", "dangling symlink", "permission denied"} {
		t.Run(invalid, func(t *testing.T) {
			svc, store := remoteResolutionFixture(t)
			local := seedRemoteResolutionRepo(t, store, "local", sourceTypeLocal, false)
			before, err := store.GetRepository(context.Background(), local.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch invalid {
			case "missing git metadata":
				if err := os.RemoveAll(filepath.Join(local.LocalPath, ".git")); err != nil {
					t.Fatal(err)
				}
			case "symlink replacement", "dangling symlink":
				if err := os.RemoveAll(local.LocalPath); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if invalid == "symlink replacement" {
					makeRepo(t, target)
				}
				if err := os.Symlink(target, local.LocalPath); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "permission denied":
				if err := os.Chmod(local.LocalPath, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(local.LocalPath, 0o755) })
				if _, err := os.ReadDir(local.LocalPath); !errors.Is(err, os.ErrPermission) {
					t.Skip("executor bypasses directory permissions")
				}
			}
			_, _, _, err = svc.ResolveRepositoryRef(context.Background(), "ws-remote", remoteResolutionInput())
			if !errors.Is(err, ErrInvalidRepositorySettings) {
				t.Fatalf("got %v, want validation failure", err)
			}
			after, err := store.GetRepository(context.Background(), local.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("inspection failure mutated local registration")
			}
			rows, err := store.ListRepositories(context.Background(), "ws-remote")
			if err != nil || len(rows) != 1 {
				t.Fatalf("failure created fallback: rows=%d err=%v", len(rows), err)
			}
		})
	}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.1
// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.5
func TestResolveRepositoryRef_RemoteSelectionProviderIdentity(t *testing.T) {
	for _, input := range []TaskRepositoryInput{
		{GitHubURL: "https://github.com/acme/widgets"},
		{RemoteURL: "git@github.com:acme/widgets.git"},
		{RemoteURL: "https://gitlab.com/acme/widgets.git"},
		{RemoteURL: "https://gitlab.example.test/acme/widgets.git", Provider: "gitlab", TrustedRemote: true},
		{RemoteURL: "https://dev.azure.com/acme/Project/_git/widgets"},
		{RemoteURL: "https://forge.example.test/scm/acme/widgets.git", Provider: "forge", ProviderHost: "https://forge.example.test", ProviderScope: "instance-a", ProviderRepoID: "42", ProviderOwner: "acme", ProviderName: "widgets", TrustedProviderDescriptor: true},
	} {
		t.Run(effectiveRemoteURL(input), func(t *testing.T) {
			svc, store := remoteResolutionFixture(t)
			id, _, _, err := svc.ResolveRepositoryRef(context.Background(), "ws-remote", input)
			if err != nil {
				t.Fatal(err)
			}
			first, err := store.GetRepository(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			first.SourceType = sourceTypeLocal
			first.LocalPath = filepath.Join(t.TempDir(), "deleted")
			if err := store.UpdateRepository(context.Background(), first); err != nil {
				t.Fatal(err)
			}
			for index, change := range []func(*models.Repository){
				func(r *models.Repository) {
					r.ProviderHost = "https://foreign.example.test"
					r.ProviderScope = "instance-b"
				},
				func(r *models.Repository) { r.WorkspaceID = "ws-foreign" },
				func(r *models.Repository) { r.ProviderScope = ""; r.ProviderRepoID = "other"; r.ProviderName = "other" },
			} {
				foreign := *first
				foreign.ID = fmt.Sprintf("foreign-%d", index)
				foreign.SourceType, foreign.LocalPath = sourceTypeProvider, ""
				change(&foreign)
				if err := store.CreateRepository(context.Background(), &foreign); err != nil {
					t.Fatal(err)
				}
			}
			fallback, _, created, err := svc.ResolveRepositoryRef(context.Background(), "ws-remote", input)
			if err != nil || !created || fallback == first.ID {
				t.Fatalf("fallback=%s created=%v err=%v", fallback, created, err)
			}
			got, err := store.GetRepository(context.Background(), fallback)
			if err != nil {
				t.Fatal(err)
			}
			if got.ProviderHost != first.ProviderHost || got.ProviderScope != first.ProviderScope || got.RemoteURL != first.RemoteURL {
				t.Fatalf("fallback lost provider identity: %+v", got)
			}
		})
	}
}

func remoteResolutionInput() TaskRepositoryInput {
	return TaskRepositoryInput{RemoteURL: "https://github.com/acme/widgets.git", BaseBranch: "main"}
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.5
func TestResolveRepositoryRef_RemoteSelectionScopedFallback(t *testing.T) {
	svc, store := remoteResolutionFixture(t)
	ctx := context.Background()
	local := seedRemoteResolutionRepo(t, store, "deleted", sourceTypeLocal, true)
	local.ProviderScope, local.ProviderRepoID = "instance-a", "42"
	if err := store.UpdateRepository(ctx, local); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []struct{ scope, id string }{{"instance-b", "42"}, {"", "42"}, {"instance-a", "99"}} {
		foreign := seedRemoteResolutionRepo(t, store, "foreign-"+identity.scope+identity.id, sourceTypeProvider, false)
		foreign.ProviderScope, foreign.ProviderRepoID = identity.scope, identity.id
		if err := store.UpdateRepository(ctx, foreign); err != nil {
			t.Fatal(err)
		}
	}
	resolved, created, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID: "ws-remote", Provider: "github", ProviderHost: "https://github.com",
		ProviderScope: "instance-a", ProviderRepoID: "42", ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://github.com/acme/widgets.git",
	})
	if err != nil || !created || resolved.ID == local.ID {
		t.Fatalf("scoped fallback = %+v created=%v err=%v", resolved, created, err)
	}
	if resolved.ProviderScope != "instance-a" || resolved.ProviderRepoID != "42" {
		t.Fatalf("fallback selected a different provider instance: %+v", resolved)
	}
}

func remoteResolutionFixture(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	svc, _, store := createTestService(t)
	for _, id := range []string{"ws-remote", "ws-foreign"} {
		if err := store.CreateWorkspace(context.Background(), &models.Workspace{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	return svc, store
}

func seedRemoteResolutionRepo(t *testing.T, store *sqliterepo.Repository, id, source string, deleted bool) *models.Repository {
	t.Helper()
	r := &models.Repository{
		ID: id, WorkspaceID: "ws-remote", Name: "acme/widgets", SourceType: source,
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets", DefaultBranch: "main",
	}
	if source == sourceTypeLocal || deleted {
		r.LocalPath = filepath.Join(t.TempDir(), "checkout")
		makeRepo(t, r.LocalPath)
		writeRemoteSelectionOrigin(t, r.LocalPath, "https://github.com/acme/widgets.git")
		if deleted {
			if err := os.RemoveAll(r.LocalPath); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := store.CreateRepository(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	return r
}
