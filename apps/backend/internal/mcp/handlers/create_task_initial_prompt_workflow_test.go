package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestHandleCreateTask_InitialPromptUsesRequestedWorkflowStepMode(t *testing.T) {
	for _, tc := range []struct {
		name              string
		explicitStep      bool
		wantIntent        string
		wantInitialPrompt bool
		wantDeferredStart bool
		wantRequestCount  int
	}{
		{
			name:              "explicit step admits turn before start",
			explicitStep:      true,
			wantIntent:        "start_created",
			wantInitialPrompt: true,
			wantDeferredStart: true,
			wantRequestCount:  2,
		},
		{
			name:             "omitted step uses destination auto start",
			explicitStep:     false,
			wantIntent:       "start",
			wantRequestCount: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			taskSvc, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
			workspace, workflow := defaultWorkspaceAndWorkflow(t, ctx, taskSvc)
			seedWorkflowStep(t, ctx, workflowRepo, &workflowmodels.WorkflowStep{
				ID:             "step-backlog-" + tc.name,
				WorkflowID:     workflow.ID,
				Name:           "Backlog",
				Position:       0,
				IsStartStep:    true,
				AgentProfileID: "profile-backlog",
			})
			destination := seedWorkflowStep(t, ctx, workflowRepo, &workflowmodels.WorkflowStep{
				ID:             "step-spec-" + tc.name,
				WorkflowID:     workflow.ID,
				Name:           "Spec",
				Position:       1,
				AgentProfileID: "profile-destination",
				Events: workflowmodels.StepEvents{
					OnEnter: []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterAutoStartAgent}},
				},
			})
			taskSvc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{
				"step-backlog-" + tc.name: {
					ID: "step-backlog-" + tc.name, WorkflowID: workflow.ID, Name: "Backlog", Position: 0,
					IsStartStep: true, AgentProfileID: "profile-backlog",
				},
				destination.ID: destination,
			}})

			source, err := taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
				WorkspaceID:    workspace.ID,
				WorkflowID:     workflow.ID,
				WorkflowStepID: "step-backlog-" + tc.name,
				Title:          "Source task",
			})
			require.NoError(t, err)
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID:                "source-session-" + tc.name,
				TaskID:            source.Task.ID,
				AgentProfileID:    "profile-source",
				ExecutorProfileID: "executor-source",
				State:             models.TaskSessionStateWaitingForInput,
				IsPrimary:         true,
				StartedAt:         time.Now().UTC(),
			}))

			launcher := newMockSessionLauncher()
			launcher.calls = make(chan struct{}, tc.wantRequestCount)
			h := NewHandlers(taskSvc, workflowCtrl, nil, nil, nil, repo, repo, nil, nil, nil, launcher, nil, testLogger(t))
			settings := &mcpUserSettingsProvider{settings: &usermodels.UserSettings{
				MCPTaskAgentProfileDefault: usermodels.MCPTaskAgentProfileDefaultCurrentTask,
			}}
			h.SetUserSettingsProvider(settings)

			payload := map[string]interface{}{
				"source_task_id":    source.Task.ID,
				"source_session_id": "source-session-" + tc.name,
				"workspace_id":      workspace.ID,
				"workflow_id":       workflow.ID,
				"title":             "Created task",
				"description":       "write the specification",
			}
			if tc.explicitStep {
				payload["workflow_step_id"] = destination.ID
			}

			resp, err := h.handleCreateTask(
				mcpTestKanbanContext(ctx, workspace.ID, source.Task.ID, "source-session-"+tc.name),
				makeWSMessage(t, ws.ActionMCPCreateTask, payload),
			)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type, "payload: %s", string(resp.Payload))

			for index := 0; index < tc.wantRequestCount; index++ {
				select {
				case <-launcher.calls:
				case <-time.After(2 * time.Second):
					t.Fatalf("LaunchSession call %d did not complete", index+1)
				}
			}
			launcher.mu.Lock()
			requests := append([]*orchestrator.LaunchSessionRequest(nil), launcher.requests...)
			launcher.mu.Unlock()
			require.Len(t, requests, tc.wantRequestCount)
			require.Equal(t, orchestrator.SessionIntent(tc.wantIntent), requests[len(requests)-1].Intent)
			require.Equal(t, tc.wantInitialPrompt, requests[len(requests)-1].InitialCreatePrompt)
			require.Equal(t, tc.wantDeferredStart, requests[0].DeferredStart)

			if tc.explicitStep {
				require.Equal(t, orchestrator.IntentPrepare, requests[0].Intent)
				require.True(t, requests[0].DeferredStart)
				require.Equal(t, destination.ID, requests[0].WorkflowStepID)
				require.Equal(t, orchestrator.IntentStartCreated, requests[1].Intent)
				require.Equal(t, "session-1", requests[1].SessionID)
				require.True(t, requests[1].InitialCreatePrompt)
				require.Equal(t, "write the specification", requests[1].Prompt)
			} else {
				require.Equal(t, orchestrator.IntentStart, requests[0].Intent)
				require.Equal(t, destination.ID, requests[0].WorkflowStepID)
				require.False(t, requests[0].InitialCreatePrompt)
				require.Equal(t, "write the specification", requests[0].Prompt)
			}
			for _, request := range requests {
				require.Equal(t, "profile-destination", request.AgentProfileID,
					"the destination step profile must override source task settings")
				require.Equal(t, "executor-source", request.ExecutorProfileID,
					"the verified source session must supply the inherited executor setting")
			}
			require.GreaterOrEqual(t, settings.calls, 1, "MCP user settings must participate in profile resolution")

			var result struct {
				ID             string `json:"id"`
				WorkflowStepID string `json:"workflow_step_id"`
			}
			require.NoError(t, json.Unmarshal(resp.Payload, &result))
			require.Equal(t, destination.ID, result.WorkflowStepID)
			rows := ledgerRowsForTask(t, repo, result.ID)
			require.Equal(t, "source-session-"+tc.name, *rows[0].actorID,
				"the original MCP request session must remain the creation actor")
		})
	}
}
