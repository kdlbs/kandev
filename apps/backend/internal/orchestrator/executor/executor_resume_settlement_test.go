package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestResumeSettlesCleanedExecutionBeforeSuccessorLaunch(t *testing.T) {
	for _, terminal := range []bool{true, false} {
		for _, cleanupFails := range []bool{false, true} {
			name := "stale-fallback"
			if terminal {
				name = "terminal-preflight"
			}
			if cleanupFails {
				name += "/cleanup-failed"
			}
			t.Run(name, func(t *testing.T) {
				repo := newMockRepository()
				setupLiveResumeTestFixture(repo)
				if terminal {
					repo.sessions["sess-1"].State = models.TaskSessionStateCancelled
				}
				var calls []string
				manager := &mockAgentManager{
					cleanupStaleExecutionFunc: func(context.Context, string) error {
						calls = append(calls, "cleanup")
						if cleanupFails {
							return errors.New("runtime stop unconfirmed")
						}
						return nil
					},
					launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
						calls = append(calls, "launch")
						if !terminal && len(calls) == 1 {
							return nil, lifecycle.ErrAgentAlreadyRunning
						}
						return &LaunchAgentResponse{AgentExecutionID: "exec-new", Status: v1.AgentStatusStarting}, nil
					},
				}
				exec := newTestExecutor(t, manager, repo)
				exec.SetOnStaleExecutionCleanup(func(_ context.Context, sessionID string) {
					require.Equal(t, "sess-1", sessionID)
					calls = append(calls, "settle")
				})
				_, err := exec.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
				want := []string{"cleanup", "settle", "launch"}
				if cleanupFails {
					require.Error(t, err)
					want = []string{"cleanup"}
				} else {
					require.NoError(t, err)
				}
				if !terminal {
					want = append([]string{"launch"}, want...)
				}
				require.Equal(t, want, calls)
			})
		}
	}
}
