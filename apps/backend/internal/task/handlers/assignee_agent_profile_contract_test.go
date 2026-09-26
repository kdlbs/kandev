package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// assigneeContractProfileReader is a fixed lookup table standing in for the
// agent-settings store: production's AgentProfileReader is satisfied by any
// type with this one method, so the HTTP contract test does not need the
// real settings-store package to exercise the new validation.
type assigneeContractProfileReader struct {
	profiles map[string]*settingsmodels.AgentProfile
}

func (r assigneeContractProfileReader) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	return r.profiles[id], nil
}

type assigneeContractStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (g assigneeContractStepGetter) GetStep(_ context.Context, id string) (*wfmodels.WorkflowStep, error) {
	step, ok := g.steps[id]
	if !ok {
		return nil, taskrepo.ErrTaskNotFound
	}
	return step, nil
}

func (g assigneeContractStepGetter) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

// assigneeContractFixture is the real-sqlite-backed harness this contract
// needs: the assignee_agent_profile_id column task.go exposes is a computed
// projection over workflow_step_participants (runnerProjection), not a
// stored column, so only a real repository's GetTask/ListTasksByWorkflowStep
// can prove the runner seat actually landed. A mock repository would just
// echo back whatever struct field was set, which is exactly the kind of
// false-positive this regression test must not produce.
type assigneeContractFixture struct {
	handlers    *TaskHandlers
	repo        *taskrepo.Repository
	workspaceID string
	workflowID  string
	stepID      string
}

func newAssigneeContractFixture(t *testing.T, profiles map[string]*settingsmodels.AgentProfile) assigneeContractFixture {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	workspaceID := "ws-assignee-contract"
	workflowID := "wf-assignee-contract"
	stepID := "step-assignee-contract"

	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Workflow"}))
	now := time.Now().UTC()
	_, err = repo.DB().Exec(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stepID, workflowID, stepID, 0, now, now)
	require.NoError(t, err)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews:       repo,
		AgentProfiles: assigneeContractProfileReader{profiles: profiles},
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(assigneeContractStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepID: {ID: stepID, WorkflowID: workflowID, Name: stepID, Position: 0},
	}})
	return assigneeContractFixture{
		handlers:    &TaskHandlers{service: svc, logger: log},
		repo:        repo,
		workspaceID: workspaceID,
		workflowID:  workflowID,
		stepID:      stepID,
	}
}

func (f assigneeContractFixture) createTaskBody(title, assigneeAgentProfileID, externalID string) string {
	body := map[string]any{
		"workspace_id":     f.workspaceID,
		"workflow_id":      f.workflowID,
		"workflow_step_id": f.stepID,
		"title":            title,
	}
	if assigneeAgentProfileID != "" {
		body["assignee_agent_profile_id"] = assigneeAgentProfileID
	}
	if externalID != "" {
		body["external_id"] = externalID
	}
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// TestBetaOfficeCreateContract pins ISSUE-7: the New Task dialog's assignee
// pick must reach the backend as a top-level, validated field rather than
// being silently dropped inside metadata. Before this fix,
// httpCreateTaskRequest had no assignee_agent_profile_id field at all, so
// every case below either 500s on an unknown field (n/a — JSON silently
// drops it) or, worse, would let an arbitrary/foreign-workspace string
// through untouched once the field existed but was unvalidated.
func TestBetaOfficeCreateContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	validProfile := &settingsmodels.AgentProfile{ID: "agent-office-1", Enabled: true, WorkspaceID: "ws-assignee-contract"}
	foreignProfile := &settingsmodels.AgentProfile{ID: "agent-foreign", Enabled: true, WorkspaceID: "ws-some-other-workspace"}
	disabledProfile := &settingsmodels.AgentProfile{ID: "agent-disabled", Enabled: false, WorkspaceID: "ws-assignee-contract"}
	globalProfile := &settingsmodels.AgentProfile{ID: "agent-global", Enabled: true, WorkspaceID: ""}
	profiles := map[string]*settingsmodels.AgentProfile{
		validProfile.ID:    validProfile,
		foreignProfile.ID:  foreignProfile,
		disabledProfile.ID: disabledProfile,
		globalProfile.ID:   globalProfile,
	}

	t.Run("valid office assignee seats the runner", func(t *testing.T) {
		f := newAssigneeContractFixture(t, profiles)
		rec := doCreateTask(f.handlers, f.createTaskBody("Assigned", validProfile.ID, ""))
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		taskID, _ := resp["id"].(string)
		require.NotEmpty(t, taskID)

		stored, err := f.repo.GetTask(context.Background(), taskID)
		require.NoError(t, err)
		assert.Equal(t, validProfile.ID, stored.AssigneeAgentProfileID,
			"the runner participant row must exist before any assignment wake can fire")
	})

	t.Run("nonexistent profile is rejected before any task row is written", func(t *testing.T) {
		f := newAssigneeContractFixture(t, profiles)
		rec := doCreateTask(f.handlers, f.createTaskBody("Bad assignee", "does-not-exist", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

		tasks, err := f.repo.ListTasksByWorkflowStep(context.Background(), f.stepID)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("profile scoped to another workspace is rejected", func(t *testing.T) {
		f := newAssigneeContractFixture(t, profiles)
		rec := doCreateTask(f.handlers, f.createTaskBody("Foreign assignee", foreignProfile.ID, ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

		tasks, err := f.repo.ListTasksByWorkflowStep(context.Background(), f.stepID)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("disabled profile is rejected", func(t *testing.T) {
		f := newAssigneeContractFixture(t, profiles)
		rec := doCreateTask(f.handlers, f.createTaskBody("Disabled assignee", disabledProfile.ID, ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())

		tasks, err := f.repo.ListTasksByWorkflowStep(context.Background(), f.stepID)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("global (kanban-legacy) profile without a workspace is rejected", func(t *testing.T) {
		// Office eligibility per selectOfficeAgentProfiles/ListAgentInstances
		// is a non-empty workspace_id match, not "empty or matching" — a
		// global profile is not something the New Task dialog ever offers as
		// an assignee, so it must not validate as one either.
		f := newAssigneeContractFixture(t, profiles)
		rec := doCreateTask(f.handlers, f.createTaskBody("Global assignee", globalProfile.ID, ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("duplicate external_id short-circuits before re-validating the assignee", func(t *testing.T) {
		f := newAssigneeContractFixture(t, profiles)
		first := doCreateTask(f.handlers, f.createTaskBody("First", validProfile.ID, "ext-assignee-1"))
		require.Equal(t, http.StatusOK, first.Code, "body: %s", first.Body.String())
		var firstResp map[string]interface{}
		require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))

		// The retry names a profile that would fail validation on its own,
		// proving the Found-outcome path returns the existing task rather
		// than re-running create-time validation.
		retry := doCreateTask(f.handlers, f.createTaskBody("Retry", "does-not-exist", "ext-assignee-1"))
		require.Equal(t, http.StatusOK, retry.Code, "body: %s", retry.Body.String())
		var retryResp map[string]interface{}
		require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryResp))
		assert.Equal(t, firstResp["id"], retryResp["id"])
		assert.Equal(t, true, retryResp["deduplicated"])

		tasks, err := f.repo.ListTasksByWorkflowStep(context.Background(), f.stepID)
		require.NoError(t, err)
		assert.Len(t, tasks, 1, "no duplicate task should exist")
	})

	t.Run("no assignee is unaffected", func(t *testing.T) {
		f := newAssigneeContractFixture(t, profiles)
		rec := doCreateTask(f.handlers, f.createTaskBody("No assignee", "", ""))
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		taskID, _ := resp["id"].(string)
		stored, err := f.repo.GetTask(context.Background(), taskID)
		require.NoError(t, err)
		assert.Empty(t, stored.AssigneeAgentProfileID)
	})
}
