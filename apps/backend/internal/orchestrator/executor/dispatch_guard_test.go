package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAssistantContextGuardCoversNativeDispatch(t *testing.T) {
	for _, path := range []string{"launch", "resume", "prompt", "steer", "pty"} {
		t.Run(path, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			manager := &mockAgentManager{isPassthroughSessionFunc: func(context.Context, string) bool { return path == "pty" }}
			manager.getExecutionIDForSessionFunc = func(context.Context, string) (string, error) { return "execution", nil }
			e := newTestExecutor(t, manager, repo)
			denied := errors.New("stale assistant context")
			e.SetDispatchGuard(func(_ context.Context, task *models.Task, session *models.TaskSession, profile string) error {
				require.Equal(t, "task-1", task.ID)
				require.Equal(t, "sess-1", session.ID)
				require.Equal(t, "profile-1", profile)
				return denied
			})
			var err error
			switch path {
			case "launch":
				_, err = e.LaunchPreparedSession(context.Background(), &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, "sess-1", LaunchOptions{AgentProfileID: "profile-1", StartAgent: true})
			case "resume":
				_, err = e.ResumeSession(context.Background(), repo.sessions["sess-1"], true)
			case "steer":
				_, err = e.SteerWithDispatchCallback(context.Background(), "task-1", "sess-1", "old", nil, true, func() {})
			default:
				_, err = e.Prompt(context.Background(), "task-1", "sess-1", "old", nil, true)
			}
			require.ErrorIs(t, err, denied)
			require.Zero(t, manager.launchAgentCallCount)
			require.Zero(t, manager.promptAgentCallCount)
			require.Empty(t, manager.writePassthroughStdinCalls)
		})
	}
}
