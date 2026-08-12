package azuredevops

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestWatchDueRespectsTheNormalizedInterval(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time {
		value := now.Add(offset)
		return &value
	}
	tests := []struct {
		name     string
		last     *time.Time
		interval int
		want     bool
	}{
		{name: "never polled", want: true},
		{name: "exactly at the interval", last: at(-5 * time.Minute), interval: 300, want: true},
		{name: "one second early", last: at(-299 * time.Second), interval: 300},
		{name: "well past the interval", last: at(-time.Hour), interval: 300, want: true},
		{name: "zero interval falls back to the default", last: at(-4 * time.Minute), interval: 0},
		{name: "sub-minimum interval is clamped up", last: at(-30 * time.Second), interval: 1},
		{name: "sub-minimum interval clears the clamped minimum", last: at(-90 * time.Second), interval: 1, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := watchDue(tt.last, tt.interval, now); got != tt.want {
				t.Fatalf("watchDue(%v, %d) = %t, want %t", tt.last, tt.interval, got, tt.want)
			}
		})
	}
}

func TestNewPollerRequiresAServiceAndToleratesANilLogger(t *testing.T) {
	if poller := NewPoller(nil, logger.Default()); poller != nil {
		t.Fatalf("NewPoller(nil) = %+v, want nil", poller)
	}
	service, _ := newWatchService(t, &watchClient{})
	poller := NewPoller(service, nil)
	if poller == nil || poller.logger == nil || poller.auth == nil {
		t.Fatalf("NewPoller = %+v, want a usable poller with a default logger", poller)
	}

	// A nil poller must stay safe to drive: production wiring skips Azure
	// entirely when the integration is not constructed.
	var absent *Poller
	absent.Start(context.Background())
	absent.Stop()
}

// TestPollerStartRunsChecksImmediatelyAndOnEveryTick drives the real
// timer-owned loop under fake time: the first pass happens on Start and each
// watcherPollTick produces exactly one more pass.
func TestPollerStartRunsChecksImmediatelyAndOnEveryTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &watchClient{work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101, WebURL: "https://azure/101"}}}}
		service, store := newWatchService(t, client)
		watch := seedWorkItemWatch(t, store)
		watch.PollIntervalSeconds = minWatchPollIntervalSeconds
		if err := store.UpdateWorkItemWatch(t.Context(), watch); err != nil {
			t.Fatalf("set poll interval: %v", err)
		}

		poller := NewPoller(service, logger.Default())
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() {
			cancel()
			poller.Stop()
		})
		poller.Start(ctx)
		synctest.Wait()

		rows, err := store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
		if err != nil || len(rows) != 1 || rows[0].WorkItemID != 101 {
			t.Fatalf("reservations after the start pass = %+v, %v", rows, err)
		}

		// A second work item appears upstream; the next tick must import it.
		client.mu.Lock()
		client.work = &WorkItemSearchResult{Items: []WorkItem{{ID: 101}, {ID: 102, WebURL: "https://azure/102"}}}
		client.mu.Unlock()
		time.Sleep(watcherPollTick)
		synctest.Wait()

		rows, err = store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
		if err != nil || len(rows) != 2 {
			t.Fatalf("reservations after one tick = %+v, %v", rows, err)
		}
		if rows[1].WorkItemID != 102 || rows[1].WorkItemURL != "https://azure/102" {
			t.Fatalf("second reservation = %+v", rows[1])
		}
	})
}

// TestPollerStopDrainsAndRestarts pins the Start/Stop contract: Stop waits for
// the loop, repeated calls are no-ops, and a stopped poller can start again.
func TestPollerStopDrainsAndRestarts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &watchClient{work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101}}}}
		service, store := newWatchService(t, client)
		watch := seedWorkItemWatch(t, store)

		poller := NewPoller(service, logger.Default())
		ctx, cancel := context.WithCancel(context.Background())
		// Registered up front, before any t.Fatalf path: Stop is idempotent, so
		// this stays correct across the explicit Stop and restart below.
		t.Cleanup(func() {
			cancel()
			poller.Stop()
		})

		poller.Start(ctx)
		poller.Start(ctx) // second Start must not launch a second loop
		synctest.Wait()
		poller.Stop()
		poller.Stop() // idempotent

		if poller.started || poller.cancel != nil {
			t.Fatalf("poller state after Stop = started %t, cancel %v", poller.started, poller.cancel)
		}

		// Nothing else runs while stopped, even after a full tick elapses.
		before, err := store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
		if err != nil || len(before) != 1 {
			t.Fatalf("reservations before the quiet period = %+v, %v", before, err)
		}
		client.mu.Lock()
		client.work = &WorkItemSearchResult{Items: []WorkItem{{ID: 101}, {ID: 102}}}
		client.mu.Unlock()
		// Long enough that the watch is comfortably due again once restarted.
		time.Sleep(2 * defaultWatchPollIntervalSeconds * time.Second)
		synctest.Wait()
		after, err := store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
		if err != nil || len(after) != 1 {
			t.Fatalf("stopped poller kept polling: %+v, %v", after, err)
		}

		poller.Start(ctx)
		synctest.Wait()
		restarted, err := store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
		if err != nil || len(restarted) != 2 {
			t.Fatalf("restarted poller reservations = %+v, %v", restarted, err)
		}
	})
}

func TestRunWatchChecksSkipsWatchesThatAreNotDue(t *testing.T) {
	client := &watchClient{
		work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101}}},
		page: &PullRequestPage{Items: []PullRequest{{ID: 42, ProjectID: "project-1", RepositoryID: "azure-repo-1"}}},
	}
	service, store := newWatchService(t, client)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	polledAt := time.Now().UTC()
	if err := store.ClearWorkItemWatchError(t.Context(), work.ID, polledAt); err != nil {
		t.Fatalf("mark work-item watch polled: %v", err)
	}
	if err := store.ClearPullRequestWatchError(t.Context(), pr.ID, polledAt); err != nil {
		t.Fatalf("mark pull-request watch polled: %v", err)
	}

	poller := NewPoller(service, logger.Default())
	if matched := poller.runWatchChecks(t.Context()); matched != 0 {
		t.Fatalf("matched = %d, want 0 for watches inside their interval", matched)
	}
	if client.lastWIQL != [2]string{} || client.filter().ProjectID != "" {
		t.Fatalf("upstream was queried for a watch that was not due: %v / %+v", client.lastWIQL, client.filter())
	}
	if rows, _ := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation); len(rows) != 0 {
		t.Fatalf("reservations created for a watch that was not due: %+v", rows)
	}
}

// TestRunWatchChecksContinuesPastAFailingWatch proves one broken workspace
// cannot stall every other watcher in the poll pass.
func TestRunWatchChecksContinuesPastAFailingWatch(t *testing.T) {
	client := &watchClient{
		work: &WorkItemSearchResult{Items: []WorkItem{{ID: 101}}},
		page: &PullRequestPage{Items: []PullRequest{{ID: 42, ProjectID: "project-1", RepositoryID: "azure-repo-1"}}},
	}
	service, store := newWatchService(t, client)
	// "ws-broken" has watches but no stored credentials, so its checks fail.
	broken := &WorkItemWatch{WorkspaceID: "ws-broken", ProjectID: "project-1", WIQL: "SELECT"}
	if err := store.CreateWorkItemWatch(t.Context(), broken); err != nil {
		t.Fatalf("create broken work-item watch: %v", err)
	}
	brokenPR := &PullRequestWatch{WorkspaceID: "ws-broken", ProjectID: "project-1", AzureRepositoryID: "azure-repo-1"}
	if err := store.CreatePullRequestWatch(t.Context(), brokenPR); err != nil {
		t.Fatalf("create broken pull-request watch: %v", err)
	}
	healthy := seedWorkItemWatch(t, store)
	healthyPR := seedPullRequestWatch(t, store)

	poller := NewPoller(service, logger.Default())
	if matched := poller.runWatchChecks(t.Context()); matched != 2 {
		t.Fatalf("matched = %d, want 2 from the healthy watches", matched)
	}
	if rows, _ := store.ListWorkItemWatchTasks(t.Context(), healthy.ID, healthy.Generation); len(rows) != 1 {
		t.Fatalf("healthy work-item watch produced %d reservations", len(rows))
	}
	if rows, _ := store.ListPullRequestWatchTasks(t.Context(), healthyPR.ID, healthyPR.Generation); len(rows) != 1 {
		t.Fatalf("healthy pull-request watch produced %d reservations", len(rows))
	}
	gotBroken, _ := store.GetWorkItemWatch(t.Context(), broken.ID)
	if gotBroken.LastError != ErrNotConfigured.Error() {
		t.Fatalf("broken work-item watch error = %q", gotBroken.LastError)
	}
	gotBrokenPR, _ := store.GetPullRequestWatch(t.Context(), brokenPR.ID)
	if gotBrokenPR.LastError != ErrNotConfigured.Error() {
		t.Fatalf("broken pull-request watch error = %q", gotBrokenPR.LastError)
	}
}

// TestRunWatchChecksRemovesTasksForTerminalItems covers the per-poll cleanup
// hop: items and pull requests that reached a terminal state have their Kandev
// task trees deleted and their dedup rows released.
func TestRunWatchChecksRemovesTasksForTerminalItems(t *testing.T) {
	client := &watchClient{
		workItems:    map[int]*WorkItem{101: {ID: 101, State: "Closed"}, 102: {ID: 102, State: "Active"}},
		pullRequests: map[int]*PullRequest{42: {ID: 42, Status: "completed"}, 43: {ID: 43, Status: "active"}},
	}
	service, store := newWatchService(t, client)
	deleter := &recordingDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	seedWorkItemReservation(t, store, work, 101, "task-101")
	seedWorkItemReservation(t, store, work, 102, "task-102")
	seedPullRequestReservation(t, store, pr, 42, "task-42")
	seedPullRequestReservation(t, store, pr, 43, "task-43")

	poller := NewPoller(service, logger.Default())
	if matched := poller.runWatchChecks(t.Context()); matched != 0 {
		t.Fatalf("matched = %d, want 0 (no new upstream items)", matched)
	}

	ids := deleter.ids()
	if len(ids) != 2 || ids[0] != "task-101" || ids[1] != "task-42" {
		t.Fatalf("cascade-deleted tasks = %v, want the terminal ones only", ids)
	}
	workRows, _ := store.ListWorkItemWatchTasks(t.Context(), work.ID, work.Generation)
	if len(workRows) != 1 || workRows[0].WorkItemID != 102 {
		t.Fatalf("work-item reservations after cleanup = %+v", workRows)
	}
	prRows, _ := store.ListPullRequestWatchTasks(t.Context(), pr.ID, pr.Generation)
	if len(prRows) != 1 || prRows[0].PullRequestID != 43 {
		t.Fatalf("pull-request reservations after cleanup = %+v", prRows)
	}
}

// TestPollCleanupSkipsWatchesItMustNotTouch guards the early returns in the
// per-poll cleanup helpers.
func TestPollCleanupSkipsWatchesItMustNotTouch(t *testing.T) {
	client := &watchClient{
		workItems:    map[int]*WorkItem{101: {ID: 101, State: "Closed"}},
		pullRequests: map[int]*PullRequest{42: {ID: 42, Status: "completed"}},
	}
	service, store := newWatchService(t, client)
	deleter := &recordingDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	work := seedWorkItemWatch(t, store)
	pr := seedPullRequestWatch(t, store)
	seedWorkItemReservation(t, store, work, 101, "task-101")
	seedPullRequestReservation(t, store, pr, 42, "task-42")

	if cleaned := service.cleanupWorkItemWatchForPoll(t.Context(), "missing-watch"); cleaned != 0 {
		t.Fatalf("cleanup for a missing work-item watch = %d", cleaned)
	}
	if cleaned := service.cleanupPullRequestWatchForPoll(t.Context(), "missing-watch"); cleaned != 0 {
		t.Fatalf("cleanup for a missing pull-request watch = %d", cleaned)
	}
	if err := store.DisableWorkItemWatchWithError(t.Context(), work.ID, "disabled"); err != nil {
		t.Fatalf("disable work-item watch: %v", err)
	}
	if err := store.DisablePullRequestWatchWithError(t.Context(), pr.ID, "disabled"); err != nil {
		t.Fatalf("disable pull-request watch: %v", err)
	}
	if cleaned := service.cleanupWorkItemWatchForPoll(t.Context(), work.ID); cleaned != 0 {
		t.Fatalf("cleanup ran for a disabled work-item watch: %d", cleaned)
	}
	if cleaned := service.cleanupPullRequestWatchForPoll(t.Context(), pr.ID); cleaned != 0 {
		t.Fatalf("cleanup ran for a disabled pull-request watch: %d", cleaned)
	}
	if ids := deleter.ids(); len(ids) != 0 {
		t.Fatalf("tasks were deleted for skipped watches: %v", ids)
	}

	// Re-enable, then drop the workspace credentials: cleanup must bail out
	// rather than run without a client.
	work.Enabled = true
	if err := store.UpdateWorkItemWatch(t.Context(), work); err != nil {
		t.Fatalf("re-enable work-item watch: %v", err)
	}
	pr.Enabled = true
	if err := store.UpdatePullRequestWatch(t.Context(), pr); err != nil {
		t.Fatalf("re-enable pull-request watch: %v", err)
	}
	if err := service.DeleteConfigForWorkspace(t.Context(), "ws-1"); err != nil {
		t.Fatalf("delete config: %v", err)
	}
	if cleaned := service.cleanupWorkItemWatchForPoll(t.Context(), work.ID); cleaned != 0 {
		t.Fatalf("cleanup ran without credentials: %d", cleaned)
	}
	if cleaned := service.cleanupPullRequestWatchForPoll(t.Context(), pr.ID); cleaned != 0 {
		t.Fatalf("cleanup ran without credentials: %d", cleaned)
	}
	if ids := deleter.ids(); len(ids) != 0 {
		t.Fatalf("tasks were deleted without credentials: %v", ids)
	}
}

func TestAuthProberReportsConfiguredWorkspacesAndPersistsHealth(t *testing.T) {
	client := &fakeClient{result: &TestConnectionResult{OK: false, Error: "401 unauthorized"}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	prober := authProber{service: service}

	configured, err := prober.HasConfig(t.Context())
	if err != nil || configured {
		t.Fatalf("HasConfig before any config = %t, %v; want false", configured, err)
	}
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{
		OrganizationURL: "https://dev.azure.com/acme", PAT: "pat",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if err := store.ResetAuthHealth(t.Context(), "ws-1"); err != nil {
		t.Fatalf("reset health: %v", err)
	}

	configured, err = prober.HasConfig(t.Context())
	if err != nil || !configured {
		t.Fatalf("HasConfig after config = %t, %v; want true", configured, err)
	}
	prober.RecordAuthHealth(t.Context())
	cfg, err := service.GetConfigForWorkspace(t.Context(), "ws-1")
	if err != nil || cfg.LastCheckedAt == nil || cfg.LastOK || cfg.LastError != "401 unauthorized" {
		t.Fatalf("persisted health = %+v, %v", cfg, err)
	}

	client.result, client.err = nil, errors.New("network timeout")
	prober.RecordAuthHealth(t.Context())
	cfg, _ = service.GetConfigForWorkspace(t.Context(), "ws-1")
	if cfg.LastOK || cfg.LastError != "network timeout" {
		t.Fatalf("health after a transport failure = %+v", cfg)
	}
}
