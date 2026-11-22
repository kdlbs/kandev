package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestResumeSession_PreparesFreshManagedGitEnvironment(t *testing.T) {
	for _, issuerAvailable := range []bool{true, false} {
		name := "fresh lease and reissue capability"
		if !issuerAvailable {
			name = "missing issuer prevents launch"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			attachManagedGitHubRepositoryForResume(t, repo)
			session := repo.sessions["sess-1"]
			session.State = models.TaskSessionStateCancelled
			session.Metadata = map[string]interface{}{
				models.SessionMetaKeyGitCredentialSnapshot: models.GitCredentialSnapshot{
					Version: 1, Policy: taskGitCredentialsModeManaged, Source: "workspace", Transport: "managed_https",
				},
			}
			var launchedEnv map[string]string
			agentMgr := &mockAgentManager{launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
				launchedEnv = req.Env
				return &LaunchAgentResponse{AgentExecutionID: "exec-fresh", Status: v1.AgentStatusStarting}, nil
			}}
			exec := newTestExecutor(t, agentMgr, repo)
			exec.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{
				policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeManaged},
			})
			if issuerAvailable {
				issuer := &fakeGitCredentialLeaseReissuer{
					fakeGitHubCredentialLeaseIssuer: fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "fresh-lease"}},
					capability:                      gitcredentials.ReissueCapability{Token: "fresh-capability"},
				}
				exec.SetGitHubCredentialBroker(issuer, "http://localhost:8080/api/v1/git/credentials/resolve")
			}

			_, err := exec.ResumeSession(t.Context(), session, true)
			if !issuerAvailable {
				require.ErrorContains(t, err, "managed GitHub credentials require the Git credential broker")
				require.Empty(t, launchedEnv)
				require.Equal(t, models.TaskSessionStateCancelled, repo.sessions[session.ID].State)
				return
			}
			require.NoError(t, err)
			require.Equal(t, session.ID, launchedEnv[githubauth.CredentialSessionIDEnv])
			require.Equal(t, "fresh-lease", launchedEnv[githubauth.CredentialLeaseEnv])
			require.Equal(t, "fresh-capability", launchedEnv[githubauth.CredentialReissueCapabilityEnv])
		})
	}
}
