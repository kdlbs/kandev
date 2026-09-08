package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestEnsureRepoLocalPath_ReconcilesGitHubOriginForCredentialPolicy(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tests := []struct {
		name       string
		policy     string
		origin     string
		wantOrigin string
	}{
		{
			name:       "executor inheritance uses host SSH clone protocol",
			policy:     taskGitCredentialsModeExecutor,
			origin:     "https://github.com/acme/widgets.git",
			wantOrigin: "git@github.com:acme/widgets.git",
		},
		{
			name:       "managed credentials restore HTTPS origin",
			policy:     "managed",
			origin:     "git@github.com:acme/widgets.git",
			wantOrigin: "https://github.com/acme/widgets.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := initGitRepoWithOrigin(t, tt.origin)
			exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
				policy: TaskGitCredentialPolicy{Mode: tt.policy},
			})
			exec.SetRepoCloner(&cloneTransportTestCloner{cloneURL: "git@github.com:acme/widgets.git"}, nil)

			repository := &models.Repository{
				WorkspaceID:   "workspace-1",
				SourceType:    "provider",
				Provider:      "github",
				ProviderOwner: "acme",
				ProviderName:  "widgets",
				LocalPath:     repoPath,
			}
			if err := exec.ensureRepoLocalPath(context.Background(), repository); err != nil {
				t.Fatalf("ensureRepoLocalPath() error = %v", err)
			}
			if got := gitOriginURL(t, repoPath); got != tt.wantOrigin {
				t.Fatalf("origin = %q, want %q", got, tt.wantOrigin)
			}
		})
	}
}

func TestEnsureRepoLocalPathReevaluatesGitHubProtocol(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := initGitRepoWithOrigin(t, "https://github.com/acme/widgets.git")
	resolver := &mutableExecutorGitProtocolResolver{protocol: repoclone.ProtocolSSH}
	cloner := repoclone.NewClonerWithProtocolResolver(
		repoclone.Config{BasePath: t.TempDir()}, resolver, "", nil,
	)
	executor := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	executor.SetRepoCloner(cloner, nil)
	repository := &models.Repository{
		WorkspaceID:   "workspace-1",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
		LocalPath:     repoPath,
	}

	if err := executor.ensureRepoLocalPath(context.Background(), repository); err != nil {
		t.Fatalf("first ensureRepoLocalPath() error = %v", err)
	}
	if got := gitOriginURL(t, repoPath); got != "git@github.com:acme/widgets.git" {
		t.Fatalf("first origin = %q, want SSH URL", got)
	}

	resolver.protocol = repoclone.ProtocolHTTPS
	if err := executor.ensureRepoLocalPath(context.Background(), repository); err != nil {
		t.Fatalf("second ensureRepoLocalPath() error = %v", err)
	}
	if got := gitOriginURL(t, repoPath); got != "https://github.com/acme/widgets.git" {
		t.Fatalf("second origin = %q, want HTTPS URL", got)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.11
// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.12
func TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart(t *testing.T) {
	fixture := newPreparedWorkspaceOriginFixture(t)
	executor := newTestExecutor(t, fixture.agentManager, fixture.repo)
	executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	executor.SetRepoCloner(fixture.cloner, nil)

	_, err := executor.LaunchPreparedSession(context.Background(), fixture.task, fixture.sessionID, LaunchOptions{
		AgentProfileID: "profile-prepared-origins",
		StartAgent:     true,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession() error = %v", err)
	}

	select {
	case err := <-fixture.startDone:
		if err != nil {
			t.Fatalf("agent started before origin reconciliation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for StartAgentProcess")
	}
	for _, path := range fixture.originPaths {
		if got := fixture.cloner.originCallCount(path); got != 1 {
			t.Errorf("SetOriginURL calls for %q = %d, want 1", path, got)
		}
	}
}

type preparedWorkspaceOriginFixture struct {
	repo         *mockRepository
	task         *v1.Task
	sessionID    string
	agentManager *mockAgentManager
	cloner       *countingRepoCloner
	startDone    chan error
	originPaths  []string
}

func newPreparedWorkspaceOriginFixture(t *testing.T) *preparedWorkspaceOriginFixture {
	t.Helper()
	const (
		taskID      = "task-prepared-origins"
		sessionID   = "session-prepared-origins"
		workspaceID = "workspace-prepared-origins"
	)
	firstPath := initGitRepoWithOrigin(t, "https://github.com/acme/first.git")
	secondPath := initGitRepoWithOrigin(t, "git@github.com:acme/second.git")
	configLock := filepath.Join(secondPath, ".git", "config.lock")
	if err := os.WriteFile(configLock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(configLock) })

	fixture := &preparedWorkspaceOriginFixture{
		repo:      newPreparedWorkspaceOriginRepository(taskID, sessionID, workspaceID, firstPath, secondPath),
		task:      &v1.Task{ID: taskID, WorkspaceID: workspaceID, Title: "prepared origin reconciliation"},
		sessionID: sessionID,
		cloner: &countingRepoCloner{
			Cloner:         repoclone.NewCloner(repoclone.Config{}, repoclone.ProtocolSSH, "", logger.Default()),
			setOriginCalls: make(map[string]int),
		},
		startDone:   make(chan error, 1),
		originPaths: []string{firstPath, secondPath},
	}
	fixture.agentManager = newPreparedWorkspaceOriginAgentManager(fixture)
	return fixture
}

func newPreparedWorkspaceOriginRepository(taskID, sessionID, workspaceID, firstPath, secondPath string) *mockRepository {
	repo := newMockRepository()
	repo.sessions[sessionID] = &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: "profile-prepared-origins",
		State: models.TaskSessionStateCreated, StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	repo.executorsRunning[sessionID] = &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID,
		AgentExecutionID: "exec-prepared-origins", Status: models.ExecutorRunningStatusReady,
	}
	repo.taskRepositories["task-repo-first"] = &models.TaskRepository{
		ID: "task-repo-first", TaskID: taskID, RepositoryID: "repo-first", Position: 0,
	}
	repo.taskRepositories["task-repo-second"] = &models.TaskRepository{
		ID: "task-repo-second", TaskID: taskID, RepositoryID: "repo-second", Position: 1,
	}
	repo.repositories["repo-first"] = preparedWorkspaceOriginRepository(
		"repo-first", workspaceID, "first", firstPath,
	)
	repo.repositories["repo-second"] = preparedWorkspaceOriginRepository(
		"repo-second", workspaceID, "second", secondPath,
	)
	return repo
}

func preparedWorkspaceOriginRepository(id, workspaceID, name, localPath string) *models.Repository {
	return &models.Repository{
		ID: id, WorkspaceID: workspaceID, SourceType: "provider", Provider: "github",
		ProviderOwner: "acme", ProviderName: name, LocalPath: localPath,
	}
}

func newPreparedWorkspaceOriginAgentManager(fixture *preparedWorkspaceOriginFixture) *mockAgentManager {
	return &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "exec-prepared-origins", nil
		},
		startAgentProcessFunc: func(ctx context.Context, _ string) error {
			err := fixture.verifyOriginsAtAgentStart(ctx)
			fixture.startDone <- err
			return err
		},
	}
}

func (fixture *preparedWorkspaceOriginFixture) verifyOriginsAtAgentStart(ctx context.Context) error {
	for _, check := range []struct {
		path string
		want string
	}{
		{path: fixture.originPaths[0], want: "git@github.com:acme/first.git"},
		{path: fixture.originPaths[1], want: "git@github.com:acme/second.git"},
	} {
		got, err := readGitOriginURL(ctx, check.path)
		if err != nil {
			return err
		}
		if got != check.want {
			return fmt.Errorf("origin for %s = %q, want %q", check.path, got, check.want)
		}
	}
	return nil
}

func TestEnsureRepoLocalPath_DoesNotRewriteUserManagedOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	const origin = "git@github.com:acme/widgets.git"
	repoPath := initGitRepoWithOrigin(t, origin)
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: "managed"},
	})
	exec.SetRepoCloner(&cloneTransportTestCloner{cloneURL: "https://github.com/acme/widgets.git"}, nil)

	repository := &models.Repository{
		WorkspaceID:   "workspace-1",
		SourceType:    sourceTypeLocal,
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
		LocalPath:     repoPath,
	}
	if err := exec.ensureRepoLocalPath(context.Background(), repository); err != nil {
		t.Fatalf("ensureRepoLocalPath() error = %v", err)
	}
	if got := gitOriginURL(t, repoPath); got != origin {
		t.Fatalf("origin = %q, want unchanged %q", got, origin)
	}
}

func TestEnsureRepoLocalPath_ReconcilesFreshGitHubCheckoutOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := initGitRepoWithOrigin(t, "https://github.com/acme/widgets.git")
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	exec.SetRepoCloner(&cloneTransportTestCloner{
		cloneURL:   "git@github.com:acme/widgets.git",
		returnPath: repoPath,
	}, nil)

	repository := &models.Repository{
		WorkspaceID:   "workspace-1",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
	}
	if err := exec.ensureRepoLocalPath(context.Background(), repository); err != nil {
		t.Fatalf("ensureRepoLocalPath() error = %v", err)
	}
	if repository.LocalPath != repoPath {
		t.Fatalf("LocalPath = %q, want %q", repository.LocalPath, repoPath)
	}
	if got := gitOriginURL(t, repoPath); got != "git@github.com:acme/widgets.git" {
		t.Fatalf("origin = %q, want SSH URL", got)
	}
}

func TestEnsureRepoLocalPath_ReturnsOriginUpdateFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := initGitRepoWithOrigin(t, "https://github.com/acme/widgets.git")
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	exec.SetRepoCloner(&cloneTransportTestCloner{
		cloneURL:     "git@github.com:acme/widgets.git",
		setOriginErr: fmt.Errorf("read-only repository"),
	}, nil)

	err := exec.ensureRepoLocalPath(context.Background(), &models.Repository{
		WorkspaceID:   "workspace-1",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
		LocalPath:     repoPath,
	})
	if err == nil || !strings.Contains(err.Error(), "read-only repository") {
		t.Fatalf("ensureRepoLocalPath() error = %v, want origin-update failure", err)
	}
}

func TestEnsureRepoLocalPath_PersistsFreshCloneBeforeOriginFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := initGitRepoWithOrigin(t, "https://github.com/acme/widgets.git")
	updater := &localPathRecordingRepoUpdater{}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})
	exec.SetRepoCloner(&cloneTransportTestCloner{
		cloneURL:     "git@github.com:acme/widgets.git",
		returnPath:   repoPath,
		setOriginErr: fmt.Errorf("read-only repository"),
	}, updater)

	err := exec.ensureRepoLocalPath(context.Background(), &models.Repository{
		ID:            "repo-1",
		WorkspaceID:   "workspace-1",
		SourceType:    "provider",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
	})
	if err == nil || !strings.Contains(err.Error(), "read-only repository") {
		t.Fatalf("ensureRepoLocalPath() error = %v, want origin-update failure", err)
	}
	if updater.localPath != repoPath {
		t.Fatalf("persisted local path = %q, want %q", updater.localPath, repoPath)
	}
}

func TestEnsureRepoClonedPrefersDeclaredRemoteURL(t *testing.T) {
	cloner := &cloneTransportTestCloner{cloneURL: "https://wrong.example/acme/widgets.git"}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetRepoCloner(cloner, nil)

	_, err := exec.ensureRepoCloned(context.Background(), &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", Provider: "bitbucket",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://bitbucket.example/scm/ENG/widgets.git",
	})
	if err != nil {
		t.Fatalf("ensureRepoCloned(): %v", err)
	}
	if got, want := cloner.requestedCloneURL, "https://bitbucket.example/scm/ENG/widgets.git"; got != want {
		t.Fatalf("clone URL = %q, want declared remote %q", got, want)
	}
}

func TestEnsureRepoClonedUsesConfiguredProtocolForLegacyRepository(t *testing.T) {
	cloner := &cloneTransportTestCloner{cloneURL: "git@github.com:acme/widgets.git"}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetRepoCloner(cloner, nil)

	_, err := exec.ensureRepoCloned(context.Background(), &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", Provider: "github",
		ProviderOwner: "acme", ProviderName: "widgets",
	})
	if err != nil {
		t.Fatalf("ensureRepoCloned(): %v", err)
	}
	if got, want := cloner.requestedCloneURL, "git@github.com:acme/widgets.git"; got != want {
		t.Fatalf("clone URL = %q, want configured protocol %q", got, want)
	}
}

func TestResolveTaskRepoInfoDefersPluginRefreshUntilWorktreeMaterialization(t *testing.T) {
	repoPath := initGitRepoWithOrigin(t, "https://bitbucket.org/acme/widgets.git")
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider",
		Provider: "bitbucket", ProviderHost: "https://bitbucket.org",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://bitbucket.org/acme/widgets.git", LocalPath: repoPath,
		DefaultBranch: "main", PullBeforeWorktree: true,
	}
	cloner := &cloneTransportTestCloner{}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetRepoCloner(cloner, nil)

	info, err := exec.resolveTaskRepoInfoForSession(context.Background(), "session-1", &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfoForSession(): %v", err)
	}
	if cloner.refreshCalls != 0 || info.RemoteSyncHandled || !info.PullBeforeWorktree || info.RefreshRepository == nil {
		t.Fatalf("deferred refresh state = calls %d, remote sync handled %v, pull before worktree %v, callback nil %v",
			cloner.refreshCalls, info.RemoteSyncHandled, info.PullBeforeWorktree, info.RefreshRepository == nil)
	}
	if err := info.RefreshRepository(context.Background()); err != nil {
		t.Fatalf("RefreshRepository(): %v", err)
	}
	if cloner.refreshCalls != 1 {
		t.Fatalf("refresh calls after materialization = %d, want 1", cloner.refreshCalls)
	}
	if got := cloner.refreshRequest; got.TaskID != "task-1" || got.SessionID != "session-1" ||
		got.RepositoryID != "repo-1" || got.CloneURL != "https://bitbucket.org/acme/widgets.git" {
		t.Fatalf("refresh request lost exact scope: %+v", got)
	}
	if cloner.refreshPath != repoPath {
		t.Fatalf("refresh path = %q, want %q", cloner.refreshPath, repoPath)
	}
}

func TestResolveTaskRepoInfoManagedGitHubRefreshCarriesPRHead(t *testing.T) {
	repoPath := initGitRepoWithOrigin(t, "https://github.com/acme/widgets.git")
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://github.com/acme/widgets.git", LocalPath: repoPath,
		DefaultBranch: "main", PullBeforeWorktree: true,
	}
	cloner := &cloneTransportTestCloner{}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetRepoCloner(cloner, nil)
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged},
	})

	info, err := exec.resolveTaskRepoInfoForSession(context.Background(), "session-1", &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
		CheckoutBranch: "feature/pr", Metadata: map[string]interface{}{"pr_number": 42},
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfoForSession(): %v", err)
	}
	if info.RefreshRepository == nil {
		t.Fatal("resolveTaskRepoInfoForSession() returned no managed refresh callback")
	}
	if err := info.RefreshRepository(context.Background()); err != nil {
		t.Fatalf("RefreshRepository(): %v", err)
	}
	if got := cloner.refreshRequest; got.CheckoutBranch != "feature/pr" || got.PRNumber != 42 {
		t.Fatalf("refresh request PR identity = %q/%d, want feature/pr/42", got.CheckoutBranch, got.PRNumber)
	}
}

func TestResolveTaskRepoInfoFailsWhenManagedGitHubRefreshFails(t *testing.T) {
	repoPath := initGitRepoWithOrigin(t, "https://github.com/acme/widgets.git")
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://github.com/acme/widgets.git", LocalPath: repoPath,
		DefaultBranch: "main", PullBeforeWorktree: true,
	}
	cloner := &cloneTransportTestCloner{refreshErr: fmt.Errorf("refresh failed: authentication denied")}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetRepoCloner(cloner, nil)
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged},
	})

	info, err := exec.resolveTaskRepoInfoForSession(context.Background(), "session-1", &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfoForSession() error = %v, want deferred callback", err)
	}
	if info.RefreshRepository == nil {
		t.Fatal("resolveTaskRepoInfoForSession() returned no managed refresh callback")
	}
	err = info.RefreshRepository(context.Background())
	if err == nil || !strings.Contains(err.Error(), "authentication denied") {
		t.Fatalf("RefreshRepository() error = %v, want managed refresh failure", err)
	}
}

func TestResolveTaskRepoInfoLeavesExecutorGitHubRefreshToWorktreeManager(t *testing.T) {
	repoPath := initGitRepoWithOrigin(t, "git@github.com:acme/widgets.git")
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider",
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
		LocalPath: repoPath, DefaultBranch: "main", PullBeforeWorktree: true,
	}
	cloner := &cloneTransportTestCloner{
		cloneURL: "git@github.com:acme/widgets.git", refreshErr: fmt.Errorf("refresh must not run"),
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetRepoCloner(cloner, nil)
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})

	info, err := exec.resolveTaskRepoInfoForSession(context.Background(), "session-1", &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfoForSession() error = %v", err)
	}
	if cloner.refreshCalls != 0 || info.RemoteSyncHandled || !info.PullBeforeWorktree {
		t.Fatalf("executor route = refresh calls %d, remote sync handled %v, pull before worktree %v",
			cloner.refreshCalls, info.RemoteSyncHandled, info.PullBeforeWorktree)
	}
}

func TestResolveTaskRepoInfoRefreshesManagedGitLabBeforeWorktree(t *testing.T) {
	repoPath := initGitRepoWithOrigin(t, "https://gitlab.example/acme/widgets.git")
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider",
		Provider: "gitlab", ProviderHost: "https://gitlab.example",
		ProviderOwner: "acme", ProviderName: "widgets",
		RemoteURL: "https://gitlab.example/acme/widgets.git", LocalPath: repoPath,
		DefaultBranch: "main", PullBeforeWorktree: true,
	}
	cloner := &cloneTransportTestCloner{}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetRepoCloner(cloner, nil)
	exec.SetGitLabCredentialResolver(&fakeGitLabCredentialResolver{byWorkspace: map[string]struct{ host, token string }{
		"workspace-1": {host: "https://gitlab.example", token: "gitlab-token"},
	}})

	info, err := exec.resolveTaskRepoInfoForSession(context.Background(), "session-1", &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfoForSession() error = %v", err)
	}
	if cloner.refreshCalls != 0 || info.RemoteSyncHandled || !info.PullBeforeWorktree || info.RefreshRepository == nil {
		t.Fatalf("GitLab deferred route = refresh calls %d, remote sync handled %v, pull before worktree %v, callback nil %v",
			cloner.refreshCalls, info.RemoteSyncHandled, info.PullBeforeWorktree, info.RefreshRepository == nil)
	}
	if err := info.RefreshRepository(context.Background()); err != nil {
		t.Fatalf("GitLab RefreshRepository(): %v", err)
	}
	if cloner.refreshCalls != 1 {
		t.Fatalf("GitLab refresh calls after materialization = %d, want 1", cloner.refreshCalls)
	}
	if cloner.refreshCredentialOrigin != "https://gitlab.example" || cloner.refreshToken != "gitlab-token" {
		t.Fatalf("GitLab refresh credentials = %q/%q", cloner.refreshCredentialOrigin, cloner.refreshToken)
	}
}

func TestResolveTaskRepoInfoRefreshesAzureDevOpsBeforeWorktree(t *testing.T) {
	repoPath := initGitRepoWithOrigin(t, "https://dev.azure.com/acme/Platform/_git/widgets")
	repositoryStore := newMockRepository()
	repositoryStore.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider",
		Provider: providerAzureDevOps, ProviderHost: "https://dev.azure.com/acme",
		ProviderOwner: "Platform", ProviderName: "widgets",
		RemoteURL: "https://dev.azure.com/acme/Platform/_git/widgets", LocalPath: repoPath,
		DefaultBranch: "main", PullBeforeWorktree: true,
	}
	cloner := &cloneTransportTestCloner{}
	exec := newTestExecutor(t, &mockAgentManager{}, repositoryStore)
	exec.SetRepoCloner(cloner, nil)
	exec.secretStore = &mockSecretStore{secrets: map[string]string{
		"azure_devops:workspace-1:pat": "azure-token",
	}}

	info, err := exec.resolveTaskRepoInfoForSession(context.Background(), "session-1", &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfoForSession() error = %v", err)
	}
	if cloner.basicRefreshCalls != 0 || info.RemoteSyncHandled || !info.PullBeforeWorktree || info.RefreshRepository == nil {
		t.Fatalf("Azure DevOps deferred route = basic refresh calls %d, remote sync handled %v, pull before worktree %v, callback nil %v",
			cloner.basicRefreshCalls, info.RemoteSyncHandled, info.PullBeforeWorktree, info.RefreshRepository == nil)
	}
	if err := info.RefreshRepository(context.Background()); err != nil {
		t.Fatalf("Azure DevOps RefreshRepository(): %v", err)
	}
	if cloner.basicRefreshCalls != 1 {
		t.Fatalf("Azure DevOps refresh calls after materialization = %d, want 1", cloner.basicRefreshCalls)
	}
	if cloner.basicRefreshPassword != "azure-token" || cloner.basicRefreshUsername != "kandev" {
		t.Fatalf("Azure DevOps refresh credentials = %q/%q", cloner.basicRefreshUsername, cloner.basicRefreshPassword)
	}
}

func TestBuildResumeRequestPreparesEachAttachedRepositoryOnce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPathOne := initGitRepoWithOrigin(t, "https://github.com/acme/one.git")
	repoPathTwo := initGitRepoWithOrigin(t, "https://github.com/acme/two.git")
	repo := newMockRepository()
	repo.tasks["task-1"] = &models.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "resume"}
	repo.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: "provider", Provider: "github",
		ProviderOwner: "acme", ProviderName: "one", LocalPath: repoPathOne,
	}
	repo.repositories["repo-2"] = &models.Repository{
		ID: "repo-2", WorkspaceID: "workspace-1", SourceType: "provider", Provider: "github",
		ProviderOwner: "acme", ProviderName: "two", LocalPath: repoPathTwo,
	}
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1", Position: 0}
	repo.taskRepositories["task-repo-2"] = &models.TaskRepository{ID: "task-repo-2", TaskID: "task-1", RepositoryID: "repo-2", Position: 1}
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	cloner := &cloneTransportTestCloner{cloneURL: "git@github.com:acme/widgets.git"}
	executor.SetRepoCloner(cloner, nil)
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", ExecutorID: models.ExecutorIDLocal,
		State: models.TaskSessionStateWaitingForInput,
	}

	if _, _, _, _, _, err := executor.buildResumeRequest(context.Background(), &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "resume"}, session, true); err != nil {
		t.Fatalf("buildResumeRequest() error = %v", err)
	}
	for _, repositoryPath := range []string{repoPathOne, repoPathTwo} {
		calls := 0
		for _, preparedPath := range cloner.setOriginPaths {
			if preparedPath == repositoryPath {
				calls++
			}
		}
		if calls != 1 {
			t.Fatalf("origin reconciliation calls for %q = %d, want one", repositoryPath, calls)
		}
	}
}

type localPathRecordingRepoUpdater struct {
	localPath string
}

func (u *localPathRecordingRepoUpdater) UpdateRepositoryLocalPath(_ context.Context, _, localPath string) error {
	u.localPath = localPath
	return nil
}

func (u *localPathRecordingRepoUpdater) UpdateRepositoryDefaultBranch(context.Context, string, string) error {
	return nil
}

type cloneTransportTestCloner struct {
	cloneURL                string
	requestedCloneURL       string
	returnPath              string
	setOriginErr            error
	refreshCalls            int
	refreshErr              error
	refreshRequest          repoclone.GitCredentialRequest
	refreshPath             string
	refreshCredentialOrigin string
	refreshToken            string
	basicRefreshCalls       int
	basicRefreshUsername    string
	basicRefreshPassword    string
	setOriginPaths          []string
}

type mutableExecutorGitProtocolResolver struct {
	protocol string
}

func (r *mutableExecutorGitProtocolResolver) ResolveGitProtocol(context.Context, string) string {
	return r.protocol
}

type countingRepoCloner struct {
	*repoclone.Cloner
	mu             sync.Mutex
	setOriginCalls map[string]int
}

func (c *countingRepoCloner) SetOriginURL(ctx context.Context, repositoryPath, originURL string) error {
	c.mu.Lock()
	c.setOriginCalls[repositoryPath]++
	c.mu.Unlock()
	return c.Cloner.SetOriginURL(ctx, repositoryPath, originURL)
}

func (c *countingRepoCloner) originCallCount(repositoryPath string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.setOriginCalls[repositoryPath]
}

var _ authenticatedRepoCloner = (*cloneTransportTestCloner)(nil)

func (c *cloneTransportTestCloner) EnsureWorkspaceClonedWithCredentialRequest(
	_ context.Context, request repoclone.GitCredentialRequest, _, _ string,
) (string, error) {
	c.requestedCloneURL = request.CloneURL
	return c.returnPath, nil
}

func (c *cloneTransportTestCloner) RefreshWorkspaceRepositoryWithCredentialRequest(
	_ context.Context, request repoclone.GitCredentialRequest, repositoryPath, credentialOrigin, token string,
) error {
	c.refreshCalls++
	c.refreshRequest = request
	c.refreshPath = repositoryPath
	c.refreshCredentialOrigin = credentialOrigin
	c.refreshToken = token
	return c.refreshErr
}

func (c *cloneTransportTestCloner) RefreshWorkspaceRepositoryWithBasicAuth(
	_ context.Context, _, _, _, _, _, _, repositoryPath, username, password string,
) error {
	c.basicRefreshCalls++
	c.refreshPath = repositoryPath
	c.basicRefreshUsername = username
	c.basicRefreshPassword = password
	return c.refreshErr
}

func (c *cloneTransportTestCloner) EnsureWorkspaceClonedWithBasicAuth(
	_ context.Context, _, _, _, _, _, _, _, _ string,
) (string, error) {
	return c.returnPath, nil
}

func (c *cloneTransportTestCloner) ShouldRecloneForWorkspace(string, string) bool { return false }

func (c *cloneTransportTestCloner) SetOriginURL(ctx context.Context, repositoryPath, originURL string) error {
	c.setOriginPaths = append(c.setOriginPaths, repositoryPath)
	if c.setOriginErr != nil {
		return c.setOriginErr
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repositoryPath, "remote", "set-url", "origin", "--", originURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set origin: %w: %s", err, out)
	}
	return nil
}

func (c *cloneTransportTestCloner) BuildCloneURLWithHost(context.Context, string, string, string, string) (string, error) {
	return c.cloneURL, nil
}

func initGitRepoWithOrigin(t *testing.T, origin string) string {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "repo")
	runGitInTest(t, "", "init", "--quiet", repoPath)
	runGitInTest(t, repoPath, "remote", "add", "origin", origin)
	return repoPath
}

func gitOriginURL(t *testing.T, repoPath string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git remote get-url origin: %v", err)
	}
	return string(out[:len(out)-1])
}

func readGitOriginURL(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
