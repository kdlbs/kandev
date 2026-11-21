package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	officeagents "github.com/kandev/kandev/internal/office/agents"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	officeservice "github.com/kandev/kandev/internal/office/service"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// authzRuns, authzAgents, authzTasks and authzBoundary are configurable fakes
// for the gateway's authorization inputs. Zero values describe a healthy,
// claimed run of agent-1 in ws-1 whose source task is src-task.
type authzRuns struct {
	run *runmodels.Run
	err error
}

func (r authzRuns) GetRun(context.Context, string) (*runmodels.Run, error) {
	if r.err != nil || r.run != nil {
		return r.run, r.err
	}
	return healthyAuthzRun(), nil
}

func healthyAuthzRun() *runmodels.Run {
	return &runmodels.Run{
		Status: runmodels.RunStatusClaimed, AgentProfileID: "agent-1", SessionID: "src-sess",
		Payload: `{"task_id":"src-task"}`,
	}
}

type authzAgents struct {
	agent *officemodels.AgentInstance
	err   error
}

func (a authzAgents) GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error) {
	if a.err != nil || a.agent != nil {
		return a.agent, a.err
	}
	return &officemodels.AgentInstance{ID: "agent-1", WorkspaceID: "ws-1"}, nil
}

// authzTasks serves tasks, sessions and workflows from maps and records which
// task getter each target lookup used.
type authzTasks struct {
	tasks      map[string]*models.Task
	sessions   map[string]*models.TaskSession
	workflows  map[string]*models.Workflow
	workspaces []*models.Workspace
	usedGetter *[]string
}

func newAuthzTasks() authzTasks {
	return authzTasks{
		tasks: map[string]*models.Task{
			"src-task":    {ID: "src-task", WorkspaceID: "ws-1"},
			"own-task":    {ID: "own-task", WorkspaceID: "ws-1"},
			"target-task": {ID: "target-task", WorkspaceID: "ws-2"},
		},
		sessions: map[string]*models.TaskSession{
			"src-sess": {ID: "src-sess", TaskID: "src-task", State: models.TaskSessionStateRunning},
		},
		workflows:  map[string]*models.Workflow{"wf-2": {ID: "wf-2", WorkspaceID: "ws-2"}},
		usedGetter: &[]string{},
	}
}

func (t authzTasks) lookup(id string) (*models.Task, error) {
	task, ok := t.tasks[id]
	if !ok {
		return nil, repoerrors.ErrTaskNotFound
	}
	return task, nil
}

func (t authzTasks) GetTask(_ context.Context, id string) (*models.Task, error) {
	*t.usedGetter = append(*t.usedGetter, "GetTask:"+id)
	return t.lookup(id)
}

func (t authzTasks) GetTaskForPrompt(_ context.Context, id string) (*models.Task, error) {
	*t.usedGetter = append(*t.usedGetter, "GetTaskForPrompt:"+id)
	return t.lookup(id)
}

func (t authzTasks) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	session, ok := t.sessions[id]
	if !ok {
		return nil, repoerrors.ErrTaskNotFound
	}
	return session, nil
}

func (authzTasks) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	return &models.Workspace{ID: id}, nil
}

func (t authzTasks) GetWorkflow(_ context.Context, id string) (*models.Workflow, error) {
	workflow, ok := t.workflows[id]
	if !ok {
		return nil, repoerrors.ErrWorkflowNotFound
	}
	return workflow, nil
}

func (t authzTasks) ListWorkspaces(context.Context) ([]*models.Workspace, error) {
	return t.workspaces, nil
}

// authzBoundary decides per workspace and action; the default allows everything.
type authzBoundary struct {
	disabled  bool
	authorize func(workspaceID string, action models.WorkspaceCollaborationAction) error
}

func (b authzBoundary) Enabled() bool                     { return !b.disabled }
func (authzBoundary) IsConversation(context.Context) bool { return false }
func (authzBoundary) CallerTaskID(context.Context) string { return "" }
func (b authzBoundary) Authorize(_ context.Context, workspaceID string, action models.WorkspaceCollaborationAction) error {
	if b.authorize == nil {
		return nil
	}
	return b.authorize(workspaceID, action)
}

type authzFixture struct {
	runs     authzRuns
	agents   authzAgents
	tasks    authzTasks
	boundary taskservice.WorkspaceCollaborationBoundary
	actions  *crossWorkspaceActions
}

func newAuthzFixture() *authzFixture {
	return &authzFixture{tasks: newAuthzTasks(), boundary: authzBoundary{}, actions: &crossWorkspaceActions{}}
}

func (f *authzFixture) gateway() *officeCrossWorkspaceGateway {
	return &officeCrossWorkspaceGateway{
		runs: f.runs, agents: f.agents, tasks: f.tasks, scoper: crossWorkspaceScoper{},
		boundary: f.boundary, mcp: f.actions,
	}
}

// authzOperations are the gateway entry points that start from scope(): a
// read-class one (TaskPlan) and the prompt-class one (SendMessage).
var authzOperations = map[string]func(*officeCrossWorkspaceGateway, officeruntime.RunContext) error{
	"TaskPlan": func(g *officeCrossWorkspaceGateway, rc officeruntime.RunContext) error {
		_, err := g.TaskPlan(context.Background(), rc, "own-task")
		return err
	},
	"SendMessage": func(g *officeCrossWorkspaceGateway, rc officeruntime.RunContext) error {
		_, err := g.SendMessage(context.Background(), rc,
			officeruntime.CrossWorkspaceMessage{TaskID: "own-task", Prompt: "hi"})
		return err
	},
}

func TestOfficeCrossWorkspaceHealthyRunReachesBackend(t *testing.T) {
	for name, op := range authzOperations {
		f := newAuthzFixture()

		if err := op(f.gateway(), crossWorkspaceRunContext()); err != nil || f.actions.calls != 1 {
			t.Fatalf("%s: err = %v, calls = %d; want success with one backend call", name, err, f.actions.calls)
		}
	}
}

func TestOfficeCrossWorkspaceRunValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(f *authzFixture, rc *officeruntime.RunContext)
	}{
		{"run is queued", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.Status = runmodels.RunStatusQueued
			f.runs = authzRuns{run: run}
		}},
		{"run is finished", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.Status = runmodels.RunStatusFinished
			f.runs = authzRuns{run: run}
		}},
		{"run is failed", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.Status = runmodels.RunStatusFailed
			f.runs = authzRuns{run: run}
		}},
		{"run lookup fails", func(f *authzFixture, _ *officeruntime.RunContext) {
			f.runs = authzRuns{err: errors.New("db down")}
		}},
		{"run agent differs from signed agent", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.AgentProfileID = "agent-2"
			f.runs = authzRuns{run: run}
		}},
		{"run session differs from signed session", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.SessionID = "other-sess"
			f.runs = authzRuns{run: run}
		}},
		{"run payload task differs from signed task", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.Payload = `{"task_id":"other-task"}`
			f.runs = authzRuns{run: run}
		}},
		{"run payload has no task", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.Payload = `{}`
			f.runs = authzRuns{run: run}
		}},
		{"run payload is not JSON", func(f *authzFixture, _ *officeruntime.RunContext) {
			run := healthyAuthzRun()
			run.Payload = `not json`
			f.runs = authzRuns{run: run}
		}},
		{"signed run lacks the capability", func(_ *authzFixture, rc *officeruntime.RunContext) {
			rc.Capabilities = officeruntime.Capabilities{}
		}},
		{"agent lookup fails", func(f *authzFixture, _ *officeruntime.RunContext) {
			f.agents = authzAgents{err: errors.New("db down")}
		}},
		{"agent belongs to another workspace", func(f *authzFixture, _ *officeruntime.RunContext) {
			f.agents = authzAgents{agent: &officemodels.AgentInstance{ID: "agent-1", WorkspaceID: "ws-9"}}
		}},
		{"signed identity is missing the run id", func(_ *authzFixture, rc *officeruntime.RunContext) { rc.RunID = "" }},
		{"signed identity is missing the session", func(_ *authzFixture, rc *officeruntime.RunContext) { rc.SessionID = "" }},
		{"signed identity is missing the task", func(_ *authzFixture, rc *officeruntime.RunContext) { rc.TaskID = "" }},
		{"signed identity is missing the workspace", func(_ *authzFixture, rc *officeruntime.RunContext) { rc.WorkspaceID = "" }},
		{"source task is archived", func(f *authzFixture, _ *officeruntime.RunContext) {
			archived := time.Now()
			f.tasks.tasks["src-task"] = &models.Task{ID: "src-task", WorkspaceID: "ws-1", ArchivedAt: &archived}
		}},
		{"source task is in another workspace", func(f *authzFixture, _ *officeruntime.RunContext) {
			f.tasks.tasks["src-task"] = &models.Task{ID: "src-task", WorkspaceID: "ws-2"}
		}},
		{"source task no longer exists", func(f *authzFixture, _ *officeruntime.RunContext) {
			delete(f.tasks.tasks, "src-task")
		}},
		{"source session belongs to another task", func(f *authzFixture, _ *officeruntime.RunContext) {
			f.tasks.sessions["src-sess"] = &models.TaskSession{ID: "src-sess", TaskID: "other-task", State: models.TaskSessionStateRunning}
		}},
		{"source session no longer exists", func(f *authzFixture, _ *officeruntime.RunContext) {
			delete(f.tasks.sessions, "src-sess")
		}},
	}
	for _, tc := range tests {
		for name, op := range authzOperations {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				f := newAuthzFixture()
				rc := crossWorkspaceRunContext()
				tc.mutate(f, &rc)

				err := op(f.gateway(), rc)

				requireCrossWorkspaceStatus(t, err, http.StatusForbidden, "")
				if f.actions.calls != 0 {
					t.Fatalf("a refused caller reached %d backend actions", f.actions.calls)
				}
			})
		}
	}
}

// Each operation needs exactly its own capability: reads need list_tasks and
// SendMessage needs post_comment, and neither is satisfied by the other.
func TestOfficeCrossWorkspaceCapabilityPerOperation(t *testing.T) {
	listOnly := officeruntime.Capabilities{CanListTasks: true}
	postOnly := officeruntime.Capabilities{CanPostComments: true}
	tests := []struct {
		op      string
		caps    officeruntime.Capabilities
		allowed bool
	}{
		{"TaskPlan", listOnly, true},
		{"TaskPlan", postOnly, false},
		{"SendMessage", postOnly, true},
		{"SendMessage", listOnly, false},
	}
	for _, tc := range tests {
		f := newAuthzFixture()
		rc := crossWorkspaceRunContext()
		rc.Capabilities = tc.caps

		err := authzOperations[tc.op](f.gateway(), rc)

		if tc.allowed && (err != nil || f.actions.calls != 1) {
			t.Fatalf("%s with %+v: err = %v, calls = %d; want success", tc.op, tc.caps, err, f.actions.calls)
		}
		if !tc.allowed {
			requireCrossWorkspaceStatus(t, err, http.StatusForbidden, "")
			if f.actions.calls != 0 {
				t.Fatalf("%s with %+v reached %d backend actions", tc.op, tc.caps, f.actions.calls)
			}
		}
	}
}

func TestOfficeCrossWorkspaceUnconfiguredGatewayFailsClosed(t *testing.T) {
	gateway := newAuthzFixture().gateway()
	gateway.runs = nil

	_, err := gateway.TaskPlan(context.Background(), crossWorkspaceRunContext(), "own-task")

	if !errors.Is(err, officeruntime.ErrRuntimeDependencyMissing) {
		t.Fatalf("err = %v, want ErrRuntimeDependencyMissing", err)
	}
}

func TestOfficeCrossWorkspaceDisabledCollaboration(t *testing.T) {
	tests := []struct {
		name       string
		taskID     string
		wantStatus int
	}{
		{"foreign target is refused", "target-task", http.StatusForbidden},
		{"own workspace target still works", "own-task", 0},
	}
	for _, tc := range tests {
		for name, send := range map[string]func(*officeCrossWorkspaceGateway, string) error{
			"TaskPlan": func(g *officeCrossWorkspaceGateway, id string) error {
				_, err := g.TaskPlan(context.Background(), crossWorkspaceRunContext(), id)
				return err
			},
			"SendMessage": func(g *officeCrossWorkspaceGateway, id string) error {
				_, err := g.SendMessage(context.Background(), crossWorkspaceRunContext(),
					officeruntime.CrossWorkspaceMessage{TaskID: id, Prompt: "hi"})
				return err
			},
		} {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				f := newAuthzFixture()
				f.boundary = authzBoundary{disabled: true}

				err := send(f.gateway(), tc.taskID)

				if tc.wantStatus == 0 {
					if err != nil || f.actions.calls != 1 {
						t.Fatalf("err = %v, calls = %d; want an own-workspace call to go through", err, f.actions.calls)
					}
					return
				}
				requireCrossWorkspaceStatus(t, err, tc.wantStatus, "")
				if f.actions.calls != 0 {
					t.Fatalf("disabled collaboration reached %d backend actions", f.actions.calls)
				}
			})
		}
	}
}

func TestOfficeCrossWorkspaceDirectoryReportsDisabledFlag(t *testing.T) {
	f := newAuthzFixture()
	f.boundary = authzBoundary{disabled: true}
	f.tasks.workspaces = []*models.Workspace{{ID: "ws-1", Name: "own"}, {ID: "ws-2", Name: "foreign"}}

	body, err := f.gateway().Directory(context.Background(), crossWorkspaceRunContext())

	got := decodeAuthzDirectory(t, body, err)
	if got.Enabled || len(got.Workspaces) != 1 || got.Workspaces[0].ID != "ws-1" || !got.Workspaces[0].Self {
		t.Fatalf("directory = %+v, want only the own workspace with the feature flag off", got)
	}
}

// A grant that allows read but not prompt must let the caller read the target
// and still refuse SendMessage. This runs against the production boundary so
// the grant lookup and action matching are the real ones.
type authzGrantStore struct {
	grant *models.WorkspaceCollaborationGrant
	err   error
}

func (authzGrantStore) GetTask(_ context.Context, id string) (*models.Task, error) {
	return &models.Task{ID: id, WorkspaceID: "ws-1"}, nil
}

func (authzGrantStore) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: id, TaskID: "src-task"}, nil
}

func (authzGrantStore) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	return &models.Workspace{ID: id}, nil
}

func (s authzGrantStore) GetWorkspaceCollaborationGrant(context.Context, string, string, string) (*models.WorkspaceCollaborationGrant, error) {
	return s.grant, s.err
}

func TestOfficeCrossWorkspaceReadGrantDoesNotAllowSendMessage(t *testing.T) {
	tests := []struct {
		name       string
		store      authzGrantStore
		wantRead   bool
		wantStatus int
	}{
		{"read only grant", authzGrantStore{grant: &models.WorkspaceCollaborationGrant{Read: true}}, true, http.StatusNotFound},
		{"read and report grant", authzGrantStore{grant: &models.WorkspaceCollaborationGrant{Read: true, Report: true}}, true, http.StatusNotFound},
		{"no grant", authzGrantStore{}, false, http.StatusNotFound},
		{"grant lookup unavailable", authzGrantStore{err: errors.New("db down")}, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthzFixture()
			f.boundary = &workspaceCollaborationBoundary{enabled: true, store: tc.store}
			gateway := f.gateway()

			_, readErr := gateway.TaskPlan(context.Background(), crossWorkspaceRunContext(), "target-task")
			readCalls := f.actions.calls
			_, sendErr := gateway.SendMessage(context.Background(), crossWorkspaceRunContext(),
				officeruntime.CrossWorkspaceMessage{TaskID: "target-task", Prompt: "hi"})

			if tc.wantRead && (readErr != nil || readCalls != 1) {
				t.Fatalf("read with a read grant: err = %v, calls = %d", readErr, readCalls)
			}
			if !tc.wantRead && readErr == nil {
				t.Fatal("read without a read grant succeeded")
			}
			if sendErr == nil {
				t.Fatal("SendMessage succeeded without a prompt grant")
			}
			if tc.wantStatus != 0 {
				requireCrossWorkspaceStatus(t, sendErr, tc.wantStatus, "")
			}
			if f.actions.calls != readCalls {
				t.Fatalf("SendMessage without a prompt grant reached the backend (%d calls after %d)", f.actions.calls, readCalls)
			}
		})
	}
}

func TestOfficeCrossWorkspacePromptGrantAllowsSendMessage(t *testing.T) {
	f := newAuthzFixture()
	f.boundary = &workspaceCollaborationBoundary{
		enabled: true, store: authzGrantStore{grant: &models.WorkspaceCollaborationGrant{Read: true, Prompt: true}},
	}

	_, err := f.gateway().SendMessage(context.Background(), crossWorkspaceRunContext(),
		officeruntime.CrossWorkspaceMessage{TaskID: "target-task", Prompt: "hi"})

	if err != nil || f.actions.calls != 1 {
		t.Fatalf("err = %v, calls = %d; want the prompt grant to allow one send", err, f.actions.calls)
	}
}

func TestOfficeCrossWorkspaceTargetLookupUsesActionSpecificGetter(t *testing.T) {
	f := newAuthzFixture()
	gateway := f.gateway()

	if _, err := gateway.TaskPlan(context.Background(), crossWorkspaceRunContext(), "target-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.SendMessage(context.Background(), crossWorkspaceRunContext(),
		officeruntime.CrossWorkspaceMessage{TaskID: "target-task", Prompt: "hi"}); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(*f.tasks.usedGetter, ",")
	for _, want := range []string{"GetTask:target-task", "GetTaskForPrompt:target-task"} {
		if !strings.Contains(got, want) {
			t.Fatalf("getters used = %s, missing %s", got, want)
		}
	}
}

// A foreign workspace the caller may not reach must look the same as one that
// does not exist: 404, never 403, and never a backend call.
func TestOfficeCrossWorkspaceHidesUnreachableTargetsAsNotFound(t *testing.T) {
	denyForeign := authzBoundary{authorize: func(workspaceID string, _ models.WorkspaceCollaborationAction) error {
		if workspaceID != "ws-1" {
			return taskservice.ErrForbidden
		}
		return nil
	}}
	ctx := context.Background()
	rc := crossWorkspaceRunContext()
	calls := map[string]func(*officeCrossWorkspaceGateway, string) error{
		"Coordinator": func(g *officeCrossWorkspaceGateway, id string) error {
			_, err := g.Coordinator(ctx, rc, id)
			return err
		},
		"ListTasks": func(g *officeCrossWorkspaceGateway, id string) error {
			_, err := g.ListTasks(ctx, rc, id, "")
			return err
		},
		"TaskSessions": func(g *officeCrossWorkspaceGateway, id string) error {
			_, err := g.TaskSessions(ctx, rc, id)
			return err
		},
		"TaskConversation": func(g *officeCrossWorkspaceGateway, id string) error {
			_, err := g.TaskConversation(ctx, rc, id, officeruntime.ConversationQuery{})
			return err
		},
		"TaskPlan": func(g *officeCrossWorkspaceGateway, id string) error {
			_, err := g.TaskPlan(ctx, rc, id)
			return err
		},
		"SendMessage": func(g *officeCrossWorkspaceGateway, id string) error {
			_, err := g.SendMessage(ctx, rc, officeruntime.CrossWorkspaceMessage{TaskID: id, Prompt: "hi"})
			return err
		},
	}
	workspaceTargets := map[string]bool{"Coordinator": true, "ListTasks": true}
	for name, call := range calls {
		id := "target-task"
		if workspaceTargets[name] {
			id = "ws-2"
		}
		t.Run(name+"/foreign workspace without grant", func(t *testing.T) {
			f := newAuthzFixture()
			f.boundary = denyForeign

			requireCrossWorkspaceStatus(t, call(f.gateway(), id), http.StatusNotFound, "")
			if f.actions.calls != 0 {
				t.Fatalf("hidden target reached %d backend actions", f.actions.calls)
			}
		})
		if workspaceTargets[name] {
			continue
		}
		t.Run(name+"/missing task", func(t *testing.T) {
			f := newAuthzFixture()

			requireCrossWorkspaceStatus(t, call(f.gateway(), "no-such-task"), http.StatusNotFound, "")
			if f.actions.calls != 0 {
				t.Fatalf("missing task reached %d backend actions", f.actions.calls)
			}
		})
	}
}

func TestOfficeCrossWorkspaceListTasksHidesForeignWorkflow(t *testing.T) {
	f := newAuthzFixture()
	f.tasks.workflows["wf-2"] = &models.Workflow{ID: "wf-2", WorkspaceID: "ws-2"}

	_, err := f.gateway().ListTasks(context.Background(), crossWorkspaceRunContext(), "ws-1", "wf-2")

	requireCrossWorkspaceStatus(t, err, http.StatusNotFound, "")
	if f.actions.calls != 0 {
		t.Fatalf("a workflow of another workspace reached %d backend actions", f.actions.calls)
	}
}

func TestOfficeCrossWorkspaceBoundaryFailureIsNotHiddenAsNotFound(t *testing.T) {
	unavailable := taskservice.ErrWorkspaceCollaborationUnavailable
	f := newAuthzFixture()
	f.boundary = authzBoundary{authorize: func(string, models.WorkspaceCollaborationAction) error { return unavailable }}

	_, err := f.gateway().TaskPlan(context.Background(), crossWorkspaceRunContext(), "target-task")

	if !errors.Is(err, unavailable) || f.actions.calls != 0 {
		t.Fatalf("err = %v, calls = %d; want the boundary failure surfaced and no backend call", err, f.actions.calls)
	}
}

type authzDirectory struct {
	Enabled    bool `json:"cross_workspace_access_enabled"`
	Workspaces []struct {
		ID   string `json:"id"`
		Self bool   `json:"self"`
	} `json:"workspaces"`
}

func decodeAuthzDirectory(t *testing.T, body json.RawMessage, err error) authzDirectory {
	t.Helper()
	if err != nil {
		t.Fatalf("Directory: %v", err)
	}
	var got authzDirectory
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode directory %s: %v", body, err)
	}
	return got
}

func TestOfficeCrossWorkspaceDirectorySkipsUnauthorizedWorkspaces(t *testing.T) {
	f := newAuthzFixture()
	f.tasks.workspaces = []*models.Workspace{
		{ID: "ws-1", Name: "own"}, {ID: "ws-2", Name: "no grant"}, nil, {ID: "ws-3", Name: "granted"}, {ID: "ws-4", Name: "disabled"},
	}
	f.boundary = authzBoundary{authorize: func(workspaceID string, action models.WorkspaceCollaborationAction) error {
		if action != models.CollaborationRead {
			t.Errorf("Directory asked for %q, want read", action)
		}
		switch workspaceID {
		case "ws-2":
			return taskservice.ErrForbidden
		case "ws-4":
			return &officeruntime.CrossWorkspaceError{Status: http.StatusForbidden, Message: "workspace collaboration is disabled"}
		}
		return nil
	}}

	body, err := f.gateway().Directory(context.Background(), crossWorkspaceRunContext())

	got := decodeAuthzDirectory(t, body, err)
	if !got.Enabled || len(got.Workspaces) != 2 ||
		got.Workspaces[0].ID != "ws-1" || !got.Workspaces[0].Self ||
		got.Workspaces[1].ID != "ws-3" || got.Workspaces[1].Self {
		t.Fatalf("directory = %+v, want ws-1 (self) and ws-3 only", got)
	}
}

func TestOfficeCrossWorkspaceDirectorySurfacesBoundaryFailure(t *testing.T) {
	f := newAuthzFixture()
	f.tasks.workspaces = []*models.Workspace{{ID: "ws-1"}, {ID: "ws-2"}}
	f.boundary = authzBoundary{authorize: func(string, models.WorkspaceCollaborationAction) error {
		return taskservice.ErrWorkspaceCollaborationUnavailable
	}}

	_, err := f.gateway().Directory(context.Background(), crossWorkspaceRunContext())

	if !errors.Is(err, taskservice.ErrWorkspaceCollaborationUnavailable) {
		t.Fatalf("err = %v, want the boundary failure rather than a silently empty directory", err)
	}
}

func TestOfficeCrossWorkspaceDirectoryRequiresListTasksCapability(t *testing.T) {
	f := newAuthzFixture()
	rc := crossWorkspaceRunContext()
	rc.Capabilities = officeruntime.Capabilities{CanPostComments: true}

	_, err := f.gateway().Directory(context.Background(), rc)

	requireCrossWorkspaceStatus(t, err, http.StatusForbidden, "")
}

// recordingCrossWorkspaceGateway answers every route with {"via":"gateway"}.
type recordingCrossWorkspaceGateway struct{ calls []string }

func (g *recordingCrossWorkspaceGateway) reply(name string) (json.RawMessage, error) {
	g.calls = append(g.calls, name)
	return json.RawMessage(`{"via":"gateway"}`), nil
}

func (g *recordingCrossWorkspaceGateway) Directory(context.Context, officeruntime.RunContext) (json.RawMessage, error) {
	return g.reply("Directory")
}

func (g *recordingCrossWorkspaceGateway) Coordinator(context.Context, officeruntime.RunContext, string) (json.RawMessage, error) {
	return g.reply("Coordinator")
}

func (g *recordingCrossWorkspaceGateway) ListTasks(context.Context, officeruntime.RunContext, string, string) (json.RawMessage, error) {
	return g.reply("ListTasks")
}

func (g *recordingCrossWorkspaceGateway) TaskSessions(context.Context, officeruntime.RunContext, string) (json.RawMessage, error) {
	return g.reply("TaskSessions")
}

func (g *recordingCrossWorkspaceGateway) TaskConversation(
	context.Context, officeruntime.RunContext, string, officeruntime.ConversationQuery,
) (json.RawMessage, error) {
	return g.reply("TaskConversation")
}

func (g *recordingCrossWorkspaceGateway) TaskPlan(context.Context, officeruntime.RunContext, string) (json.RawMessage, error) {
	return g.reply("TaskPlan")
}

func (g *recordingCrossWorkspaceGateway) SendMessage(
	context.Context, officeruntime.RunContext, officeruntime.CrossWorkspaceMessage,
) (json.RawMessage, error) {
	return g.reply("SendMessage")
}

type crossWorkspaceRoute struct {
	method, path, body, gatewayCall string
}

var crossWorkspaceRoutes = []crossWorkspaceRoute{
	{http.MethodGet, "/api/v1/office/runtime/workspaces", "", "Directory"},
	{http.MethodGet, "/api/v1/office/runtime/workspaces/ws-2/coordinator", "", "Coordinator"},
	{http.MethodGet, "/api/v1/office/runtime/workspaces/ws-2/tasks", "", "ListTasks"},
	{http.MethodGet, "/api/v1/office/runtime/tasks/t-2/sessions", "", "TaskSessions"},
	{http.MethodGet, "/api/v1/office/runtime/tasks/t-2/conversation", "", "TaskConversation"},
	{http.MethodGet, "/api/v1/office/runtime/tasks/t-2/plan", "", "TaskPlan"},
	{http.MethodPost, "/api/v1/office/runtime/tasks/t-2/messages", `{"prompt":"hi"}`, "SendMessage"},
}

// crossWorkspaceRouteEngine mounts the production Office route group with a
// real agent service, so the runtime token is verified by the shipped code.
func crossWorkspaceRouteEngine(t *testing.T, caps officeruntime.Capabilities, gateway ...officeruntime.CrossWorkspaceGateway) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := newOfficeScopeHarness(t)
	agentSvc := officeagents.NewAgentService(h.officeRepo, testLogger(t), nil)
	agentSvc.SetAuth(officeagents.NewAgentAuth("cross-workspace-route-test-key"))
	agent := &officemodels.AgentInstance{ID: "agent-route", WorkspaceID: "ws-1", Name: "CEO", Role: officemodels.AgentRoleCEO}
	if err := h.officeRepo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	capsJSON, err := officeruntime.MarshalCapabilities(caps)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	token, err := agentSvc.MintRuntimeJWT("agent-route", "src-task", "ws-1", "run-1", "src-sess", capsJSON)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	svcs := officeTestServices()
	svcs.Agents = agentSvc
	// Successful and denied calls append run events, so Workspaces needs a real repo.
	svcs.Workspaces = officeservice.NewService(officeservice.ServiceOptions{Repo: h.officeRepo, Logger: testLogger(t)})
	engine := gin.New()
	mountOfficeRoutes(engine, svcs, nil, h.taskSvc, h.officeRepo, nil, officeruntime.HandoffDependencies{}, testLogger(t), gateway...)
	return engine, token
}

func serveCrossWorkspaceRoute(engine *gin.Engine, route crossWorkspaceRoute, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
	if route.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestMountOfficeRoutesServesCrossWorkspaceRoutes(t *testing.T) {
	gateway := &recordingCrossWorkspaceGateway{}
	engine, token := crossWorkspaceRouteEngine(t,
		officeruntime.Capabilities{CanPostComments: true, CanListTasks: true}, gateway)

	for _, route := range crossWorkspaceRoutes {
		rec := serveCrossWorkspaceRoute(engine, route, token)

		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"via":"gateway"`) {
			t.Errorf("%s %s: status = %d, body = %s; want the gateway's answer", route.method, route.path, rec.Code, rec.Body.String())
		}
	}
	if len(gateway.calls) != len(crossWorkspaceRoutes) {
		t.Fatalf("gateway calls = %v, want one per route", gateway.calls)
	}
	for i, route := range crossWorkspaceRoutes {
		if gateway.calls[i] != route.gatewayCall {
			t.Errorf("%s %s reached %s, want %s", route.method, route.path, gateway.calls[i], route.gatewayCall)
		}
	}
}

func TestMountOfficeRoutesFailsClosedWithoutCrossWorkspaceGateway(t *testing.T) {
	caps := officeruntime.Capabilities{CanPostComments: true, CanListTasks: true}
	tests := map[string][]officeruntime.CrossWorkspaceGateway{
		"no gateway argument":  nil,
		"explicit nil gateway": {nil},
	}
	for name, gateways := range tests {
		t.Run(name, func(t *testing.T) {
			engine, token := crossWorkspaceRouteEngine(t, caps, gateways...)

			for _, route := range crossWorkspaceRoutes {
				rec := serveCrossWorkspaceRoute(engine, route, token)

				if rec.Code != http.StatusServiceUnavailable {
					t.Errorf("%s %s: status = %d, body = %s; want 503", route.method, route.path, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestMountOfficeRoutesCrossWorkspaceRoutesRequireTokenAndCapability(t *testing.T) {
	gateway := &recordingCrossWorkspaceGateway{}
	noCaps, noCapsToken := crossWorkspaceRouteEngine(t, officeruntime.Capabilities{}, gateway)

	for _, route := range crossWorkspaceRoutes {
		if rec := serveCrossWorkspaceRoute(noCaps, route, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: status = %d, want 401", route.method, route.path, rec.Code)
		}
		if rec := serveCrossWorkspaceRoute(noCaps, route, noCapsToken); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s without the capability: status = %d, want 403", route.method, route.path, rec.Code)
		}
	}
	if len(gateway.calls) != 0 {
		t.Fatalf("refused callers reached the gateway: %v", gateway.calls)
	}
}
