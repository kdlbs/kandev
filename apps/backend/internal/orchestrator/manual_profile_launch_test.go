package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestLaunchSessionExplicitProfilePrecedence(t *testing.T) {
	tests := []struct {
		name          string
		requestJSON   string
		wantProfileID string
	}{
		{
			name:          "explicit profile bypasses workflow step",
			requestJSON:   `{"task_id":"task-1","intent":"start","agent_profile_id":"profile-b","profile_explicit":true}`,
			wantProfileID: "profile-b",
		},
		{
			name:          "omitted explicit flag keeps workflow step",
			requestJSON:   `{"task_id":"task-1","intent":"start","agent_profile_id":"profile-b"}`,
			wantProfileID: "profile-a",
		},
		{
			name:          "explicit empty profile keeps workflow step",
			requestJSON:   `{"task_id":"task-1","intent":"start","agent_profile_id":"","profile_explicit":true}`,
			wantProfileID: "profile-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSessionWithStep(t, repo, "task-1", "existing-session", "step-a")

			stepGetter := newMockStepGetter()
			stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
				ID:             "step-a",
				WorkflowID:     "wf1",
				AgentProfileID: "profile-a",
			}
			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task-1"] = &v1.Task{
				ID:          "task-1",
				WorkspaceID: "ws1",
				Title:       "Task",
				Description: "Do the work",
				State:       v1.TaskStateInProgress,
			}

			var launchedRequest *executor.LaunchAgentRequest
			agentManager := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					requestCopy := *req
					launchedRequest = &requestCopy
					return &executor.LaunchAgentResponse{
						AgentExecutionID: "execution-1",
						Status:           v1.AgentStatusStarting,
					}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)

			var request LaunchSessionRequest
			if err := json.Unmarshal([]byte(tt.requestJSON), &request); err != nil {
				t.Fatalf("decode launch request: %v", err)
			}

			response, err := svc.LaunchSession(ctx, &request)
			if err != nil {
				t.Fatalf("LaunchSession returned error: %v", err)
			}
			if response.AgentProfileID != tt.wantProfileID {
				t.Fatalf("response agent_profile_id = %q, want %q", response.AgentProfileID, tt.wantProfileID)
			}
			if response.AgentExecutionID != "execution-1" {
				t.Fatalf("response agent_execution_id = %q, want execution-1", response.AgentExecutionID)
			}

			storedSession, err := repo.GetTaskSession(ctx, response.SessionID)
			if err != nil {
				t.Fatalf("load launched session: %v", err)
			}
			if storedSession.AgentProfileID != tt.wantProfileID {
				t.Fatalf("stored session agent_profile_id = %q, want %q", storedSession.AgentProfileID, tt.wantProfileID)
			}
			if launchedRequest == nil {
				t.Fatal("agent launch request was not captured")
			}
			if launchedRequest.AgentProfileID != tt.wantProfileID {
				t.Fatalf("agent launch agent_profile_id = %q, want %q", launchedRequest.AgentProfileID, tt.wantProfileID)
			}
		})
	}
}
