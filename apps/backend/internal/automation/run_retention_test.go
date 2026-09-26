package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// seedRunAt records one run for an automation with its created_at pinned, so
// retention's ordering assertions don't depend on how many nanoseconds apart
// two CreateRun calls landed.
func seedRunAt(t *testing.T, store *Store, automationID, taskID string, status RunStatus, at time.Time) {
	t.Helper()
	r := &AutomationRun{
		AutomationID: automationID,
		TriggerType:  TriggerTypeScheduled,
		TaskID:       taskID,
		Status:       status,
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	setRunCreatedAt(t, store, r.ID, at)
}

// seedTerminalRunSeries records n succeeded runs for an automation, one minute
// apart, owning tasks named <prefix>-000 … <prefix>-NNN oldest first. Each task
// gets the checkout a real run leaves behind, because a run without one is not
// a retention candidate at all.
func seedTerminalRunSeries(t *testing.T, store *Store, automationID, prefix string, n int) []string {
	t.Helper()
	base := time.Now().UTC().Truncate(time.Second).Add(-time.Duration(n) * time.Hour)
	taskIDs := make([]string, 0, n)
	for i := range n {
		taskID := fmt.Sprintf("%s-%03d", prefix, i)
		seedRunAt(t, store, automationID, taskID, RunStatusSucceeded, base.Add(time.Duration(i)*time.Minute))
		seedLiveWorktree(t, store, taskID)
		taskIDs = append(taskIDs, taskID)
	}
	return taskIDs
}

// seedLiveWorktree gives a task the environment + repository rows that say it
// still holds a checkout on disk. Retention offers only runs that have one, so
// every series has to carry them or the assertions below would all pass on an
// empty result.
func seedLiveWorktree(t *testing.T, store *Store, taskID string) {
	t.Helper()
	if _, err := store.db.Exec(
		`INSERT INTO task_environments (id, task_id) VALUES (?, ?)`,
		"env-"+taskID, taskID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO task_environment_repos (id, task_environment_id, worktree_id, worktree_path, status)
		 VALUES (?, ?, ?, ?, 'active')`,
		taskID+"-ter", "env-"+taskID, "wt-"+taskID, "/workspaces/"+taskID,
	); err != nil {
		t.Fatal(err)
	}
}

// reclaimWorktree marks a task's checkout gone exactly as
// worktree.Manager.ReleaseWorktreeReference does when a removal completes.
func reclaimWorktree(t *testing.T, store *Store, taskID string) {
	t.Helper()
	if _, err := store.db.Exec(
		`UPDATE task_environment_repos SET status = 'deleted', deleted_at = ? WHERE worktree_id = ?`,
		time.Now().UTC(), "wt-"+taskID,
	); err != nil {
		t.Fatal(err)
	}
}

func newRetentionAutomation(t *testing.T, store *Store, name string) *Automation {
	t.Helper()
	a := &Automation{WorkspaceID: "ws-1", Name: name, WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func assertTaskIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

// The defect this closes: every firing keeps a full worktree forever, so a
// frequent schedule fills the disk. Retention has to draw the line at a fixed
// count of the most recent runs and offer everything older — no more, so the
// runs a user actually goes back to are still openable.
func TestPrunableRunTaskIDs_KeepsTheNewestTerminalRunsAndOffersOnlyTheRest(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "nightly sweep")

	tasks := seedTerminalRunSeries(t, store, a.ID, "task", DefaultRunWorktreeRetention+3)
	newest := tasks[len(tasks)-1]

	got, err := store.PrunableRunTaskIDs(context.Background(), newest, DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	// Newest-first among the aged-out remainder: the run that just fell out of
	// the window leads, so a single sweep always reaches it.
	assertTaskIDs(t, got, []string{tasks[2], tasks[1], tasks[0]})
}

// A run still at task_created may have an agent mid-turn. It is not a
// retention candidate at all — neither reclaimed, nor counted toward the N
// that are kept, which would otherwise let three stuck runs evict three
// perfectly good workspaces.
func TestPrunableRunTaskIDs_IgnoresRunsThatHaveNotReachedATerminalStatus(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "nightly sweep")

	base := time.Now().UTC().Truncate(time.Second)
	for i := range 3 {
		openTaskID := "open-" + string(rune('a'+i))
		seedRunAt(t, store, a.ID, openTaskID, RunStatusTaskCreated, base.Add(time.Duration(i)*time.Minute))
		// Given a checkout of its own, so the run's *status* is the only thing
		// keeping it out of the result.
		seedLiveWorktree(t, store, openTaskID)
	}
	tasks := seedTerminalRunSeries(t, store, a.ID, "done", DefaultRunWorktreeRetention)

	got, err := store.PrunableRunTaskIDs(context.Background(), tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nothing prunable — only %d terminal runs exist — got %v", DefaultRunWorktreeRetention, got)
	}
}

// A user can reply to an aged-out run and put it back to work. Reclaiming the
// workspace out from under a live agent is data loss, not a disk saving — so
// age loses to a session that is starting or running. WAITING_FOR_INPUT is
// where every *successful* run parks, so it must not count as in-use or
// nothing would ever be prunable.
func TestPrunableRunTaskIDs_SkipsRunsWhoseAgentIsStillWorking(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "nightly sweep")

	tasks := seedTerminalRunSeries(t, store, a.ID, "task", DefaultRunWorktreeRetention+3)
	insertPrimarySession(t, store, tasks[0], "WAITING_FOR_INPUT")
	insertPrimarySession(t, store, tasks[1], "RUNNING")
	insertPrimarySession(t, store, tasks[2], "STARTING")

	got, err := store.PrunableRunTaskIDs(context.Background(), tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{tasks[0]})
}

// Retention is per-automation on purpose: one noisy automation must not evict
// the single monthly run of a quiet one sharing its workspace.
func TestPrunableRunTaskIDs_IsScopedToTheAutomationOwningTheFinalizedTask(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	noisy := newRetentionAutomation(t, store, "every five minutes")
	quiet := newRetentionAutomation(t, store, "monthly report")

	noisyTasks := seedTerminalRunSeries(t, store, noisy.ID, "noisy", DefaultRunWorktreeRetention+3)
	quietTasks := seedTerminalRunSeries(t, store, quiet.ID, "quiet", DefaultRunWorktreeRetention+3)

	got, err := store.PrunableRunTaskIDs(context.Background(), noisyTasks[len(noisyTasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{noisyTasks[2], noisyTasks[1], noisyTasks[0]})
	for _, quietTask := range quietTasks {
		for _, offered := range got {
			if offered == quietTask {
				t.Fatalf("a run from another automation (%s) was offered for reclamation", quietTask)
			}
		}
	}
}

// Runs with no task never had a workspace — the concurrency-cap skip rows are
// the common case — so they must not consume a slot in the aged-out window and
// push a real candidate out of reach.
func TestPrunableRunTaskIDs_IgnoresRunsThatNeverProducedATask(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "nightly sweep")

	tasks := seedTerminalRunSeries(t, store, a.ID, "task", DefaultRunWorktreeRetention+1)
	base := time.Now().UTC().Truncate(time.Second)
	// A session row carrying the empty task id, so the checkout predicate would
	// happily match these rows and only the empty-task_id filter keeps them out.
	seedLiveWorktree(t, store, "")
	for i := range 5 {
		seedRunAt(t, store, a.ID, "", RunStatusSkipped, base.Add(time.Duration(i)*time.Minute))
	}

	got, err := store.PrunableRunTaskIDs(context.Background(), tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{tasks[0]})
}

// A live session does not have to be the primary one. A resume can race the
// is_primary flag over to a new row, and a passthrough session runs alongside
// the primary — in both cases an agent is holding the checkout while
// is_primary = 1 points somewhere else. Scoping the in-use check to the
// primary session meant exactly that agent's workspace was offered up for
// deletion, which is the data-loss case the check exists to prevent.
func TestPrunableRunTaskIDs_SkipsRunsWhoseNonPrimarySessionIsStillWorking(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "nightly sweep")

	tasks := seedTerminalRunSeries(t, store, a.ID, "task", DefaultRunWorktreeRetention+3)
	insertStaleSession(t, store, tasks[0], "RUNNING")
	insertPrimarySession(t, store, tasks[0], "WAITING_FOR_INPUT")

	got, err := store.PrunableRunTaskIDs(context.Background(), tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{tasks[2], tasks[1]})
}

// Nothing else records that a run's workspace has been reclaimed, so a run
// whose checkout is already gone was offered again on every single
// finalization — a five-minute schedule re-attempting the same ~200 removals
// forever. The worktree row going away is the record.
func TestPrunableRunTaskIDs_StopsOfferingARunWhoseWorkspaceIsAlreadyReclaimed(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "nightly sweep")

	tasks := seedTerminalRunSeries(t, store, a.ID, "task", DefaultRunWorktreeRetention+3)
	reclaimWorktree(t, store, tasks[0])

	got, err := store.PrunableRunTaskIDs(context.Background(), tasks[len(tasks)-1], DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, got, []string{tasks[2], tasks[1]})
}

// The other half of the same defect. The sweep window is LIMIT n OFFSET keep
// over the candidate set, so while reclaimed runs stayed in that set the
// window never moved: a run that had fallen past rank keep+n could not be
// reached by any number of sweeps, and an install that predated retention kept
// its backlog forever. Dropping reclaimed runs out of the set slides the
// window down, so the backlog drains one firing at a time.
func TestPrunableRunTaskIDs_DrainsABacklogAcrossSuccessiveSweeps(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	createTasksTable(t, store)
	a := newRetentionAutomation(t, store, "every five minutes")

	// Five runs deeper than one sweep can see, so the oldest five start out
	// beyond the reach of any single firing.
	const stranded = 5
	total := DefaultRunWorktreeRetention + runWorktreeSweepWindow + stranded
	tasks := seedTerminalRunSeries(t, store, a.ID, "task", total)
	newest := tasks[len(tasks)-1]

	first, err := store.PrunableRunTaskIDs(ctx, newest, DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != runWorktreeSweepWindow {
		t.Fatalf("expected a full %d-run window, got %d", runWorktreeSweepWindow, len(first))
	}
	for _, offered := range first {
		for _, unreachable := range tasks[:stranded] {
			if offered == unreachable {
				t.Fatalf("precondition failed: %s was expected to sit past the sweep window", unreachable)
			}
		}
	}

	for _, taskID := range first {
		reclaimWorktree(t, store, taskID)
	}

	second, err := store.PrunableRunTaskIDs(ctx, newest, DefaultRunWorktreeRetention)
	if err != nil {
		t.Fatal(err)
	}
	assertTaskIDs(t, second, []string{tasks[4], tasks[3], tasks[2], tasks[1], tasks[0]})
}

// The pre-removal re-check the sweep makes once per workspace. It answers for
// the task, not for one session of it, and it agrees with the selection query
// about what counts as in use — WAITING_FOR_INPUT is where every successful
// run parks and must not read as busy.
func TestRunWorkspaceInUse_ReportsAnySessionThatIsStartingOrRunning(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	createTasksTable(t, store)

	insertPrimarySession(t, store, "task-parked", "WAITING_FOR_INPUT")
	insertPrimarySession(t, store, "task-replied", "STARTING")
	insertStaleSession(t, store, "task-passthrough", "RUNNING")
	insertPrimarySession(t, store, "task-passthrough", "WAITING_FOR_INPUT")

	for taskID, want := range map[string]bool{
		"task-parked":       false,
		"task-replied":      true,
		"task-passthrough":  true,
		"task-with-no-rows": false,
		"":                  false,
	} {
		got, err := store.RunWorkspaceInUse(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("RunWorkspaceInUse(%q) = %v, want %v", taskID, got, want)
		}
	}
}
