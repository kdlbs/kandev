package executor

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.10
func TestResumeSessionCleanupFailure(t *testing.T) {
	for _, terminal := range []bool{true, false} {
		name := "stale-fallback"
		if terminal {
			name = "terminal-preflight"
		}
		for _, cleanupFails := range []bool{true, false} {
			outcome := "success"
			if cleanupFails {
				outcome = "failure"
			}
			t.Run(name+"/"+outcome, func(t *testing.T) {
				repo := newMockRepository()
				setupLiveResumeTestFixture(repo)
				initialState := repo.sessions["sess-1"].State
				if terminal {
					initialState = models.TaskSessionStateFailed
					repo.sessions["sess-1"].State = initialState
				}
				cleanupErr := errors.New("kubelet 192.0.2.10:10250 unreachable; private-path /var/secret")
				launchCalls := 0
				manager := &mockAgentManager{
					cleanupStaleExecutionFunc: func(context.Context, string) error {
						if cleanupFails {
							return cleanupErr
						}
						return nil
					},
					launchAgentFunc: func(_ context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
						launchCalls++
						if !terminal && launchCalls == 1 {
							return nil, lifecycle.ErrAgentAlreadyRunning
						}
						require.Equal(t, "token-abc", req.ACPSessionID)
						return &LaunchAgentResponse{AgentExecutionID: "exec-new", Status: v1.AgentStatusStarting}, nil
					},
				}
				exec := newTestExecutor(t, manager, repo)
				_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
				if !cleanupFails {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, cleanupErr)
				wantLaunchCalls := 1
				if terminal {
					wantLaunchCalls = 0
				}
				require.Equal(t, wantLaunchCalls, launchCalls)
				require.Equal(t, initialState, repo.sessions["sess-1"].State)
				require.Equal(t, "token-abc", repo.executorsRunning["sess-1"].ResumeToken)
				var failure *agentruntime.BootstrapFailure
				require.ErrorAs(t, err, &failure)
				require.Equal(t, "runtime_cleanup_failed", failure.SafeCode())
				require.Equal(t, models.AgentErrorCauseOperationResume, failure.Operation)
				require.NotContains(t, failure.SafeDetail(), "192.0.2.")
				require.Equal(t, true, *failure.PromptNotSent)
			})
		}
	}
}
