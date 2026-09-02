package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/db"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Wire-payload field name constants, extracted so repeated literals across
// this file's many payload-building call sites don't trip goconst.
const (
	handoffFieldTargetWorkspaceID = "target_workspace_id"
	handoffFieldWorkflowID        = "workflow_id"
	handoffFieldTitle             = "title"
	handoffFieldPrompt            = "prompt"
	handoffFieldAgentProfileID    = "agent_profile_id"
	handoffFieldExecutorProfileID = "executor_profile_id"
	handoffFieldRepositoryID      = "repository_id"
	handoffFieldBaseBranch        = "base_branch"
	handoffFieldStartAgent        = "start_agent"
	handoffFieldExternalID        = "external_id"
)

// --- Fixture ---

// handoffFixture wires a real task service/repo, a real workflow controller,
// and a real SQLite-backed agent settings controller, then seeds a source
// workspace/task/session (the caller) and a target workspace/workflow/
// step/agent-profile/executor-profile (the handoff destination). Individual
// tests layer additional seeding (a second source session, a mis-scoped
// profile, a corrupted handoffs blob, ...) on top.
type handoffFixture struct {
	svc          *service.Service
	repo         *sqliterepo.Repository
	workflowRepo *workflowrepo.Repository
	agentStore   agentsettingsstore.Repository
	h            *Handlers

	sourceWorkspaceID    string
	targetWorkspaceID    string
	sourceTaskID         string
	sourceSessionID      string
	callerAgentProfileID string
	targetWorkflowID     string
	targetStepID         string
	targetAgentProfileID string
	targetExecProfileID  string
	targetRepositoryID   string
}

func newHandoffAgentSettingsController(t *testing.T) (agentsettingsstore.Repository, *agentsettingscontroller.Controller) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "agent-settings.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := agentsettingsstore.Provide(sqlxDB, sqlxDB, testLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })
	ctrl := agentsettingscontroller.NewController(repo, nil, nil, nil, testLogger(t))
	return repo, ctrl
}

func seedHandoffAgentProfile(
	t *testing.T, repo agentsettingsstore.Repository, id string, role agentsettingsmodels.AgentRole, workspaceID, permissions string,
) {
	t.Helper()
	ctx := context.Background()
	agent := &agentsettingsmodels.Agent{ID: id + "-agent", Name: id + "-agent"}
	require.NoError(t, repo.CreateAgent(ctx, agent))
	profile := &agentsettingsmodels.AgentProfile{
		ID:               id,
		AgentID:          agent.ID,
		Name:             id,
		AgentDisplayName: id,
		Role:             role,
		WorkspaceID:      workspaceID,
		Permissions:      permissions,
	}
	require.NoError(t, repo.CreateAgentProfile(ctx, profile))
}

func seedHandoffWorkspace(t *testing.T, repo *sqliterepo.Repository, id string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: id, Name: id, CreatedAt: now, UpdatedAt: now,
	}))
}

func seedHandoffSourceTaskAndSession(t *testing.T, repo *sqliterepo.Repository, workspaceID, taskID, sessionID, agentProfileID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: "Source Task",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: agentProfileID,
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
}

func seedHandoffExecutorProfile(t *testing.T, repo *sqliterepo.Repository, id string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID: "exec-" + id, Name: "exec-" + id, Type: models.ExecutorTypeLocal,
	}))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: id, ExecutorID: "exec-" + id, Name: id,
	}))
}

// newHandoffFixture builds the standard two-workspace scenario. callerRole
// and callerPermissions control the caller (source) agent profile's
// can_handoff_tasks resolution; launcher may be nil when the test doesn't
// exercise start_agent.
func newHandoffFixture(t *testing.T, callerRole agentsettingsmodels.AgentRole, callerPermissions string, launcher SessionLauncher) *handoffFixture {
	t.Helper()
	ctx := context.Background()
	svc, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
	agentStore, agentCtrl := newHandoffAgentSettingsController(t)

	f := &handoffFixture{
		svc:                  svc,
		repo:                 repo,
		workflowRepo:         workflowRepo,
		agentStore:           agentStore,
		sourceWorkspaceID:    "ws-handoff-source",
		targetWorkspaceID:    "ws-handoff-target",
		sourceTaskID:         "task-handoff-source",
		sourceSessionID:      "session-handoff-source",
		callerAgentProfileID: "profile-caller",
		targetWorkflowID:     "wf-handoff-target",
		targetAgentProfileID: "profile-target-agent",
		targetExecProfileID:  "profile-target-executor",
		targetRepositoryID:   "repo-handoff-target",
	}

	seedHandoffWorkspace(t, repo, f.sourceWorkspaceID)
	seedHandoffWorkspace(t, repo, f.targetWorkspaceID)

	seedHandoffAgentProfile(t, agentStore, f.callerAgentProfileID, callerRole, "", callerPermissions)
	seedHandoffSourceTaskAndSession(t, repo, f.sourceWorkspaceID, f.sourceTaskID, f.sourceSessionID, f.callerAgentProfileID)

	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: f.targetWorkflowID, WorkspaceID: f.targetWorkspaceID, Name: "Delivery",
	}))
	step := seedWorkflowStep(t, ctx, workflowRepo, &workflowmodels.WorkflowStep{
		WorkflowID: f.targetWorkflowID, Name: "Start", Position: 1, IsStartStep: true, AllowManualMove: true,
	})
	f.targetStepID = step.ID
	// CreateTask validates the resolved step through a WorkflowStepGetter;
	// production wires this to the live workflow service, tests use the
	// static fake already defined in handlers_test.go.
	svc.SetWorkflowStepGetter(&staticWorkflowStepGetter{steps: map[string]*workflowmodels.WorkflowStep{step.ID: step}})

	seedHandoffAgentProfile(t, agentStore, f.targetAgentProfileID, agentsettingsmodels.AgentRoleWorker, f.targetWorkspaceID, "")
	seedHandoffExecutorProfile(t, repo, f.targetExecProfileID)
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: f.targetRepositoryID, WorkspaceID: f.targetWorkspaceID, Name: "delivery-repo",
		SourceType: "local", DefaultBranch: "main",
	}))

	h := NewHandlers(svc, workflowCtrl, nil, nil, nil, repo, repo, nil, nil, nil, launcher, nil, testLogger(t))
	h.SetConfigDeps(nil, agentCtrl, nil)
	f.h = h
	return f
}

// ctx attaches the source task's principal to a fresh background context.
func (f *handoffFixture) ctx() context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    f.sourceTaskID,
		CallerSessionID: f.sourceSessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	})
}

// validPayload returns a fully valid handoff_task_kandev payload with the
// given overrides layered on top.
func (f *handoffFixture) validPayload(overrides map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		handoffFieldTargetWorkspaceID: f.targetWorkspaceID,
		handoffFieldWorkflowID:        f.targetWorkflowID,
		handoffFieldTitle:             "Deliver the thing",
		handoffFieldPrompt:            "Do the work",
		handoffFieldAgentProfileID:    f.targetAgentProfileID,
		handoffFieldExecutorProfileID: f.targetExecProfileID,
	}
	for k, v := range overrides {
		payload[k] = v
	}
	return payload
}

// seedAdditionalSourceSession seeds a second, independent source task and
// session (with its own granted-permission agent profile) in the same
// source workspace, for AC-25a's cross-source-collision case.
func (f *handoffFixture) seedAdditionalSourceSession(t *testing.T, taskID, sessionID, agentProfileID string) context.Context {
	t.Helper()
	seedHandoffAgentProfile(t, f.agentStore, agentProfileID, agentsettingsmodels.AgentRoleCEO, "", "")
	seedHandoffSourceTaskAndSession(t, f.repo, f.sourceWorkspaceID, taskID, sessionID, agentProfileID)
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    taskID,
		CallerSessionID: sessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	})
}

func countTasksInWorkspace(t *testing.T, svc *service.Service, workspaceID string) int {
	t.Helper()
	tasks, _, err := svc.ListTasksByWorkspace(context.Background(), workspaceID, "", "", "", 1, 1000, "", true, true, false, false)
	require.NoError(t, err)
	return len(tasks)
}

func decodeHandoffResult(t *testing.T, resp *ws.Message) handoffTaskResult {
	t.Helper()
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "expected a success response, got: %s", string(resp.Payload))
	var result handoffTaskResult
	require.NoError(t, json.Unmarshal(resp.Payload, &result))
	return result
}

func assertWSErrorContains(t *testing.T, resp *ws.Message, substrs ...string) {
	t.Helper()
	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	for _, substr := range substrs {
		assert.Contains(t, ep.Message, substr)
	}
}

// handoffFakeLauncher wraps mockSessionLauncher so tests can inject a
// LaunchSession failure while keeping the rest of the SessionLauncher
// surface (unused by handoff_task_kandev) satisfied by the existing stub.
type handoffFakeLauncher struct {
	*mockSessionLauncher
	launchErr error
}

func newHandoffFakeLauncher(launchErr error) *handoffFakeLauncher {
	return &handoffFakeLauncher{mockSessionLauncher: newMockSessionLauncher(), launchErr: launchErr}
}

func (l *handoffFakeLauncher) LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	if l.launchErr != nil {
		return nil, l.launchErr
	}
	return l.mockSessionLauncher.LaunchSession(ctx, req)
}

// --- 1. Happy path ---

func TestHandleHandoffTask_HappyPathCreatesInDifferentWorkspace(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	ctx := f.ctx()

	resp, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
	assert.True(t, result.CreationComplete)
	assert.False(t, result.Started)
	assert.True(t, result.ReverseLinkRecorded)
	assert.Empty(t, result.ReverseLinkError)
	require.NotEmpty(t, result.HandedOffAt)
	_, parseErr := time.Parse(time.RFC3339, result.HandedOffAt)
	assert.NoError(t, parseErr, "handed_off_at must be RFC3339: %q", result.HandedOffAt)
	assert.Equal(t, f.targetWorkspaceID, result.WorkspaceID)

	deliveryTask, err := f.svc.GetTask(context.Background(), result.TaskID)
	require.NoError(t, err)
	src, ok := deliveryTask.Metadata[models.MetaKeyHandoffSource].(map[string]interface{})
	require.True(t, ok, "delivery task must carry handoff_source metadata")
	assert.Equal(t, f.sourceTaskID, src[handoffSourceTaskIDKey])
	assert.Equal(t, f.sourceWorkspaceID, src["source_workspace_id"])
	assert.Equal(t, f.sourceSessionID, src["source_session_id"])
	assert.Equal(t, f.callerAgentProfileID, src["source_agent_profile_id"])
	assert.NotEmpty(t, src[handoffHandedOffAtKey])

	sourceTask, err := f.svc.GetTask(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	handoffs, ok := sourceTask.Metadata[models.MetaKeyHandoffs].([]interface{})
	require.True(t, ok, "source task must carry a handoffs array")
	require.Len(t, handoffs, 1)
	entry, ok := handoffs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, result.TaskID, entry[keyTaskID])
	assert.Equal(t, f.targetWorkspaceID, entry["target_workspace_id"])
	assert.NotEmpty(t, entry[handoffHandedOffAtKey])
}

// --- 2. AC-22 same-workspace refusal ---

func TestHandleHandoffTask_SameWorkspaceRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.sourceWorkspaceID)

	payload := f.validPayload(map[string]interface{}{handoffFieldTargetWorkspaceID: f.sourceWorkspaceID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)

	assert.Equal(t, before, countTasksInWorkspace(t, f.svc, f.sourceWorkspaceID), "no task should have been created")
}

// --- 3. AC-9/AC-10 permission denial ---

func TestHandleHandoffTask_PermissionDeniedForNonCEOWithoutOverride(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleWorker, "", nil)

	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	assert.Contains(t, ep.Message, "can_handoff_tasks")
	assert.Contains(t, ep.Message, "per agent or per role")
	assert.NotContains(t, ep.Message, f.targetWorkspaceID)
}

// --- 4. AC-11 target workspace not found ---

func TestHandleHandoffTask_TargetWorkspaceNotFound(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)

	payload := f.validPayload(map[string]interface{}{handoffFieldTargetWorkspaceID: "ws-does-not-exist"})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

// --- 5. AC-5/AC-5a argument shape ---

func basicHandoffPayload() map[string]interface{} {
	return map[string]interface{}{
		handoffFieldTargetWorkspaceID: "ws-target",
		handoffFieldWorkflowID:        "wf-target",
		handoffFieldTitle:             "Deliver",
		handoffFieldPrompt:            "Do it",
		handoffFieldAgentProfileID:    "agent-profile",
		handoffFieldExecutorProfileID: "executor-profile",
	}
}

func TestHandleHandoffTask_ArgumentShapeValidation(t *testing.T) {
	svc, _ := newTestTaskService(t)
	_, agentCtrl := newHandoffAgentSettingsController(t)
	h := &Handlers{taskSvc: svc, logger: testLogger(t)}
	h.SetConfigDeps(nil, agentCtrl, nil)
	ctx := context.Background()

	t.Run("unknown argument rejected", func(t *testing.T) {
		payload := basicHandoffPayload()
		payload["bogus_field"] = "nope"
		resp, err := h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
		assertWSErrorContains(t, resp, "bogus_field")
	})

	t.Run("blank required field rejected", func(t *testing.T) {
		payload := basicHandoffPayload()
		payload[handoffFieldTitle] = ""
		resp, err := h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
		assertWSErrorContains(t, resp, handoffFieldTitle)
	})

	t.Run("present-but-blank repository_id rejected", func(t *testing.T) {
		payload := basicHandoffPayload()
		payload[handoffFieldRepositoryID] = ""
		resp, err := h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
		assertWSErrorContains(t, resp, handoffFieldRepositoryID)
	})

	t.Run("title over 60 runes rejected", func(t *testing.T) {
		payload := basicHandoffPayload()
		payload[handoffFieldTitle] = strings.Repeat("a", 61)
		resp, err := h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeValidation)
		assertWSErrorContains(t, resp, "60", "61")
	})
}

// --- 6. AC-14b agent_profile_id / executor_profile_id ---

func TestHandleHandoffTask_AgentProfileScopedToSourceWorkspaceRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const sourceScopedProfileID = "profile-scoped-to-source"
	seedHandoffAgentProfile(t, f.agentStore, sourceScopedProfileID, agentsettingsmodels.AgentRoleWorker, f.sourceWorkspaceID, "")

	payload := f.validPayload(map[string]interface{}{handoffFieldAgentProfileID: sourceScopedProfileID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp, handoffFieldAgentProfileID)
}

func TestHandleHandoffTask_ExecutorProfileNotFoundRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)

	payload := f.validPayload(map[string]interface{}{handoffFieldExecutorProfileID: "does-not-exist"})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp, handoffFieldExecutorProfileID)
}

// --- 7. AC-5b repository_id / base_branch ---

func TestHandleHandoffTask_BaseBranchWithoutRepositoryIDRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)

	payload := f.validPayload(map[string]interface{}{handoffFieldBaseBranch: "main"})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp, handoffFieldBaseBranch)
}

func TestHandleHandoffTask_RepositoryFromDifferentWorkspaceRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const wrongWorkspaceRepoID = "repo-wrong-workspace"
	require.NoError(t, f.repo.CreateRepository(context.Background(), &models.Repository{
		ID: wrongWorkspaceRepoID, WorkspaceID: f.sourceWorkspaceID, Name: "wrong-workspace-repo",
		SourceType: "local", DefaultBranch: "main",
	}))

	payload := f.validPayload(map[string]interface{}{handoffFieldRepositoryID: wrongWorkspaceRepoID})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
	assertWSErrorContains(t, resp, handoffFieldRepositoryID)
}

// --- 8. Idempotency (AC-26) ---

func TestHandleHandoffTask_IdempotentReplaySameExternalIDReturnsSameTask(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	ctx := f.ctx()
	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-idempotent-1"})

	resp1, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)
	require.Equal(t, handoffOutcomeCreated, result1.Outcome)

	resp2, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result2 := decodeHandoffResult(t, resp2)

	assert.Equal(t, handoffOutcomeFoundSettled, result2.Outcome)
	assert.True(t, result2.CreationComplete)
	assert.Equal(t, result1.TaskID, result2.TaskID)

	sourceTask, err := f.svc.GetTask(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	handoffs, ok := sourceTask.Metadata[models.MetaKeyHandoffs].([]interface{})
	require.True(t, ok)
	assert.Len(t, handoffs, 1, "at-most-once: replay must not add a second reverse-link entry")
}

// --- 9. AC-25a cross-source collision ---

func TestHandleHandoffTask_CrossSourceExternalIDCollisionRefused(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const sharedExternalID = "ext-cross-source-collision"

	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: sharedExternalID})
	resp1, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result1 := decodeHandoffResult(t, resp1)
	require.Equal(t, handoffOutcomeCreated, result1.Outcome)

	secondCtx := f.seedAdditionalSourceSession(t, "task-handoff-source-2", "session-handoff-source-2", "profile-caller-2")
	resp2, err := f.h.handleHandoffTask(secondCtx, makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	assertWSError(t, resp2, ws.ErrorCodeValidation)

	sourceTask, err := f.svc.GetTask(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	handoffs, ok := sourceTask.Metadata[models.MetaKeyHandoffs].([]interface{})
	require.True(t, ok)
	require.Len(t, handoffs, 1, "the losing source must not gain a reverse-link entry on the winner's task")
	entry, ok := handoffs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, result1.TaskID, entry[keyTaskID])
}

// --- 10. AC-27 corrupt-data guard ---

func TestHandleHandoffTask_CorruptHandoffsMetadataStillCreatesTask(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	const corrupted = `{"not":"an array"}`
	stored, _, err := f.repo.SetTaskHandoffsIfUnchanged(context.Background(), f.sourceTaskID, "", corrupted)
	require.NoError(t, err)
	require.True(t, stored, "precondition: corrupting the fresh task's handoffs metadata must succeed")

	payload := f.validPayload(map[string]interface{}{handoffFieldExternalID: "ext-corrupt-1"})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
	assert.True(t, result.CreationComplete)
	assert.False(t, result.ReverseLinkRecorded)
	assert.NotEmpty(t, result.ReverseLinkError)

	raw, err := f.repo.GetTaskHandoffsRaw(context.Background(), f.sourceTaskID)
	require.NoError(t, err)
	assert.Equal(t, corrupted, raw, "the corrupt metadata must be left byte-identical, not repaired or dropped")
}

// --- 11. AC-32 start_agent launch outcome ---

func TestHandleHandoffTask_StartAgentLaunchFailureStillSucceeds(t *testing.T) {
	launcher := newHandoffFakeLauncher(errors.New("boom: launch failed"))
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)

	payload := f.validPayload(map[string]interface{}{handoffFieldStartAgent: true})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
	assert.True(t, result.CreationComplete)
	assert.False(t, result.Started)
	assert.NotEmpty(t, result.StartError)
}

func TestHandleHandoffTask_StartAgentLaunchSuccessReportsStarted(t *testing.T) {
	launcher := newHandoffFakeLauncher(nil)
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", launcher)

	payload := f.validPayload(map[string]interface{}{handoffFieldStartAgent: true})
	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, payload))
	require.NoError(t, err)
	result := decodeHandoffResult(t, resp)

	assert.Equal(t, handoffOutcomeCreated, result.Outcome)
	assert.True(t, result.Started)
	assert.Empty(t, result.StartError)
}
