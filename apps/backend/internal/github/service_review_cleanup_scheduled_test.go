package github

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type countingReviewCleanupClient struct {
	Client
	feedbackCalls   int
	feedbackNumbers []int
	feedbackErrors  map[int]error
	reviewPRNumbers map[int]bool
}

type missingTaskReviewDeleter struct {
	calls []string
}

func (d *missingTaskReviewDeleter) DeleteTask(_ context.Context, taskID string) error {
	d.calls = append(d.calls, taskID)
	if taskID == "hard-deleted-task-enabled" || taskID == "hard-deleted-task-disabled" {
		return ErrTaskNotFound
	}
	return nil
}

func (c *countingReviewCleanupClient) GetPRFeedback(
	ctx context.Context, owner, repo string, number int,
) (*PRFeedback, error) {
	c.recordFeedbackCall(number)
	if err := c.feedbackErrors[number]; err != nil {
		return nil, err
	}
	return c.Client.GetPRFeedback(ctx, owner, repo, number)
}

func (c *countingReviewCleanupClient) GetPR(ctx context.Context, owner, repo string, number int) (*PR, error) {
	c.recordFeedbackCall(number)
	if err := c.feedbackErrors[number]; err != nil {
		return nil, err
	}
	return c.Client.GetPR(ctx, owner, repo, number)
}

func (c *countingReviewCleanupClient) recordFeedbackCall(number int) {
	if c.reviewPRNumbers != nil && !c.reviewPRNumbers[number] {
		return
	}
	c.feedbackCalls++
	c.feedbackNumbers = append(c.feedbackNumbers, number)
}

func setupArchivedReviewCleanup(t *testing.T, watchEnabled bool) (*Poller, *Service, *Store, *ReviewWatch, *countingReviewCleanupClient) {
	t.Helper()
	poller, service, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	watch := &ReviewWatch{
		ID:            "review-watch",
		WorkspaceID:   "ws-1",
		Enabled:       watchEnabled,
		CleanupPolicy: CleanupPolicyAuto,
	}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	seedTask(t, store, "archived-review-task", true)
	if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      42,
		PRURL:         "https://github.com/acme/widget/pull/42",
		TaskID:        "archived-review-task",
	}); err != nil {
		t.Fatalf("create review PR task: %v", err)
	}
	mockClient.AddPR(&PR{
		Number:    42,
		State:     prStateMerged,
		RepoOwner: "acme",
		RepoName:  "widget",
	})
	service.SetTaskSessionChecker(&recordingSessionChecker{hasUserMsg: true})
	service.SetTaskDeleter(&recordingTaskDeleter{})
	countingClient := &countingReviewCleanupClient{
		Client: mockClient, reviewPRNumbers: map[int]bool{42: true},
	}
	configureTestWorkspaceAuth(t, service, countingClient, testWorkspaceID, "ws-1", "ws1")
	return poller, service, store, watch, countingClient
}

func assertReviewTaskIsPreserved(t *testing.T, store *Store, watch *ReviewWatch, taskID string) {
	t.Helper()
	rows, err := store.ListReviewPRTasksByWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("list review PR tasks: %v", err)
	}
	if len(rows) != 1 || rows[0].TaskID != taskID {
		t.Fatalf("review task rows = %+v, want task %q's dedup row retained", rows, taskID)
	}
}

func TestCleanupMergedReviewTasks_ArchivedEnabledWatch(t *testing.T) {
	poller, _, store, watch, client := setupArchivedReviewCleanup(t, true)
	for range 2 {
		poller.checkReviewWatches(context.Background())
	}
	if client.feedbackCalls != 0 {
		t.Errorf("feedback calls = %d, want 0 for an archived task", client.feedbackCalls)
	}
	assertReviewTaskIsPreserved(t, store, watch, "archived-review-task")
}

func TestCleanupAllOrphanedReviewTasks_ArchivedDisabledWatch(t *testing.T) {
	poller, _, store, watch, client := setupArchivedReviewCleanup(t, false)
	for range 2 {
		poller.checkReviewWatches(context.Background())
	}
	if client.feedbackCalls != 0 {
		t.Errorf("feedback calls = %d, want 0 for an archived task", client.feedbackCalls)
	}
	assertReviewTaskIsPreserved(t, store, watch, "archived-review-task")
}

func setupActiveReviewCleanup(
	t *testing.T, checker *recordingSessionChecker, lifecyclePrompt bool,
) (*Poller, *Service, *Store, *ReviewWatch, *countingReviewCleanupClient) {
	t.Helper()
	poller, service, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	watch := &ReviewWatch{
		ID: "review-watch", WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAuto,
	}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	seedTask(t, store, "active-review-task", false)
	if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
		ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: 42,
		PRURL: "https://github.com/acme/widget/pull/42", TaskID: "active-review-task",
	}); err != nil {
		t.Fatalf("create review PR task: %v", err)
	}
	mockClient.AddPR(&PR{
		Number: 42, State: prStateMerged, RepoOwner: "acme", RepoName: "widget",
	})
	if lifecyclePrompt {
		enabled := true
		if _, err := store.UpdateTaskPRAutomationOptions(ctx, "active-review-task", "", 42,
			TaskPRAutomationOptionsPatch{PromptOnMerged: &enabled}, false); err != nil {
			t.Fatalf("enable merged lifecycle prompt: %v", err)
		}
	}
	service.SetTaskSessionChecker(checker)
	service.SetTaskDeleter(&recordingTaskDeleter{})
	countingClient := &countingReviewCleanupClient{
		Client: mockClient, reviewPRNumbers: map[int]bool{42: true},
	}
	configureTestWorkspaceAuth(t, service, countingClient, testWorkspaceID, "ws-1", "ws1")
	return poller, service, store, watch, countingClient
}

func setupOpenReviewCleanupRecords(
	t *testing.T, policy string, prNumbers ...int,
) (*Poller, *Service, *Store, *ReviewWatch, *countingReviewCleanupClient) {
	t.Helper()
	poller, service, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	watch := &ReviewWatch{
		ID: "disabled-review-watch", WorkspaceID: "ws-1", Enabled: false, CleanupPolicy: policy,
	}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	for _, number := range prNumbers {
		taskID := fmt.Sprintf("review-task-%d", number)
		seedTask(t, store, taskID, false)
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: number, TaskID: taskID,
		}); err != nil {
			t.Fatalf("create review task #%d: %v", number, err)
		}
		mockClient.AddPR(&PR{
			Number: number, State: "open", RepoOwner: "acme", RepoName: "widget",
		})
	}
	service.SetTaskSessionChecker(&recordingSessionChecker{})
	service.SetTaskDeleter(&recordingTaskDeleter{})
	countingClient := &countingReviewCleanupClient{
		Client: mockClient, feedbackErrors: make(map[int]error), reviewPRNumbers: make(map[int]bool),
	}
	for _, number := range prNumbers {
		countingClient.reviewPRNumbers[number] = true
	}
	configureTestWorkspaceAuth(t, service, countingClient, testWorkspaceID, "ws-1", "ws1")
	return poller, service, store, watch, countingClient
}

func TestCleanupReviewTasks_AutoRetentionSkipsFeedback(t *testing.T) {
	cases := []struct {
		name            string
		checker         *recordingSessionChecker
		lifecyclePrompt bool
	}{
		{name: "user message", checker: &recordingSessionChecker{hasUserMsg: true}},
		{name: "lifecycle prompt", checker: &recordingSessionChecker{}, lifecyclePrompt: true},
		{name: "failed user message check", checker: &recordingSessionChecker{err: errors.New("message lookup failed")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			poller, _, _, _, client := setupActiveReviewCleanup(t, tc.checker, tc.lifecyclePrompt)
			poller.checkReviewWatches(context.Background())
			if client.feedbackCalls != 0 {
				t.Errorf("feedback calls = %d, want 0 when Auto cleanup retains the task", client.feedbackCalls)
			}
		})
	}
}

func TestListScheduledReviewPRTasks(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()
	watch := &ReviewWatch{ID: "review-watch", WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	seedTask(t, store, "archived-task", true)
	seedTask(t, store, "active-task", false)
	rows := []struct {
		taskID   string
		prNumber int
	}{
		{taskID: "archived-task", prNumber: 41},
		{taskID: "active-task", prNumber: 42},
		{taskID: "", prNumber: 43},
		{taskID: "hard-deleted-task", prNumber: 44},
	}
	for _, row := range rows {
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: row.prNumber,
			TaskID: row.taskID,
		}); err != nil {
			t.Fatalf("create review PR task #%d: %v", row.prNumber, err)
		}
	}
	perWatch, err := store.ListScheduledReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("list scheduled review tasks by watch: %v", err)
	}
	all, err := store.ListScheduledReviewPRTasks(ctx)
	if err != nil {
		t.Fatalf("list all scheduled review tasks: %v", err)
	}
	if len(perWatch) != 3 || len(all) != 3 {
		t.Fatalf("scheduled rows: per_watch=%d all=%d, want 3 each", len(perWatch), len(all))
	}
	for _, candidates := range [][]*ReviewPRTask{perWatch, all} {
		eligible := make(map[string]bool, len(candidates))
		for _, row := range candidates {
			eligible[row.TaskID] = true
		}
		if len(eligible) != 3 || !eligible["active-task"] || !eligible[""] || !eligible["hard-deleted-task"] {
			t.Errorf("scheduled candidates = %v, want active, reservation, and hard-deleted task", eligible)
		}
	}
	complete, err := store.ListAllReviewPRTasks(ctx)
	if err != nil {
		t.Fatalf("list complete review task inventory: %v", err)
	}
	if len(complete) != 4 {
		t.Errorf("complete inventory rows = %d, want archived rows to remain visible", len(complete))
	}
}

func TestCleanupReviewTasks_ScheduledEligibilityAndOrphanRecovery(t *testing.T) {
	poller, service, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	enabledWatch := &ReviewWatch{
		ID: "enabled-watch", WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAuto,
	}
	disabledWatch := &ReviewWatch{
		ID: "disabled-watch", WorkspaceID: "ws-1", Enabled: false, CleanupPolicy: CleanupPolicyAuto,
	}
	for _, watch := range []*ReviewWatch{enabledWatch, disabledWatch} {
		if err := store.CreateReviewWatch(ctx, watch); err != nil {
			t.Fatalf("create review watch %q: %v", watch.ID, err)
		}
	}
	client := &countingReviewCleanupClient{
		Client: mockClient, reviewPRNumbers: make(map[int]bool),
	}
	deleter := &missingTaskReviewDeleter{}
	service.SetTaskSessionChecker(&recordingSessionChecker{})
	service.SetTaskDeleter(deleter)
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID, "ws-1", "ws1")

	rows := []struct {
		watch    *ReviewWatch
		taskID   string
		archived bool
		missing  bool
		prNumber int
	}{
		{watch: enabledWatch, taskID: "archived-task-enabled", archived: true, prNumber: 41},
		{watch: enabledWatch, taskID: "active-task-enabled", prNumber: 42},
		{watch: enabledWatch, taskID: "", prNumber: 43},
		{watch: enabledWatch, taskID: "hard-deleted-task-enabled", missing: true, prNumber: 44},
		{watch: disabledWatch, taskID: "archived-task-disabled", archived: true, prNumber: 51},
		{watch: disabledWatch, taskID: "active-task-disabled", prNumber: 52},
		{watch: disabledWatch, taskID: "", prNumber: 53},
		{watch: disabledWatch, taskID: "hard-deleted-task-disabled", missing: true, prNumber: 54},
	}
	for _, row := range rows {
		client.reviewPRNumbers[row.prNumber] = true
		if row.taskID != "" && !row.missing {
			seedTask(t, store, row.taskID, row.archived)
		}
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: row.watch.ID, RepoOwner: "acme", RepoName: "widget", PRNumber: row.prNumber,
			PRURL: "https://github.com/acme/widget/pull/" + fmt.Sprint(row.prNumber), TaskID: row.taskID,
		}); err != nil {
			t.Fatalf("create review PR task #%d: %v", row.prNumber, err)
		}
		mockClient.AddPR(&PR{
			Number: row.prNumber, State: prStateMerged, RepoOwner: "acme", RepoName: "widget",
		})
	}

	poller.checkReviewWatches(ctx)
	poller.checkReviewWatches(ctx)
	if client.feedbackCalls != 6 {
		t.Fatalf("feedback calls = %d, want 6 for active tasks, reservations, and hard-deleted tasks", client.feedbackCalls)
	}
	if len(deleter.calls) != 4 {
		t.Fatalf("task deletion calls = %v, want only the 2 active and 2 hard-deleted tasks", deleter.calls)
	}
	remaining, err := store.ListAllReviewPRTasks(ctx)
	if err != nil {
		t.Fatalf("list review PR tasks: %v", err)
	}
	remainingTasks := make(map[string]bool, len(remaining))
	for _, row := range remaining {
		remainingTasks[row.TaskID] = true
	}
	if len(remainingTasks) != 2 || !remainingTasks["archived-task-enabled"] || !remainingTasks["archived-task-disabled"] {
		t.Fatalf("remaining review task IDs = %v, want both archived rows only", remainingTasks)
	}
}

func TestCleanupReviewTasks_UnarchiveMakesExistingRecordEligible(t *testing.T) {
	poller, service, store, watch, client := setupArchivedReviewCleanup(t, true)
	rows, err := store.ListReviewPRTasksByWatch(context.Background(), watch.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("initial review task rows = %+v, err = %v", rows, err)
	}
	recordID := rows[0].ID
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 0 {
		t.Fatalf("archived task feedback calls = %d, want 0", client.feedbackCalls)
	}
	if _, err := store.db.Exec(`UPDATE tasks SET archived_at = NULL WHERE id = ?`, "archived-review-task"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	service.SetTaskSessionChecker(&recordingSessionChecker{})
	mockClient := client.Client.(*MockClient)
	mockClient.AddPR(&PR{Number: 42, State: "open", RepoOwner: "acme", RepoName: "widget"})
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("feedback calls after unarchive = %d, want 1", client.feedbackCalls)
	}
	rows, err = store.ListReviewPRTasksByWatch(context.Background(), watch.ID)
	if err != nil || len(rows) != 1 || rows[0].ID != recordID {
		t.Fatalf("unarchived review task rows = %+v, err = %v; want the original dedup record", rows, err)
	}
}

func TestCleanupAllReviewTasks_ArchivedTaskRemainsExplicitlyEligible(t *testing.T) {
	_, service, store, watch, client := setupArchivedReviewCleanup(t, false)
	deleted, err := service.CleanupAllReviewTasks(context.Background())
	if err != nil {
		t.Fatalf("manual cleanup: %v", err)
	}
	if deleted != 0 || client.feedbackCalls != 1 {
		t.Fatalf("manual cleanup deleted=%d feedback_calls=%d, want 0 and 1", deleted, client.feedbackCalls)
	}
	assertReviewTaskIsPreserved(t, store, watch, "archived-review-task")
}

func TestResetReviewWatch_ArchivedTask(t *testing.T) {
	_, service, store, watch, _ := setupArchivedReviewCleanup(t, false)
	service.SetCascadeTaskDeleter(noopCascadeTaskDeleter{})
	deleted, err := service.ResetReviewWatch(context.Background(), watch.ID)
	if err != nil {
		t.Fatalf("reset review watch: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("reset deleted = %d, want the archived task", deleted)
	}
	rows, err := store.ListReviewPRTasksByWatch(context.Background(), watch.ID)
	if err != nil || len(rows) != 0 {
		t.Fatalf("review task rows after reset = %+v, err = %v, want none", rows, err)
	}
}

func TestReviewCleanupCircuit_StopsAfterSharedFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "authentication", err: &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls/41"}},
		{name: "rate limit", err: &GitHubAPIError{StatusCode: 403, Endpoint: "/pulls/41", Body: "API rate limit exceeded"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			poller, _, _, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41, 42)
			client.feedbackErrors[41] = tc.err
			client.feedbackErrors[42] = tc.err
			poller.checkReviewWatches(context.Background())
			if client.feedbackCalls != 1 {
				t.Fatalf("feedback calls after shared failure = %d, want 1", client.feedbackCalls)
			}
			poller.checkPRWatches(context.Background())
			poller.checkReviewWatches(context.Background())
			if client.feedbackCalls != 1 {
				t.Fatalf("feedback calls while circuit is open = %d, want 1", client.feedbackCalls)
			}
		})
	}
}

func TestReviewCleanupCoreQuota(t *testing.T) {
	poller, service, _, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	quotaSkipsBefore := reviewCleanupCoreQuotaSkipsTotal.Value()
	service.RateTracker().Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 0, ResetAt: time.Now().Add(time.Minute), UpdatedAt: time.Now(),
	})
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 0 {
		t.Fatalf("feedback calls with exhausted Core quota = %d, want 0", client.feedbackCalls)
	}
	if got := reviewCleanupCoreQuotaSkipsTotal.Value(); got <= quotaSkipsBefore {
		t.Fatalf("Core quota skip metric = %d, want an increment", got)
	}
}

func TestReviewCleanupCoreQuota_ExpiredSnapshotAllowsFeedback(t *testing.T) {
	poller, service, _, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	tracker := service.RateTracker()
	tracker.Record(RateSnapshot{
		Resource: ResourceCore, Remaining: 0, ResetAt: time.Now().Add(time.Minute), UpdatedAt: time.Now(),
	})
	tracker.mu.Lock()
	snapshot := tracker.snapshots[ResourceCore]
	snapshot.ResetAt = time.Now().Add(-time.Minute)
	tracker.snapshots[ResourceCore] = snapshot
	tracker.mu.Unlock()
	if !tracker.IsExhausted(ResourceCore) || tracker.WaitDuration(ResourceCore) > 0 {
		t.Fatal("test setup must retain the stale exhausted flag after the reset time")
	}

	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("feedback calls after Core reset = %d, want one scheduled probe", client.feedbackCalls)
	}
}

func TestReviewCleanupFailureScope(t *testing.T) {
	poller, _, _, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41, 42)
	client.feedbackErrors[41] = ErrRepoNotResolvable
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 2 {
		t.Fatalf("feedback calls after PR-specific config failure = %d, want 2", client.feedbackCalls)
	}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 3 {
		t.Fatalf("feedback calls after record circuit opens = %d, want only the healthy sibling", client.feedbackCalls)
	}
	if client.feedbackNumbers[len(client.feedbackNumbers)-1] != 42 {
		t.Fatalf("last feedback PR = %d, want healthy sibling PR 42", client.feedbackNumbers[len(client.feedbackNumbers)-1])
	}
}

func TestReviewCleanupManualBypass(t *testing.T) {
	poller, service, _, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	client.feedbackErrors[41] = &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls/41"}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("scheduled feedback calls = %d, want 1 before opening the circuit", client.feedbackCalls)
	}
	delete(client.feedbackErrors, 41)
	if _, err := service.CleanupAllReviewTasks(context.Background()); err != nil {
		t.Fatalf("manual cleanup: %v", err)
	}
	if client.feedbackCalls != 2 {
		t.Fatalf("manual feedback calls with scheduled circuit open = %d, want 2", client.feedbackCalls)
	}
}

func seedReviewCleanupConnection(t *testing.T, store *Store, workspaceID string, generation int64) {
	t.Helper()
	err := store.UpsertWorkspaceConnection(context.Background(), &WorkspaceConnection{
		WorkspaceID: workspaceID, Source: ConnectionSourcePAT, GitHubHost: defaultGitHubHost,
		Login: "test-user", Status: ConnectionStatusActive, CredentialGeneration: generation,
	})
	if err != nil {
		t.Fatalf("upsert workspace connection: %v", err)
	}
}

func TestReviewCleanupCircuit_FingerprintReset(t *testing.T) {
	poller, _, store, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	seedReviewCleanupConnection(t, store, "ws-1", 1)
	client.feedbackErrors[41] = &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls/41"}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("initial feedback calls = %d, want 1", client.feedbackCalls)
	}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("feedback calls before credential change = %d, want 1", client.feedbackCalls)
	}
	delete(client.feedbackErrors, 41)
	seedReviewCleanupConnection(t, store, "ws-1", 2)
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 2 {
		t.Fatalf("feedback calls after credential change = %d, want one immediate probe", client.feedbackCalls)
	}
}

func TestReviewCleanupCircuit_WatchPollDoesNotResetRecordButConfigEditDoes(t *testing.T) {
	poller, _, store, watch, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	watch.Enabled = true
	if err := store.UpdateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("enable review watch: %v", err)
	}
	client.feedbackErrors[41] = ErrRepoNotResolvable
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("initial feedback calls = %d, want one failing record", client.feedbackCalls)
	}

	// An enabled watch successfully polls before cleanup runs again. This
	// advances last_polled_at and updated_at without changing its configuration.
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("feedback calls after successful watch poll = %d, want the record circuit to stay open", client.feedbackCalls)
	}

	updatedWatch, err := store.GetReviewWatch(context.Background(), watch.ID)
	if err != nil || updatedWatch == nil {
		t.Fatalf("load review watch for config edit: watch=%v err=%v", updatedWatch, err)
	}
	updatedWatch.CustomQuery = "is:pr is:open"
	if err := store.UpdateReviewWatch(context.Background(), updatedWatch); err != nil {
		t.Fatalf("change review watch config: %v", err)
	}
	delete(client.feedbackErrors, 41)
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 2 {
		t.Fatalf("feedback calls after config edit = %d, want the record circuit to reset", client.feedbackCalls)
	}
}

func TestReviewCleanupCircuit_DueProbe(t *testing.T) {
	poller, _, _, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	client.feedbackErrors[41] = &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls/41"}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("initial feedback calls = %d, want 1", client.feedbackCalls)
	}
	poller.reviewCleanupCircuits.mu.Lock()
	state := poller.reviewCleanupCircuits.workspaces["ws-1"]
	due := time.Now().UTC().Add(-time.Second)
	state.NextRetryAt = &due
	poller.reviewCleanupCircuits.workspaces["ws-1"] = state
	poller.reviewCleanupCircuits.mu.Unlock()

	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 2 {
		t.Fatalf("feedback calls after due probe = %d, want 2", client.feedbackCalls)
	}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 2 {
		t.Fatalf("feedback calls after failed due probe = %d, want circuit reopened", client.feedbackCalls)
	}
}

func TestReviewCleanupCircuit_SeparateWorkspaces(t *testing.T) {
	poller, service, mockClient, store := setupPollerTest(t)
	ctx := context.Background()
	client := &countingReviewCleanupClient{
		Client: mockClient, feedbackErrors: make(map[int]error), reviewPRNumbers: make(map[int]bool),
	}
	service.SetTaskDeleter(&recordingTaskDeleter{})
	configureTestWorkspaceAuth(t, service, client, testWorkspaceID, "ws-1", "ws1", "ws-2")
	for _, fixture := range []struct {
		watchID     string
		workspaceID string
		taskID      string
		prNumber    int
	}{
		{watchID: "watch-one", workspaceID: "ws-1", taskID: "task-one", prNumber: 41},
		{watchID: "watch-two", workspaceID: "ws-2", taskID: "task-two", prNumber: 51},
	} {
		client.reviewPRNumbers[fixture.prNumber] = true
		watch := &ReviewWatch{
			ID: fixture.watchID, WorkspaceID: fixture.workspaceID,
			Enabled: false, CleanupPolicy: CleanupPolicyAlways,
		}
		if err := store.CreateReviewWatch(ctx, watch); err != nil {
			t.Fatalf("create review watch: %v", err)
		}
		seedTask(t, store, fixture.taskID, false)
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: watch.ID, RepoOwner: "acme", RepoName: "widget",
			PRNumber: fixture.prNumber, TaskID: fixture.taskID,
		}); err != nil {
			t.Fatalf("create review task: %v", err)
		}
		mockClient.AddPR(&PR{
			Number: fixture.prNumber, State: "open", RepoOwner: "acme", RepoName: "widget",
		})
	}
	client.feedbackErrors[41] = &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls/41"}
	poller.checkReviewWatches(ctx)
	poller.checkReviewWatches(ctx)
	counts := make(map[int]int)
	for _, number := range client.feedbackNumbers {
		counts[number]++
	}
	if counts[41] != 1 || counts[51] != 2 {
		t.Fatalf("feedback call counts by PR = %v, want one for failed ws-1 and two for healthy ws-2", counts)
	}
}

func TestReviewCleanupCircuit_PRMonitorSuccessDoesNotClear(t *testing.T) {
	poller, _, store, _, client := setupOpenReviewCleanupRecords(t, CleanupPolicyAlways, 41)
	client.feedbackErrors[41] = &GitHubAPIError{StatusCode: 401, Endpoint: "/pulls/41"}
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("initial review feedback calls = %d, want 1", client.feedbackCalls)
	}

	seedTask(t, store, "pr-monitor-task", false)
	prWatch := &PRWatch{
		ID: "pr-watch", WorkspaceID: "ws-1", SessionID: "session-1", TaskID: "pr-monitor-task",
		Owner: "acme", Repo: "widget", PRNumber: 99, Branch: "feature",
	}
	if err := store.CreatePRWatch(context.Background(), prWatch); err != nil {
		t.Fatalf("create PR watch: %v", err)
	}
	client.Client.(*MockClient).AddPR(&PR{
		Number: 99, State: "open", RepoOwner: "acme", RepoName: "widget", HeadBranch: "feature",
	})
	poller.checkSinglePRWatch(context.Background(), prWatch)
	poller.checkReviewWatches(context.Background())
	if client.feedbackCalls != 1 {
		t.Fatalf("review feedback calls after PR-monitor success = %d, want the cleanup circuit to remain open", client.feedbackCalls)
	}
}
