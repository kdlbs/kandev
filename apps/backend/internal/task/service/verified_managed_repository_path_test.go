package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type verifiedManagedPathUpdater interface {
	UpdateVerifiedManagedRepositoryPath(context.Context, *models.Repository, string) error
}

func requireVerifiedManagedPathUpdater(t *testing.T, svc *Service) verifiedManagedPathUpdater {
	t.Helper()
	updater, ok := any(svc).(verifiedManagedPathUpdater)
	if !ok {
		t.Fatal("internal verified managed-path persistence is unavailable")
	}
	return updater
}

func verifiedManagedPathFixture(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, *models.Repository, string) {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "managed-ws", Name: "Managed", OwnerID: "managed-owner"}); err != nil {
		t.Fatal(err)
	}
	entity := &models.Repository{ID: "managed-repo", WorkspaceID: "managed-ws", Name: "Managed", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "group/team", ProviderName: "repo", RemoteURL: "https://gitlab.example/group/team/repo.git", DefaultBranch: "main", PullBeforeWorktree: true, WorktreeBranchPrefix: "feat", WorktreeBranchTemplate: "{{task_id}}", SetupScript: "setup", CleanupScript: "cleanup", DevScript: "dev", CopyFiles: ".env"}
	bindings := []models.RepositorySecretBinding{{Key: "TOKEN", SecretID: "retained-secret"}}
	if err := repo.CreateRepositoryWithSecretBindings(context.Background(), entity, bindings); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetRepository(context.Background(), entity.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(canonicalTempDir(t), "checkout")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	return svc, eventBus, repo, stored, path
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
func TestVerifiedManagedRepositoryPathPreservesLexicalIdentity(t *testing.T) {
	for _, scenario := range []string{"provider", "parent-alias", "pathless-nonlocal", "scoped-provider", "trailing-newline"} {
		t.Run(scenario, func(t *testing.T) {
			svc, eventBus, repo, snapshot, path := verifiedManagedPathFixture(t)
			switch scenario {
			case "parent-alias":
				physical := filepath.Dir(path)
				alias := physical + "-alias"
				if err := os.Symlink(physical, alias); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				t.Cleanup(func() { _ = os.Remove(alias) })
				path = filepath.Join(alias, "checkout")
			case "pathless-nonlocal":
				snapshot.SourceType = ""
			case "scoped-provider":
				snapshot.ProviderScope = "connection"
				snapshot.ProviderRepoID = "immutable"
				snapshot.ProviderOwner = ""
				snapshot.ProviderName = ""
			case "trailing-newline":
				path = filepath.Join(filepath.Dir(path), "checkout \n")
				if err := os.Mkdir(path, 0755); err != nil {
					t.Skipf("exact whitespace names unavailable: %v", err)
				}
				info, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Name() != "checkout \n" {
					t.Skip("filesystem normalizes trailing whitespace")
				}
			}
			if err := repo.UpdateRepository(context.Background(), snapshot); err != nil {
				t.Fatal(err)
			}
			persisted, err := repo.GetRepository(context.Background(), snapshot.ID)
			if err != nil {
				t.Fatal(err)
			}
			admitted := *persisted
			// Non-identity snapshot fields cannot overwrite the current persisted settings.
			admitted.Name = "forged-name"
			admitted.DefaultBranch = "forged-branch"
			admitted.PullBeforeWorktree = false
			admitted.SecretBindings = nil
			admitted.SetupScript = "forged-script"
			callerCopy := admitted
			updater := requireVerifiedManagedPathUpdater(t, svc)
			if err := updater.UpdateVerifiedManagedRepositoryPath(ctxAs("managed-owner"), &admitted, path); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetRepository(context.Background(), snapshot.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !stored.UpdatedAt.After(persisted.UpdatedAt) {
				t.Fatal("verified update did not advance repository version")
			}
			expected := *persisted
			expected.LocalPath = path
			expected.UpdatedAt = stored.UpdatedAt
			if !reflect.DeepEqual(*stored, expected) {
				t.Fatalf("verified write changed unrelated fields: got=%+v want=%+v", stored, expected)
			}
			if !reflect.DeepEqual(admitted, callerCopy) {
				t.Fatal("admitted snapshot mutated")
			}
			published := eventBus.GetPublishedEvents()
			if len(published) != 1 || published[0].Type != events.RepositoryUpdated {
				t.Fatalf("verified update publication=%+v", published)
			}
			data, ok := published[0].Data.(map[string]interface{})
			if !ok || data["id"] != snapshot.ID || data["workspace_id"] != snapshot.WorkspaceID || data["local_path"] != path || data["default_branch"] != stored.DefaultBranch {
				t.Fatalf("verified event data=%+v", published[0].Data)
			}
		})
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.3, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
func TestVerifiedManagedRepositoryPathRejectsInvalidAdmission(t *testing.T) {
	cases := []string{"nil", "id", "workspace", "version", "deleted", "explicit-local", "untyped-saved-path", "provider", "host", "owner", "name", "scope-without-id", "scope-invalid", "empty-path", "relative-path", "unclean-path", "nul-path", "absent-path", "file-path", "link-path", "denied", "cancelled", "stale-version", "forged-workspace", "forged-source", "forged-host", "forged-scope", "forged-repo-id", "forged-owner", "forged-name", "forged-url", "forged-local-path"}
	for _, scenario := range cases {
		t.Run(scenario, func(t *testing.T) {
			svc, eventBus, repo, current, path := verifiedManagedPathFixture(t)
			snapshot := *current
			admitted := &snapshot
			ctx := ctxAs("managed-owner")
			switch scenario {
			case "nil":
				admitted = nil
			case "id":
				snapshot.ID = ""
			case "workspace":
				snapshot.WorkspaceID = ""
			case "version":
				snapshot.UpdatedAt = time.Time{}
			case "deleted":
				now := time.Now().UTC()
				snapshot.DeletedAt = &now
			case "explicit-local":
				snapshot.SourceType = "local"
			case "untyped-saved-path":
				snapshot.SourceType = ""
				snapshot.LocalPath = path
			case "provider":
				snapshot.Provider = ""
			case "host":
				snapshot.ProviderHost = ""
			case "owner":
				snapshot.ProviderOwner = ""
			case "name":
				snapshot.ProviderName = ""
			case "scope-without-id":
				snapshot.ProviderScope = "connection"
				snapshot.ProviderRepoID = ""
			case "scope-invalid":
				snapshot.ProviderScope = "bad\x00scope"
				snapshot.ProviderRepoID = "immutable"
			case "empty-path":
				path = ""
			case "relative-path":
				path = "relative-checkout"
			case "unclean-path":
				path = path + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(path)
			case "nul-path":
				path += "\x00"
			case "absent-path":
				path = filepath.Join(filepath.Dir(path), "absent")
			case "file-path":
				path = filepath.Join(filepath.Dir(path), "file")
				if err := os.WriteFile(path, []byte("retain"), 0600); err != nil {
					t.Fatal(err)
				}
			case "link-path":
				link := path + "-link"
				if err := os.Symlink(path, link); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				path = link
			case "denied":
				ctx = ctxAs("foreign-owner")
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "stale-version":
				snapshot.UpdatedAt = snapshot.UpdatedAt.Add(-time.Second)
			case "forged-workspace":
				snapshot.WorkspaceID = "other-workspace"
			case "forged-source":
				snapshot.SourceType = ""
			case "forged-host":
				snapshot.ProviderHost = "https://other.example"
			case "forged-scope":
				snapshot.ProviderScope = "other-connection"
				snapshot.ProviderRepoID = "immutable"
			case "forged-repo-id":
				snapshot.ProviderRepoID = "other-id"
			case "forged-owner":
				snapshot.ProviderOwner = "other-owner"
			case "forged-name":
				snapshot.ProviderName = "other-repository"
			case "forged-url":
				snapshot.RemoteURL = "https://other.example/team/repo.git"
			case "forged-local-path":
				snapshot.LocalPath = path
			}
			switch scenario {
			case "explicit-local", "untyped-saved-path", "provider", "host", "owner", "name", "scope-without-id", "scope-invalid":
				if err := repo.UpdateRepository(context.Background(), &snapshot); err != nil {
					t.Fatal(err)
				}
				var err error
				current, err = repo.GetRepository(context.Background(), snapshot.ID)
				if err != nil {
					t.Fatal(err)
				}
				snapshot = *current
			}
			err := requireVerifiedManagedPathUpdater(t, svc).UpdateVerifiedManagedRepositoryPath(ctx, admitted, path)
			if err == nil {
				t.Fatal("invalid verified-path admission succeeded")
			}
			if scenario == "denied" && !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
				t.Fatalf("denial leaked repository: %v", err)
			}
			if scenario == "stale-version" && !errors.Is(err, repoerrors.ErrTaskVersionConflict) {
				t.Fatalf("stale version=%v", err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation=%v", err)
			}
			stored, readErr := repo.GetRepository(context.Background(), current.ID)
			if readErr != nil || !reflect.DeepEqual(stored, current) || len(eventBus.GetPublishedEvents()) != 0 {
				t.Fatalf("rejected write changed repository/events: %v %v", stored, readErr)
			}
		})
	}
}

type verifiedPathRepository struct {
	repository.RepositoryEntityRepository
	exact       exactRepositoryUpdater
	readErr     error
	writeErr    error
	nilRow      bool
	beforeWrite func()
}

func (r *verifiedPathRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	if r.nilRow {
		return nil, nil
	}
	return r.RepositoryEntityRepository.GetRepository(ctx, id)
}
func (r *verifiedPathRepository) UpdateRepositoryIfUnchanged(ctx context.Context, entity *models.Repository, expected time.Time) error {
	if r.beforeWrite != nil {
		r.beforeWrite()
	}
	if r.writeErr != nil {
		return r.writeErr
	}
	return r.exact.UpdateRepositoryIfUnchanged(ctx, entity, expected)
}
func (r *verifiedPathRepository) UpdateRepositoryWithSecretBindingsIfUnchanged(ctx context.Context, entity *models.Repository, bindings []models.RepositorySecretBinding, expected time.Time) error {
	return r.exact.UpdateRepositoryWithSecretBindingsIfUnchanged(ctx, entity, bindings, expected)
}

type verifiedPathWithoutExactUpdater struct {
	repository.RepositoryEntityRepository
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
func TestVerifiedManagedRepositoryPathPersistenceFailures(t *testing.T) {
	for _, scenario := range []string{"read-error", "missing-row", "missing-exact-updater", "write-error", "cancel-before-write", "concurrent-version"} {
		t.Run(scenario, func(t *testing.T) {
			svc, eventBus, repo, snapshot, path := verifiedManagedPathFixture(t)
			fault := errors.New("injected verified-path persistence fault")
			ctx, cancel := context.WithCancel(ctxAs("managed-owner"))
			t.Cleanup(cancel)
			store := &verifiedPathRepository{RepositoryEntityRepository: repo, exact: repo}
			svc.repoEntities = store
			switch scenario {
			case "read-error":
				store.readErr = fault
			case "missing-row":
				store.nilRow = true
			case "missing-exact-updater":
				svc.repoEntities = verifiedPathWithoutExactUpdater{RepositoryEntityRepository: repo}
			case "write-error":
				store.writeErr = fault
			case "cancel-before-write":
				store.beforeWrite = cancel
			case "concurrent-version":
				store.beforeWrite = func() {
					row, err := repo.GetRepository(context.Background(), snapshot.ID)
					if err != nil {
						t.Fatal(err)
					}
					row.Name = "concurrent setting"
					row.DefaultBranch = "concurrent-branch"
					if err := repo.UpdateRepository(context.Background(), row); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := requireVerifiedManagedPathUpdater(t, svc).UpdateVerifiedManagedRepositoryPath(ctx, snapshot, path)
			if err == nil {
				t.Fatal("unverified persistence succeeded")
			}
			if (scenario == "read-error" || scenario == "write-error") && !errors.Is(err, fault) {
				t.Fatalf("persistence error lost: %v", err)
			}
			if scenario == "cancel-before-write" && !errors.Is(err, context.Canceled) {
				t.Fatalf("write cancellation lost: %v", err)
			}
			if scenario == "concurrent-version" && !errors.Is(err, repoerrors.ErrTaskVersionConflict) {
				t.Fatalf("CAS failure lost: %v", err)
			}
			stored, readErr := repo.GetRepository(context.Background(), snapshot.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if stored.LocalPath != snapshot.LocalPath || len(eventBus.GetPublishedEvents()) != 0 {
				t.Fatal("failed write published path/event")
			}
			if scenario == "concurrent-version" {
				if stored.Name != "concurrent setting" || stored.DefaultBranch != "concurrent-branch" {
					t.Fatal("verified write overwrote concurrent settings")
				}
			} else if !reflect.DeepEqual(stored, snapshot) {
				t.Fatal("failed verified write mutated repository")
			}
		})
	}
}
