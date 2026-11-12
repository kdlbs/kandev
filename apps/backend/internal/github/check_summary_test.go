package github

import (
	"context"
	"testing"
)

func selectedCheckFixtures() []CheckRun {
	return []CheckRun{
		{Name: "build", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSuccess},
		{Name: "lint", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
		{Name: "docs", Source: checkSourceStatusContext, Status: checkStatusCompleted, Conclusion: checkConclusionNeutral},
		{Name: "deploy", Source: checkSourceCheckRun, Status: checkStatusCompleted, Conclusion: checkConclusionCancelled},
		{Name: "test", Source: checkSourceCheckRun, Status: checkStatusInProgress},
	}
}

func TestCheckSummarySelectedSkipped(t *testing.T) {
	summary := summarizeSelectedChecks("head-a", selectedCheckFixtures())
	if summary == nil {
		t.Fatal("summarizeSelectedChecks returned nil")
	}
	if summary.HeadSHA != "head-a" {
		t.Fatalf("head = %q, want head-a", summary.HeadSHA)
	}
	if summary.Observed != 5 {
		t.Fatalf("observed = %d, want 5", summary.Observed)
	}
	if summary.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", summary.Skipped)
	}
	if summary.AllSkipped() {
		t.Fatal("mixed snapshot reported all skipped")
	}
}

func TestCheckSummaryAllSkippedRequiresEveryRecord(t *testing.T) {
	tests := []struct {
		name   string
		checks []CheckRun
		want   bool
	}{
		{
			name: "all skipped",
			checks: []CheckRun{
				{Name: "a", Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
				{Name: "b", Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
			},
			want: true,
		},
		{
			name: "skipped and cancelled",
			checks: []CheckRun{
				{Name: "a", Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
				{Name: "b", Status: checkStatusCompleted, Conclusion: checkConclusionCancelled},
			},
			want: false,
		},
		{
			name: "skipped and neutral",
			checks: []CheckRun{
				{Name: "a", Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
				{Name: "b", Status: checkStatusCompleted, Conclusion: checkConclusionNeutral},
			},
			want: false,
		},
		{
			name: "skipped plus running",
			checks: []CheckRun{
				{Name: "a", Status: checkStatusCompleted, Conclusion: checkConclusionSkipped},
				{Name: "b", Status: checkStatusInProgress},
			},
			want: false,
		},
		{name: "empty", checks: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := summarizeSelectedChecks("head-a", test.checks)
			if got := summary.AllSkipped(); got != test.want {
				t.Fatalf("AllSkipped = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCheckSummaryRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name    string
		summary *CheckSummary
		head    string
	}{
		{name: "nil", summary: nil, head: "head-a"},
		{name: "wrong head", summary: &CheckSummary{HeadSHA: "head-b", Observed: 2}, head: "head-a"},
		{name: "empty head", summary: &CheckSummary{HeadSHA: "head-a", Observed: 2}, head: ""},
		{name: "skipped beyond observed", summary: &CheckSummary{HeadSHA: "head-a", Observed: 1, Skipped: 2}, head: "head-a"},
		{name: "negative observed", summary: &CheckSummary{HeadSHA: "head-a", Observed: -1}, head: "head-a"},
		{name: "oversized", summary: &CheckSummary{HeadSHA: "head-a", Observed: maxCheckSummaryObserved + 1}, head: "head-a"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validCheckSummary(test.summary, test.head); got != nil {
				t.Fatalf("validCheckSummary = %#v, want nil", got)
			}
		})
	}
	valid := &CheckSummary{HeadSHA: "head-a", Observed: 3, Skipped: 3}
	if got := validCheckSummary(valid, "head-a"); got != valid {
		t.Fatalf("validCheckSummary rejected a valid summary: %#v", got)
	}
}

func TestCheckSummaryEmptySnapshotIsExplicit(t *testing.T) {
	summary := summarizeSelectedChecks("head-a", []CheckRun{})
	if summary == nil || summary.Observed != 0 || summary.Skipped != 0 {
		t.Fatalf("empty snapshot = %#v, want observed=0 skipped=0", summary)
	}
	if summary.AllSkipped() {
		t.Fatal("empty snapshot reported all skipped")
	}
}

func TestResolveTaskPRCheckSummaryLifecycle(t *testing.T) {
	previous := &TaskPR{
		HeadSHA:      "head-a",
		CheckSummary: &CheckSummary{HeadSHA: "head-a", Observed: 4, Skipped: 3},
	}

	rollupOnly := &PRStatus{PR: &PR{HeadSHA: "head-a"}}
	got := resolveTaskPRCheckSummary(previous, rollupOnly, "head-a")
	if got == nil || got.Skipped != 3 {
		t.Fatalf("rollup-only same-head summary = %#v, want preserved skipped=3", got)
	}

	detailed := &PRStatus{
		PR:              &PR{HeadSHA: "head-a"},
		ChecksPopulated: true,
		CheckSummary:    &CheckSummary{HeadSHA: "head-a", Observed: 2, Skipped: 0},
	}
	got = resolveTaskPRCheckSummary(previous, detailed, "head-a")
	if got == nil || got.Observed != 2 || got.Skipped != 0 {
		t.Fatalf("detailed replacement = %#v, want observed=2 skipped=0", got)
	}

	empty := &PRStatus{
		PR:              &PR{HeadSHA: "head-a"},
		ChecksPopulated: true,
		CheckSummary:    &CheckSummary{HeadSHA: "head-a", Observed: 0, Skipped: 0},
	}
	got = resolveTaskPRCheckSummary(previous, empty, "head-a")
	if got == nil || got.Observed != 0 {
		t.Fatalf("authoritative empty replacement = %#v, want observed=0", got)
	}

	newHead := &PRStatus{PR: &PR{HeadSHA: "head-b"}}
	got = resolveTaskPRCheckSummary(previous, newHead, "head-b")
	if got != nil {
		t.Fatalf("head change summary = %#v, want nil", got)
	}

	staleHeadSummary := &PRStatus{
		PR:              &PR{HeadSHA: "head-b"},
		ChecksPopulated: true,
		CheckSummary:    &CheckSummary{HeadSHA: "head-a", Observed: 1, Skipped: 1},
	}
	got = resolveTaskPRCheckSummary(previous, staleHeadSummary, "head-b")
	if got != nil {
		t.Fatalf("wrong-head detailed summary = %#v, want nil", got)
	}
}

func TestSyncTaskPRCheckSummaryLifecycle(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	taskPR := &TaskPR{
		TaskID: "task-check-summary", Owner: "owner", Repo: "repo", PRNumber: 51,
		PRURL: "https://example.test/51", PRTitle: "skipped", State: "open",
		HeadSHA: "head-a", CreatedAt: nowUTC(),
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}

	detailed := &PRStatus{
		PR:              &PR{Number: 51, State: "open", HeadSHA: "head-a", RepoOwner: "owner", RepoName: "repo"},
		ChecksPopulated: true,
		CheckSummary:    &CheckSummary{HeadSHA: "head-a", Observed: 3, Skipped: 3},
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, detailed); err != nil {
		t.Fatalf("SyncTaskPR (detailed): %v", err)
	}
	got := mustGetTaskPR(t, store, ctx, taskPR.ID)
	if got.CheckSummary == nil || got.CheckSummary.Skipped != 3 {
		t.Fatalf("persisted check summary = %#v, want skipped=3", got.CheckSummary)
	}

	rollupOnly := &PRStatus{
		PR: &PR{Number: 51, State: "open", HeadSHA: "head-a", RepoOwner: "owner", RepoName: "repo"},
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, rollupOnly); err != nil {
		t.Fatalf("SyncTaskPR (rollup): %v", err)
	}
	got = mustGetTaskPR(t, store, ctx, taskPR.ID)
	if got.CheckSummary == nil || got.CheckSummary.Skipped != 3 {
		t.Fatalf("same-head rollup summary = %#v, want preserved skipped=3", got.CheckSummary)
	}

	newHead := &PRStatus{
		PR:              &PR{Number: 51, State: "open", HeadSHA: "head-b", RepoOwner: "owner", RepoName: "repo"},
		ChecksPopulated: true,
		CheckSummary:    &CheckSummary{HeadSHA: "head-b", Observed: 1, Skipped: 0},
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, newHead); err != nil {
		t.Fatalf("SyncTaskPR (new head): %v", err)
	}
	got = mustGetTaskPR(t, store, ctx, taskPR.ID)
	if got.CheckSummary == nil || got.CheckSummary.HeadSHA != "head-b" || got.CheckSummary.Skipped != 0 {
		t.Fatalf("new-head summary = %#v, want head-b skipped=0", got.CheckSummary)
	}
}

func TestSyncTaskPRCheckSummaryPublishesOnlyOnChange(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	taskPR := &TaskPR{
		TaskID: "task-check-publish", Owner: "owner", Repo: "repo", PRNumber: 52,
		PRURL: "https://example.test/52", PRTitle: "publish", State: "open",
		HeadSHA: "head-a", CreatedAt: nowUTC(),
	}
	if err := store.CreateTaskPR(ctx, taskPR); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}
	status := &PRStatus{
		PR:              &PR{Number: 52, State: "open", HeadSHA: "head-a", RepoOwner: "owner", RepoName: "repo"},
		ChecksPopulated: true,
		CheckSummary:    &CheckSummary{HeadSHA: "head-a", Observed: 2, Skipped: 2},
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, status); err != nil {
		t.Fatalf("first SyncTaskPR: %v", err)
	}
	first := eventBus.publishedCount()
	if first == 0 {
		t.Fatal("first sync with a new check summary published no event")
	}
	if err := svc.SyncTaskPR(ctx, taskPR.TaskID, status); err != nil {
		t.Fatalf("second SyncTaskPR: %v", err)
	}
	if eventBus.publishedCount() != first {
		t.Fatalf("unchanged check summary republished: %d -> %d", first, eventBus.publishedCount())
	}
}
