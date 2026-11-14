package workflowsync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseWorkflowConfigHostRoundTrip(t *testing.T) {
	store := setupTestStore(t)
	var request SetConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{"repo_owner":"acme","repo_name":"workflows","github_host":"https://GIT.example.com:8443/"}`), &request))
	require.NoError(t, request.Normalize())
	_, err := store.UpsertConfigForWorkspace(context.Background(), "workspace", &request)
	require.NoError(t, err)
	loaded, err := store.GetConfigForWorkspace(context.Background(), "workspace")
	require.NoError(t, err)
	encoded, err := json.Marshal(loaded)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"github_host":"git.example.com:8443"`)
}

func TestEnterpriseWorkflowConfigRejectsInvalidHost(t *testing.T) {
	var request SetConfigRequest
	require.NoError(t, json.Unmarshal([]byte(`{"repo_owner":"acme","repo_name":"workflows","github_host":"http://git.example.com"}`), &request))
	require.ErrorIs(t, request.Normalize(), ErrInvalidConfig)
}

type workflowHostBindingKey struct{}

type replacingWorkflowClients struct {
	fakeGitHubClients
	generation    int
	host          string
	replaceOnRead bool
}

func (f *replacingWorkflowClients) BindWorkspaceGitHubHost(ctx context.Context, _, host string) (context.Context, error) {
	if host != f.host {
		return ctx, github.ErrGitHubHostMismatch
	}
	return context.WithValue(ctx, workflowHostBindingKey{}, f.generation), nil
}

func (f *replacingWorkflowClients) CommitWorkspaceGitHubHost(ctx context.Context, _ string, apply func(context.Context) error) error {
	if generation, ok := ctx.Value(workflowHostBindingKey{}).(int); !ok || generation != f.generation {
		return github.ErrGitHubHostMismatch
	}
	return apply(ctx)
}

func (f *replacingWorkflowClients) GetRepoFileContentForWorkspace(ctx context.Context, workspaceID, owner, repo, path, ref string) ([]byte, error) {
	content, err := f.fakeGitHubClients.GetRepoFileContentForWorkspace(ctx, workspaceID, owner, repo, path, ref)
	if f.replaceOnRead {
		f.generation++
	}
	return content, err
}

func TestEnterpriseWorkflowSyncRejectsDelayedReplacement(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	clients := &replacingWorkflowClients{fakeGitHubClients: fakeGitHubClients{client: seededMockClient()}, host: "git.example.com", generation: 1}
	svc.githubClients = clients
	request := &SetConfigRequest{RepoOwner: "acme", RepoName: "flows", GitHubHost: clients.host}
	_, err := svc.SetConfigForWorkspace(context.Background(), "workspace", request)
	require.NoError(t, err)
	clients.replaceOnRead = true
	_, err = svc.SyncWorkspace(context.Background(), "workspace")
	require.ErrorIs(t, err, github.ErrGitHubHostMismatch)
	require.Zero(t, applier.callCount(), "old-host files must never reach the workflow applier")
	clients.replaceOnRead = false
	_, err = svc.SyncWorkspace(context.Background(), "workspace")
	require.NoError(t, err)
	require.Equal(t, 1, applier.callCount())
	request.GitHubHost = "github.com"
	_, err = svc.SetConfigForWorkspace(context.Background(), "workspace", request)
	require.ErrorIs(t, err, github.ErrGitHubHostMismatch)
	config, err := svc.GetConfigForWorkspace(context.Background(), "workspace")
	require.NoError(t, err)
	require.Equal(t, clients.host, config.GitHubHost)
}
