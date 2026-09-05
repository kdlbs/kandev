package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// routingStepResolver reports which destination rule the create request asked
// for, so the transport mapping can be asserted without a workflow database.
type routingStepResolver struct{}

func (routingStepResolver) ResolveStartStep(context.Context, string) (string, error) {
	return "start-step", nil
}

func (routingStepResolver) ResolveFirstStep(context.Context, string) (string, error) {
	return "first-step", nil
}

func (routingStepResolver) ResolveAutoStartStep(context.Context, string) (string, error) {
	return "auto-start-step", nil
}

// The service-level routing tests exercise resolveWorkflowStep directly, which
// says nothing about whether a transport actually carries `start_agent` into
// CreateTaskRequest. Each transport maps the field by hand, so each one can drop
// it independently — and a dropped flag is invisible on the built-in templates,
// where the start step and the auto-start step are the same step. The resolved
// step ID on the persisted task is the observable proof the flag arrived.

func TestHTTPCreateTask_StartAgentSelectsDestinationStep(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for name, tc := range map[string]struct {
		startAgent bool
		planMode   bool
		wantStep   string
	}{
		"start_agent=true routes to the auto-start step": {startAgent: true, wantStep: "auto-start-step"},
		"start_agent=false routes to the start step":     {startAgent: false, wantStep: "start-step"},
		// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.1
		"start_agent=true with plan_mode routes to the auto-start step": {
			startAgent: true, planMode: true, wantStep: "auto-start-step",
		},
	} {
		t.Run(name, func(t *testing.T) {
			log := newTestLogger(t)
			repo := &captureCreateTaskRepo{}
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo,
			}, nil, log, service.RepositoryDiscoveryConfig{})
			svc.SetWorkflowStepGetter(repo)
			svc.SetStartStepResolver(routingStepResolver{})
			h := &TaskHandlers{service: svc, orchestrator: &captureOrchestrator{}, logger: log}

			body := `{
				"workspace_id": "ws-1",
				"workflow_id": "wf-1",
				"title": "Routed by intent",
				"description": "do the thing",
				"agent_profile_id": "profile-1",
				"start_agent": ` + boolLiteral(tc.startAgent) + `,
				"plan_mode": ` + boolLiteral(tc.planMode) + `
			}`
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			h.httpCreateTask(c)

			require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
			require.NotNil(t, repo.captured, "CreateTask must persist the task")
			assert.Equal(t, tc.wantStep, repo.captured.WorkflowStepID)
		})
	}
}

func TestWSCreateTask_StartAgentSelectsDestinationStep(t *testing.T) {
	for name, tc := range map[string]struct {
		payload  map[string]any
		wantStep string
	}{
		"start_agent=true routes to the auto-start step": {
			payload: map[string]any{
				"workspace_id": "ws-b", "workflow_id": "wf-b", "title": "Routed by intent",
				"agent_profile_id": "profile-1", "start_agent": true,
			},
			wantStep: "auto-start-step",
		},
		"start_agent=false routes to the start step": {
			payload: map[string]any{
				"workspace_id": "ws-b", "workflow_id": "wf-b", "title": "Routed by intent",
				"agent_profile_id": "profile-1", "start_agent": false,
			},
			wantStep: "start-step",
		},
		// @covers AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.1
		"start_agent=true with plan_mode routes to the auto-start step": {
			payload: map[string]any{
				"workspace_id": "ws-b", "workflow_id": "wf-b", "title": "Routed by intent",
				"agent_profile_id": "profile-1", "start_agent": true, "plan_mode": true,
			},
			wantStep: "auto-start-step",
		},
		"omitted start_agent routes to the start step": {
			payload: map[string]any{
				"workspace_id": "ws-b", "workflow_id": "wf-b", "title": "Routed by intent",
			},
			wantStep: "start-step",
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &wsTaskRepo{}
			h := newWSTaskHandlers(t, repo)
			h.service.SetStartStepResolver(routingStepResolver{})

			resp, err := h.wsCreateTask(context.Background(),
				wsWorkflowRequest(t, ws.ActionTaskCreate, tc.payload))

			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, resp.Type, "body: %s", resp.Payload)
			require.Len(t, repo.created, 1, "task.create must persist the task")
			assert.Equal(t, tc.wantStep, repo.created[0].WorkflowStepID)
		})
	}
}

func boolLiteral(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
