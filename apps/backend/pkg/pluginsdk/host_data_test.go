package pluginsdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// dataRecordingHost is a fake Go-native Host implementation whose Host data
// API (ADR 0043) accessors are backed by in-memory fixtures, used to prove
// grpcHostServer -> grpcHostClient reachability for the read RPCs added in
// task-03. State/secrets/emit are inherited (unimplemented) from
// UnimplementedHostData since these tests only exercise the data accessors.
type dataRecordingHost struct {
	UnimplementedHostData

	tasks         map[string]Task
	taskList      []Task
	taskPageInfo  *PageInfo
	sessions      []Session
	sessionPage   *PageInfo
	codeStats     []SessionCodeStats
	codeStatsPage *PageInfo
	workspaces    []Workspace
	workflows     []Workflow
	workflowSteps []WorkflowStep
	agentProfiles []AgentProfile
	repositories  []Repository

	lastTaskFilter    TaskFilter
	lastSessionFilter SessionFilter
	lastWorkspaceID   string
	lastWorkflowID    string
}

func (h *dataRecordingHost) GetState(context.Context, string, string, string) (map[string]any, bool, error) {
	return nil, false, nil
}
func (h *dataRecordingHost) SetState(context.Context, string, string, string, map[string]any) error {
	return nil
}
func (h *dataRecordingHost) DeleteState(context.Context, string, string, string) error { return nil }
func (h *dataRecordingHost) ListState(context.Context, string, string) ([]StateEntry, error) {
	return nil, nil
}
func (h *dataRecordingHost) GetConfig(context.Context) (map[string]any, error)    { return nil, nil }
func (h *dataRecordingHost) RevealSecret(context.Context, string) (string, error) { return "", nil }
func (h *dataRecordingHost) EmitEvent(context.Context, string, map[string]any) error {
	return nil
}

func (h *dataRecordingHost) Tasks() TaskReader           { return dataRecordingTaskReader{h} }
func (h *dataRecordingHost) Sessions() SessionReader     { return dataRecordingSessionReader{h} }
func (h *dataRecordingHost) Workspaces() WorkspaceReader { return dataRecordingWorkspaceReader{h} }
func (h *dataRecordingHost) Workflows() WorkflowReader   { return dataRecordingWorkflowReader{h} }
func (h *dataRecordingHost) AgentProfiles() AgentProfileReader {
	return dataRecordingAgentProfileReader{h}
}
func (h *dataRecordingHost) Repositories() RepositoryReader {
	return dataRecordingRepositoryReader{h}
}

type dataRecordingTaskReader struct{ h *dataRecordingHost }

func (r dataRecordingTaskReader) List(_ context.Context, filter TaskFilter, _ Page) ([]Task, *PageInfo, error) {
	r.h.lastTaskFilter = filter
	return r.h.taskList, r.h.taskPageInfo, nil
}

func (r dataRecordingTaskReader) Get(_ context.Context, id string) (*Task, error) {
	task, ok := r.h.tasks[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "task %q not found", id)
	}
	return &task, nil
}

type dataRecordingSessionReader struct{ h *dataRecordingHost }

func (r dataRecordingSessionReader) List(_ context.Context, filter SessionFilter, _ Page) ([]Session, *PageInfo, error) {
	r.h.lastSessionFilter = filter
	return r.h.sessions, r.h.sessionPage, nil
}

func (r dataRecordingSessionReader) CodeStats(_ context.Context, filter SessionFilter, _ Page) ([]SessionCodeStats, *PageInfo, error) {
	r.h.lastSessionFilter = filter
	return r.h.codeStats, r.h.codeStatsPage, nil
}

type dataRecordingWorkspaceReader struct{ h *dataRecordingHost }

func (r dataRecordingWorkspaceReader) List(context.Context, Page) ([]Workspace, *PageInfo, error) {
	return r.h.workspaces, nil, nil
}

type dataRecordingWorkflowReader struct{ h *dataRecordingHost }

func (r dataRecordingWorkflowReader) List(_ context.Context, workspaceID string, _ Page) ([]Workflow, *PageInfo, error) {
	r.h.lastWorkspaceID = workspaceID
	return r.h.workflows, nil, nil
}

func (r dataRecordingWorkflowReader) ListSteps(_ context.Context, workflowID string) ([]WorkflowStep, error) {
	r.h.lastWorkflowID = workflowID
	return r.h.workflowSteps, nil
}

type dataRecordingAgentProfileReader struct{ h *dataRecordingHost }

func (r dataRecordingAgentProfileReader) List(context.Context, Page) ([]AgentProfile, *PageInfo, error) {
	return r.h.agentProfiles, nil, nil
}

type dataRecordingRepositoryReader struct{ h *dataRecordingHost }

func (r dataRecordingRepositoryReader) List(_ context.Context, workspaceID string, _ Page) ([]Repository, *PageInfo, error) {
	r.h.lastWorkspaceID = workspaceID
	return r.h.repositories, nil, nil
}

func TestHostData_TasksListAndGet(t *testing.T) {
	impl := &dataRecordingHost{
		tasks: map[string]Task{
			"task-1": {ID: "task-1", Title: "Fix the bug", State: "todo"},
		},
		taskList:     []Task{{ID: "task-1", Title: "Fix the bug", State: "todo"}},
		taskPageInfo: &PageInfo{NextCursor: "next-1", HasMore: true},
	}
	host := dialHostOverBufconn(t, impl)

	tasks, pageInfo, err := host.Tasks().List(context.Background(), TaskFilter{States: []string{"todo"}}, Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, []Task{{ID: "task-1", Title: "Fix the bug", State: "todo"}}, tasks)
	require.Equal(t, &PageInfo{NextCursor: "next-1", HasMore: true}, pageInfo)
	require.Equal(t, TaskFilter{States: []string{"todo"}}, impl.lastTaskFilter)

	task, err := host.Tasks().Get(context.Background(), "task-1")
	require.NoError(t, err)
	require.Equal(t, "Fix the bug", task.Title)

	_, err = host.Tasks().Get(context.Background(), "missing")
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestHostData_SessionsListAndCodeStats(t *testing.T) {
	impl := &dataRecordingHost{
		sessions:  []Session{{ID: "session-1", TaskID: "task-1", State: "running"}},
		codeStats: []SessionCodeStats{{SessionID: "session-1", LinesAddedCommitted: 10}},
	}
	host := dialHostOverBufconn(t, impl)

	sessions, _, err := host.Sessions().List(context.Background(), SessionFilter{TaskIDs: []string{"task-1"}}, Page{})
	require.NoError(t, err)
	require.Equal(t, impl.sessions, sessions)
	require.Equal(t, SessionFilter{TaskIDs: []string{"task-1"}}, impl.lastSessionFilter)

	stats, _, err := host.Sessions().CodeStats(context.Background(), SessionFilter{TaskIDs: []string{"task-1"}}, Page{})
	require.NoError(t, err)
	require.Equal(t, impl.codeStats, stats)
}

func TestHostData_Workspaces(t *testing.T) {
	impl := &dataRecordingHost{workspaces: []Workspace{{ID: "ws-1", Name: "Acme"}}}
	host := dialHostOverBufconn(t, impl)

	workspaces, _, err := host.Workspaces().List(context.Background(), Page{})
	require.NoError(t, err)
	require.Equal(t, impl.workspaces, workspaces)
}

func TestHostData_WorkflowsAndSteps(t *testing.T) {
	impl := &dataRecordingHost{
		workflows:     []Workflow{{ID: "wf-1", WorkspaceID: "ws-1", Name: "Default"}},
		workflowSteps: []WorkflowStep{{ID: "step-1", WorkflowID: "wf-1", Name: "Review"}},
	}
	host := dialHostOverBufconn(t, impl)

	workflows, _, err := host.Workflows().List(context.Background(), "ws-1", Page{})
	require.NoError(t, err)
	require.Equal(t, impl.workflows, workflows)
	require.Equal(t, "ws-1", impl.lastWorkspaceID)

	steps, err := host.Workflows().ListSteps(context.Background(), "wf-1")
	require.NoError(t, err)
	require.Equal(t, impl.workflowSteps, steps)
	require.Equal(t, "wf-1", impl.lastWorkflowID)
}

func TestHostData_AgentProfiles(t *testing.T) {
	impl := &dataRecordingHost{agentProfiles: []AgentProfile{{ID: "profile-1", DisplayName: "Claude"}}}
	host := dialHostOverBufconn(t, impl)

	profiles, _, err := host.AgentProfiles().List(context.Background(), Page{})
	require.NoError(t, err)
	require.Equal(t, impl.agentProfiles, profiles)
}

func TestHostData_Repositories(t *testing.T) {
	impl := &dataRecordingHost{repositories: []Repository{{ID: "repo-1", WorkspaceID: "ws-1", Name: "kdlbs/kandev"}}}
	host := dialHostOverBufconn(t, impl)

	repos, _, err := host.Repositories().List(context.Background(), "ws-1", Page{})
	require.NoError(t, err)
	require.Equal(t, impl.repositories, repos)
	require.Equal(t, "ws-1", impl.lastWorkspaceID)
}

// TestHostData_UnimplementedHostData_ReturnsUnimplemented proves that a Host
// implementation embedding UnimplementedHostData (task-03's default for
// task-04 to override) compiles against the Host interface and, over the
// wire, returns a gRPC Unimplemented status for every Host data API RPC —
// so a plugin author calling an unwired accessor gets a clear error rather
// than a panic or a zero-value success.
func TestHostData_UnimplementedHostData_ReturnsUnimplemented(t *testing.T) {
	impl := &recordingHost{}
	host := dialHostOverBufconn(t, impl)

	_, _, err := host.Tasks().List(context.Background(), TaskFilter{}, Page{})
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = host.Tasks().Get(context.Background(), "task-1")
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))

	_, _, err = host.Sessions().CodeStats(context.Background(), SessionFilter{}, Page{})
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
