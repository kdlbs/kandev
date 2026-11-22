package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type retainedCleanupStore struct{ Store }

func (retainedCleanupStore) IsWorkspaceRetained(context.Context, string, string, string) (bool, error) {
	return true, nil
}
func TestExplicitRetentionBlocksWorktreeRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-retain", "session-retain", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-retain", "session-retain")
	mgr.store = retainedCleanupStore{Store: mgr.store}
	require.Error(t, mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}))
	_, err := os.Stat(wt.Path)
	require.NoError(t, err)
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
}

type retainedCheckoutFixture struct {
	manager  *Manager
	store    *mockStore
	repoPath string
	worktree *Worktree
	request  CreateRequest
}

func newRetainedCheckoutFixture(t *testing.T) *retainedCheckoutFixture {
	t.Helper()
	repoPath := initGitRepoWithRemote(t)
	store := newMockStore()
	manager, err := NewManager(newTestConfig(t), store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	request := CreateRequest{
		TaskID: "task-retained", SessionID: "session-original", TaskEnvironmentID: "environment-original",
		RepositoryID: "repo-retained", RepositoryPath: repoPath, BaseBranch: "main",
		CheckoutBranch: "feature/pr-branch", BranchIdentitySlug: "feature-pr-branch",
		TaskDirName: "retained-task", RepoName: "repo",
	}
	retained, err := manager.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("create initial checkout: %v", err)
	}
	return &retainedCheckoutFixture{manager: manager, store: store, repoPath: repoPath, worktree: retained, request: request}
}

func (f *retainedCheckoutFixture) nextRequest() CreateRequest {
	request := f.request
	request.SessionID = "session-next"
	return request
}

func TestCreate_RetainedCheckoutRegistersWithoutCreationCallback(t *testing.T) {
	f := newRetainedCheckoutFixture(t)
	delete(f.store.worktrees, f.worktree.ID)
	marker := filepath.Join(f.worktree.Path, "retained-notes.txt")
	if err := os.WriteFile(marker, []byte("keep this work\n"), 0o644); err != nil {
		t.Fatalf("write retained file: %v", err)
	}

	request := f.nextRequest()
	callbackCalled := false
	request.OnWorktreeCreated = func(*Worktree) { callbackCalled = true }
	got, err := f.manager.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("Create() retained checkout: %v", err)
	}
	if got.ID == f.worktree.ID || got.Path != f.worktree.Path {
		t.Fatalf("registered retained checkout = (%q, %q), want a new ID for %q", got.ID, got.Path, f.worktree.Path)
	}
	if callbackCalled {
		t.Fatal("OnWorktreeCreated fired for a directory that existed before this request")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep this work\n" {
		t.Fatalf("retained file = %q, %v; want original contents", data, err)
	}
	if f.store.worktrees[got.ID] == nil || f.store.worktrees[got.ID].Path != f.worktree.Path {
		t.Fatalf("retained checkout registration = %+v, want stored record at original path", f.store.worktrees[got.ID])
	}
}

func TestCreate_RetainedCheckoutPreservesRegisteredIdentity(t *testing.T) {
	f := newRetainedCheckoutFixture(t)
	got, err := f.manager.Create(context.Background(), f.nextRequest())
	if err != nil {
		t.Fatalf("Create() retained checkout: %v", err)
	}
	if got.ID != f.worktree.ID || got.Path != f.worktree.Path {
		t.Fatalf("reused checkout = (%q, %q), want original (%q, %q)", got.ID, got.Path, f.worktree.ID, f.worktree.Path)
	}
}

type retainedCheckoutRegistrationErrorStore struct {
	Store
	err error
}

func (s retainedCheckoutRegistrationErrorStore) CreateWorktree(context.Context, *Worktree) error {
	return s.err
}

func TestCreate_RetainedCheckoutRegistrationFailureLeavesFilesUntouched(t *testing.T) {
	f := newRetainedCheckoutFixture(t)
	delete(f.store.worktrees, f.worktree.ID)
	marker := filepath.Join(f.worktree.Path, "retained-notes.txt")
	if err := os.WriteFile(marker, []byte("keep this work\n"), 0o644); err != nil {
		t.Fatalf("write retained file: %v", err)
	}
	registrationErr := errors.New("registration unavailable")
	f.manager.store = retainedCheckoutRegistrationErrorStore{Store: f.store, err: registrationErr}

	_, err := f.manager.Create(context.Background(), f.nextRequest())
	if !errors.Is(err, registrationErr) {
		t.Fatalf("Create() error = %v, want registration error", err)
	}
	if _, err := os.Stat(f.worktree.Path); err != nil {
		t.Fatalf("registration failure removed retained checkout: %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep this work\n" {
		t.Fatalf("retained file = %q, %v; want original contents", data, err)
	}
}

func TestCreate_RetainedCheckoutRejectsConflictingIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*testing.T, *retainedCheckoutFixture, *CreateRequest)
		want   error
	}{
		{
			name: "owner",
			change: func(_ *testing.T, _ *retainedCheckoutFixture, request *CreateRequest) {
				request.TaskID = "another-task"
			},
			want: ErrWorktreePathOwnedByAnotherTask,
		},
		{
			name: "repository",
			change: func(t *testing.T, _ *retainedCheckoutFixture, request *CreateRequest) {
				request.RepositoryPath = initGitRepoWithRemote(t)
			},
		},
		{
			name: "branch",
			change: func(_ *testing.T, _ *retainedCheckoutFixture, request *CreateRequest) {
				request.CheckoutBranch = "main"
			},
		},
		{
			name: "environment",
			change: func(_ *testing.T, _ *retainedCheckoutFixture, request *CreateRequest) {
				request.TaskEnvironmentID = "another-environment"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newRetainedCheckoutFixture(t)
			request := f.nextRequest()
			test.change(t, f, &request)
			_, err := f.manager.Create(context.Background(), request)
			if err == nil {
				t.Fatal("Create() accepted conflicting retained checkout identity")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
			if _, statErr := os.Stat(f.worktree.Path); statErr != nil {
				t.Fatalf("identity conflict removed retained checkout: %v", statErr)
			}
		})
	}
}

func TestCreate_RetainedCheckoutRejectsConflictingContributionDestination(t *testing.T) {
	f := newRetainedCheckoutFixture(t)
	destination := models.ContributionDestination{
		Version:  models.ContributionDestinationVersion,
		Provider: models.ContributionDestinationProviderGitHub,
		SourceRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "contributor/widget", ProviderID: "source", RemoteURL: "https://github.com/contributor/widget.git",
		},
		TargetRepository: models.ContributionDestinationRepository{
			Host: "github.com", Path: "target/widget", ProviderID: "target", RemoteURL: "https://github.com/target/widget.git",
		},
	}
	remoteName := destination.ContributionRemoteName()
	runGit(t, f.repoPath, "remote", "add", remoteName, destination.TargetRepository.RemoteURL)
	runGit(t, f.repoPath, "config", "--add", "remote."+remoteName+".pushurl", "https://github.com/other/widget.git")
	request := f.nextRequest()
	request.ContributionDestination = &destination

	_, err := f.manager.Create(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "push URL does not match the validated target") {
		t.Fatalf("Create() error = %v, want conflicting destination push URL rejection", err)
	}
	if _, statErr := os.Stat(f.worktree.Path); statErr != nil {
		t.Fatalf("destination conflict removed retained checkout: %v", statErr)
	}
}

func TestTryReuseRetainedTaskCheckoutRefusesPRContributionPaths(t *testing.T) {
	f := newRetainedCheckoutFixture(t)
	for _, test := range []struct {
		name   string
		change func(*CreateRequest)
	}{
		{name: "pull request", change: func(request *CreateRequest) { request.PRNumber = 42 }},
		{name: "remote contribution", change: func(request *CreateRequest) { request.RemoteContribution = &models.RemoteContribution{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := f.nextRequest()
			test.change(&request)
			if _, handled, err := f.manager.tryReuseRetainedTaskCheckout(context.Background(), request); err != nil || handled {
				t.Fatalf("tryReuseRetainedTaskCheckout() = handled %t, error %v; want no retained-path reuse", handled, err)
			}
		})
	}
}
