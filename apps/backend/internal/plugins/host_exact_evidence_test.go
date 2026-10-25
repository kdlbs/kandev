package plugins

import (
	"context"
	"testing"
	"time"

	githubsvc "github.com/kandev/kandev/internal/github"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactObservationTestData struct {
	*fakeTaskDataSource
	statusSummaries map[string]*statussummary.TaskStatusSummary
	taskUsage       map[string]*taskmodels.TaskUsageTotals
	sessionUsage    map[string]*taskmodels.TaskUsageTotals
}

func (d *exactObservationTestData) GetTaskStatusSummaries(context.Context, []string) (map[string]*statussummary.TaskStatusSummary, error) {
	return d.statusSummaries, nil
}

func (d *exactObservationTestData) GetTaskUsageTotals(_ context.Context, taskID string) (*taskmodels.TaskUsageTotals, error) {
	return d.taskUsage[taskID], nil
}

func (d *exactObservationTestData) GetTaskSessionUsageTotals(_ context.Context, _, sessionID string) (*taskmodels.TaskUsageTotals, error) {
	return d.sessionUsage[sessionID], nil
}

func TestExactEvidenceAndUsage(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	taskOne := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	taskTwo := &taskmodels.Task{ID: "task-two", WorkspaceID: "workspace-one", State: "review", UpdatedAt: base.Add(time.Second)}
	session := &taskmodels.TaskSession{ID: "session-one", TaskID: taskOne.ID, State: "completed", UpdatedAt: base.Add(2 * time.Second)}
	data := &exactObservationTestData{
		fakeTaskDataSource: &fakeTaskDataSource{
			workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
			tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {taskOne, taskTwo}},
			tasksByID:        map[string]*taskmodels.Task{taskOne.ID: taskOne, taskTwo.ID: taskTwo},
			sessionsByTask:   map[string][]*taskmodels.TaskSession{taskOne.ID: {session}},
		},
		taskUsage: map[string]*taskmodels.TaskUsageTotals{
			taskOne.ID: {TokensIn: 100, TokensOut: 50, TokensTotal: 150, CostSubcents: 12, EventCount: 2, EstimatedEventCount: 1, UnpricedEventCount: 1, OutputTokensComplete: false, FirstEventAt: timePtr(base), LastEventAt: timePtr(base.Add(time.Minute))},
			taskTwo.ID: {OutputTokensComplete: true},
		},
		sessionUsage: map[string]*taskmodels.TaskUsageTotals{session.ID: {OutputTokensComplete: true}},
	}
	host := newExactReadHost(t, data,
		[]string{"tasks", "change_requests", "usage"},
		[]string{"host.v2.read:tasks", "host.v2.read:change_requests", "host.v2.read:usage"},
	)
	secondPRSync := base.Add(3 * time.Minute)
	firstPR := &githubsvc.TaskPR{ID: "pr-one", TaskID: taskOne.ID, RepositoryID: "repo-one", Owner: "acme", Repo: "widgets", PRNumber: 7, PRURL: "https://example.test/pull/7", HeadSHA: "head-one", ReviewState: "approved", ChecksState: "success", ChecksTotal: 2, ChecksPassing: 2, UpdatedAt: base.Add(time.Second), LastSyncedAt: &secondPRSync}
	secondPR := &githubsvc.TaskPR{ID: "pr-two", TaskID: taskOne.ID, RepositoryID: "repo-one", Owner: "acme", Repo: "widgets", PRNumber: 8, PRURL: "https://example.test/pull/8", HeadSHA: "head-two-old", ReviewState: "pending", ChecksState: "pending", UpdatedAt: base.Add(2 * time.Second), LastSyncedAt: &secondPRSync}
	prSource := &stubPRSource{byTask: map[string][]*githubsvc.TaskPR{taskOne.ID: {firstPR, secondPR}}}
	host.taskPRs = prSource
	host.taskPRsDep = nil

	firstPage, err := host.ListChangeRequestEvidence(context.Background(), pluginsdk.ExactChangeRequestEvidenceQuery{
		RequestID: "evidence-first-page", WorkspaceID: "workspace-one", TaskIDs: []string{taskOne.ID}, Page: pluginsdk.ExactReadPage{Limit: 1},
	})
	if err != nil {
		t.Fatalf("ListChangeRequestEvidence first page: %v", err)
	}
	if len(firstPage.Items) != 1 || firstPage.Items[0].HeadRevision != "head-one" || firstPage.Items[0].ProviderState != "available" || firstPage.Items[0].FetchedAt == nil {
		t.Fatalf("first change evidence = %+v, want fetched exact-head evidence", firstPage.Items)
	}
	oldHeadVersion := firstPage.Items[0].ResourceVersion
	if oldHeadVersion == "" || firstPage.PageInfo.NextCursor == "" {
		t.Fatalf("first evidence page info = %+v, want resource and cursor versions", firstPage.PageInfo)
	}
	secondPR.HeadSHA = "head-two-new"
	secondPR.UpdatedAt = base.Add(4 * time.Second)
	secondPage, err := host.ListChangeRequestEvidence(context.Background(), pluginsdk.ExactChangeRequestEvidenceQuery{
		RequestID: "evidence-second-page", WorkspaceID: "workspace-one", TaskIDs: []string{taskOne.ID},
		Page: pluginsdk.ExactReadPage{Limit: 1, Cursor: firstPage.PageInfo.NextCursor, SnapshotVersion: firstPage.PageInfo.SnapshotVersion},
	})
	if err != nil {
		t.Fatalf("ListChangeRequestEvidence second page: %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].HeadRevision != "head-two-old" {
		t.Fatalf("second evidence page = %+v, want the original head-bound snapshot", secondPage.Items)
	}
	refreshed, err := host.ListChangeRequestEvidence(context.Background(), pluginsdk.ExactChangeRequestEvidenceQuery{
		RequestID: "evidence-refresh", WorkspaceID: "workspace-one", TaskIDs: []string{taskOne.ID}, Page: pluginsdk.ExactReadPage{Limit: 2},
	})
	if err != nil || len(refreshed.Items) != 2 || refreshed.Items[1].HeadRevision != "head-two-new" || refreshed.Items[1].ResourceVersion == secondPage.Items[0].ResourceVersion {
		t.Fatalf("refreshed evidence = %+v, err=%v; want changed head and version", refreshed.Items, err)
	}

	usage, err := host.ListTaskUsage(context.Background(), pluginsdk.ExactTaskUsageQuery{
		RequestID: "usage-read", WorkspaceID: "workspace-one", TaskIDs: []string{taskOne.ID}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if err != nil {
		t.Fatalf("ListTaskUsage: %v", err)
	}
	if len(usage.Items) != 2 {
		t.Fatalf("usage rows = %+v, want one task and one session aggregate", usage.Items)
	}
	if usage.Items[0].EventCount != 2 || usage.Items[0].TokensTotal != 150 || usage.Items[0].CostSubcents == nil || *usage.Items[0].CostSubcents != 12 || usage.Items[0].CostComplete || usage.Items[0].CostUnknownReason == nil || *usage.Items[0].CostUnknownReason != "unpriced_events" {
		t.Fatalf("task usage = %+v, want preserved partial ledger totals and explicit unknown cost", usage.Items[0])
	}
	if usage.Items[1].ObservationState != "no_observations" || usage.Items[1].CostSubcents != nil || usage.Items[1].CostUnknownReason == nil {
		t.Fatalf("session usage = %+v, want explicit no-observation state and unknown cost", usage.Items[1])
	}

	unsupportedHost := newExactReadHost(t, data, []string{"tasks", "change_requests"}, []string{"host.v2.read:tasks", "host.v2.read:change_requests"})
	unsupported, err := unsupportedHost.ListChangeRequestEvidence(context.Background(), pluginsdk.ExactChangeRequestEvidenceQuery{
		RequestID: "provider-unsupported", WorkspaceID: "workspace-one", TaskIDs: []string{taskTwo.ID}, Page: pluginsdk.ExactReadPage{Limit: 1},
	})
	if err != nil || len(unsupported.Items) != 1 || unsupported.Items[0].ProviderState != "unsupported" {
		t.Fatalf("unsupported provider evidence = %+v, err=%v; want explicit unsupported state", unsupported.Items, err)
	}
}

func TestExactUsageNilTotalsFailClosed(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	task := &taskmodels.Task{ID: "task-one", WorkspaceID: "workspace-one", State: "in_progress", UpdatedAt: base}
	data := &exactObservationTestData{fakeTaskDataSource: &fakeTaskDataSource{
		workspaces:       []*taskmodels.Workspace{{ID: "workspace-one"}},
		tasksByWorkspace: map[string][]*taskmodels.Task{"workspace-one": {task}},
	}}
	host := newExactReadHost(t, data, []string{"tasks", "usage"}, []string{"host.v2.read:tasks", "host.v2.read:usage"})
	_, err := host.ListTaskUsage(context.Background(), pluginsdk.ExactTaskUsageQuery{
		RequestID: "nil-usage", WorkspaceID: "workspace-one", TaskIDs: []string{task.ID}, Page: pluginsdk.ExactReadPage{Limit: 10},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("nil usage totals error = %v, want Unavailable", err)
	}
}

func timePtr(value time.Time) *time.Time { return &value }
