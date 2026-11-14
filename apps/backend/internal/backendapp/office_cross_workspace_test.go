package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type crossWorkspaceRuns struct{}

func (crossWorkspaceRuns) GetRun(context.Context, string) (*runmodels.Run, error) {
	return &runmodels.Run{Status: runmodels.RunStatusClaimed, AgentProfileID: "agent-1", SessionID: "src-sess",
		Payload: `{"task_id":"src-task"}`}, nil
}

type crossWorkspaceAgents struct{}

func (crossWorkspaceAgents) GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error) {
	return &officemodels.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1"}, nil
}

type crossWorkspaceTasks struct {
	sessionState models.TaskSessionState
	targetErr    error
}

func (crossWorkspaceTasks) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{ID: id, WorkspaceID: "ws-1"}, nil
}

func (t crossWorkspaceTasks) GetTaskForPrompt(_ context.Context, id string) (*models.Task, error) {
	if t.targetErr != nil {
		return nil, t.targetErr
	}
	return &models.Task{ID: id, WorkspaceID: "ws-2"}, nil
}

func (t crossWorkspaceTasks) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: id, TaskID: "src-task", State: t.sessionState}, nil
}

func (crossWorkspaceTasks) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return nil, repoerrors.ErrWorkspaceNotFound
}

func (crossWorkspaceTasks) GetWorkflow(context.Context, string) (*models.Workflow, error) {
	return nil, repoerrors.ErrWorkflowNotFound
}

func (crossWorkspaceTasks) ListWorkspaces(context.Context) ([]*models.Workspace, error) {
	return nil, nil
}

type crossWorkspaceScoper struct{}

func (crossWorkspaceScoper) ScopeOverridingIdentity(ctx context.Context, _ string) (context.Context, error) {
	return ctx, nil
}

func (crossWorkspaceScoper) ScopePrincipal(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	return mcpscope.WithPrincipal(ctx, mcpscope.Principal{
		WorkspaceID: "ws-1", Surface: mcpprofile.SurfaceOfficeTask, CallerTaskID: taskID, CallerSessionID: sessionID,
	}), nil
}

type crossWorkspaceBoundary struct{}

func (crossWorkspaceBoundary) Enabled() bool                       { return true }
func (crossWorkspaceBoundary) IsConversation(context.Context) bool { return false }
func (crossWorkspaceBoundary) CallerTaskID(context.Context) string { return "" }
func (crossWorkspaceBoundary) Authorize(context.Context, string, models.WorkspaceCollaborationAction) error {
	return nil
}

// crossWorkspaceActions answers each dispatched action from a queue of results.
type crossWorkspaceActions struct {
	results []error
	calls   int
}

func (a *crossWorkspaceActions) RequestPayload(_ context.Context, _ string, _ interface{}, result interface{}) error {
	a.calls++
	if len(a.results) > 0 {
		err := a.results[0]
		a.results = a.results[1:]
		if err != nil {
			return err
		}
	}
	return json.Unmarshal([]byte(`{"status":"queued","replayed":true}`), result)
}

func newTestCrossWorkspaceGateway(tasks crossWorkspaceTasks, actions *crossWorkspaceActions) *officeCrossWorkspaceGateway {
	return &officeCrossWorkspaceGateway{
		runs: crossWorkspaceRuns{}, agents: crossWorkspaceAgents{}, tasks: tasks, scoper: crossWorkspaceScoper{},
		boundary: crossWorkspaceBoundary{}, mcp: actions,
	}
}

func crossWorkspaceRunContext() officeruntime.RunContext {
	return officeruntime.RunContext{
		WorkspaceID: "ws-1", AgentID: "agent-1", TaskID: "src-task", RunID: "run-1", SessionID: "src-sess",
		Capabilities: officeruntime.Capabilities{CanPostComments: true, CanListTasks: true},
	}
}

func requireCrossWorkspaceStatus(t *testing.T, err error, status int, code string) {
	t.Helper()
	var target *officeruntime.CrossWorkspaceError
	if !errors.As(err, &target) || target.Status != status || target.Code != code {
		t.Fatalf("err = %v, want CrossWorkspaceError with status %d and code %q", err, status, code)
	}
}

func TestOfficeCrossWorkspaceDeniesEndedSourceSession(t *testing.T) {
	for _, state := range []models.TaskSessionState{
		models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled,
	} {
		actions := &crossWorkspaceActions{}
		gateway := newTestCrossWorkspaceGateway(crossWorkspaceTasks{sessionState: state}, actions)

		_, err := gateway.TaskPlan(context.Background(), crossWorkspaceRunContext(), "target-task")

		requireCrossWorkspaceStatus(t, err, http.StatusForbidden, "")
		if actions.calls != 0 {
			t.Fatalf("%s source session reached %d backend actions", state, actions.calls)
		}
	}
}

func TestOfficeCrossWorkspaceSendReplaysBeforeTargetCheck(t *testing.T) {
	actions := &crossWorkspaceActions{}
	gateway := newTestCrossWorkspaceGateway(crossWorkspaceTasks{
		sessionState: models.TaskSessionStateRunning, targetErr: repoerrors.ErrTaskNotFound,
	}, actions)

	body, err := gateway.SendMessage(context.Background(), crossWorkspaceRunContext(),
		officeruntime.CrossWorkspaceMessage{TaskID: "deleted-task", Prompt: "hello", OperationID: "op-1"})

	if err != nil || actions.calls != 1 || string(body) != `{"status":"queued","replayed":true}` {
		t.Fatalf("replay = %s, %v after %d calls; want the retained receipt from one call", body, err, actions.calls)
	}
}

func TestOfficeCrossWorkspaceSendWithoutRetainedRecordChecksTarget(t *testing.T) {
	actions := &crossWorkspaceActions{results: []error{
		&mcpserver.BackendError{Code: mcphandlers.SendOperationNotRetainedCode, Message: "not retained"},
	}}
	gateway := newTestCrossWorkspaceGateway(crossWorkspaceTasks{
		sessionState: models.TaskSessionStateRunning, targetErr: repoerrors.ErrTaskNotFound,
	}, actions)

	_, err := gateway.SendMessage(context.Background(), crossWorkspaceRunContext(),
		officeruntime.CrossWorkspaceMessage{TaskID: "deleted-task", Prompt: "hello", OperationID: "op-1"})

	requireCrossWorkspaceStatus(t, err, http.StatusNotFound, "")
	if actions.calls != 1 {
		t.Fatalf("an unreachable target must not be sent to; got %d backend calls", actions.calls)
	}
}

func TestOfficeCrossWorkspaceSendPreservesOperationConflict(t *testing.T) {
	actions := &crossWorkspaceActions{results: []error{
		&mcpserver.BackendError{Code: mcphandlers.SendOperationConflictCode, Message: "conflict"},
	}}
	gateway := newTestCrossWorkspaceGateway(crossWorkspaceTasks{sessionState: models.TaskSessionStateRunning}, actions)

	_, err := gateway.SendMessage(context.Background(), crossWorkspaceRunContext(),
		officeruntime.CrossWorkspaceMessage{TaskID: "target-task", Prompt: "changed", OperationID: "op-1"})

	requireCrossWorkspaceStatus(t, err, http.StatusConflict, mcphandlers.SendOperationConflictCode)
}
