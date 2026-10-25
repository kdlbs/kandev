package executor

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.4
func TestHostAttachmentRepositoryRecovery(t *testing.T) {
	for _, scenario := range []string{"foreign-saved", "missing-scope", "empty-output", "foreign-output", "persistence", "cancelled", "cloner-unavailable", "inspector-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			c := repoclone.NewCloner(repoclone.Config{BasePath: t.TempDir()}, repoclone.ProtocolHTTPS, "", logger.Default())
			fake := &attachmentOutputCloner{Cloner: c, output: filepath.Join(t.TempDir(), "foreign")}
			e := NewExecutor(nil, nil, logger.Default(), ExecutorConfig{})
			updater := &attachmentFailingUpdater{}
			e.SetRepoCloner(fake, updater)
			r := &models.Repository{ID: "repo", WorkspaceID: "ws", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "team", ProviderName: "repo", RemoteURL: "https://gitlab.example/team/repo.git"}
			task, session := "task", "session"
			if scenario == "foreign-saved" {
				r.LocalPath = fake.output
			}
			if scenario == "missing-scope" {
				session = ""
			}
			if scenario == "empty-output" {
				fake.output = ""
			}
			if scenario == "persistence" {
				current, err := c.WorkspaceProviderRepositoryPath(r.WorkspaceID, r.Provider, r.ProviderHost, "", "", r.ProviderOwner, r.ProviderName)
				if err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command("git", "init", "-b", "main", current).CombinedOutput(); err != nil {
					t.Fatalf("git init: %v %s", err, out)
				}
				if out, err := exec.Command("git", "-C", current, "remote", "add", "origin", r.RemoteURL).CombinedOutput(); err != nil {
					t.Fatalf("git origin: %v %s", err, out)
				}
				fake.output = current
			}
			if scenario == "cloner-unavailable" {
				e.SetRepoCloner(nil, updater)
			}
			if scenario == "inspector-unavailable" {
				e.SetRepoCloner(&fakeRepoCloner{returnPath: fake.output}, updater)
			}
			ctx := context.Background()
			if scenario == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			original := r.LocalPath
			_, err := e.EnsureRepositoryClonedForSession(ctx, task, session, r)
			if err == nil {
				t.Fatal("unverified attachment path accepted")
			}
			if scenario == "persistence" && (updater.calls != 1 || !strings.Contains(err.Error(), "persist verified")) {
				t.Fatalf("verified persistence not reached: calls=%d err=%v", updater.calls, err)
			}
			if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if r.LocalPath != original {
				t.Fatal("failed recovery changed caller path")
			}
		})
	}
}

type attachmentOutputCloner struct {
	*repoclone.Cloner
	output string
}

func (c *attachmentOutputCloner) EnsureWorkspaceClonedWithCredentialRequestAndState(context.Context, repoclone.GitCredentialRequest, string, string) (string, repoclone.RemoteRefState, error) {
	return c.output, repoclone.RemoteRefStateHasRefs, nil
}

type attachmentFailingUpdater struct{ calls int }

func (u *attachmentFailingUpdater) UpdateRepositoryLocalPath(context.Context, string, string) error {
	u.calls++
	return errors.New("persistence refused")
}
func (*attachmentFailingUpdater) UpdateRepositoryDefaultBranch(context.Context, string, string) error {
	return nil
}

func TestHostAttachmentRepositoryRecoveryDeniedCredentialScope(t *testing.T) {
	for _, provider := range []string{"github", "custom-provider"} {
		t.Run(provider, func(t *testing.T) {
			c := repoclone.NewCloner(repoclone.Config{BasePath: t.TempDir()}, repoclone.ProtocolHTTPS, "", logger.Default())
			credential := &attachmentDeniedCredential{}
			c.SetGitCredentialProvider(credential)
			e := NewExecutor(nil, nil, logger.Default(), ExecutorConfig{})
			e.SetRepoCloner(c, &attachmentFailingUpdater{})
			r := &models.Repository{ID: "repo", WorkspaceID: "ws", SourceType: "provider", Provider: provider, ProviderHost: "https://github.com", ProviderOwner: "team", ProviderName: "repo", RemoteURL: "https://github.com/team/repo.git"}
			if provider == "custom-provider" {
				r.ProviderScope = "connection-scope"
				r.ProviderRepoID = "immutable-project"
			}
			_, err := e.EnsureRepositoryClonedForSession(context.Background(), "task", "session", r)
			if err == nil || credential.request == nil {
				t.Fatalf("credential denial not reached: %v", err)
			}
			request := credential.request
			if request.TaskID != "task" || request.SessionID != "session" || request.WorkspaceID != "ws" || request.RepositoryID != "repo" || request.ProviderScope != r.ProviderScope || request.ProviderRepositoryID != r.ProviderRepoID {
				t.Fatalf("credential request=%+v", request)
			}
			if r.LocalPath != "" {
				t.Fatal("denied clone published a path")
			}
		})
	}
}

type attachmentDeniedCredential struct {
	request *repoclone.GitCredentialRequest
}

func (c *attachmentDeniedCredential) ResolveGitCredential(_ context.Context, r repoclone.GitCredentialRequest) (string, string, error) {
	c.request = &r
	return "", "", errors.New("workspace credential denied")
}

func (u *attachmentFailingUpdater) UpdateVerifiedRepositoryLocalPath(ctx context.Context, repository *models.Repository, path string) error {
	return u.UpdateRepositoryLocalPath(ctx, repository.ID, path)
}
