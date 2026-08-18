package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newPreflightTestExecutor(t *testing.T, repo *mockRepository) *Executor {
	t.Helper()
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	return exec
}

func seedPreflightTaskRepository(repo *mockRepository, taskID, repositoryID string, repository *models.Repository) {
	repo.taskRepositories[taskID+"/"+repositoryID] = &models.TaskRepository{
		ID: taskID + "/" + repositoryID, TaskID: taskID, RepositoryID: repositoryID,
	}
	repo.repositories[repositoryID] = repository
}

// TestPreflightManagedGitCredentialsAcceptsValidLegacyAndPluginGitHubRows
// covers AC-1/AC-2: legacy rows with either GitHub HTTPS/SSH spellings and an
// empty or plugin-specific provider must pass the preflight cleanly.
func TestPreflightManagedGitCredentialsAcceptsValidLegacyAndPluginGitHubRows(t *testing.T) {
	for name, remoteURL := range map[string]string{
		"empty provider, ssh remote":   "git@github.com:acme/widgets.git",
		"empty provider, https remote": "https://github.com/acme/widgets.git",
	} {
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{
				ID: "repo-1", SourceType: sourceTypeLocal, RemoteURL: remoteURL,
			})
			exec := newPreflightTestExecutor(t, repo)

			if err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1"); err != nil {
				t.Fatalf("PreflightManagedGitCredentials() error = %v", err)
			}
		})
	}

	t.Run("plugin provider, ssh remote", func(t *testing.T) {
		repo := newMockRepository()
		seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{
			ID: "repo-1", SourceType: sourceTypeLocal, Provider: "kandev-plugin-tags",
			RemoteURL: "ssh://git@github.com/acme/widgets.git",
		})
		exec := newPreflightTestExecutor(t, repo)

		if err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1"); err != nil {
			t.Fatalf("PreflightManagedGitCredentials() error = %v", err)
		}
	})
}

// TestPreflightManagedGitCredentialsRejectsIrreparableIdentity covers AC-9: a
// repository whose remote host conflicts with an untrusted/custom provider
// (no explicit provider host) must fail the preflight with a safe,
// repository-scoped error, before any session row is ever persisted.
func TestPreflightManagedGitCredentialsRejectsIrreparableIdentity(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{
		ID: "repo-1", SourceType: sourceTypeLocal, Provider: "acme-forge",
		RemoteURL: "https://forge.example/acme/widgets.git",
	})
	exec := newPreflightTestExecutor(t, repo)

	err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1")
	if err == nil {
		t.Fatal("PreflightManagedGitCredentials() error = nil, want rejection of an untrusted custom host")
	}
	if !strings.Contains(err.Error(), "repo-1") {
		t.Fatalf("error %q does not name the safe repository id repo-1", err.Error())
	}
}

// TestPreflightManagedGitCredentialsValidatesEveryBindingInOrder covers the
// multi-repository requirement: a valid primary binding alongside an
// irreparable secondary binding must still fail, so a multi-repo task cannot
// slip one bad repository past the check.
func TestPreflightManagedGitCredentialsValidatesEveryBindingInOrder(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-primary", &models.Repository{
		ID: "repo-primary", SourceType: sourceTypeLocal, RemoteURL: "https://github.com/acme/widgets.git",
	})
	seedPreflightTaskRepository(repo, "task-1", "repo-secondary", &models.Repository{
		ID: "repo-secondary", SourceType: sourceTypeLocal, Provider: "acme-forge",
		RemoteURL: "git@forge.example:acme/widgets2.git",
	})
	exec := newPreflightTestExecutor(t, repo)

	err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1")
	if err == nil {
		t.Fatal("PreflightManagedGitCredentials() error = nil, want rejection for the irreparable secondary binding")
	}
	if !strings.Contains(err.Error(), "repo-secondary") {
		t.Fatalf("error %q does not name the failing secondary repository id", err.Error())
	}
}

// TestPreflightManagedGitCredentialsSkipsWhenExecutorModePolicy covers the
// workspace policy applicability rule: when the resolved policy routes Git
// credentials through the executor rather than the managed broker, no
// repository identity is validated at all - matching
// configureGitCredentialBrokerForRepositories' own unconditional skip.
func TestPreflightManagedGitCredentialsSkipsWhenExecutorModePolicy(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{
		ID: "repo-1", SourceType: sourceTypeLocal, Provider: "acme-forge",
		RemoteURL: "https://forge.example/acme/widgets.git",
	})
	exec := newPreflightTestExecutor(t, repo)
	exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
		policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor},
	})

	if err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1"); err != nil {
		t.Fatalf("PreflightManagedGitCredentials() error = %v, want skip under executor-mode policy", err)
	}
}

// TestPreflightManagedGitCredentialsNoopWithoutBroker covers the feature-off
// case: with no credential broker configured, the preflight must not fail a
// launch that never would have attempted managed credential issuance anyway.
func TestPreflightManagedGitCredentialsNoopWithoutBroker(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{ID: "repo-1", SourceType: sourceTypeLocal})
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	if err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1"); err != nil {
		t.Fatalf("PreflightManagedGitCredentials() error = %v, want nil with no broker configured", err)
	}
}

// TestPreflightManagedGitCredentialsIsNonMutating asserts the preflight never
// touches the repository store: it must not create a session, issue a
// lease, or write anything, even when it rejects a binding.
func TestPreflightManagedGitCredentialsIsNonMutating(t *testing.T) {
	repo := newMockRepository()
	seedPreflightTaskRepository(repo, "task-1", "repo-1", &models.Repository{
		ID: "repo-1", SourceType: sourceTypeLocal, Provider: "acme-forge",
		RemoteURL: "https://forge.example/acme/widgets.git",
	})
	exec := newPreflightTestExecutor(t, repo)
	issuer := exec.gitCredentialIssuer.(*fakeGitHubCredentialLeaseIssuer)

	if err := exec.PreflightManagedGitCredentials(context.Background(), "workspace-1", "task-1"); err == nil {
		t.Fatal("expected rejection")
	}
	if issuer.calls != 0 {
		t.Fatalf("Issue calls = %d, want 0 (preflight must never issue a lease)", issuer.calls)
	}
	if len(repo.createTaskSessionCalls) != 0 {
		t.Fatalf("CreateTaskSession calls = %d, want 0 (preflight must never persist a session)", len(repo.createTaskSessionCalls))
	}
}

// TestPrepareSessionRejectsIrreparableManagedCredentialRepositoryBeforePersisting
// covers AC-9 end to end at the PrepareSession seam: a task bound to a
// repository with an irreparable managed-credential identity must fail
// before a session row exists at all - not create one that only fails later
// when LaunchPreparedSession attempts credential issuance.
func TestPrepareSessionRejectsIrreparableManagedCredentialRepositoryBeforePersisting(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-123", RepositoryID: "repo-123",
	}
	repo.repositories["repo-123"] = &models.Repository{
		ID: "repo-123", SourceType: sourceTypeLocal, Provider: "acme-forge",
		RemoteURL: "https://forge.example/acme/widgets.git",
	}
	exec := newPreflightTestExecutor(t, repo)

	task := &v1.Task{ID: "task-123", WorkspaceID: "workspace-123"}
	_, err := exec.PrepareSession(context.Background(), task, "profile-123", "", "", "")
	if err == nil {
		t.Fatal("PrepareSession() error = nil, want rejection of the irreparable repository binding")
	}
	if !strings.Contains(err.Error(), "repo-123") {
		t.Fatalf("error %q does not name the safe repository id repo-123", err.Error())
	}
	if len(repo.createTaskSessionCalls) != 0 {
		t.Fatalf("CreateTaskSession calls = %d, want 0 (no session row should be persisted)", len(repo.createTaskSessionCalls))
	}
}
