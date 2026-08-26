package plugins

import (
	"context"
	"testing"
	"time"

	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/plugins/manifest"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

// The step a task stands on, its labels, and whether it is archived were all
// unreadable from a plugin before this. A plugin could see a task's State
// (created / in progress / review / completed) but not WHERE on the board it
// was, so it could not render or group by a workflow at all.
func TestTaskModelToDTOCarriesBoardAndPlanningFields(t *testing.T) {
	archived := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	queued := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{
		ID:                     "t1",
		WorkflowStepID:         "step-pr-review",
		Position:               3,
		AssigneeAgentProfileID: "agent-opus",
		Labels:                 `["office","unblock"]`,
		Autopilot:              true,
		WIPAdmitted:            true,
		QueuedForStepID:        "step-build",
		QueuedAt:               &queued,
		ProjectID:              "proj-1",
		ExternalID:             "WO-101",
		ArchivedAt:             &archived,
	}

	dto := taskModelToDTO(task)

	require.Equal(t, "step-pr-review", dto.WorkflowStepID)
	require.Equal(t, int32(3), dto.Position)
	require.Equal(t, "agent-opus", dto.AssigneeAgentProfileID)
	require.Equal(t, []string{"office", "unblock"}, dto.Labels)
	require.True(t, dto.Autopilot)
	require.True(t, dto.WIPAdmitted)
	require.Equal(t, "step-build", dto.QueuedForStepID)
	require.NotNil(t, dto.QueuedAt)
	require.Equal(t, "2026-08-19T09:00:00Z", *dto.QueuedAt)
	require.Equal(t, "proj-1", dto.ProjectID)
	require.Equal(t, "WO-101", dto.ExternalID)
	require.NotNil(t, dto.ArchivedAt)
	require.Equal(t, "2026-08-20T10:00:00Z", *dto.ArchivedAt)
}

// A live task must report no archive timestamp at all, not a zero time — the
// difference is exactly how a plugin tells delivered work from in-flight work.
func TestTaskModelToDTOLeavesArchivedAtNilForALiveTask(t *testing.T) {
	dto := taskModelToDTO(&taskmodels.Task{ID: "t1"})
	require.Nil(t, dto.ArchivedAt)
	require.Nil(t, dto.QueuedAt)
}

// Labels are a JSON-array string on the model. A row holding something
// unparseable must cost that row its labels, never the whole task read.
func TestDecodeTaskLabelsToleratesEmptyAndMalformedValues(t *testing.T) {
	require.Equal(t, []string{"a", "b"}, decodeTaskLabels(`["a","b"]`))
	require.Nil(t, decodeTaskLabels(""))
	require.Nil(t, decodeTaskLabels("[]"))
	require.Nil(t, decodeTaskLabels("  "))
	require.Nil(t, decodeTaskLabels("not json"))
	require.Nil(t, decodeTaskLabels(`{"a":1}`))
}

// Review and check state ride along because kandev's PR watcher already syncs
// them; a plugin asking "what can merge" would otherwise re-query the forge
// once per pull request.
func TestTaskPRsToDTOsCarriesReadinessNotJustIdentity(t *testing.T) {
	merged := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	draft := true
	prs := []*githubsvc.TaskPR{{
		PRNumber: 3019, PRURL: "https://example/3019", PRTitle: "expose comments",
		State: "open", HeadBranch: "feature/x", BaseBranch: "main",
		IsDraft: &draft, AuthorLogin: "nova28",
		ReviewState: "changes_requested", ChecksState: "failure", MergeableState: "blocked",
		UnresolvedReviewThreads: 3, ChecksTotal: 12, ChecksPassing: 10,
		Additions: 400, Deletions: 25, MergedAt: &merged,
	}}

	out := taskPRsToDTOs(prs)

	require.Len(t, out, 1)
	require.Equal(t, int64(3019), out[0].Number)
	require.Equal(t, "github", out[0].Provider)
	require.True(t, out[0].IsDraft)
	require.Equal(t, "changes_requested", out[0].ReviewState)
	require.Equal(t, "failure", out[0].ChecksState)
	require.Equal(t, int32(3), out[0].UnresolvedReviewThreads)
	require.Equal(t, int32(10), out[0].ChecksPassing)
	require.NotNil(t, out[0].MergedAt)
	require.Equal(t, "2026-08-21T12:00:00Z", *out[0].MergedAt)
}

func TestTaskPRsToDTOsReturnsNewestFirst(t *testing.T) {
	prs := []*githubsvc.TaskPR{
		{PRNumber: 100, PRTitle: "older", CreatedAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)},
		{PRNumber: 200, PRTitle: "newer", CreatedAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)},
	}

	out := taskPRsToDTOs(prs)

	require.Len(t, out, 2)
	require.Equal(t, int64(200), out[0].Number)
	require.Equal(t, "newer", out[0].Title)
	require.Equal(t, int64(100), out[1].Number)
}

// IsDraft is nullable on the row: unknown before the first sync. Nil must read
// as not-draft rather than panicking the whole task list.
func TestTaskPRsToDTOsTreatsAnUnknownDraftFlagAsNotDraft(t *testing.T) {
	out := taskPRsToDTOs([]*githubsvc.TaskPR{{PRNumber: 1}, nil})
	require.Len(t, out, 1, "a nil row is skipped, not mapped")
	require.False(t, out[0].IsDraft)
}

type stubPRSource struct {
	byTask map[string][]*githubsvc.TaskPR
	err    error
	calls  int
}

func (s *stubPRSource) ListTaskPRsByTaskIDs(_ context.Context, ids []string) (map[string][]*githubsvc.TaskPR, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.byTask, nil
}

func TestAttachPullRequestsFillsOnlyTheTasksThatHaveThem(t *testing.T) {
	src := &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{
		"t1": {{PRNumber: 2907, State: "open"}},
	}}
	h := &pluginHost{taskPRs: src}
	tasks := []pluginsdk.Task{{ID: "t1"}, {ID: "t2"}}

	h.attachPullRequests(context.Background(), tasks)

	require.Len(t, tasks[0].PullRequests, 1)
	require.Equal(t, int64(2907), tasks[0].PullRequests[0].Number)
	require.Empty(t, tasks[1].PullRequests)
	require.Equal(t, 1, src.calls, "one batched lookup, not one per task")
}

// Pull requests are supplementary to a task read. Refusing the whole list when
// the lookup fails would take away more than it protects.
func TestAttachPullRequestsIsSilentWhenTheSourceIsMissingOrFailing(t *testing.T) {
	tasks := []pluginsdk.Task{{ID: "t1"}}

	(&pluginHost{}).attachPullRequests(context.Background(), tasks)
	require.Empty(t, tasks[0].PullRequests)

	failing := &stubPRSource{err: context.DeadlineExceeded}
	(&pluginHost{taskPRs: failing}).attachPullRequests(context.Background(), tasks)
	require.Empty(t, tasks[0].PullRequests)
	require.Equal(t, 1, failing.calls)
}

func TestTaskReaderGetAttachesPullRequests(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	task := &taskmodels.Task{ID: "task-1", Title: "with a PR"}
	d.tasks.tasksByID = map[string]*taskmodels.Task{"task-1": task}
	src := &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{
		"task-1": {{PRNumber: 42, PRTitle: "fix"}},
	}}
	d.host.taskPRs = src

	got, err := d.host.Tasks().Get(context.Background(), "task-1")

	require.NoError(t, err)
	require.Len(t, got.PullRequests, 1)
	require.Equal(t, int64(42), got.PullRequests[0].Number)
}

func TestAttachPullRequestsUsesSourceWiredAfterHostCreation(t *testing.T) {
	var source taskPRSource
	h := &pluginHost{taskPRsDep: func() taskPRSource { return source }}
	tasks := []pluginsdk.Task{{ID: "task-1"}}

	h.attachPullRequests(context.Background(), tasks)
	require.Empty(t, tasks[0].PullRequests)

	source = &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{
		"task-1": {{PRNumber: 99}},
	}}
	h.attachPullRequests(context.Background(), tasks)

	require.Len(t, tasks[0].PullRequests, 1)
	require.Equal(t, int64(99), tasks[0].PullRequests[0].Number)
}

// A board plugin reads the step's own colour rather than inventing a palette,
// so its columns match what the operator already sees in kandev.
func TestWorkflowStepModelToDTOCarriesPresentationAndCapacity(t *testing.T) {
	step := wfmodels.WorkflowStep{
		ID: "s1", Name: "PR Review", Position: 8, Color: "bg-indigo-500",
		IsStartStep: true, WIPLimit: 3, AgentProfileID: "agent-1",
	}
	dto := workflowStepModelToDTO(&step)
	require.Equal(t, "bg-indigo-500", dto.Color)
	require.True(t, dto.IsStartStep)
	require.Equal(t, int32(3), dto.WIPLimit)
	require.Equal(t, "agent-1", dto.AgentProfileID)
}

// Archived tasks were unreachable from a plugin: the fetch hardcoded
// includeArchived=false and TaskFilter had no way to ask. A plugin reporting on
// delivery needs them — an archived task is usually a delivered one, so leaving
// them out makes finished work look like it never happened.
func TestTasksListHonoursIncludeArchivedFromTheFilter(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}

	_, _, err := d.host.Tasks().List(context.Background(),
		pluginsdk.TaskFilter{IncludeArchived: true}, pluginsdk.Page{})
	require.NoError(t, err)
	require.Equal(t, []bool{true}, d.tasks.gotIncludeArchived)
}

// Not asking still means live tasks only, so a plugin written before the field
// existed keeps exactly the visibility it had.
func TestTasksListDefaultsToLiveTasksOnly(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"tasks"}})
	d.tasks.workspaces = []*taskmodels.Workspace{{ID: "ws-1"}}
	d.tasks.tasksByWorkspace = map[string][]*taskmodels.Task{"ws-1": {{ID: "task-1", WorkspaceID: "ws-1"}}}

	_, _, err := d.host.Tasks().List(context.Background(), pluginsdk.TaskFilter{}, pluginsdk.Page{})
	require.NoError(t, err)
	require.Equal(t, []bool{false}, d.tasks.gotIncludeArchived)
}
