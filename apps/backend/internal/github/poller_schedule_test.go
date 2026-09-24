package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scheduleActivityProvider struct {
	activities map[string]PRWatchTaskActivity
	err        error
	calls      int
}

func (p *scheduleActivityProvider) LoadPRWatchTaskActivity(
	_ context.Context, taskIDs []string,
) (map[string]PRWatchTaskActivity, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	result := make(map[string]PRWatchTaskActivity, len(taskIDs))
	for _, taskID := range taskIDs {
		if activity, ok := p.activities[taskID]; ok {
			result[taskID] = activity
		}
	}
	return result, nil
}

func TestSearchingWatchAdaptiveSchedule(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	lastChecked := now.Add(-15 * time.Minute)
	createdAt := now.Add(-48 * time.Hour)

	tests := []struct {
		name      string
		watch     *PRWatch
		activity  PRWatchTaskActivity
		wantEvery time.Duration
		wantDue   bool
	}{
		{
			name:      "running is fast",
			watch:     scheduleWatch("running", 0, timePtr(lastChecked), createdAt),
			activity:  PRWatchTaskActivity{Running: true, LastActivityAt: now.Add(-12 * time.Hour)},
			wantEvery: searchFastPollInterval,
			wantDue:   true,
		},
		{
			name:      "below two hours is fast",
			watch:     scheduleWatch("recent", 0, timePtr(lastChecked), createdAt),
			activity:  PRWatchTaskActivity{LastActivityAt: now.Add(-2*time.Hour + time.Second)},
			wantEvery: searchFastPollInterval,
			wantDue:   true,
		},
		{
			name:      "two hour boundary is fifteen minutes",
			watch:     scheduleWatch("two-hours", 0, timePtr(now.Add(-15*time.Minute)), createdAt),
			activity:  PRWatchTaskActivity{LastActivityAt: now.Add(-2 * time.Hour)},
			wantEvery: searchIdlePollInterval,
			wantDue:   true,
		},
		{
			name:      "just below twenty four hours is fifteen minutes",
			watch:     scheduleWatch("day-minus", 0, timePtr(now.Add(-15*time.Minute)), createdAt),
			activity:  PRWatchTaskTaskActivityAt(now.Add(-24*time.Hour + time.Second)),
			wantEvery: searchIdlePollInterval,
			wantDue:   true,
		},
		{
			name:      "twenty four hour boundary is thirty minutes",
			watch:     scheduleWatch("day", 0, timePtr(now.Add(-30*time.Minute)), createdAt),
			activity:  PRWatchTaskTaskActivityAt(now.Add(-24 * time.Hour)),
			wantEvery: searchVeryIdlePollInterval,
			wantDue:   true,
		},
		{
			name:      "slow target waits for its interval",
			watch:     scheduleWatch("slow", 0, timePtr(now.Add(-10*time.Minute)), createdAt),
			activity:  PRWatchTaskTaskActivityAt(now.Add(-24 * time.Hour)),
			wantEvery: searchVeryIdlePollInterval,
			wantDue:   false,
		},
		{
			name:      "new watch is due",
			watch:     scheduleWatch("new", 0, nil, now),
			activity:  PRWatchTaskActivity{},
			wantEvery: searchFastPollInterval,
			wantDue:   true,
		},
		{
			name:      "unknown activity stays fast",
			watch:     scheduleWatch("unknown", 0, timePtr(now.Add(-time.Minute)), createdAt),
			activity:  PRWatchTaskActivity{},
			wantEvery: searchFastPollInterval,
			wantDue:   true,
		},
		{
			name:      "known PR stays fast",
			watch:     scheduleWatch("known", 42, timePtr(now.Add(-time.Minute)), createdAt),
			activity:  PRWatchTaskActivity{LastActivityAt: now.Add(-48 * time.Hour)},
			wantEvery: defaultPRPollInterval,
			wantDue:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prWatchSearchInterval(now, tt.watch, tt.activity); got != tt.wantEvery {
				t.Fatalf("interval = %s, want %s", got, tt.wantEvery)
			}
			if got := prWatchIsDue(now, tt.watch, tt.activity); got != tt.wantDue {
				t.Fatalf("due = %t, want %t", got, tt.wantDue)
			}
		})
	}
}

func TestSearchingWatchCreationKeepsNewWatchFast(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	watch := scheduleWatch("old-task-new-watch", 0, timePtr(now.Add(-2*time.Minute)), now.Add(-30*time.Minute))
	activity := PRWatchTaskActivity{LastActivityAt: now.Add(-48 * time.Hour)}

	if got := prWatchSearchInterval(now, watch, activity); got != searchFastPollInterval {
		t.Fatalf("new watch interval = %s, want %s", got, searchFastPollInterval)
	}
	if !prWatchIsDue(now, watch, activity) {
		t.Fatal("new watch after its first completed check is not due on the fast tier")
	}
}

func TestSearchingWatchAdaptiveScheduleSharedTargets(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	provider := &scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{
		"fast-task": {LastActivityAt: now.Add(-time.Hour)},
		"slow-task": {LastActivityAt: now.Add(-24 * time.Hour)},
	}}
	poller := &Poller{taskActivityProvider: provider, clock: func() time.Time { return now }}

	sharedFast := scheduleWatch("fast-task", 0, timePtr(now.Add(-time.Minute)), now.Add(-time.Hour))
	sharedFast.Owner = "owner"
	sharedFast.Repo = "repo"
	sharedFast.Branch = "shared"
	sharedSlow := scheduleWatch("slow-task", 0, timePtr(now.Add(-time.Minute)), now.Add(-24*time.Hour))
	sharedSlow.Owner = "owner"
	sharedSlow.Repo = "repo"
	sharedSlow.Branch = "shared"
	allIdle := scheduleWatch("slow-task", 0, timePtr(now.Add(-time.Minute)), now.Add(-24*time.Hour))
	allIdle.Owner = "owner"
	allIdle.Repo = "repo"
	allIdle.Branch = "idle"

	got := poller.selectDuePRWatches(context.Background(), []*PRWatch{sharedSlow, sharedFast, allIdle})
	if len(got) != 2 {
		t.Fatalf("due watches = %d, want shared target only (2)", len(got))
	}
	if got[0] != sharedSlow || got[1] != sharedFast {
		t.Fatalf("due watches = [%p, %p], want shared watches [%p, %p]", got[0], got[1], sharedSlow, sharedFast)
	}
	if provider.calls != 1 {
		t.Fatalf("activity provider calls = %d, want 1 bulk call", provider.calls)
	}
}

func TestSearchingWatchAdaptiveScheduleSeparatesNumberedTarget(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	provider := &scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{
		"searching-task": {LastActivityAt: now.Add(-24 * time.Hour)},
	}}
	poller := &Poller{taskActivityProvider: provider, clock: func() time.Time { return now }}

	numbered := scheduleWatch("numbered-task", 42, timePtr(now.Add(-time.Minute)), now.Add(-time.Hour))
	numbered.Owner, numbered.Repo, numbered.Branch = "owner", "repo", "shared"
	searching := scheduleWatch("searching-task", 0, timePtr(now.Add(-time.Minute)), now.Add(-24*time.Hour))
	searching.Owner, searching.Repo, searching.Branch = "owner", "repo", "shared"

	got := poller.selectDuePRWatches(context.Background(), []*PRWatch{searching, numbered})
	if len(got) != 1 || got[0] != numbered {
		t.Fatalf("due watches = %#v, want only numbered watch", got)
	}
}

func TestSearchingWatchAdaptiveScheduleActivityAndRestart(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	watch := scheduleWatch("task-1", 0, timePtr(now.Add(-10*time.Minute)), now.Add(-48*time.Hour))
	activity := PRWatchTaskActivity{LastActivityAt: now.Add(-24 * time.Hour)}
	provider := &scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{"task-1": activity}}

	first := &Poller{taskActivityProvider: provider, clock: func() time.Time { return now }}
	if got := first.selectDuePRWatches(context.Background(), []*PRWatch{watch}); len(got) != 0 {
		t.Fatalf("slow watch due before interval = %d, want 0", len(got))
	}

	// A fresh activity observation makes the same persisted last check due on
	// the next scheduler tick, even though the previous cycle used the slow tier.
	provider.activities["task-1"] = PRWatchTaskActivity{LastActivityAt: now.Add(-time.Minute)}
	if got := first.selectDuePRWatches(context.Background(), []*PRWatch{watch}); len(got) != 1 {
		t.Fatalf("watch after fresh activity due = %d, want 1", len(got))
	}

	// A new poller sees the same persisted timestamp and activity projection.
	provider.activities["task-1"] = activity
	restarted := &Poller{taskActivityProvider: provider, clock: func() time.Time { return now.Add(20 * time.Minute) }}
	if got := restarted.selectDuePRWatches(context.Background(), []*PRWatch{watch}); len(got) != 1 {
		t.Fatalf("watch after restart due = %d, want 1", len(got))
	}
}

func TestSearchingWatchAdaptiveScheduleProviderFailureFailsOpen(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	watch := scheduleWatch("task-1", 0, timePtr(now.Add(-time.Minute)), now.Add(-48*time.Hour))
	poller := &Poller{
		taskActivityProvider: &scheduleActivityProvider{err: errors.New("activity unavailable")},
		clock:                func() time.Time { return now },
	}
	if got := poller.selectDuePRWatches(context.Background(), []*PRWatch{watch}); len(got) != 1 {
		t.Fatalf("watch after activity read failure due = %d, want 1", len(got))
	}
}

func TestSearchingWatchAdaptiveScheduleKeepsExternalPRDiscoverable(t *testing.T) {
	now := time.Date(2026, time.September, 22, 12, 0, 0, 0, time.UTC)
	watch := scheduleWatch("task-external", 0, timePtr(now.Add(-30*time.Minute)), now.Add(-48*time.Hour))
	activity := PRWatchTaskActivity{LastActivityAt: now.Add(-48 * time.Hour)}
	poller := &Poller{
		taskActivityProvider: &scheduleActivityProvider{activities: map[string]PRWatchTaskActivity{
			watch.TaskID: activity,
		}},
		clock: func() time.Time { return now },
	}
	if got := poller.selectDuePRWatches(context.Background(), []*PRWatch{watch}); len(got) != 1 {
		t.Fatalf("external PR first slow-tier check = %d, want 1", len(got))
	}
	// No local commit or task activity is added. The 30-minute tier remains a
	// periodic search, so a PR opened externally remains discoverable.
	poller.clock = func() time.Time { return now.Add(30 * time.Minute) }
	if got := poller.selectDuePRWatches(context.Background(), []*PRWatch{watch}); len(got) != 1 {
		t.Fatalf("external PR later slow-tier check = %d, want 1", len(got))
	}
}

func TestSearchingWatchAdaptiveScheduleExcludesArchivedAndDeletedTasks(t *testing.T) {
	poller, _, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "task-schedule-active", false)
	seedTask(t, store, "task-schedule-archived", true)
	for _, taskID := range []string{"task-schedule-active", "task-schedule-archived"} {
		if err := store.CreatePRWatch(ctx, withTestWorkspace(&PRWatch{
			SessionID: "session-" + taskID, TaskID: taskID, Owner: "owner", Repo: "repo", Branch: taskID,
		})); err != nil {
			t.Fatalf("create watch for %s: %v", taskID, err)
		}
	}

	active, err := store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list active watches: %v", err)
	}
	if len(active) != 1 || active[0].TaskID != "task-schedule-active" {
		t.Fatalf("active watches = %+v, want active task only", active)
	}
	if _, err := store.db.Exec(`DELETE FROM tasks WHERE id = ?`, "task-schedule-active"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	active, err = store.ListActivePRWatches(ctx)
	if err != nil {
		t.Fatalf("list watches after delete: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("watches after task delete = %+v, want none", active)
	}
	_ = poller
}

func scheduleWatch(taskID string, prNumber int, lastChecked *time.Time, createdAt time.Time) *PRWatch {
	return &PRWatch{
		ID:            taskID + "-watch",
		TaskID:        taskID,
		WorkspaceID:   "workspace-1",
		Owner:         "owner-" + taskID,
		Repo:          "repo-" + taskID,
		Branch:        "branch-" + taskID,
		PRNumber:      prNumber,
		LastCheckedAt: lastChecked,
		CreatedAt:     createdAt,
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func PRWatchTaskTaskActivityAt(at time.Time) PRWatchTaskActivity {
	return PRWatchTaskActivity{LastActivityAt: at}
}
