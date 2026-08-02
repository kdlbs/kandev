package executor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	cloneURL       string
	returnPath     string
	setOriginErr   error
	setOriginPaths []string
}

func (c *cloneTransportTestCloner) EnsureWorkspaceClonedForProvider(
	context.Context, string, string, string, string, string, string, string, string,
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

func (c *cloneTransportTestCloner) BuildCloneURLWithHost(string, string, string, string) (string, error) {
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
