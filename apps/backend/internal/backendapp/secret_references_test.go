package backendapp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type secretReferenceFixture struct {
	agents       []*settingsmodels.Agent
	profiles     []*settingsmodels.AgentProfile
	executors    []*models.ExecutorProfile
	workspaces   []*models.Workspace
	repositories map[string][]*models.Repository
	fail         string
}

var errReferenceRead = errors.New("reference storage unavailable")

func (f *secretReferenceFixture) ListAgents(context.Context) ([]*settingsmodels.Agent, error) {
	if f.fail == "agents" {
		return nil, errReferenceRead
	}
	return f.agents, nil
}
func (f *secretReferenceFixture) ListAgentProfiles(context.Context, string) ([]*settingsmodels.AgentProfile, error) {
	if f.fail == "profiles" {
		return nil, errReferenceRead
	}
	return f.profiles, nil
}
func (f *secretReferenceFixture) ListAllExecutorProfiles(context.Context) ([]*models.ExecutorProfile, error) {
	if f.fail == "executors" {
		return nil, errReferenceRead
	}
	return f.executors, nil
}
func (f *secretReferenceFixture) ListWorkspaces(context.Context) ([]*models.Workspace, error) {
	if f.fail == "workspaces" {
		return nil, errReferenceRead
	}
	return f.workspaces, nil
}
func (f *secretReferenceFixture) ListRepositories(_ context.Context, workspaceID string) ([]*models.Repository, error) {
	if f.fail == "repositories" {
		return nil, errReferenceRead
	}
	return f.repositories[workspaceID], nil
}

func newSecretReferenceFixture() *secretReferenceFixture {
	return &secretReferenceFixture{
		agents: []*settingsmodels.Agent{{ID: "claude"}},
		profiles: []*settingsmodels.AgentProfile{{ID: "profile", Name: "Claude", EnvVars: []settingsmodels.ProfileEnvVar{
			{Key: "MY_TOKEN", SecretID: "secret"}, {Key: "LITERAL", Value: "secret"}, {Key: "OTHER", SecretID: "other"},
		}}},
		executors:  []*models.ExecutorProfile{{ID: "executor", Name: "Local", EnvVars: []models.ProfileEnvVar{{Key: "EXEC_TOKEN", SecretID: "secret"}}}},
		workspaces: []*models.Workspace{{ID: "visible"}, {ID: "private"}},
		repositories: map[string][]*models.Repository{
			"visible": {{ID: "repo", Name: "App", WorkspaceID: "visible", SecretBindings: []models.RepositorySecretBinding{{Key: "REPO_TOKEN", SecretID: "secret"}}}},
			"private": {{ID: "private-id", Name: "Private app", WorkspaceID: "private", SecretBindings: []models.RepositorySecretBinding{{Key: "PRIVATE_KEY", SecretID: "secret"}}}},
		},
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.9
func TestSecretReferenceDiscoveryAndRedaction(t *testing.T) {
	f := newSecretReferenceFixture()
	checker := secretReferenceChecker{agents: f, tasks: f, authorizeWorkspace: func(_ context.Context, id string) error {
		if id == "private" {
			return repoerrors.ErrWorkspaceNotFound
		}
		return nil
	}}
	refs, err := checker.list(context.Background(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	want := []secrets.Reference{
		{Kind: "agent_profile", ID: "profile", Name: "Claude", Key: "MY_TOKEN"},
		{Kind: "executor_profile", ID: "executor", Name: "Local", Key: "EXEC_TOKEN"},
		{Kind: "repository", ID: "repo", Name: "App", Key: "REPO_TOKEN"},
		{Kind: "repository"},
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("references = %#v, want %#v", refs, want)
	}
	refs, err = checker.list(context.Background(), "absent")
	if err != nil || len(refs) != 0 {
		t.Fatalf("unreferenced = %v, %v", refs, err)
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.11
func TestSecretReferenceReadFailure(t *testing.T) {
	for _, stage := range []string{"agents", "profiles", "executors", "workspaces", "repositories", "authorization"} {
		t.Run(stage, func(t *testing.T) {
			f := newSecretReferenceFixture()
			f.fail = stage
			checker := secretReferenceChecker{agents: f, tasks: f, authorizeWorkspace: func(context.Context, string) error {
				if stage == "authorization" {
					return errReferenceRead
				}
				return nil
			}}
			refs, err := checker.list(context.Background(), "secret")
			if !errors.Is(err, errReferenceRead) || refs != nil {
				t.Fatalf("references = %v, error = %v", refs, err)
			}
		})
	}
}
