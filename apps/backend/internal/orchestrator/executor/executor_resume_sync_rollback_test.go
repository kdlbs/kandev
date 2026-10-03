package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestResumeSessionSynchronousStartFailureRespectsAttemptIdentity(t *testing.T) {
	for _, successor := range []bool{false, true} {
		name := "owned-attempt"
		if successor {
			name = "successor-attempt"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			setupLiveResumeTestFixture(repo)
			startErr := errors.New("synchronous start failed")
			agentManager := &mockAgentManager{
				launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
					return &LaunchAgentResponse{
						AgentExecutionID: "exec-resumed",
						WorkspacePath:    "/tasks/task-1/materialized",
						Status:           v1.AgentStatusStarting,
					}, nil
				},
				startAgentProcessFunc: func(context.Context, string) error {
					if successor {
						repo.sessions["sess-1"].Metadata[models.SessionMetaKeyAgentStartAttemptID] = "successor"
					}
					return startErr
				},
			}
			exec := newTestExecutor(t, agentManager, repo)
			_, err := exec.ResumeSessionWithOptions(context.Background(), repo.sessions["sess-1"], true,
				ResumeOptions{StartAgentSynchronously: true})
			if !errors.Is(err, startErr) {
				t.Fatalf("ResumeSessionWithOptions error = %v, want %v", err, startErr)
			}
			want := models.TaskSessionStateWaitingForInput
			if successor {
				want = models.TaskSessionStateStarting
				if got := repo.sessions["sess-1"].Metadata[models.SessionMetaKeyAgentStartAttemptID]; got != "successor" {
					t.Fatalf("attempt identity = %v, want successor", got)
				}
			}
			if got := repo.sessions["sess-1"].State; got != want {
				t.Fatalf("session state = %s, want %s", got, want)
			}
		})
	}
}
