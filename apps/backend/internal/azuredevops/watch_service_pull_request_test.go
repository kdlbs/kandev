package azuredevops

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// watchClient is the programmable Client used by watcher tests. Matches and
// upstream failures are configurable per call so each error hop of
// TriggerWorkItemWatch / TriggerPullRequestWatch can be asserted.
type watchClient struct {
	invalidClient
	mu           sync.Mutex
	work         *WorkItemSearchResult
	workErr      error
	page         *PullRequestPage
	pageErr      error
	lastWIQL     [2]string
	lastPRFilter PullRequestFilter
	workItems    map[int]*WorkItem
	pullRequests map[int]*PullRequest
}

func (c *watchClient) QueryWIQL(_ context.Context, projectID, wiql string, _ int) (*WorkItemSearchResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastWIQL = [2]string{projectID, wiql}
	if c.workErr != nil {
		return nil, c.workErr
	}
	if c.work == nil {
		return &WorkItemSearchResult{}, nil
	}
	return c.work, nil
}

func (c *watchClient) ListPullRequests(_ context.Context, filter PullRequestFilter) (*PullRequestPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPRFilter = filter
	if c.pageErr != nil {
		return nil, c.pageErr
	}
	if c.page == nil {
		return &PullRequestPage{}, nil
	}
	return c.page, nil
}

func (c *watchClient) GetWorkItem(_ context.Context, _ string, id int) (*WorkItem, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.workItems[id]
	if !ok {
		return nil, errors.New("work item not found")
	}
	return item, nil
}

func (c *watchClient) GetPullRequest(_ context.Context, _, _ string, id int) (*PullRequest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pr, ok := c.pullRequests[id]
	if !ok {
		return nil, errors.New("pull request not found")
	}
	return pr, nil
}

func (c *watchClient) filter() PullRequestFilter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastPRFilter
}

// failingEventBus rejects every publish so the watcher's release-and-record
// compensation can be observed.
type failingEventBus struct{ err error }

func (b *failingEventBus) Publish(context.Context, string, *bus.Event) error { return b.err }
func (b *failingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, b.err
}

func (b *failingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, b.err
}

func (b *failingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, b.err
}
func (b *failingEventBus) Close()            {}
func (b *failingEventBus) IsConnected() bool { return false }

// recordingDeleter captures the task trees a watcher asks to cascade-delete.
type recordingDeleter struct {
	mu      sync.Mutex
	deleted []string
	err     error
}

func (d *recordingDeleter) DeleteTaskTree(_ context.Context, rootID string, cascade bool) (*taskservice.CascadeOutcome, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !cascade {
		return nil, errors.New("watcher cleanup must cascade")
	}
	if d.err != nil {
		return nil, d.err
	}
	d.deleted = append(d.deleted, rootID)
	return &taskservice.CascadeOutcome{}, nil
}

func (d *recordingDeleter) ids() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.deleted...)
}

func newWatchService(t *testing.T, client Client) (*Service, *Store) {
	t.Helper()
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	return service, store
}

func validPullRequestWatchRequest() *CreatePullRequestWatchRequest {
	return &CreatePullRequestWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		ProjectID: "project-1", AzureRepositoryID: "azure-repo-1", Status: "active",
		RepositoryID: "repo-1", BaseBranch: "main",
		AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
		Prompt: "Review {{title}}",
	}
}

func TestCreatePullRequestWatchRejectsIncompleteRequests(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	if _, err := service.CreatePullRequestWatch(t.Context(), nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil request error = %v, want ErrInvalidConfig", err)
	}
	blank := validPullRequestWatchRequest()
	blank.WorkspaceID = "  "
	if _, err := service.CreatePullRequestWatch(t.Context(), blank); !errors.Is(err, ErrInvalidWorkspaceID) {
		t.Fatalf("blank workspace error = %v, want ErrInvalidWorkspaceID", err)
	}
	for _, field := range []string{"workflow", "step", "agent", "executor", "repository"} {
		t.Run("missing "+field, func(t *testing.T) {
			req := validPullRequestWatchRequest()
			switch field {
			case "workflow":
				req.WorkflowID = ""
			case "step":
				req.WorkflowStepID = ""
			case "agent":
				req.AgentProfileID = ""
			case "executor":
				req.ExecutorProfileID = ""
			case "repository":
				req.RepositoryID = " "
			}
			if _, err := service.CreatePullRequestWatch(t.Context(), req); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("error = %v, want ErrInvalidConfig", err)
			}
		})
	}
	watches, err := store.ListPullRequestWatches(t.Context(), "ws-1")
	if err != nil || len(watches) != 0 {
		t.Fatalf("rejected requests persisted watches: %+v %v", watches, err)
	}
}

func TestCreatePullRequestWatchDeniesForeignWorkspaceAccess(t *testing.T) {
	service, _ := newWatchService(t, &watchClient{})
	denied := errors.New("workspace denied")
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return denied })
	if _, err := service.CreatePullRequestWatch(t.Context(), validPullRequestWatchRequest()); !errors.Is(err, denied) {
		t.Fatalf("error = %v, want the authorizer denial", err)
	}
	if _, err := service.ListPullRequestWatches(t.Context(), "ws-1"); !errors.Is(err, denied) {
		t.Fatalf("list error = %v, want the authorizer denial", err)
	}
	if _, err := service.ListPullRequestWatches(t.Context(), ""); !errors.Is(err, ErrInvalidWorkspaceID) {
		t.Fatalf("blank workspace list error = %v, want ErrInvalidWorkspaceID", err)
	}
}

// TestCreatePullRequestWatchRunsTheInitialCheck proves the create path fires
// its own first poll rather than waiting a full interval for the poller.
//
// CreatePullRequestWatch runs that first check on an untracked goroutine, so
// the test runs inside a synctest bubble and joins it with synctest.Wait
// instead of polling the store or racing a wall-clock deadline.
func TestCreatePullRequestWatchRunsTheInitialCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &watchClient{page: &PullRequestPage{Items: []PullRequest{{
			ID: 42, ProjectID: "project-1", RepositoryID: "azure-repo-1",
			WebURL: "https://azure/pr/42", Title: "Ship it",
		}}}}
		service, store := newWatchService(t, client)
		eventBus := bus.NewMemoryEventBus(logger.Default())
		service.SetEventBus(eventBus)
		published := make(chan *PullRequestWatchEvent, 1)
		if _, err := eventBus.Subscribe(events.AzureDevOpsPullRequestWatchMatch, func(_ context.Context, event *bus.Event) error {
			published <- event.Data.(*PullRequestWatchEvent)
			return nil
		}); err != nil {
			t.Fatalf("subscribe: %v", err)
		}

		req := validPullRequestWatchRequest()
		req.CreatorID = "creator-1"
		req.ReviewerID = "reviewer-1"
		req.PollIntervalSeconds = 5
		watch, err := service.CreatePullRequestWatch(t.Context(), req)
		if err != nil {
			t.Fatalf("CreatePullRequestWatch: %v", err)
		}
		if watch.ID == "" || !watch.Enabled || watch.Generation != 1 ||
			watch.PollIntervalSeconds != minWatchPollIntervalSeconds ||
			watch.CleanupPolicy != CleanupPolicyAuto {
			t.Fatalf("created watch = %+v", watch)
		}
		synctest.Wait()

		select {
		case event := <-published:
			if event.PullRequest.ID != 42 || event.WatchID != watch.ID ||
				event.AzureRepositoryID != "azure-repo-1" || event.WorkspaceID != "ws-1" ||
				event.WorkflowStepID != "step-1" || event.RepositoryID != "repo-1" ||
				event.BaseBranch != "main" || event.Prompt != "Review {{title}}" ||
				!event.ReservationClaimed {
				t.Fatalf("initial watch event = %+v", event)
			}
		default:
			t.Fatal("initial pull-request check never published a match")
		}

		filter := client.filter()
		if filter.ProjectID != "project-1" || filter.RepositoryID != "azure-repo-1" ||
			filter.Status != "active" || filter.CreatorID != "creator-1" ||
			filter.ReviewerID != "reviewer-1" || filter.Top != 100 {
			t.Fatalf("upstream filter = %+v", filter)
		}
		rows, err := store.ListPullRequestWatchTasks(t.Context(), watch.ID, watch.Generation)
		if err != nil || len(rows) != 1 || rows[0].PullRequestID != 42 {
			t.Fatalf("reservations after the initial check = %+v, %v", rows, err)
		}
	})
}

func TestUpdatePullRequestWatchAppliesEveryRequestedField(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedPullRequestWatch(t, store)

	str := func(value string) *string { return &value }
	enabled := false
	interval := 1200
	maxInflight := 3
	updated, err := service.UpdatePullRequestWatch(t.Context(), "ws-1", watch.ID, &UpdatePullRequestWatchRequest{
		WorkflowID: str("wf-2"), WorkflowStepID: str("step-2"), ProjectID: str("project-2"),
		AzureRepositoryID: str("azure-repo-2"), Status: str("completed"),
		CreatorID: str("creator-2"), ReviewerID: str("reviewer-2"),
		RepositoryID: str("repo-2"), BaseBranch: str("develop"),
		AgentProfileID: str("agent-2"), ExecutorProfileID: str("executor-2"),
		Prompt: str("Fix {{title}}"), Enabled: &enabled,
		PollIntervalSeconds: &interval, CleanupPolicy: str(CleanupPolicyAlways),
		MaxInflightTasks: &maxInflight,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestWatch: %v", err)
	}
	if updated.WorkflowID != "wf-2" || updated.WorkflowStepID != "step-2" ||
		updated.ProjectID != "project-2" || updated.AzureRepositoryID != "azure-repo-2" ||
		updated.Status != "completed" || updated.CreatorID != "creator-2" ||
		updated.ReviewerID != "reviewer-2" || updated.RepositoryID != "repo-2" ||
		updated.BaseBranch != "develop" || updated.AgentProfileID != "agent-2" ||
		updated.ExecutorProfileID != "executor-2" || updated.Prompt != "Fix {{title}}" ||
		updated.Enabled || updated.PollIntervalSeconds != 1200 ||
		updated.CleanupPolicy != CleanupPolicyAlways ||
		updated.MaxInflightTasks == nil || *updated.MaxInflightTasks != 3 {
		t.Fatalf("updated watch = %+v", updated)
	}

	persisted, err := store.GetPullRequestWatch(t.Context(), watch.ID)
	if err != nil || persisted.Prompt != "Fix {{title}}" || persisted.Status != "completed" {
		t.Fatalf("persisted watch = %+v, %v", persisted, err)
	}
}

func TestUpdatePullRequestWatchClearsUnlimitedInflightAndKeepsUnsetFields(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedPullRequestWatch(t, store)
	watch.MaxInflightTasks = intPtr(5)
	if err := store.UpdatePullRequestWatch(t.Context(), watch); err != nil {
		t.Fatalf("seed max inflight: %v", err)
	}

	unlimited := 0
	updated, err := service.UpdatePullRequestWatch(t.Context(), "ws-1", watch.ID, &UpdatePullRequestWatchRequest{
		MaxInflightTasks: &unlimited,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestWatch: %v", err)
	}
	if updated.MaxInflightTasks != nil {
		t.Fatalf("max inflight = %v, want nil for unlimited", *updated.MaxInflightTasks)
	}
	if updated.ProjectID != "project-1" || updated.AzureRepositoryID != "azure-repo-1" ||
		updated.RepositoryID != "repo-1" || !updated.Enabled {
		t.Fatalf("unset fields were mutated: %+v", updated)
	}

	// A nil request body is a no-op rather than a wipe.
	unchanged, err := service.UpdatePullRequestWatch(t.Context(), "ws-1", watch.ID, nil)
	if err != nil || unchanged.ProjectID != "project-1" || !unchanged.Enabled {
		t.Fatalf("nil-request update = %+v, %v", unchanged, err)
	}
}

func TestPullRequestWatchOperationsRejectOtherWorkspaces(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedPullRequestWatch(t, store)
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-2", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/other", PAT: "pat",
	}); err != nil {
		t.Fatalf("set second config: %v", err)
	}

	operations := map[string]func(id string) error{
		"trigger": func(id string) error {
			_, err := service.TriggerPullRequestWatch(t.Context(), "ws-2", id)
			return err
		},
		"update": func(id string) error {
			_, err := service.UpdatePullRequestWatch(t.Context(), "ws-2", id, &UpdatePullRequestWatchRequest{})
			return err
		},
		"delete": func(id string) error { return service.DeletePullRequestWatch(t.Context(), "ws-2", id) },
		"preview reset": func(id string) error {
			_, err := service.PreviewResetPullRequestWatch(t.Context(), "ws-2", id)
			return err
		},
		"reset": func(id string) error {
			_, err := service.ResetPullRequestWatch(t.Context(), "ws-2", id)
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(watch.ID); !errors.Is(err, ErrWatchNotFound) {
				t.Fatalf("cross-workspace %s error = %v, want ErrWatchNotFound", name, err)
			}
			if err := operation("no-such-watch"); !errors.Is(err, ErrWatchNotFound) {
				t.Fatalf("unknown-watch %s error = %v, want ErrWatchNotFound", name, err)
			}
			if err := operation(""); !errors.Is(err, ErrWatchNotFound) {
				t.Fatalf("blank-id %s error = %v, want ErrWatchNotFound", name, err)
			}
		})
	}
	if _, err := service.TriggerPullRequestWatch(t.Context(), "", watch.ID); !errors.Is(err, ErrInvalidWorkspaceID) {
		t.Fatalf("blank workspace error = %v, want ErrInvalidWorkspaceID", err)
	}
	if remaining, err := store.GetPullRequestWatch(t.Context(), watch.ID); err != nil || remaining == nil {
		t.Fatalf("watch was removed by a foreign-workspace call: %+v %v", remaining, err)
	}
}

func TestDeletePullRequestWatchCascadesItsCreatedTasks(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedPullRequestWatch(t, store)
	deleter := &recordingDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	seedPullRequestReservation(t, store, watch, 42, "task-42")
	seedPullRequestReservation(t, store, watch, 43, "")

	if err := service.DeletePullRequestWatch(t.Context(), "ws-1", watch.ID); err != nil {
		t.Fatalf("DeletePullRequestWatch: %v", err)
	}
	if ids := deleter.ids(); len(ids) != 1 || ids[0] != "task-42" {
		t.Fatalf("cascade-deleted task ids = %v, want [task-42]", ids)
	}
	if got, err := store.GetPullRequestWatch(t.Context(), watch.ID); err != nil || got != nil {
		t.Fatalf("watch after delete = %+v, %v", got, err)
	}
}

func TestPreviewAndResetPullRequestWatchRotateTheGeneration(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedPullRequestWatch(t, store)
	deleter := &recordingDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	seedPullRequestReservation(t, store, watch, 42, "task-42")
	seedPullRequestReservation(t, store, watch, 43, "task-43")

	count, err := service.PreviewResetPullRequestWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || count != 2 {
		t.Fatalf("PreviewResetPullRequestWatch = %d, %v; want 2", count, err)
	}

	result, err := service.ResetPullRequestWatch(t.Context(), "ws-1", watch.ID)
	if err != nil {
		t.Fatalf("ResetPullRequestWatch: %v", err)
	}
	if result.Generation != watch.Generation+1 || len(result.TaskIDs) != 2 {
		t.Fatalf("reset result = %+v", result)
	}
	if ids := deleter.ids(); len(ids) != 2 || ids[0] != "task-42" || ids[1] != "task-43" {
		t.Fatalf("cascade-deleted task ids = %v", ids)
	}
	if rows, err := store.ListPullRequestWatchTasks(t.Context(), watch.ID, watch.Generation); err != nil || len(rows) != 0 {
		t.Fatalf("old-generation reservations survived the reset: %+v %v", rows, err)
	}
	if count, err := service.PreviewResetPullRequestWatch(t.Context(), "ws-1", watch.ID); err != nil || count != 0 {
		t.Fatalf("preview after reset = %d, %v; want 0", count, err)
	}
}

// TestResetWatchToleratesCascadeDeleteFailures proves a task that cannot be
// deleted does not abort the reset: the generation still rotates so the watch
// can re-import.
func TestResetWatchToleratesCascadeDeleteFailures(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	watch := seedPullRequestWatch(t, store)
	service.SetCascadeTaskDeleter(&recordingDeleter{err: errors.New("task already gone")})
	seedPullRequestReservation(t, store, watch, 42, "task-42")

	result, err := service.ResetPullRequestWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || result.Generation != watch.Generation+1 {
		t.Fatalf("reset result = %+v, %v", result, err)
	}
}

func seedPullRequestReservation(t *testing.T, store *Store, watch *PullRequestWatch, pullRequestID int, taskID string) {
	t.Helper()
	reserved, err := store.ReservePullRequestWatchTask(
		t.Context(), watch.ID, watch.Generation, watch.ProjectID,
		watch.AzureRepositoryID, pullRequestID, "https://azure/pr",
	)
	if err != nil || !reserved {
		t.Fatalf("reserve pull request %d: reserved=%v err=%v", pullRequestID, reserved, err)
	}
	if taskID == "" {
		return
	}
	if err := store.AssignPullRequestWatchTaskID(
		t.Context(), watch.ID, watch.Generation, watch.ProjectID,
		watch.AzureRepositoryID, pullRequestID, taskID,
	); err != nil {
		t.Fatalf("assign task %q: %v", taskID, err)
	}
}

// TestServiceReservationSurfaceDelegatesToTheStore covers the orchestrator
// facing reservation methods, which are the seam the shared watcher dispatch
// pipeline calls into.
func TestServiceReservationSurfaceDelegatesToTheStore(t *testing.T) {
	service, store := newWatchService(t, &watchClient{})
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	ctx := t.Context()

	reserved, err := service.ReserveWorkItemWatchTask(ctx, work.ID, work.Generation, "project-1", 101, "https://azure/101")
	if err != nil || !reserved {
		t.Fatalf("ReserveWorkItemWatchTask = %v, %v", reserved, err)
	}
	if again, err := service.ReserveWorkItemWatchTask(ctx, work.ID, work.Generation, "project-1", 101, "https://azure/101"); err != nil || again {
		t.Fatalf("duplicate work-item reservation = %v, %v", again, err)
	}
	if err := service.AssignWorkItemWatchTaskID(ctx, work.ID, work.Generation, "project-1", 101, "task-101"); err != nil {
		t.Fatalf("AssignWorkItemWatchTaskID: %v", err)
	}
	if err := service.ReleaseWorkItemWatchTask(ctx, work.ID, work.Generation, "project-1", 101); err != nil {
		t.Fatalf("ReleaseWorkItemWatchTask: %v", err)
	}
	if rows, _ := store.ListWorkItemWatchTasks(ctx, work.ID, work.Generation); len(rows) != 0 {
		t.Fatalf("work-item reservation survived release: %+v", rows)
	}

	reserved, err = service.ReservePullRequestWatchTask(ctx, pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "https://azure/pr/42")
	if err != nil || !reserved {
		t.Fatalf("ReservePullRequestWatchTask = %v, %v", reserved, err)
	}
	if err := service.AssignPullRequestWatchTaskID(ctx, pr.ID, pr.Generation, "project-1", "azure-repo-1", 42, "task-42"); err != nil {
		t.Fatalf("AssignPullRequestWatchTaskID: %v", err)
	}
	if err := service.ReleasePullRequestWatchTask(ctx, pr.ID, pr.Generation, "project-1", "azure-repo-1", 42); err != nil {
		t.Fatalf("ReleasePullRequestWatchTask: %v", err)
	}

	if err := service.DisableWorkItemWatchWithError(ctx, work.ID, "workflow step deleted"); err != nil {
		t.Fatalf("DisableWorkItemWatchWithError: %v", err)
	}
	if err := service.DisablePullRequestWatchWithError(ctx, pr.ID, "repository unlinked"); err != nil {
		t.Fatalf("DisablePullRequestWatchWithError: %v", err)
	}
	gotWork, _ := store.GetWorkItemWatch(ctx, work.ID)
	gotPR, _ := store.GetPullRequestWatch(ctx, pr.ID)
	if gotWork.Enabled || gotWork.LastError != "workflow step deleted" ||
		gotPR.Enabled || gotPR.LastError != "repository unlinked" {
		t.Fatalf("disabled watches = %+v / %+v", gotWork, gotPR)
	}
}
