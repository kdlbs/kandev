package automation

import (
	"context"
	"strings"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// DefaultRunWorktreeRetention is how many of an automation's most recent
// terminal runs keep their git worktree. Every firing is now a persistent task
// that deliberately retains its workspace, so a five-minute schedule would
// otherwise accumulate a full checkout every five minutes, forever — the disk
// on a long-lived install fills with workspaces nobody will ever open again.
//
// Ten is chosen to be larger than any plausible "go back and look at the last
// few runs" window while still bounding the cost at ten checkouts per
// automation. It is deliberately per-automation rather than global: a
// workspace with one noisy automation must not evict the single monthly run of
// a quiet one.
const DefaultRunWorktreeRetention = 10

// runWorktreeSweepWindow bounds how many aged-out runs a single finalization
// will look at. Candidates are only ever runs that still hold a checkout (see
// runHasLiveWorktreeSQL), so in the steady state exactly one run enters the
// window per firing and the slack is there to drain a backlog left by a
// restart or by an install that predates retention. Without the cap, an
// automation with a year of history would walk its entire run table on every
// single firing.
//
// The window opens where retention ends and reads *newest*-first, so the run
// that just aged out is always the first candidate. Reading oldest-first would
// permanently park the window on the far end of history, where every candidate
// has already been reclaimed, and the run that actually needs reclaiming would
// never be reached.
const runWorktreeSweepWindow = 200

// runHasLiveWorktreeSQL restricts the candidate set to runs that still have a
// checkout to reclaim. It is what makes the sweep converge, and it fixes two
// halves of the same defect.
//
// Nothing else records that a run's workspace was already reclaimed, so
// without this clause every finalization re-offered the same aged-out runs
// forever: a five-minute schedule spent ~57,000 pointless removal attempts a
// day handing already-removed checkouts back to git. Worse, because the window
// is `LIMIT n OFFSET keep` over that unfiltered set, any run that had fallen
// past rank keep+n was permanently out of reach — a pre-existing backlog could
// never drain, no matter how many times the sweep ran.
//
// Excluding the reclaimed ones makes the window slide: each firing sees only
// runs that still cost disk, so the backlog drains across successive firings
// and the steady state is a window one deep. The worktree row is reached
// through the task environment because that is how a checkout is keyed — a
// task owns an environment, an environment owns worktree rows — and
// `status <> 'deleted'` is the exact inverse of what
// worktree.Manager.ReleaseWorktreeReference writes when a checkout goes away.
const runHasLiveWorktreeSQL = `
			EXISTS (
				SELECT 1 FROM task_environment_repos ter
				JOIN task_environments wte ON wte.id = ter.task_environment_id
				WHERE wte.task_id = ar.task_id AND ter.status <> ?
			)`

// PrunableRunTaskIDs names the tasks whose workspaces may be reclaimed, given a
// task whose run has just reached a terminal status. The automation is resolved
// from that task rather than passed in because the orchestrator's finalization
// hook knows only which task finished.
//
// Four things are deliberately excluded. A run that has not reached a stored
// terminal status is still outstanding — its agent may be mid-turn — and is not
// a retention candidate at all, so it neither gets reclaimed nor counts against
// the newest-N that are kept. A run with no task never had a workspace (the
// concurrency-cap skip rows). A run whose workspace has already been reclaimed
// is done with (runHasLiveWorktreeSQL). And a run with a session that is
// STARTING or RUNNING is being worked on right now, however old the row is — a
// user can reply to an aged-out run and put it back to work, and pulling the
// floor out from under a live agent is a data-loss bug, not a disk saving.
//
// The in-use check deliberately looks at *every* session of the task, not only
// the is_primary one. A task can hold a live session that is not flagged
// primary (a resume races the flag, a passthrough session runs alongside), and
// scoping the check to is_primary = 1 let exactly that agent's checkout be
// deleted underneath it. Cheap over-protection here costs one retained
// workspace for one more sweep; under-protection costs a user's work.
//
// This list is a *selection*, not a licence: by the time the orchestrator gets
// round to removing a checkout the run may have gone live, so the caller
// re-checks with RunWorkspaceInUse immediately before each removal. See
// pruneAutomationRunWorktrees.
//
// WAITING_FOR_INPUT is pointedly *not* treated as in-use. That is where every
// successful automation run parks (see the orchestrator's
// handleAutomationTurnComplete), so excluding it would make nothing prunable
// and the retention policy a no-op. Losing the ability to reply to a run older
// than the retention window is the accepted cost of bounding the disk.
//
// Rows are never touched: this is a read, and the run row, its error message,
// and its transcript all outlive the workspace. Reclaiming the checkout is the
// whole of the policy.
func (s *Store) PrunableRunTaskIDs(ctx context.Context, finalizedTaskID string, keep int) ([]string, error) {
	if finalizedTaskID == "" {
		return nil, nil
	}
	if keep < 0 {
		keep = 0
	}
	args := []any{finalizedTaskID, string(RunStatusTriggered), string(RunStatusTaskCreated)}
	args = append(args, runWorktreeInUseArgs()...)
	args = append(args, worktree.StatusDeleted, runWorktreeSweepWindow, keep)

	var taskIDs []string
	// A reusable automation can have many terminal run rows for one task. The
	// retention unit is the distinct task checkout, not each run row. Keep the
	// newest run ordering for each task, and protect the current continuation
	// even when it has already parked after a successful turn.
	err := s.ro.SelectContext(ctx, &taskIDs, `
		WITH terminal_tasks AS (
			SELECT ar.automation_id, ar.task_id,
				MAX(ar.created_at) AS latest_created_at,
				MAX(ar.id) AS latest_id
			FROM automation_runs ar
			WHERE ar.automation_id = (
					SELECT automation_id FROM automation_runs
					WHERE task_id = ? ORDER BY created_at DESC, id DESC LIMIT 1
				)
				AND ar.task_id != ''
				AND ar.status NOT IN (?, ?)
			GROUP BY ar.automation_id, ar.task_id
		)
		SELECT tt.task_id FROM terminal_tasks tt
		WHERE NOT EXISTS (
				SELECT 1 FROM automations a
				WHERE a.id = tt.automation_id AND a.continuation_task_id = tt.task_id
			)
			AND NOT EXISTS (
				SELECT 1 FROM task_sessions ts
				WHERE ts.task_id = tt.task_id AND ts.state IN (?, ?)
			)
			AND`+strings.ReplaceAll(runHasLiveWorktreeSQL, "ar.task_id", "tt.task_id")+`
		ORDER BY tt.latest_created_at DESC, tt.latest_id DESC
		LIMIT ? OFFSET ?`, args...)
	return taskIDs, err
}

// RunWorkspaceInUse reports whether any session of the given task is starting
// or running right now.
//
// It exists because PrunableRunTaskIDs answers a question that goes stale.
// Selection happens once per finalization and the removals then run one after
// another against git and the filesystem; a user replying to an aged-out run
// anywhere in that gap puts an agent back to work in a checkout the sweep is
// already committed to deleting. That is silent data loss — the agent's
// working tree disappears mid-turn — and the worktree manager does not catch
// it: removeWorktree counts active references with the worktree's *own*
// session excluded, and for an automation run that own session is precisely
// the one that just came alive.
//
// So the orchestrator re-asks this immediately before each removal, which is
// as close to the side effect as this layer can get. The window is not closed
// — only a lock the manager does not offer would close it — but it shrinks
// from "the whole sweep" to "one query". A run that turns out to be live is
// left alone; it ages out again the next time its automation fires.
func (s *Store) RunWorkspaceInUse(ctx context.Context, taskID string) (bool, error) {
	if taskID == "" {
		return false, nil
	}
	var inUse bool
	args := append([]any{taskID}, runWorktreeInUseArgs()...)
	err := s.ro.GetContext(ctx, &inUse, `
		SELECT EXISTS (
			SELECT 1 FROM task_sessions ts
			WHERE ts.task_id = ? AND ts.state IN (?, ?)
		)`, args...)
	return inUse, err
}

// runWorktreeInUseArgs binds the session states that mean an agent is holding
// its workspace right now. Kept next to the query, like runTaskStateArgs, so a
// new state can't be added to the IN list without its argument being obvious.
func runWorktreeInUseArgs() []any {
	return []any{
		string(taskmodels.TaskSessionStateStarting),
		string(taskmodels.TaskSessionStateRunning),
	}
}
