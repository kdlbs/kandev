package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestOfficeTaskRunExecutionLive(t *testing.T) {
	cases := []struct {
		name         string
		state        models.TaskSessionState
		agentRunning bool
		sessionID    string
		taskID       string
		agentID      string
		want         bool
	}{
		{name: "starting session", state: models.TaskSessionStateStarting, sessionID: "s1", taskID: "t1", agentID: "agent-a", want: true},
		{name: "running session with live agent", state: models.TaskSessionStateRunning, agentRunning: true, sessionID: "s1", taskID: "t1", agentID: "agent-a", want: true},
		{name: "running session without agent", state: models.TaskSessionStateRunning, sessionID: "s1", taskID: "t1", agentID: "agent-a"},
		{name: "waiting for input", state: models.TaskSessionStateWaitingForInput, agentRunning: true, sessionID: "s1", taskID: "t1", agentID: "agent-a"},
		{name: "completed", state: models.TaskSessionStateCompleted, agentRunning: true, sessionID: "s1", taskID: "t1", agentID: "agent-a"},
		{name: "task and agent lookup", state: models.TaskSessionStateRunning, agentRunning: true, taskID: "t1", agentID: "agent-a", want: true},
		{name: "session of another agent", state: models.TaskSessionStateRunning, agentRunning: true, sessionID: "s1", taskID: "t1", agentID: "agent-b"},
		{name: "session of another task", state: models.TaskSessionStateRunning, agentRunning: true, sessionID: "s1", taskID: "t2", agentID: "agent-a"},
		{name: "missing session", state: models.TaskSessionStateRunning, agentRunning: true, sessionID: "missing", taskID: "t1", agentID: "agent-a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedTaskAndOfficeSession(t, repo, "t1", "s1", "agent-a", tc.state)
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(),
				&mockAgentManager{isAgentRunning: tc.agentRunning})

			got, err := svc.OfficeTaskRunExecutionLive(context.Background(), tc.taskID, tc.agentID, tc.sessionID)
			if err != nil {
				t.Fatalf("OfficeTaskRunExecutionLive: %v", err)
			}
			if got != tc.want {
				t.Errorf("live = %v, want %v", got, tc.want)
			}
		})
	}
}
