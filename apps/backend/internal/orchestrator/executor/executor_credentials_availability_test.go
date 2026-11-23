package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestConfigureGitCredentialBroker_RequiresIssuerForManagedGitHub(t *testing.T) {
	for _, tc := range []struct {
		name     string
		policy   string
		env      map[string]string
		wantFail bool
	}{
		{name: "managed GitHub", policy: taskGitCredentialsModeManaged, wantFail: true},
		{name: "explicit GitHub token", policy: taskGitCredentialsModeManaged, env: map[string]string{envGHToken: "profile-token"}},
		{name: "executor inheritance", policy: taskGitCredentialsModeExecutor},
		{name: "no workspace credential service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
			if tc.policy != "" {
				exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
					policy: TaskGitCredentialPolicy{Mode: tc.policy},
				})
			}
			req := &LaunchAgentRequest{
				TaskID: "task-1", SessionID: "session-1", WorkspaceID: "workspace-1",
				StartAgent: true, Env: tc.env,
			}
			info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
				ID: "repo-1", Provider: "github", ProviderOwner: "acme", ProviderName: "widgets",
			}}

			err := exec.configureGitHubCredentialBroker(t.Context(), req, info)
			if tc.wantFail {
				require.ErrorContains(t, err, "managed GitHub credentials require the Git credential broker")
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, req.Env[envGitHubCredentialLease])
		})
	}
}
