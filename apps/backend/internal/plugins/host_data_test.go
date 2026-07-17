package plugins

import (
	"context"
	"errors"
	"testing"
	"time"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	analyticsmodels "github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// ── fakes for the narrow Host data API interfaces ───────────────────────

type fakeTaskDataSource struct {
	workspaces       []*taskmodels.Workspace
	tasksByWorkspace map[string][]*taskmodels.Task
	tasksByID        map[string]*taskmodels.Task
	repositories     map[string][]*taskmodels.Repository
	sessionsByTask   map[string][]*taskmodels.TaskSession
	executorRunning  map[string]*taskmodels.ExecutorRunning

	// gotIncludeArchived records the includeArchived flag of every
	// ListTasksByWorkspace call, in call order.
	gotIncludeArchived []bool
}

func (f *fakeTaskDataSource) ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error) {
	return f.workspaces, nil
}

func (f *fakeTaskDataSource) ListTasksByWorkspace(
	_ context.Context, workspaceID, _, _, _ string, _, _ int, _ string, includeArchived, _, _, _ bool,
) ([]*taskmodels.Task, int, error) {
	f.gotIncludeArchived = append(f.gotIncludeArchived, includeArchived)
	tasks := f.tasksByWorkspace[workspaceID]
	return tasks, len(tasks), nil
}

func (f *fakeTaskDataSource) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	task, ok := f.tasksByID[id]
	if !ok {
		return nil, repoerrors.ErrTaskNotFound
	}
	return task, nil
}

func (f *fakeTaskDataSource) ListRepositories(_ context.Context, workspaceID string) ([]*taskmodels.Repository, error) {
	return f.repositories[workspaceID], nil
}

func (f *fakeTaskDataSource) ListTaskSessions(_ context.Context, taskID string) ([]*taskmodels.TaskSession, error) {
	return f.sessionsByTask[taskID], nil
}

func (f *fakeTaskDataSource) GetExecutorRunningBySessionID(_ context.Context, sessionID string) (*taskmodels.ExecutorRunning, error) {
	running, ok := f.executorRunning[sessionID]
	if !ok {
		return nil, taskmodels.ErrExecutorRunningNotFound
	}
	return running, nil
}

type fakeWorkflowLister struct {
	workflows map[string][]*taskmodels.Workflow
}

func (f *fakeWorkflowLister) ListWorkflows(_ context.Context, workspaceID string, _ bool) ([]*taskmodels.Workflow, error) {
	return f.workflows[workspaceID], nil
}

type fakeWorkflowStepLister struct {
	steps map[string][]*wfmodels.WorkflowStep
}

func (f *fakeWorkflowStepLister) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error) {
	return f.steps[workflowID], nil
}

type fakeAgentProfileDataSource struct {
	resp *agentsettingsdto.ListAgentsResponse
}

func (f *fakeAgentProfileDataSource) ListAgents(context.Context) (*agentsettingsdto.ListAgentsResponse, error) {
	return f.resp, nil
}

type fakeSessionCodeStatsSource struct {
	calls      int
	lastFilter analyticsmodels.SessionCodeStatsFilter
	stats      []*analyticsmodels.SessionCodeStats
	err        error
}

func (f *fakeSessionCodeStatsSource) ListSessionCodeStats(
	_ context.Context, filter analyticsmodels.SessionCodeStatsFilter,
) ([]*analyticsmodels.SessionCodeStats, error) {
	f.calls++
	f.lastFilter = filter
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

// testDataHost bundles a pluginHost with every fake it was wired from, so
// tests can both drive Host calls and assert against the fakes' recorded
// state.
type testDataHost struct {
	host      *pluginHost
	tasks     *fakeTaskDataSource
	workflows *fakeWorkflowLister
	steps     *fakeWorkflowStepLister
	profiles  *fakeAgentProfileDataSource
	codeStats *fakeSessionCodeStatsSource
}

// newTestDataHost builds a fully-wired pluginHost (every Host data API
// dependency set, even if a given test's capabilities don't grant every
// resource) so each test only needs to vary caps.
func newTestDataHost(caps manifest.Capabilities) *testDataHost {
	d := &testDataHost{
		tasks:     &fakeTaskDataSource{},
		workflows: &fakeWorkflowLister{},
		steps:     &fakeWorkflowStepLister{},
		profiles:  &fakeAgentProfileDataSource{resp: &agentsettingsdto.ListAgentsResponse{}},
		codeStats: &fakeSessionCodeStatsSource{},
	}
	d.host = &pluginHost{
		pluginID:         "p1",
		capabilities:     caps,
		taskData:         d.tasks,
		workflows:        d.workflows,
		workflowSteps:    d.steps,
		agentProfiles:    d.profiles,
		sessionCodeStats: d.codeStats,
	}
	return d
}

// ── capability gating: denied without api_read:<resource> ──────────────

func TestPluginHost_Tasks_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:tasks")

	_, err = d.host.Tasks().Get(context.Background(), "task-1")
	assertPermissionDenied(t, err, "api_read:tasks")
}

func TestPluginHost_Sessions_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, _, err := d.host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:sessions")

	_, _, err = d.host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:sessions")
}

func TestPluginHost_Workspaces_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, _, err := d.host.Workspaces().List(context.Background(), pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:workspaces")
}

func TestPluginHost_Workflows_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, _, err := d.host.Workflows().List(context.Background(), "ws-1", pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:workflows")

	_, err = d.host.Workflows().ListSteps(context.Background(), "wf-1")
	assertPermissionDenied(t, err, "api_read:workflows")
}

func TestPluginHost_AgentProfiles_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, _, err := d.host.AgentProfiles().List(context.Background(), pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:agent_profiles")
}

func TestPluginHost_Repositories_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, _, err := d.host.Repositories().List(context.Background(), "ws-1", pluginsdk.Page{})
	assertPermissionDenied(t, err, "api_read:repositories")
}

// ── capability gating: succeeds with api_read:<resource> ───────────────

func TestPluginHost_Tasks_SucceedsWithCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{
		"ws-1": {{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Task 1", State: v1.TaskStateTODO}},
	}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": d.tasks.tasksByWorkspace["ws-1"][0]}

	tasks, info, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("List() = %+v, want one task-1", tasks)
	}
	if info == nil || info.HasMore {
		t.Fatalf("PageInfo = %+v, want HasMore=false", info)
	}

	got, err := d.host.Tasks().Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got == nil || got.Title != "Task 1" {
		t.Fatalf("Get() = %+v, want Task 1", got)
	}

	// A missing task is (nil, nil), not an error, per grpcHostServer.GetTask's
	// contract of translating that into gRPC NotFound.
	got, err = d.host.Tasks().Get(context.Background(), "no-such-task")
	if err != nil || got != nil {
		t.Fatalf("Get() for missing task = (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestPluginHost_Workspaces_SucceedsWithCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"workspaces"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1", Name: "Workspace 1", OwnerID: "user-1"}}

	workspaces, _, err := d.host.Workspaces().List(context.Background(), pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].Name != "Workspace 1" {
		t.Fatalf("List() = %+v, want one Workspace 1", workspaces)
	}
}

func TestPluginHost_Workflows_SucceedsWithCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"workflows"}})
	d.workflows.workflows = map[string][]*taskmodels.Workflow{
		"ws-1": {{ID: "wf-1", WorkspaceID: "ws-1", Name: "Default"}},
	}
	d.steps.steps = map[string][]*wfmodels.WorkflowStep{
		"wf-1": {{ID: "step-1", WorkflowID: "wf-1", Name: "Todo", Position: 0, StageType: wfmodels.StageType("work")}},
	}

	workflows, _, err := d.host.Workflows().List(context.Background(), "ws-1", pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(workflows) != 1 || workflows[0].ID != "wf-1" {
		t.Fatalf("List() = %+v, want one wf-1", workflows)
	}

	steps, err := d.host.Workflows().ListSteps(context.Background(), "wf-1")
	if err != nil {
		t.Fatalf("ListSteps() unexpected error: %v", err)
	}
	if len(steps) != 1 || steps[0].StageType != "work" {
		t.Fatalf("ListSteps() = %+v, want one step with StageType=work", steps)
	}
}

func TestPluginHost_AgentProfiles_SucceedsWithCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"agent_profiles"}})
	d.profiles.resp = &agentsettingsdto.ListAgentsResponse{
		Agents: []agentsettingsdto.AgentDTO{
			{
				ID: "agent-1",
				Profiles: []agentsettingsdto.AgentProfileDTO{
					{ID: "profile-1", AgentID: "agent-1", Name: "Default", AgentDisplayName: "Claude", Model: "claude-x", Mode: "code"},
				},
			},
		},
	}

	profiles, _, err := d.host.AgentProfiles().List(context.Background(), pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(profiles) != 1 || profiles[0].DisplayName != "Claude" || profiles[0].Model != "claude-x" {
		t.Fatalf("List() = %+v, want one Claude/claude-x profile", profiles)
	}
}

func TestPluginHost_Repositories_SucceedsWithCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"repositories"}})
	branch := "main"
	d.tasks.repositories = map[string][]*taskmodels.Repository{
		"ws-1": {{ID: "repo-1", WorkspaceID: "ws-1", Name: "kandev", DefaultBranch: branch}},
	}

	repos, _, err := d.host.Repositories().List(context.Background(), "ws-1", pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "kandev" || repos[0].DefaultBranch == nil || *repos[0].DefaultBranch != "main" {
		t.Fatalf("List() = %+v, want one kandev repo on main", repos)
	}
}

// ── Session DTO mapping: acp_session_id sourcing ────────────────────────

func TestPluginHost_Sessions_ACPSessionIDFromMetadata(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}
	d.tasks.sessionsByTask = map[string][]*taskmodels.TaskSession{
		"task-1": {{
			ID:        "session-1",
			TaskID:    "task-1",
			State:     taskmodels.TaskSessionStateRunning,
			StartedAt: time.Now(),
			Metadata:  map[string]any{"acp": map[string]any{"session_id": "acp-from-metadata"}},
		}},
	}
	// Also seed an executors_running row so the test proves metadata wins
	// over the fallback when both are present.
	d.tasks.executorRunning = map[string]*taskmodels.ExecutorRunning{
		"session-1": {SessionID: "session-1", ResumeToken: "acp-from-fallback"},
	}

	sessions, _, err := d.host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ACPSessionID != "acp-from-metadata" {
		t.Fatalf("List() = %+v, want ACPSessionID=acp-from-metadata", sessions)
	}
}

func TestPluginHost_Sessions_ACPSessionIDFallsBackToExecutorRunning(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}
	d.tasks.sessionsByTask = map[string][]*taskmodels.TaskSession{
		"task-1": {{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning, StartedAt: time.Now()}},
	}
	d.tasks.executorRunning = map[string]*taskmodels.ExecutorRunning{
		"session-1": {SessionID: "session-1", ResumeToken: "acp-from-fallback"},
	}

	sessions, _, err := d.host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ACPSessionID != "acp-from-fallback" {
		t.Fatalf("List() = %+v, want ACPSessionID=acp-from-fallback", sessions)
	}
}

func TestPluginHost_Sessions_ACPSessionIDEmptyWhenNeitherSourceHasIt(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}
	d.tasks.sessionsByTask = map[string][]*taskmodels.TaskSession{
		"task-1": {{ID: "session-1", TaskID: "task-1", State: taskmodels.TaskSessionStateRunning, StartedAt: time.Now()}},
	}
	// No executorRunning entry for session-1: GetExecutorRunningBySessionID
	// returns ErrExecutorRunningNotFound, which must be swallowed.

	sessions, _, err := d.host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ACPSessionID != "" {
		t.Fatalf("List() = %+v, want empty ACPSessionID", sessions)
	}
}

// TestPluginHost_SessionsCodeStats_DelegatesToAnalyticsService proves
// Sessions().CodeStats reaches the injected analytics service with the
// filter translated 1:1, and maps its results back unchanged (ADR 0043(b):
// SessionCodeStats is a stable, computed shape returned as-is).
func TestPluginHost_SessionsCodeStats_DelegatesToAnalyticsService(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	d.codeStats.stats = []*analyticsmodels.SessionCodeStats{
		{SessionID: "session-1", LinesAddedCommitted: 10, LinesDeletedCommitted: 2, LinesAddedPeakPending: 5, LinesDeletedPeakPending: 1},
	}

	filter := pluginsdk.SessionFilter{TaskIDs: []string{"task-1"}, WorkspaceIDs: []string{"ws-1"}, States: []string{"RUNNING"}}
	stats, info, err := d.host.Sessions().CodeStats(context.Background(), filter, pluginsdk.Page{Limit: 10})
	if err != nil {
		t.Fatalf("CodeStats() unexpected error: %v", err)
	}
	if d.codeStats.calls != 1 {
		t.Fatalf("analytics service called %d times, want 1", d.codeStats.calls)
	}
	if len(d.codeStats.lastFilter.TaskIDs) != 1 || d.codeStats.lastFilter.TaskIDs[0] != "task-1" {
		t.Errorf("filter.TaskIDs = %v, want [task-1]", d.codeStats.lastFilter.TaskIDs)
	}
	if len(d.codeStats.lastFilter.WorkspaceIDs) != 1 || d.codeStats.lastFilter.WorkspaceIDs[0] != "ws-1" {
		t.Errorf("filter.WorkspaceIDs = %v, want [ws-1]", d.codeStats.lastFilter.WorkspaceIDs)
	}
	if len(d.codeStats.lastFilter.States) != 1 || d.codeStats.lastFilter.States[0] != "RUNNING" {
		t.Errorf("filter.States = %v, want [RUNNING]", d.codeStats.lastFilter.States)
	}
	// Limit is requested as limit+1 (the HasMore probe row) — see
	// sessionReader.CodeStats' doc comment.
	if d.codeStats.lastFilter.Limit != 11 {
		t.Errorf("filter.Limit = %d, want 11 (requested 10 + 1 probe row)", d.codeStats.lastFilter.Limit)
	}
	if len(stats) != 1 || stats[0].SessionID != "session-1" || stats[0].LinesAddedCommitted != 10 {
		t.Fatalf("CodeStats() = %+v, want session-1 passed through unchanged", stats)
	}
	if info == nil || info.HasMore {
		t.Fatalf("PageInfo = %+v, want HasMore=false", info)
	}
}

// TestPluginHost_SessionsCodeStats_HasMoreWhenExtraProbeRowReturned proves
// the limit+1 probe-row trick correctly reports HasMore and trims the extra
// row before returning results.
func TestPluginHost_SessionsCodeStats_HasMoreWhenExtraProbeRowReturned(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	d.codeStats.stats = []*analyticsmodels.SessionCodeStats{
		{SessionID: "session-1"}, {SessionID: "session-2"},
	}

	stats, info, err := d.host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{Limit: 1})
	if err != nil {
		t.Fatalf("CodeStats() unexpected error: %v", err)
	}
	if len(stats) != 1 || stats[0].SessionID != "session-1" {
		t.Fatalf("CodeStats() = %+v, want exactly one trimmed result", stats)
	}
	if info == nil || !info.HasMore || info.NextCursor != "1" {
		t.Fatalf("PageInfo = %+v, want HasMore=true NextCursor=1", info)
	}
}

func TestPluginHost_SessionsCodeStats_PropagatesAnalyticsError(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions"}})
	wantErr := errors.New("boom")
	d.codeStats.err = wantErr

	_, _, err := d.host.Sessions().CodeStats(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CodeStats() error = %v, want %v", err, wantErr)
	}
}

// ── DTO mapping ──────────────────────────────────────────────────────────

func TestTaskModelToDTO_MapsFields(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	task := &taskmodels.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Fix bug",
		Description: "details",
		State:       v1.TaskStateInProgress,
		Priority:    "high",
		Origin:      "agent_created",
		CreatedAt:   created,
		UpdatedAt:   created,
		ParentID:    "parent-1",
		Identifier:  "KAN-1",
		IsEphemeral: false,
		Repositories: []*taskmodels.TaskRepository{
			{ID: "tr-1", RepositoryID: "repo-1", BaseBranch: "main", Position: 0},
		},
		Metadata: map[string]any{"k": "v"},
	}

	dto := taskModelToDTO(task)

	if dto.ID != "task-1" || dto.State != "IN_PROGRESS" || dto.CreatedBy != "agent_created" {
		t.Fatalf("taskModelToDTO() = %+v, unexpected core fields", dto)
	}
	if dto.CreatedAt != created.Format(time.RFC3339) {
		t.Errorf("CreatedAt = %q, want RFC3339 %q", dto.CreatedAt, created.Format(time.RFC3339))
	}
	if dto.ParentID == nil || *dto.ParentID != "parent-1" {
		t.Errorf("ParentID = %v, want parent-1", dto.ParentID)
	}
	if len(dto.Repositories) != 1 || dto.Repositories[0].RepositoryID != "repo-1" {
		t.Errorf("Repositories = %+v, want one repo-1", dto.Repositories)
	}
	if dto.Metadata["k"] != "v" {
		t.Errorf("Metadata = %+v, want k=v", dto.Metadata)
	}
}

func TestTaskModelToDTO_EmptyParentIDIsNil(t *testing.T) {
	dto := taskModelToDTO(&taskmodels.Task{ID: "task-1"})
	if dto.ParentID != nil {
		t.Fatalf("ParentID = %v, want nil for a root task", dto.ParentID)
	}
}

// ── Archived-task visibility ────────────────────────────────────────────

// Sessions().List must fetch tasks WITH archived included (an archived
// task's sessions are still real sessions), while Tasks().List keeps
// archived tasks out of plugin-visible work items.
func TestPluginHost_ArchivedTaskFetchFlags(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"sessions", "tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}

	if _, _, err := d.host.Sessions().List(context.Background(), pluginsdk.SessionFilter{}, pluginsdk.Page{}); err != nil {
		t.Fatalf("Sessions().List unexpected error: %v", err)
	}
	if _, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{}); err != nil {
		t.Fatalf("Tasks().List unexpected error: %v", err)
	}

	want := []bool{true, false} // sessions read first (archived in), tasks read second (archived out)
	if len(d.tasks.gotIncludeArchived) != 2 ||
		d.tasks.gotIncludeArchived[0] != want[0] || d.tasks.gotIncludeArchived[1] != want[1] {
		t.Fatalf("includeArchived per call = %v, want %v", d.tasks.gotIncludeArchived, want)
	}
}
