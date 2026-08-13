package azuredevops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func validWorkItemWatchRequest() *CreateWorkItemWatchRequest {
	return &CreateWorkItemWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		ProjectID: "project-1", WIQL: "SELECT [System.Id] FROM WorkItems",
		RepositoryID: "repo-1", BaseBranch: "main",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
		Prompt: "Fix {{title}}",
	}
}

func TestCreateWorkItemWatchRejectsIncompleteRequests(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	if _, err := service.CreateWorkItemWatch(t.Context(), nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil request error = %v, want ErrInvalidConfig", err)
	}
	blank := validWorkItemWatchRequest()
	blank.WorkspaceID = ""
	if _, err := service.CreateWorkItemWatch(t.Context(), blank); !errors.Is(err, ErrInvalidWorkspaceID) {
		t.Fatalf("blank workspace error = %v, want ErrInvalidWorkspaceID", err)
	}
	noStep := validWorkItemWatchRequest()
	noStep.WorkflowStepID = ""
	if _, err := service.CreateWorkItemWatch(t.Context(), noStep); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing step error = %v, want ErrInvalidConfig", err)
	}
	noRepository := validWorkItemWatchRequest()
	noRepository.RepositoryID = ""
	if _, err := service.CreateWorkItemWatch(t.Context(), noRepository); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing repository error = %v, want ErrInvalidConfig", err)
	}
	noWIQL := validWorkItemWatchRequest()
	noWIQL.WIQL = ""
	if _, err := service.CreateWorkItemWatch(t.Context(), noWIQL); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing WIQL error = %v, want ErrInvalidConfig", err)
	}
	if watches, err := store.ListWorkItemWatches(t.Context(), "ws-1"); err != nil || len(watches) != 0 {
		t.Fatalf("rejected requests persisted watches: %+v %v", watches, err)
	}
}

// fakeWatchDependencies fails whichever dependency check the test names, so
// the create/update paths can be pinned to fail closed per reference.
type fakeWatchDependencies struct {
	stepOK     bool
	agentOK    bool
	executorOK bool
	err        error
}

func (f fakeWatchDependencies) WorkflowStepBelongs(context.Context, string, string, string) (bool, error) {
	return f.stepOK, f.err
}

func (f fakeWatchDependencies) AgentProfileBelongs(context.Context, string, string) (bool, error) {
	return f.agentOK, f.err
}

func (f fakeWatchDependencies) ExecutorProfileBelongs(context.Context, string, string) (bool, error) {
	return f.executorOK, f.err
}

type fakeWatchRepositories struct {
	workspaceID   string
	defaultBranch string
	ok            bool
}

func (f fakeWatchRepositories) GetRepository(context.Context, string) (string, string, bool) {
	return f.workspaceID, f.defaultBranch, f.ok
}

func TestCreateWorkItemWatchFailsClosedOnCrossWorkspaceReferences(t *testing.T) {
	tests := []struct {
		name       string
		validator  WatchDependencyValidator
		repository WatchRepositoryLookup
	}{
		{
			name:      "workflow step from another workspace",
			validator: fakeWatchDependencies{agentOK: true, executorOK: true},
		},
		{
			name:      "agent profile from another workspace",
			validator: fakeWatchDependencies{stepOK: true, executorOK: true},
		},
		{
			name:      "executor profile from another workspace",
			validator: fakeWatchDependencies{stepOK: true, agentOK: true},
		},
		{
			name:       "repository from another workspace",
			validator:  fakeWatchDependencies{stepOK: true, agentOK: true, executorOK: true},
			repository: fakeWatchRepositories{workspaceID: "ws-other", ok: true},
		},
		{
			name:       "repository does not exist",
			validator:  fakeWatchDependencies{stepOK: true, agentOK: true, executorOK: true},
			repository: fakeWatchRepositories{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, store := newWatchService(t, &watchClient{})
			service.SetWatchDependencyValidator(tt.validator)
			if tt.repository != nil {
				service.SetWatchRepositoryLookup(tt.repository)
			}
			if _, err := service.CreateWorkItemWatch(t.Context(), validWorkItemWatchRequest()); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
			if watches, _ := store.ListWorkItemWatches(t.Context(), "ws-1"); len(watches) != 0 {
				t.Fatalf("watch persisted despite an invalid reference: %+v", watches)
			}
		})
	}
}

func TestCreateWorkItemWatchSurfacesDependencyLookupErrors(t *testing.T) {
	service, _ := newWatchService(t, &watchClient{})
	lookupErr := errors.New("workflow store unavailable")
	service.SetWatchDependencyValidator(fakeWatchDependencies{err: lookupErr})
	if _, err := service.CreateWorkItemWatch(t.Context(), validWorkItemWatchRequest()); !errors.Is(err, lookupErr) {
		t.Fatalf("error = %v, want the lookup failure", err)
	}
}

func TestCreateWorkItemWatchAdoptsRepositoryDefaultBranch(t *testing.T) {
	service, _ := newWatchService(t, &watchClient{})
	service.SetWatchRepositoryLookup(fakeWatchRepositories{workspaceID: "ws-1", defaultBranch: "trunk", ok: true})
	req := validWorkItemWatchRequest()
	req.BaseBranch = ""

	watch, err := service.CreateWorkItemWatch(t.Context(), req)
	if err != nil {
		t.Fatalf("CreateWorkItemWatch: %v", err)
	}
	if watch.BaseBranch != "trunk" {
		t.Fatalf("base branch = %q, want the repository default", watch.BaseBranch)
	}

	req = validWorkItemWatchRequest()
	req.BaseBranch = "not a ref^"
	if _, err := service.CreateWorkItemWatch(t.Context(), req); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid base branch error = %v, want ErrInvalidConfig", err)
	}
}

func TestUpdateWorkItemWatchAppliesEveryRequestedField(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedWorkItemWatch(t, store)

	str := func(value string) *string { return &value }
	enabled := false
	interval := 30
	maxInflight := 2
	updated, err := service.UpdateWorkItemWatch(t.Context(), "ws-1", watch.ID, &UpdateWorkItemWatchRequest{
		WorkflowID: str("wf-2"), WorkflowStepID: str("step-2"), ProjectID: str("project-2"),
		WIQL: str("SELECT [System.Id] FROM WorkItemLinks"), RepositoryID: str("repo-2"),
		BaseBranch: str("develop"), AgentProfileID: str("agent-2"),
		ExecutorProfileID: str("executor-2"), Prompt: str("Investigate {{title}}"),
		Enabled: &enabled, PollIntervalSeconds: &interval,
		CleanupPolicy: str(CleanupPolicyNever), MaxInflightTasks: &maxInflight,
	})
	if err != nil {
		t.Fatalf("UpdateWorkItemWatch: %v", err)
	}
	if updated.WorkflowID != "wf-2" || updated.WorkflowStepID != "step-2" ||
		updated.ProjectID != "project-2" || updated.WIQL != "SELECT [System.Id] FROM WorkItemLinks" ||
		updated.RepositoryID != "repo-2" || updated.BaseBranch != "develop" ||
		updated.AgentProfileID != "agent-2" || updated.ExecutorProfileID != "executor-2" ||
		updated.Prompt != "Investigate {{title}}" || updated.Enabled ||
		updated.CleanupPolicy != CleanupPolicyNever ||
		updated.MaxInflightTasks == nil || *updated.MaxInflightTasks != 2 {
		t.Fatalf("updated watch = %+v", updated)
	}
	// Sub-minimum intervals are clamped rather than accepted verbatim.
	if updated.PollIntervalSeconds != minWatchPollIntervalSeconds {
		t.Fatalf("poll interval = %d, want the clamped minimum %d", updated.PollIntervalSeconds, minWatchPollIntervalSeconds)
	}

	unlimited := 0
	cleared, err := service.UpdateWorkItemWatch(t.Context(), "ws-1", watch.ID, &UpdateWorkItemWatchRequest{
		MaxInflightTasks: &unlimited,
	})
	if err != nil || cleared.MaxInflightTasks != nil {
		t.Fatalf("cleared watch = %+v, %v", cleared, err)
	}
	persisted, err := store.GetWorkItemWatch(t.Context(), watch.ID)
	if err != nil || persisted.WIQL != "SELECT [System.Id] FROM WorkItemLinks" || persisted.MaxInflightTasks != nil {
		t.Fatalf("persisted watch = %+v, %v", persisted, err)
	}
}

func TestWorkItemWatchOperationsRejectOtherWorkspaces(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedWorkItemWatch(t, store)
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-2", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/other", PAT: "pat",
	}); err != nil {
		t.Fatalf("set second config: %v", err)
	}

	operations := map[string]func(id string) error{
		"trigger": func(id string) error {
			_, err := service.TriggerWorkItemWatch(t.Context(), "ws-2", id)
			return err
		},
		"update": func(id string) error {
			_, err := service.UpdateWorkItemWatch(t.Context(), "ws-2", id, &UpdateWorkItemWatchRequest{})
			return err
		},
		"delete": func(id string) error { return service.DeleteWorkItemWatch(t.Context(), "ws-2", id) },
		"preview reset": func(id string) error {
			_, err := service.PreviewResetWorkItemWatch(t.Context(), "ws-2", id)
			return err
		},
		"reset": func(id string) error {
			_, err := service.ResetWorkItemWatch(t.Context(), "ws-2", id)
			return err
		},
		"cleanup": func(id string) error {
			_, err := service.CleanupWorkItemWatch(t.Context(), "ws-2", id)
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(watch.ID); !errors.Is(err, ErrWatchNotFound) {
				t.Fatalf("cross-workspace %s error = %v, want ErrWatchNotFound", name, err)
			}
		})
	}
	if remaining, err := store.GetWorkItemWatch(t.Context(), watch.ID); err != nil || remaining == nil {
		t.Fatalf("watch was removed by a foreign-workspace call: %+v %v", remaining, err)
	}
}

func TestDeleteWorkItemWatchCascadesItsCreatedTasks(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedWorkItemWatch(t, store)
	deleter := &recordingDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	seedWorkItemReservation(t, store, watch, 101, "task-101")
	seedWorkItemReservation(t, store, watch, 102, "")

	if err := service.DeleteWorkItemWatch(t.Context(), "ws-1", watch.ID); err != nil {
		t.Fatalf("DeleteWorkItemWatch: %v", err)
	}
	if ids := deleter.ids(); len(ids) != 1 || ids[0] != "task-101" {
		t.Fatalf("cascade-deleted task ids = %v, want [task-101]", ids)
	}
	if got, err := store.GetWorkItemWatch(t.Context(), watch.ID); err != nil || got != nil {
		t.Fatalf("watch after delete = %+v, %v", got, err)
	}
}

// TestDeleteWorkItemWatchWithoutCascadeDeleterStillRemovesTheWatch keeps the
// local-development path (no task service wired) usable.
func TestDeleteWorkItemWatchWithoutCascadeDeleterStillRemovesTheWatch(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedWorkItemWatch(t, store)
	seedWorkItemReservation(t, store, watch, 101, "task-101")

	if err := service.DeleteWorkItemWatch(t.Context(), "ws-1", watch.ID); err != nil {
		t.Fatalf("DeleteWorkItemWatch: %v", err)
	}
	if got, _ := store.GetWorkItemWatch(t.Context(), watch.ID); got != nil {
		t.Fatalf("watch survived deletion: %+v", got)
	}
}

func seedWorkItemReservation(t *testing.T, store *Store, watch *WorkItemWatch, workItemID int, taskID string) {
	t.Helper()
	reserved, err := store.ReserveWorkItemWatchTask(
		t.Context(), watch.ID, watch.Generation, watch.ProjectID, workItemID, "https://azure/item",
	)
	if err != nil || !reserved {
		t.Fatalf("reserve work item %d: reserved=%v err=%v", workItemID, reserved, err)
	}
	if taskID == "" {
		return
	}
	if err := store.AssignWorkItemWatchTaskID(
		t.Context(), watch.ID, watch.Generation, watch.ProjectID, workItemID, taskID,
	); err != nil {
		t.Fatalf("assign task %q: %v", taskID, err)
	}
}

// TestTriggerWatchRecordsUpstreamFailureOnTheWatchRow proves a failing check
// is surfaced to the user through last_error rather than silently retried.
func TestTriggerWatchRecordsUpstreamFailureOnTheWatchRow(t *testing.T) {
	upstream := errors.New("401 unauthorized")
	client := &watchClient{workErr: upstream, pageErr: upstream}
	service, store := newWatchService(t, client)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)

	if _, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", work.ID); !errors.Is(err, upstream) {
		t.Fatalf("work-item trigger error = %v, want the upstream failure", err)
	}
	if _, err := service.TriggerPullRequestWatch(t.Context(), "ws-1", pr.ID); !errors.Is(err, upstream) {
		t.Fatalf("pull-request trigger error = %v, want the upstream failure", err)
	}
	gotWork, _ := store.GetWorkItemWatch(t.Context(), work.ID)
	if gotWork.LastError != "401 unauthorized" || gotWork.LastErrorAt == nil || !gotWork.Enabled {
		t.Fatalf("work-item watch after failure = %+v", gotWork)
	}
	gotPR, _ := store.GetPullRequestWatch(t.Context(), pr.ID)
	if gotPR.LastError != "401 unauthorized" || gotPR.LastErrorAt == nil {
		t.Fatalf("pull-request watch after failure = %+v", gotPR)
	}
}

// TestTriggerWatchRecordsMissingCredentials covers the credential-resolution
// hop, which happens before any upstream call.
func TestTriggerWatchRecordsMissingCredentials(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	if err := service.DeleteConfigForWorkspace(t.Context(), "ws-1"); err != nil {
		t.Fatalf("delete config: %v", err)
	}

	if _, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", work.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("work-item trigger error = %v, want ErrNotConfigured", err)
	}
	if _, err := service.TriggerPullRequestWatch(t.Context(), "ws-1", pr.ID); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("pull-request trigger error = %v, want ErrNotConfigured", err)
	}
	gotWork, _ := store.GetWorkItemWatch(t.Context(), work.ID)
	gotPR, _ := store.GetPullRequestWatch(t.Context(), pr.ID)
	if gotWork.LastError != ErrNotConfigured.Error() || gotPR.LastError != ErrNotConfigured.Error() {
		t.Fatalf("watch errors = %q / %q", gotWork.LastError, gotPR.LastError)
	}
}

// TestTriggerWatchReleasesReservationWhenPublishFails is the retry-safety
// property: a match whose event could not be published must not stay claimed,
// otherwise the item is silently never imported.
func TestTriggerWatchReleasesReservationWhenPublishFails(t *testing.T) {
	client := &watchClient{
		work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101, WebURL: "https://azure/item/101"}}},
		page: &PullRequestPage{Items: []PullRequest{{ID: 42, ProjectID: "project-1", RepositoryID: "azure-repo-1"}}},
	}
	service, store := newWatchService(t, client)
	service.SetEventBus(&failingEventBus{err: errors.New("bus unavailable")})
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)

	result, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", work.ID)
	if err != nil || result.Matched != 0 {
		t.Fatalf("work-item trigger = %+v, %v; want zero matches without a hard error", result, err)
	}
	if rows, err := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation); err != nil || len(rows) != 0 {
		t.Fatalf("work-item reservation stayed claimed after a publish failure: %+v %v", rows, err)
	}
	gotWork, _ := store.GetWorkItemWatch(t.Context(), work.ID)
	if gotWork.LastError != "one or more watcher matches could not be published" {
		t.Fatalf("work-item watch error = %q", gotWork.LastError)
	}

	prResult, err := service.TriggerPullRequestWatch(t.Context(), "ws-1", pr.ID)
	if err != nil || prResult.Matched != 0 {
		t.Fatalf("pull-request trigger = %+v, %v", prResult, err)
	}
	if rows, err := store.ListPullRequestWatchTasks(t.Context(), pr.ID, pr.Generation); err != nil || len(rows) != 0 {
		t.Fatalf("pull-request reservation stayed claimed after a publish failure: %+v %v", rows, err)
	}
	gotPR, _ := store.GetPullRequestWatch(t.Context(), pr.ID)
	if gotPR.LastError != "one or more watcher matches could not be published" {
		t.Fatalf("pull-request watch error = %q", gotPR.LastError)
	}
}

// TestTriggerWatchClearsPreviousErrorOnSuccess proves a recovered watch stops
// reporting a stale failure.
func TestTriggerWatchClearsPreviousErrorOnSuccess(t *testing.T) {
	client := &watchClient{work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101, Title: "Fix build"}}}}
	service, store := newWatchService(t, client)
	eventBus := bus.NewMemoryEventBus(logger.Default())
	service.SetEventBus(eventBus)
	seen := make(chan *WorkItemWatchEvent, 1)
	if _, err := eventBus.Subscribe(events.AzureDevOpsWorkItemWatchMatch, func(_ context.Context, event *bus.Event) error {
		seen <- event.Data.(*WorkItemWatchEvent)
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	watch := seedWorkItemWatch(t, store)
	if err := store.SetWorkItemWatchError(t.Context(), watch.ID, "stale failure", time.Now().UTC()); err != nil {
		t.Fatalf("seed stale failure: %v", err)
	}

	result, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || result.Matched != 1 {
		t.Fatalf("trigger = %+v, %v", result, err)
	}
	if client.lastWIQL != [2]string{"project-1", "SELECT [System.Id] FROM WorkItems"} {
		t.Fatalf("query sent upstream = %v", client.lastWIQL)
	}
	select {
	case event := <-seen:
		if event.WorkItem.ID != 101 || event.WatchID != watch.ID || !event.ReservationClaimed {
			t.Fatalf("published event = %+v", event)
		}
	default:
		t.Fatal("no watch event was published for the match")
	}
	got, _ := store.GetWorkItemWatch(t.Context(), watch.ID)
	if got.LastError != "" || got.LastErrorAt != nil || got.LastPolledAt == nil {
		t.Fatalf("watch after a successful check = %+v", got)
	}
}

// TestTriggerWatchWithoutEventBusStillClaimsMatches keeps the nil-bus local
// path working: reservations are made even though nothing is published.
func TestTriggerWatchWithoutEventBusStillClaimsMatches(t *testing.T) {
	client := &watchClient{work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101}, {ID: 102}}}}
	service, store := newWatchService(t, client)
	watch := seedWorkItemWatch(t, store)

	result, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || result.Matched != 2 {
		t.Fatalf("trigger = %+v, %v; want both items matched", result, err)
	}
	rows, err := store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
	if err != nil || len(rows) != 2 {
		t.Fatalf("reservations = %+v, %v", rows, err)
	}
	repeat, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || repeat.Matched != 0 {
		t.Fatalf("repeat trigger = %+v, %v; want the dedup to hold", repeat, err)
	}
}

// TestTriggerWatchSkipsDeletingWatches covers the soft-delete guard in the
// authorization helpers.
func TestTriggerWatchSkipsDeletingWatches(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	marks := []struct {
		query string
		id    string
	}{
		{`UPDATE azure_devops_work_item_watches SET deleting = 1 WHERE id = ?`, work.ID},
		{`UPDATE azure_devops_pull_request_watches SET deleting = 1 WHERE id = ?`, pr.ID},
	}
	for _, mark := range marks {
		if _, err := store.db.ExecContext(t.Context(), mark.query, mark.id); err != nil {
			t.Fatalf("mark deleting: %v", err)
		}
	}

	if _, err := service.TriggerWorkItemWatch(t.Context(), "ws-1", work.ID); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("work-item trigger error = %v, want ErrWatchNotFound", err)
	}
	if _, err := service.TriggerPullRequestWatch(t.Context(), "ws-1", pr.ID); !errors.Is(err, ErrWatchNotFound) {
		t.Fatalf("pull-request trigger error = %v, want ErrWatchNotFound", err)
	}
	if watches, err := store.ListWorkItemWatches(t.Context(), "ws-1"); err != nil || len(watches) != 0 {
		t.Fatalf("deleting watch is still listed: %+v %v", watches, err)
	}
}
