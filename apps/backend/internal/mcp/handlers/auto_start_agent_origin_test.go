package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// Every launch request an MCP-created task's auto-start sends is marked as
// agent-requested, so the session ceiling admits it as automatic.
func TestLaunchAutoStartTask_MarksLaunchRequestsAgentOrigin(t *testing.T) {
	for _, initialCreatePrompt := range []bool{false, true} {
		t.Run(map[bool]string{false: "start", true: "initial create prompt"}[initialCreatePrompt], func(t *testing.T) {
			launcher := newMockSessionLauncher()
			launcher.calls = make(chan struct{}, 2)
			h := &Handlers{sessionLauncher: launcher, logger: testLogger(t)}

			h.launchAutoStartTask(context.Background(), &models.Task{
				ID: "task-agent-origin", WorkflowStepID: "step-1", Description: "initial request",
			}, mcpAutoStartConfig{
				AgentProfileID:      "agent-profile-1",
				ExecutorID:          models.ExecutorIDWorktree,
				InitialCreatePrompt: initialCreatePrompt,
			})

			want := 1
			if initialCreatePrompt {
				want = 2
			}
			for i := 0; i < want; i++ {
				select {
				case <-launcher.calls:
				case <-time.After(2 * time.Second):
					t.Fatalf("LaunchSession call %d did not complete", i+1)
				}
			}
			launcher.mu.Lock()
			requests := append([]*orchestrator.LaunchSessionRequest(nil), launcher.requests...)
			launcher.mu.Unlock()
			require.Len(t, requests, want)
			for _, req := range requests {
				require.True(t, req.Automatic, "intent %s must be marked agent-requested", req.Intent)
			}
		})
	}
}
