package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type workflowOverrideBoundaryRepo struct {
	mockRepository
	workspace       *models.Workspace
	workflow        *models.Workflow
	createdTasks    []*models.Task
	createdSessions []*models.TaskSession
}

func (r *workflowOverrideBoundaryRepo) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return r.workspace, nil
}

func (r *workflowOverrideBoundaryRepo) GetWorkflow(context.Context, string) (*models.Workflow, error) {
	return r.workflow, nil
}

func (r *workflowOverrideBoundaryRepo) CreateTask(_ context.Context, task *models.Task) error {
	r.createdTasks = append(r.createdTasks, task)
	return nil
}

func (r *workflowOverrideBoundaryRepo) CreateTaskSession(_ context.Context, session *models.TaskSession) error {
	r.createdSessions = append(r.createdSessions, session)
	return nil
}

type workflowOverrideBoundaryValidator struct {
	err   error
	calls []string
}

func (v *workflowOverrideBoundaryValidator) ValidateAgentProfileForExecutor(
	_ context.Context,
	profile *settingsmodels.AgentProfile,
	_ *models.Executor,
	_ *models.ExecutorProfile,
) error {
	v.calls = append(v.calls, profile.ID)
	return v.err
}

type workflowOverrideBoundarySteps struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (g workflowOverrideBoundarySteps) GetStep(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
	step, ok := g.steps[id]
	if !ok {
		return nil, errors.New("workflow step not found")
	}
	return step, nil
}

func (g workflowOverrideBoundarySteps) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (g workflowOverrideBoundarySteps) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error) {
	steps := make([]*wfmodels.WorkflowStep, 0, len(g.steps))
	for _, step := range g.steps {
		if step.WorkflowID == workflowID {
			steps = append(steps, step)
		}
	}
	return steps, nil
}

func newWorkflowOverrideBoundaryHandlers(t *testing.T) (*TaskHandlers, *workflowOverrideBoundaryRepo, *workflowOverrideBoundaryValidator) {
	t.Helper()
	workspaceID := "workspace-override-boundary"
	workflowID := "workflow-override-boundary"
	executorID := "executor-override-boundary"
	defaultExecutorID := executorID
	repo := &workflowOverrideBoundaryRepo{
		mockRepository: mockRepository{
			executors: map[string]*models.Executor{
				executorID: {
					ID:        executorID,
					Name:      "Boundary executor",
					Type:      models.ExecutorTypeSSH,
					Status:    models.ExecutorStatusActive,
					Resumable: true,
				},
			},
		},
		workspace: &models.Workspace{
			ID:                workspaceID,
			Name:              "Boundary workspace",
			DefaultExecutorID: &defaultExecutorID,
		},
		workflow: &models.Workflow{
			ID:          workflowID,
			WorkspaceID: workspaceID,
			Name:        "Boundary workflow",
		},
	}
	validator := &workflowOverrideBoundaryValidator{err: errors.New("replacement profile is incompatible with the executor")}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
		AgentProfiles: serviceAgentProfileReader{profile: &settingsmodels.AgentProfile{
			ID:          "replacement-profile",
			Enabled:     true,
			WorkspaceID: workspaceID,
		}},
		AgentProfileExecutorValidator: validator,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(workflowOverrideBoundarySteps{steps: map[string]*wfmodels.WorkflowStep{
		"implement": {
			ID:             "implement",
			WorkflowID:     workflowID,
			AgentProfileID: "source-profile",
		},
	}})
	return &TaskHandlers{service: svc, logger: log}, repo, validator
}

// serviceAgentProfileReader is kept separate from the boundary repository so
// the handler tests exercise the same injected service contract as production.
type serviceAgentProfileReader struct {
	profile *settingsmodels.AgentProfile
}

func (r serviceAgentProfileReader) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return r.profile, nil
}

func TestHTTPCreateTaskRejectsIncompatibleWorkflowOverrideBeforeWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo, validator := newWorkflowOverrideBoundaryHandlers(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks", jsonBody(t, map[string]any{
		"workspace_id":             repo.workspace.ID,
		"workflow_id":              repo.workflow.ID,
		"title":                    "Rejected override",
		"executor_id":              "executor-override-boundary",
		"workflow_agent_overrides": map[string]string{"source-profile": "replacement-profile"},
	}))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpCreateTask(c)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "incompatible")
	require.Len(t, validator.calls, 1)
	require.Empty(t, repo.createdTasks)
	require.Empty(t, repo.createdSessions)
}

func TestWSCreateTaskRejectsIncompatibleWorkflowOverrideBeforeWrites(t *testing.T) {
	h, repo, validator := newWorkflowOverrideBoundaryHandlers(t)
	response, err := h.wsCreateTask(context.Background(), wsWorkflowRequest(t, ws.ActionTaskCreate, map[string]any{
		"workspace_id":             repo.workspace.ID,
		"workflow_id":              repo.workflow.ID,
		"title":                    "Rejected override",
		"executor_id":              "executor-override-boundary",
		"workflow_agent_overrides": map[string]string{"source-profile": "replacement-profile"},
	}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, response)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Contains(t, payload.Message, "incompatible")
	require.Len(t, validator.calls, 1)
	require.Empty(t, repo.createdTasks)
	require.Empty(t, repo.createdSessions)
}

func jsonBody(t *testing.T, value any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return strings.NewReader(string(data))
}
