package github

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTriggerPRSync_SyncsExistingWatch(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a PR in mock client.
	mockClient.AddPR(&PR{
		Number:     10,
		Title:      "Feature PR",
		State:      "open",
		HeadSHA:    "abc123",
		HeadBranch: "feat/x",
		RepoOwner:  "org",
		RepoName:   "repo",
	})

	// Create a PR watch and TaskPR in DB.
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "org",
		Repo:      "repo",
		PRNumber:  10,
		Branch:    "feat/x",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	tp := &TaskPR{
		TaskID:   "t1",
		Owner:    "org",
		Repo:     "repo",
		PRNumber: 10,
		PRTitle:  "Feature PR",
		State:    "open",
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatal(err)
	}

	// Trigger sync.
	result, err := svc.TriggerPRSync(ctx, "t1")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil TaskPR")
	}
	if result.LastSyncedAt == nil {
		t.Error("expected LastSyncedAt to be set after sync")
	}
}

func TestTriggerPRSync_DetectsPR(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Set up a PR findable by branch.
	mockClient.AddPR(&PR{
		Number:     20,
		Title:      "New PR",
		State:      "open",
		HeadBranch: "feat/y",
		RepoOwner:  "org",
		RepoName:   "repo",
	})

	// Create a watch with pr_number=0 (still searching).
	watch := &PRWatch{
		SessionID: "s2",
		TaskID:    "t2",
		Owner:     "org",
		Repo:      "repo",
		PRNumber:  0,
		Branch:    "feat/y",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}

	result, err := svc.TriggerPRSync(ctx, "t2")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result == nil {
		t.Fatal("expected TaskPR after detection")
	}
	if result.PRNumber != 20 {
		t.Errorf("expected PR #20, got #%d", result.PRNumber)
	}
}

func TestTriggerPRSync_NoWatch(t *testing.T) {
	_, svc, _, _ := setupPollerTest(t)
	ctx := context.Background()

	result, err := svc.TriggerPRSync(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("TriggerPRSync: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil TaskPR for task without watch, got %+v", result)
	}
}

// TestTriggerPRSyncAll_ThrottlesDetectionProbe reproduces the log-flood bug:
// a pr_number=0 watch on a branch that has no PR (e.g. an unresolvable repo)
// was re-probing GitHub on every on-demand sync, because the detection path —
// unlike the status-sync path — had no freshness guard and never stamped
// last_checked_at. The frontend re-syncs every 5s while no PR is found, so
// each repeated sync hit `gh` again and logged a warning. After the fix the
// first sync probes once and stamps the watch; the immediate second sync is
// throttled within prSyncFreshnessWindow and must NOT probe again.
func TestTriggerPRSyncAll_ThrottlesDetectionProbe(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Watch with pr_number=0 on a branch that has no PR in the mock — every
	// FindPRByBranch returns (nil, nil), mirroring "PR not found yet".
	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "org",
		Repo:      "missing",
		PRNumber:  0,
		Branch:    "feat/never-merged",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.TriggerPRSyncAll(ctx, "t1"); err != nil {
		t.Fatalf("first TriggerPRSyncAll: %v", err)
	}
	if got := mockClient.FindPRByBranchCallCount(); got != 1 {
		t.Fatalf("expected exactly 1 detection probe on first sync, got %d", got)
	}

	// Second sync immediately after — well within prSyncFreshnessWindow.
	if _, err := svc.TriggerPRSyncAll(ctx, "t1"); err != nil {
		t.Fatalf("second TriggerPRSyncAll: %v", err)
	}
	if got := mockClient.FindPRByBranchCallCount(); got != 1 {
		t.Errorf("expected detection probe to be throttled on second sync (still 1 call), got %d", got)
	}
}

// TestTriggerPRDetection_CoalescesConcurrentProbes verifies that parallel
// syncs for the same watch collapse to a single GitHub probe. The freshness
// guard alone is racy — concurrent callers can all read a stale
// last_checked_at and probe simultaneously — so detection runs inside a
// per-watch singleflight. We force overlap by blocking the probe: the leader
// enters FindPRByBranch and parks; the followers must coalesce into it rather
// than issue their own probes.
func TestTriggerPRDetection_CoalescesConcurrentProbes(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &PRWatch{
		SessionID: "s1",
		TaskID:    "t1",
		Owner:     "org",
		Repo:      "missing",
		PRNumber:  0,
		Branch:    "feat/never-merged",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}

	const concurrency = 6
	entered := make(chan string, concurrency) // buffered so a broken (uncoalesced) impl can't deadlock
	release := make(chan struct{})
	mockClient.GateFindPRByBranch(entered, release)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			_, _ = svc.TriggerPRSyncAll(ctx, "t1")
		}()
	}

	// Wait for the leader probe to enter FindPRByBranch.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("no probe entered FindPRByBranch")
	}

	// Give the followers time to reach (and block in) the singleflight before
	// releasing the leader. The singleflight in-flight window isn't observable
	// via channels, so this short wait is the one unavoidable timing element;
	// it only risks a false failure if scheduling stalls >100ms.
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := mockClient.FindPRByBranchCallCount(); got != 1 {
		t.Errorf("expected concurrent detection probes to coalesce to 1 GitHub call, got %d", got)
	}
}
