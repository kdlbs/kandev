---
status: draft
created: 2026-08-07
owner: nova28
---

# Session Delete Resource Cleanup

## Why

Deleting a session removes its conversation from the UI but leaves its git
worktree on disk forever. Nothing ever reclaims it: the delete cascades away the
only database row that records the worktree, so every later cleanup path looks
for it, does not find it, and treats "not found" as "already clean". Users who
spawn and discard sessions accumulate full repository checkouts they cannot see,
cannot list, and cannot remove without deleting directories by hand.

## What

- Deleting a session SHALL reclaim every worktree that the session was the last
  live holder of. Reclaiming means the worktree directory is gone from disk and
  the source repository no longer carries a registration for it.
- A worktree still referenced by another session SHALL be preserved. Only the
  deleted session's own claim on it goes away.
- Deleting a session SHALL NOT delete any git branch. Unpushed commits survive
  the delete and remain checkoutable from the source repository.
- Session delete SHALL uphold this invariant: **a directory registered as a git
  worktree of a kandev-managed repository exists on disk only while at least one
  live `task_session_worktrees` row references it, or a cleanup job owns it.**
  No path may leave a directory that satisfies neither.

  The invariant is scoped to registered git worktrees on purpose. The tasks base
  also holds task-scoped scratch workspaces for repo-less tasks, which are
  standalone git repositories rather than worktrees of a source repo and carry no
  `task_session_worktrees` row by design. They are outside this invariant and
  outside this feature.
- The resource inventory for a session SHALL be captured and durably recorded
  **before** the session row is removed. The session row is the only pointer to
  its worktrees; once it is gone, an un-recorded worktree is unrecoverable.
- If the cleanup intent cannot be durably recorded, the delete SHALL fail and
  the session SHALL remain. A delete that cannot promise reclamation must not
  happen.
- Reclamation SHALL survive a backend restart. A crash between removing the
  session row and freeing the directory resumes and completes on next startup.
- Reclamation is idempotent. Repeating it against an already-removed directory,
  an already-pruned registration, or an already-deleted session succeeds.
- A session with no worktrees (repo-less task, quick chat, never launched) SHALL
  delete successfully with nothing to reclaim.
- A multi-repo session SHALL have every one of its worktrees evaluated
  independently. One shared worktree being preserved does not prevent its
  siblings from being reclaimed.
- Before confirming a delete, the user SHALL be shown the count of uncommitted
  files and the count of unpushed commits in the session's worktrees whenever
  either is non-zero. Confirming proceeds with the delete.
- Delete confirmation copy SHALL describe what is actually removed. It may no
  longer state that only conversation history is deleted.
- Reclamation failure SHALL be observable. A worktree that could not be freed
  after the retry budget is exhausted is reported with its path, not silently
  forgotten.
- Existing behavior is preserved: a session in `RUNNING` or `STARTING` state is
  refused; the agent execution is quiesced before removal; deleting the primary
  session promotes another session to primary.

## Data model

No new tables. Three existing stores carry this feature.

### `task_session_worktrees` (existing, unchanged shape)

The sole record of a worktree. Its cascade is what makes an unrecorded delete
unrecoverable.

```
id             string     PK
session_id     string     FK -> task_sessions.id ON DELETE CASCADE
worktree_id    string     the worktree identity; NOT unique across sessions
repository_id  string     FK-ish -> repositories.id
branch_slug    string     '' for the legacy single-branch identity
worktree_path  string     absolute path on disk
worktree_branch string    branch checked out in the worktree
status         enum       active | merged | deleted
deleted_at     timestamp  nullable
UNIQUE(session_id, worktree_id)
```

Several sessions sharing one task-environment worktree hold **one row each**
with the **same `worktree_id`**. That is the sharing relationship this feature
must respect.

A worktree is **exclusively held** by the session being deleted when no *other*
session has a row for that `worktree_id` with `status <> 'deleted'` and
`deleted_at IS NULL`. Only exclusively-held worktrees are reclaimed.

The deleted session's own row MUST be excluded from that count. This is the
single easiest thing to get wrong: evaluated naively before the session row is
removed, the count always includes the deleted session's own row, is therefore
never zero, and nothing is ever reclaimed — reproducing today's leak while
appearing to implement the feature. Either evaluate after the session row (and
its cascaded association) is gone, or evaluate before and pass the deleted
session's id as an explicit exclusion.

### `task_resource_cleanup_jobs` (existing, one new trigger value)

The durable cleanup contract defined in
[tasks/runtime-cleanup](../tasks/runtime-cleanup.md) is reused verbatim: the
same states, the same retry budget, the same snapshot-as-inventory rule. This
feature adds one trigger value:

```
trigger: session_delete
```

The job's resource snapshot carries the deleted session and its worktrees. As
with the existing triggers, the job's task reference is deliberately not
foreign-keyed, so the job outlives the session and task cascades and recovers
purely from its snapshot.

### `task_session_git_snapshots` (existing, read-only here)

Source of the pre-delete warning counts: `ahead` is the unpushed-commit count
and `files` is the uncommitted-change set. These rows cascade away with the
session, so they must be read before the session row is removed.

## API surface

### WS `session.delete` — unchanged contract

```
request  { "session_id": string }          // required, non-empty
response { "success": true }
```

Errors keep their current codes: `BAD_REQUEST` on an unparseable payload,
`VALIDATION` on a missing `session_id`, `INTERNAL_ERROR` otherwise.

The action name, payload, and response shape do not change. The documented
public action in `docs/public/websocket-api.md` stays as-is.

What changes is the **meaning of success**: `{"success": true}` asserts that the
session row is gone **and** that reclamation of its worktrees is durably owned.
It does not assert that the directory is already gone — filesystem work runs in
the durable cleanup worker. Conformance tests observe reclamation by waiting for
the owning cleanup job to reach `succeeded`.

Two new `INTERNAL_ERROR` conditions, both of which leave the session intact:

- the resource inventory could not be read;
- the cleanup intent could not be durably recorded.

### Pre-delete warning data

The confirmation surface reads the session's uncommitted-file and unpushed-commit
counts through the existing `session.git.snapshots` action. No new action is
introduced.

The dialog's in-place progress and error feedback are specified by
[ui/session-tab-delete-feedback](../ui/session-tab-delete-feedback.md) and are
unchanged. This spec adds a warning block and revises the body copy; it does not
alter the dialog's controls, spinner behavior, or toast policy.

## State machine

Reclamation of one worktree, evaluated per worktree held by the deleted session:

| State | Trigger | Actor | Next |
|---|---|---|---|
| `held` | session delete requested | user / API caller | `evaluated` |
| `evaluated` | another live session holds this `worktree_id` | cleanup worker | `preserved` (terminal) |
| `evaluated` | no other live session holds it | cleanup worker | `reclaiming` |
| `evaluated` | the reference count could not be determined | cleanup worker | `held` (backoff, retry) |
| `reclaiming` | directory removed and registration pruned | cleanup worker | `reclaimed` (terminal) |
| `reclaiming` | removal failed, retries remain | cleanup worker | `reclaiming` (backoff) |
| `reclaiming` | removal failed, retry budget exhausted | cleanup worker | `unreclaimed` (terminal, reported) |

The owning job's own lifecycle (`prepared` → `pending` → `running` →
`retry_wait` / `succeeded` / `failed`), its eight-attempt budget, and its
backoff schedule are inherited unchanged from
[tasks/runtime-cleanup](../tasks/runtime-cleanup.md).

## Permissions

- A caller may delete only a session they are authorized to reach. The existing
  session authorization check applies before any state is read or mutated, and
  runs first — before the repository, executor, or agent manager is touched.
- The durable cleanup worker runs as an internal caller. It SHALL act only on
  the resources captured in its own snapshot at delete time; it does not widen
  scope by re-querying by task or workspace.

## Failure modes

| Condition | Behavior |
|---|---|
| Session is `RUNNING` or `STARTING` | Delete refused with an error naming the state. Nothing is removed. |
| Session does not exist | Delete fails with `INTERNAL_ERROR`. Existing behavior, unchanged: a second delete of an already-deleted session is an error, not a silent success. Reclamation of the first delete is unaffected because the cleanup job recovers from its own snapshot. |
| Agent execution cannot be stopped | Existing behavior: the session row is preserved and the delete fails, so trailing frames cannot resurrect a removed session. No reclamation is attempted. |
| Worktree inventory read fails | Delete fails closed. Session row preserved. Deleting blind would recreate the permanent leak. |
| Cleanup intent cannot be persisted | Delete fails closed. Session row preserved. |
| `git worktree remove` fails | Fall back to forced directory removal, then prune stale registrations. Only if both fail does the attempt fail. |
| Worktree directory already absent | Treated as successfully reclaimed. |
| Reference-count query fails | Fail closed for that worktree: preserve it and retry. Never reclaim a worktree whose sharing status is unknown. |
| Retry budget exhausted | The job becomes terminally `failed`. The unreclaimed worktree paths are recorded on the job and logged at warning level, so the leak is discoverable rather than silent. |
| Backend restarts mid-reclamation | The job resumes from its snapshot and completes. |
| Cleanup script for the worktree fails | Non-fatal. Reclamation continues. |
| Task environment still names a reclaimed worktree | Non-fatal and self-healing. A later launch resolves no worktree for that id and creates a fresh one; a later task teardown finds nothing to destroy and succeeds. |

## Persistence guarantees

- **Survives restart:** the cleanup job and its resource snapshot; the git
  branch of every reclaimed worktree; every preserved worktree still held by a
  live session.
- **Does not survive:** the deleted session row and everything cascading from it
  (messages, turns, git snapshots, commits, file reviews, worktree associations);
  the reclaimed worktree directory and its git registration; in-memory worktree
  cache entries, git-snapshot throttle entries, push-tracker entries, and
  turn-activity tokens for the deleted session.
- **No grace period.** Reclamation is not time-delayed and does not depend on a
  periodic sweeper. The guarantees here hold on an install where no orphan-
  directory garbage collector runs at all.

## Scenarios

**Golden path**

- **GIVEN** a stopped session that is the only session holding worktree `W` at
  path `P` on branch `B`, **WHEN** the session is deleted, **THEN** the owning
  cleanup job reaches `succeeded`, `P` no longer exists on disk, the source
  repository lists no worktree registered at `P`, and branch `B` still exists in
  the source repository.

- **GIVEN** a session whose worktree `W` at path `P` had two commits that were
  never pushed, **WHEN** the session is deleted and reclamation completes,
  **THEN** `git rev-parse B` in the source repository still resolves and both
  commits are reachable from `B`.

**Sharing**

- **GIVEN** sessions `S1` and `S2` both holding rows for the same `worktree_id`
  `W` at path `P`, **WHEN** `S1` is deleted, **THEN** `P` still exists, `S2`'s
  association row is untouched, and no reclamation is attempted for `W`.

- **GIVEN** sessions `S1` and `S2` both holding `W` at path `P`, **WHEN** `S1`
  is deleted and then `S2` is deleted, **THEN** after the second delete's job
  succeeds `P` no longer exists.

- **GIVEN** a session holding worktrees `W1` (shared with another session) and
  `W2` (exclusively held), **WHEN** the session is deleted, **THEN** `W2`'s
  directory is removed and `W1`'s directory is preserved.

- **GIVEN** a task whose only session holds worktree `W` at path `P`, so `W` has
  exactly one association row and that row belongs to the session being deleted,
  **WHEN** the session is deleted, **THEN** `P` is removed. (Regression guard: an
  implementation that counts references without excluding the deleted session
  sees one reference here, preserves `P`, and silently reproduces the original
  leak while every sharing scenario above still passes.)

**Nothing to reclaim**

- **GIVEN** a session on a repo-less task with no worktree rows, **WHEN** it is
  deleted, **THEN** the delete succeeds, no directory is removed, and no
  reclamation error is reported.

**Refusals and fail-closed**

- **GIVEN** a session in `RUNNING` state, **WHEN** a delete is requested,
  **THEN** the request is rejected with an error naming the state, the session
  row still exists, and its worktree directory still exists.

- **GIVEN** the worktree inventory read fails, **WHEN** a delete is requested,
  **THEN** the request fails with `INTERNAL_ERROR`, the session row still
  exists, and no cleanup job is created.

- **GIVEN** persisting the cleanup intent fails, **WHEN** a delete is requested,
  **THEN** the request fails with `INTERNAL_ERROR` and the session row still
  exists.

- **GIVEN** a caller not authorized for the session, **WHEN** they request the
  delete, **THEN** it is denied, the session row and its worktree directories are
  unchanged, and no cleanup job is created.

**Durability**

- **GIVEN** a delete whose session row has been removed and whose cleanup job is
  `pending`, **WHEN** the backend restarts before reclamation runs, **THEN** the
  job is picked up after restart and the directory is removed.

- **GIVEN** a reclamation attempt that fails transiently, **WHEN** the job
  retries, **THEN** it follows the inherited backoff schedule and reclaims on a
  later attempt without a second delete request.

- **GIVEN** a reclamation that fails on all eight attempts, **THEN** the job
  state is `failed` and the unreclaimed worktree path appears in the job's
  recorded error and in a warning log line.

**Idempotency and downstream paths**

- **GIVEN** a session whose worktree directory was already deleted outside
  kandev, **WHEN** the session is deleted, **THEN** the job reaches `succeeded`.

- **GIVEN** a session that was deleted and its worktree reclaimed, **WHEN** the
  parent task is later deleted, **THEN** task deletion succeeds and reports no
  error for the already-reclaimed worktree.

- **GIVEN** a task whose environment still names a reclaimed `worktree_id`,
  **WHEN** a new session is launched on that task, **THEN** the launch succeeds
  with a freshly created worktree.

**Invariant**

- **GIVEN** any sequence of session deletes on a task, **WHEN** every resulting
  cleanup job has reached `succeeded`, **THEN** for every repository involved,
  every path in `git worktree list` for that repository is referenced by at
  least one live `task_session_worktrees` row.

- **GIVEN** a repo-less task with a scratch workspace under the tasks base,
  **WHEN** one of its sessions is deleted, **THEN** the scratch workspace
  directory still exists and the invariant check does not flag it.

**Confirmation surface**

- **GIVEN** a session whose worktree has 3 uncommitted files and 2 unpushed
  commits, **WHEN** the user opens the delete confirmation, **THEN** the dialog
  states both counts before the confirm control is used.

- **GIVEN** a session whose worktree is clean and level with its remote, **WHEN**
  the user opens the delete confirmation, **THEN** no uncommitted or unpushed
  warning is shown.

- **GIVEN** the delete confirmation for any session, **WHEN** it is displayed,
  **THEN** its copy does not claim that only conversation history is removed.

**Primary promotion (regression)**

- **GIVEN** a task with a primary session and one other session, **WHEN** the
  primary is deleted, **THEN** the other session becomes primary and its
  worktree is untouched.

## Out of scope

- **Deleting branches.** Session delete never runs `git branch -D`. Reclaiming
  branches left behind by discarded sessions is separate work.
- **A force flag on `session.delete`.** The confirmation warning is the guard;
  the WS payload gains no new fields.
- **Wiring the office orphan-directory garbage collector.** `NewGarbageCollector`
  in `internal/office/infra/gc.go` currently has no production caller, so no
  sweeper reclaims stray directories on any install. That is a real, separate
  gap. This spec deliberately does not depend on it: the guarantees above must
  hold with the GC absent. Wiring it needs its own decision about deletion
  authority and grace periods.
- **Ephemeral quick-chat scratch directories.** `~/.kandev/quick-chat/<session_id>`
  is session-scoped and is reclaimed only by task deletion. It is not reachable
  through the normal session-delete UI surfaces and is left alone here.
- **Task-level cleanup.** Archive, task delete, cascade, workspace delete, and
  quick-chat expiration keep the behavior specified in
  [tasks/runtime-cleanup](../tasks/runtime-cleanup.md).
- **Recovering worktrees already leaked by past deletes.** Directories orphaned
  before this change have no database record to drive a migration. Reclaiming
  them requires the sweeper listed above.
- **Renaming or deprecating the `session.delete` action.** The action keeps its
  name and its public WS contract.

## Implementation notes

- **Multi-repo aggregation for the pre-delete warning counts.** The spec says
  the confirmation surface reads `ahead`/`files` through the existing
  `session.git.snapshots` action, but that action returns a per-session
  history ordered newest-first — not one row per repo. For a multi-repo
  session, distinct repos' rows are interleaved with older rows for the same
  repo/branch rather than being distinguishable by a repo id on the row.
  Absent an Input-Inventory answer for how to disambiguate them, the frontend
  hook (`apps/web/hooks/use-session-delete-warning.ts`) fetches a bounded
  window (20 rows) and dedupes to the newest row per distinct `branch` value
  before summing `ahead` and uncommitted-file counts, on the assumption that
  each repo in a multi-repo session checks out a distinctly-named branch. Two
  repos coincidentally sharing a branch name would undercount. This is a
  documented assumption, not a verified contract; revisit if `session.git.snapshots`
  gains an explicit repository/worktree identifier on each row.

- **`git worktree prune` failure after a successful forced fallback removal
  (review-round 1, P2).** `removeWorktreeDir`'s fallback path now logs a
  prune failure at Warn instead of Debug so a stale `.git/worktrees/<id>`
  registration stays discoverable. It still does not fail the reclamation
  attempt: the Failure modes table's "git worktree remove fails" row is
  explicit that "[o]nly if both [forced directory removal and prune] fail
  does the attempt fail," and the directory removal already succeeded by the
  time prune runs. Returning an error here to satisfy "propagate prune
  failures too" (review finding wording) would make a prune-only failure
  fail the attempt, contradicting that frozen row — so the fix is
  observability (Warn-level, with the worktree and repository paths
  attached), not attempt failure.

## REVIEW-ROUND: 1

Review wave (2026-08-07) ran code-reviewer, security-reviewer, and test-supervisor
(ksdd subagents), the repo's own `code-review` skill, and a cross-vendor Codex
review over the diff (base `main`). Verdict: **production-bug residual, back to
Build.** Three independently-confirmed, severe issues, all in or around the
target-path-lock fix added this round to `ReclaimSessionWorktree` — each was
independently verified against the actual code (not taken on a reviewer's word)
before being recorded here.

1. **CRITICAL — lock-order inversion / deadlock** (code-reviewer subagent).
   `ReclaimSessionWorktree` (`apps/backend/internal/worktree/manager_cleanup.go`)
   acquires `acquireWorktreeTargetPath(wt.Path)` first, then `repoLock` second.
   Every worktree-creation path (`Create()` → `gitAddWorktree`/
   `gitAddWorktreeExisting`/`gitAddWorktreeForRecreate` in
   `manager_lifecycle.go`) acquires `repoLock` first (held via `defer` across
   the whole call) and `acquireWorktreeTargetPath` second, nested inside. This
   is a classic AB-BA inversion: when a session-delete reclaim and a same-task
   worktree creation (e.g. `EnsureSession`'s auto-continuation right after the
   delete — the exact scenario the target-path lock was added for) target the
   same path concurrently, each can end up holding the lock the other wants,
   with no deadline on either path to break the stall. This hangs the durable
   cleanup worker and the session-launch request until a backend restart.
   Fix: acquire `repoLock` before `acquireWorktreeTargetPath` in
   `ReclaimSessionWorktree`, matching the order used everywhere else in the
   package. The existing `TestReclaimSessionWorktree_
   SerializesWithConcurrentTargetPathCreate` only simulates a bare
   target-path-lock holder and does not catch this — add a test that holds
   `repoLock` on the creation side while reclaim runs concurrently.

2. **CRITICAL — `removeWorktreeDir` operates by path, not worktree ID; can
   destroy a replacement worktree** (Codex, cross-vendor review; corroborated
   by tracing `Create` → `tryReuseExisting` → `recreate`). Worktree directory
   paths are derived only from task dir + repo name
   (`prepareTaskWorktreePath`), independent of `worktree_id`. The target-path
   lock added this round prevents the two operations from running literally
   concurrently, but not the *sequential* failure: if a replacement worktree
   (different `worktree_id`) legitimately wins the lock race and finishes
   creating at the same path before reclaim acquires the lock,
   `ReclaimSessionWorktree`'s reference-count check
   (`CountActiveWorktreeReferences(ctx, wt.ID, nil)`) still correctly returns 0
   for the *old* worktree's id — but `removeWorktreeDir(wt.Path, ...)` then
   blindly force-removes whatever is *currently* registered at that path,
   destroying the replacement's live data instead of leaving it alone. A
   second, independent unprotected window: `Manager.recreate()`
   (`manager_lifecycle.go:1074-1080`) calls `os.RemoveAll(existing.Path)`
   completely outside any lock, before later acquiring the target-path lock
   via `gitAddWorktreeForRecreate`. Fix: `ReclaimSessionWorktree` must verify,
   while still holding the lock(s), that the current resident of `wt.Path` is
   still `wt.ID` before removing (e.g. query the store for any other active
   worktree row at that exact path with a different ID and skip removal if
   found); move `recreate()`'s `os.RemoveAll` inside/after its lock
   acquisition.

3. **HIGH — ambiguous `DeleteTaskSession` error unconditionally cancels the
   cleanup job, silently re-leaking the worktree** (security-reviewer
   subagent; independently verified). `task_operations.go:2772-2774` calls
   `cancelSessionDeleteResourceCleanup` unconditionally whenever
   `DeleteTaskSession` returns an error — even when the mutation actually
   committed (a realistic ambiguous-outcome class on this repo's supported
   Postgres deployment target). If the delete did commit, the cancelled job
   was the *only* remaining pointer to the worktree needing reclaim; cancelled
   jobs are filtered out of `ListPreparedTaskResourceCleanupJobs`
   (`state='prepared'` only), so reconciliation never revisits it — the
   worktree leaks forever, silently, reproducing this card's original bug via
   a different path. The fix pattern already exists in the same package for
   the task-level trigger — `resolveTaskResourceCleanupAfterMutationError`
   (`resource_cleanup_jobs.go:489`) — but was never wired into the
   session-delete path; `sessionDeleteCleanupMutationCommitted` is used only
   in the reconciliation path, not `DeleteSession`'s synchronous error branch.
   Fix: mirror `resolveTaskResourceCleanupAfterMutationError` for
   `session_delete` — check `sessionDeleteCleanupMutationCommitted` first;
   activate if committed, cancel if not, mark with the existing
   ambiguous-outcome sentinel if the check itself errors so reconciliation
   retries it.

4. **P1, spec-literal — delete confirm control not gated on the warning fetch
   resolving** (Codex). The previous QA round considered this and treated it
   as an accepted, documented design decision (there is an explicit code
   comment and test asserting "nothing gates the delete button on the fetch
   resolving"). Flagging again because an independent cross-vendor review
   raised the same concern against the scenario's literal text (this doc,
   Confirmation surface: "the dialog states both counts *before the confirm
   control is used*") — Build's call whether to gate the control on the fetch
   resolving or explicitly re-affirm the residual-risk decision, rather than
   pass through silently a second time.

5. **P2 — `git worktree prune` failure silently swallowed after
   `forceRemoveDir` fallback** (Codex). `removeWorktreeDir`'s fallback path
   (git remove fails → `forceRemoveDir` → `git worktree prune`) only logs a
   prune failure at Debug and returns nil regardless; the job is marked
   succeeded while the source repo can retain a stale `.git/worktrees/<id>`
   registration. Lower severity (no disk leak; git generally self-heals stale
   worktree admin entries) but worth tightening to match the function's own
   doc comment ("directory-removal failure is returned rather than only
   logged") — propagate prune failures too.

Everything else reviewed clean: auth-boundary ordering, durable-worker
snapshot isolation (no scope widening), the WS gateway backstop, the
regression-guard reference-exclusion invariant (re-verified against a real
SQLite store, including the sequential-sharing scenario), no `git branch -D`
in the reclaim path, no command-injection surface, no secrets/PII in the new
snapshot data, and job-claim CAS atomicity.

### Test-supervisor findings (arrived after the initial hand-off — read these too)

The test-supervisor subagent ran the suite green, then applied actual
mutations to a throwaway `git archive` copy of HEAD to check what really goes
red (method, not speculation). Verdict: FAIL, three test-honesty must-fix
items — none are new production bugs, but they mean the suite currently
provides less protection against the bugs above than its docstrings claim.
Fix these alongside 1-3 above so the regression coverage for THIS round's
fixes is real, not vacuous.

1. **The orchestrator-level "regression guard" test is ordering-blind and its
   docstring overclaims.** `TestDeleteSession_ActivatesResourceCleanupAfterCommit`
   and `TestSessionDeleteCleanupJob_FullLifecycle`
   (`apps/backend/internal/orchestrator/session_delete_resource_cleanup_test.go`,
   `apps/backend/internal/task/service/resource_cleanup_session_delete_test.go`)
   use a fake reclaimer/collaborator whose `StartPreparedTaskResourceCleanup`
   just appends to a slice — it never queries whether the session row was
   already gone. Mutation-proven: reordering `task_operations.go:2773-2780` so
   activation runs *before* `repo.DeleteTaskSession` leaves both suites green.
   (The real reference-exclusion invariant IS correctly covered elsewhere,
   against real SQLite — `TestReclaimSessionWorktree_
   RemovesExclusivelyHeldWorktree`/`...ReclaimsAfterBothSharingSessionsDeleted`
   in `manager_reclaim_session_worktree_test.go` — but this orchestrator-level
   test's own docstring claims to be *the* regression guard from the spec,
   and it isn't testing that.) Fix: have the fake's
   `StartPreparedTaskResourceCleanup` query `repo.GetTaskSession(sessionID)`
   and record whether the row was still present when called; assert it was
   already gone. Correct the docstring.
2. **Vacuous copy-regression assertion, passes on revert.** `apps/web/components/task/session-tab-menu.test.tsx`
   and `mobile-sessions-section.test.tsx`'s "does not claim only conversation
   history is removed" tests assert `not.toMatch(/only.*conversation
   history|conversation history.*only/)` — but the *old* copy ("This will
   permanently delete the conversation history with this session.") never
   contained "only" either, so the regex never matched either string.
   Reverting the `task.json` copy change leaves both green. The e2e spec does
   this correctly (positive `toContainText` on the new sentence + negative on
   the exact old sentence) — port that pair down to both unit tests.
3. **Restart-durability test asserts job state, not the downstream effect.**
   `TestSessionDeleteCleanupJob_RestartAfterCommitResumes`
   (`resource_cleanup_session_delete_test.go`) asserts only
   `job.State == succeeded` after restart-resume. Mutation-proven: making
   `executeSessionDeleteResourceCleanup` never actually reclaim anything
   (`return nil` unconditionally) leaves this test green — the fake never gets
   asked which IDs it reclaimed. Its sibling
   `RestartBeforeCommitIsCancelled` correctly asserts `reclaimedCalls == 0`;
   the positive counterpart needs the equivalent `reclaimedIDs == [...]`
   assertion. This is the exact "assert real effect, not stored state" shape
   PR #2241 finding #3 was about.

Should-fix (test-rigor, not blocking on their own, but do while in this
file): the mixed shared+exclusive multi-worktree scenario
(`TestExecuteSessionDeleteResourceCleanup_ReclaimsEveryWorktreeIndependently`)
only asserts a call count against a fake, never runs the mixed case against
real reference counting; `simulateSessionCascadeDelete` deletes the child
`task_session_worktrees` row directly rather than the parent `task_sessions`
row, so the cascade this whole feature's exclusion logic depends on is
exercised nowhere in the diff (verified to actually fire when probed
separately — cheap to switch the test helper to delete the parent row
instead); no test asserts the git-worktree *registration* is gone after
`forceRemoveDir`+`prune` (only `os.Stat` on the directory) — the package
already has `worktreeRegistrationExists` to assert against; no test drives a
transient-failure-then-succeed retry through the `session_delete` job branch;
the Primary-promotion regression scenario and both Invariant scenarios have
no test anywhere in the repo (pre-existing gaps this diff's insertion of 3 new
steps ahead of `promoteNextPrimaryAfterRemoval` makes worth closing now); no
test proves `ReclaimSessionWorktree` *returns* (vs. only logs) a removal
failure, which is what the retry budget and "reclamation failure SHALL be
observable" actually depend on.

## REVIEW-ROUND: 2

Review wave (2026-08-07/08) ran code-reviewer, security-reviewer, and
test-supervisor (ksdd subagents), a fresh inline `code-review` skill pass, and
a cross-vendor Codex review over `main...HEAD` (all 8 commits, base `main`).
**Verdict: production-bug residual, round >= 2 — staying parked here rather
than cycling back to Build a second time; documenting for human triage rather
than auto-merging.** All three round-1 CRITICAL/HIGH fixes (lock-order
inversion, path-based destructive removal, ambiguous-error blind-cancel) and
the P1/P2 fixes were independently re-verified as genuinely correct this round
— by code-reviewer, security-reviewer (APPROVE), and by directly reading the
code myself — and are **not** the residual below. This is a different,
previously-unflagged defect, first caught this round by Codex and independently
confirmed by tracing the exact code path myself before recording it here.

### CRITICAL — pre-delete warning can silently undercount uncommitted files to
zero, so the worktree removal that follows destroys unwarned-about work
(Codex; independently verified against the code, not taken on the reviewer's
word)

The spec requires (line 52-54): "the user SHALL be shown the count of
uncommitted files and the count of unpushed commits in the session's
worktrees whenever either is non-zero." The Confirmation-surface scenario
(line 320-322) requires the dialog to state the count of uncommitted files
before the confirm control is used. Both can be violated by ordinary use:

1. An agent turn completes. `saveGitStatusSnapshot` (unchanged, pre-existing)
   writes an `agent_completed` DB snapshot with a full `files` map.
2. The user manually edits a file — through the embedded terminal, an IDE, or
   any means outside an agent turn — after that turn completed. The
   live-git-status watcher fires and `persistGitStatusSnapshot`
   (`apps/backend/internal/orchestrator/event_handlers_git.go:198`) upserts a
   `live_monitor` DB row for the throttled sidebar badge. That row is written
   with `Files: nil, // intentional: badge only needs totals` — **by design**,
   for an unrelated feature (the sidebar diff badge only needs aggregate
   counts, not a file list).
3. The user opens the delete confirmation. `useSessionDeleteWarning`
   (`apps/web/hooks/use-session-delete-warning.ts:68-72`) fetches
   `session.git.snapshots`, which is served by
   `GetGitSnapshotsBySession` (`apps/backend/internal/task/repository/sqlite/git_snapshots.go:277-289`).
   Its `ORDER BY` (line 284: `CASE WHEN triggered_by = 'agent_completed' THEN
   0 ELSE 1 END, created_at DESC`) always sorts **every** `agent_completed`
   row ahead of **every** `live_monitor` row for that session, regardless of
   which is actually newer.
4. `summarizeSnapshots` (`use-session-delete-warning.ts:95-107`) dedupes to
   the *first-seen* row per branch and sums `Object.keys(files ?? {}).length`
   and `ahead`. Because step 3's ordering puts the stale `agent_completed`
   row first, the newer `live_monitor` row for that branch — the one that
   would actually reflect the terminal edit — is skipped entirely: not just
   its (empty) `files`, but also its `ahead` count. The dialog can show **0
   uncommitted files and 0 unpushed commits** while real, uncommitted work
   sits in the worktree.
5. Nothing gates the delete on this being accurate. The user, seeing no
   warning, confirms. `ReclaimSessionWorktree` →
   `removeWorktreeDir` (`apps/backend/internal/worktree/manager_cleanup.go`)
   runs `git worktree remove --force` (or the `os.RemoveAll` fallback),
   permanently destroying the directory — including the edit nobody was ever
   warned about.

This is not hypothetical or rare: "finish an agent turn, tweak one more thing
by hand in the terminal, then clean up the session" is an ordinary workflow
for this product, not an edge case.

**A fix already has a natural home.** The live WS push event this same watcher
publishes (`GitEventPayload.Status.Files`,
`apps/backend/internal/agent/runtime/lifecycle/event_types.go:291`) **does**
carry the real file list in real time — only the *DB-persisted* `live_monitor`
row drops it, deliberately, for the sidebar badge's sake. The frontend's own
`gitStatus.byEnvironmentId`/`byEnvironmentRepo` store slice
(`apps/web/lib/state/slices/session-runtime/types.ts:65-101`) is kept live
and accurate from that same event stream and already backs the Git changes
panel. `useSessionDeleteWarning` reinvented a new, weaker data path (historical
DB snapshot rows) instead of reading the store slice that already has the
right answer. Two independent fix directions, either sufficient on its own:
(a) have the warning hook prefer the live `gitStatus` store entry for the
session's repo(s) when present, falling back to snapshot history only when
no live entry exists yet; or (b) stop discarding `Files` when persisting a
`live_monitor` row (or carry the aggregate `modified`/`added`/`deleted`/
`untracked`/`renamed` counts already sitting unused in `Metadata` as a
fallback count when a row's `files` is empty), and change the ordering to
true `created_at DESC` per branch rather than an unconditional
`agent_completed`-always-wins rule, since that ordering choice is what lets a
staler-but-higher-priority row shadow a fresher one in the first place.

No test anywhere catches this — it's a data-freshness/data-source defect, not
a logic-inversion a unit test targeting the wrong assumption would exercise
by accident. Whoever fixes this should add a regression test that seeds an
`agent_completed` snapshot with clean files, then a *later* `live_monitor` row
whose `Metadata` (or, if fix direction (a) is taken, the live `gitStatus`
store) reflects new uncommitted files, and asserts the warning reflects the
newer state.

### Test-supervisor's fresh mutation audit (round 2) — for the same handoff,
not itself the reason this round is parked

test-supervisor mutation-tested both new commits (not just re-checked round
1) and returned FAIL with three Must-fix test-rigor gaps, all confirmed by
an actual mutation that survived (production code intentionally broken, full
package suite stayed green):

1. **`TestExecuteSessionDeleteResourceCleanup_ReclaimsEveryWorktreeIndependently`
   (`apps/backend/internal/task/service/resource_cleanup_session_delete_test.go:181`)
   only asserts a call *count*.** Mutating the production loop
   (`resource_cleanup_session_delete.go:165`) to reclaim the same worktree
   twice instead of two distinct worktrees — i.e. reclaim one of a
   multi-repo session's worktrees and silently leak the other — left every
   package green. Fix: assert `reclaimedIDs` set-equality, and add a mixed
   shared+exclusive case against real reference counting.
2. **No test proves `ReclaimSessionWorktree` *returns* a directory-removal
   failure** (`apps/backend/internal/worktree/manager_cleanup.go:323`)
   despite its own doc comment's claim. Swapping the `return
   fmt.Errorf(...)` for a `Warn` log + `return nil` left the whole
   `internal/worktree` package green. This is exactly what the retry budget
   and the spec's "Reclamation failure SHALL be observable" depend on.
3. **No test proves a failed `session_delete` reclamation actually retries**
   (`apps/backend/internal/task/service/resource_cleanup_jobs.go:350`).
   Making a failed reclamation swallow its error and report `succeeded`
   anyway left every package green. This kills the Durability scenarios for
   transient-retry and eight-attempt exhaustion.

Also should-fix, carried forward unclosed from round 1: `Manager.ForgetSession`
has zero direct tests (mutating it to a no-op stays green); the Invariant
scenarios and the stale-`worktree_id`-on-relaunch idempotency scenario still
have no test anywhere; `simulateSessionCascadeDelete` still deletes the child
row directly rather than the parent `task_sessions` row (confirmed low-risk —
the cascade was independently verified to actually fire in both stores — but
still untested by the helper itself).

These three Must-fix items are test-rigor gaps, not proof the current code is
broken in the ways they probe — the mutations had to actively break working
code to go undetected. They do not independently drive this round's verdict;
the CRITICAL finding above does. Both should be addressed in the same Build
pass since they touch overlapping code.

### Everything else re-confirmed clean this round

Auth-first ordering, durable-worker snapshot isolation, the WS gateway
backstop, no `git branch -D` in the reclaim path, no command-injection
surface, no secrets/PII in the new snapshot data, and the correctness of all
three round-1 CRITICAL/HIGH fixes — re-verified independently by
security-reviewer (full APPROVE) and code-reviewer (0 blockers) this round,
each reading the code directly rather than trusting commit messages or prior
review notes.

A minor test-infra bug found during this round's self-review — the new
`routeMainWebSocketWithFailedActionResponse` e2e helper
(`apps/web/e2e/helpers/ws-drop.ts`) rewrote an entire batched WS message down
to a single synthetic error frame, silently dropping any sibling frame that
happened to arrive batched alongside the targeted response — was fixed and
committed in this round (`9dcc00214`) since it's test infrastructure, not
production code, and was re-verified green before and after.

## REVIEW-ROUND: 3

Fresh-context review wave (2026-08-08) ran code-reviewer, security-reviewer,
and test-supervisor (ksdd subagents) plus a cross-vendor Codex review over the
two commits that were supposed to close round 2 (`021ac26da`, `6d14b4d1d`),
after independently re-verifying all round-1 and round-2 fixes are still
correctly in place at HEAD (confirmed by direct code reading, not by trusting
this file's own claims). **Verdict: production-bug residual, round 3 — back
to Build.** Two findings, both independently reproduced against the actual
code before being recorded here.

### MUST-FIX 1 — archive-type snapshot rows pollute the pre-delete warning

(code-reviewer, independently reproduced by tracing the full call chain)

`GetGitSnapshotsBySession`/`GetGitSnapshots` apply no `snapshot_type` filter —
confirmed by reading the query
(`apps/backend/internal/task/repository/sqlite/git_snapshots.go:281`) and
every layer between it and the `session.git.snapshots` WS handler
(`apps/backend/internal/task/service/service_turns.go:457`,
`apps/backend/internal/task/handlers/task_git_handlers.go:19-40`). A session
that was archived via `ArchiveTask` before later being deleted has an
`archive`-type row from `captureArchiveDiff`
(`apps/backend/internal/orchestrator/task_operations.go:3204-3216`), which
never sets `Branch` or `Ahead` — both zero-value, and `Branch string
json:"branch"` has no `omitempty`
(`apps/backend/internal/task/models/git.go:28`), so the row always serializes
as `{"branch": "", "ahead": 0, "files": {...real diff...}}`.
`summarizeSnapshots`'s branch-keyed dedup
(`apps/web/hooks/use-session-delete-warning.ts:128-140`) treats `branch: ""`
as its own bucket, so the archive row's file count is **added on top of**
whatever the real branch's row shows. `DeleteSession`
(`apps/backend/internal/orchestrator/task_operations.go:2726`) does not gate
on the parent task's `ArchivedAt`, so this path is reachable through the
ordinary session-delete flow, not merely a code smell.

Failure direction is mostly safe (inflates the warning rather than
suppressing it), but it collides destructively — reintroducing exactly the
class of bug round 2's CRITICAL was about — if a session's *real* live
`status.Branch` is ever also empty (e.g. detached HEAD), since both rows
would then key on the same `""` bucket and either could shadow the other.
Believed rare for kandev-managed worktrees (they always check out a named
branch) but not proven impossible, and untested either way.

Fix: filter to `snapshot_type = 'status_update'`, either in
`GetGitSnapshotsBySession`'s query or in `summarizeSnapshots` before the
branch-keyed dedup. Add a regression test seeding an `archive`-type row
(`branch: ""`, non-empty `files`) alongside a real branch's `status_update`
row, asserting the archive row's files are excluded from the reported count.

### MUST-FIX 2 — ordering fix has no deterministic tiebreak, and its own regression test doesn't prove recency ordering

(Converged independently across code-reviewer [COR-002], Codex [marked P1],
and this review's own reasoning about SQLite tie semantics; test-supervisor's
mutation testing separately proved the regression test itself is
insufficient — four legs on one underlying defect.)

`GetGitSnapshotsBySession`'s `ORDER BY created_at DESC`
(`git_snapshots.go:287`, introduced by `021ac26da`) carries no secondary sort
key. SQLite gives no ordering guarantee among rows with an identical
`created_at`, so a tie could resurrect the exact round-2 undercount
nondeterministically. Practical reachability is low — `time.Now().UTC()`
carries sub-second precision and `saveGitStatusSnapshot` proactively deletes
any existing `live_monitor` row for the session via `DeleteLiveMonitorSnapshots`
right after writing an `agent_completed` row
(`apps/backend/internal/orchestrator/task_operations.go:3173-3181`), closing
off one collision surface — but it is a real gap in a query this feature's
whole safety property depends on, and it is untested.

Separately, and more concretely: test-supervisor mutation-tested
`TestGetGitSnapshotsBySession_OrdersByRecencyRegardlessOfTriggeredBy`
(`git_snapshots_test.go:52`) against the actual production query and found it
**survives** two mutations that should kill it — deleting the `ORDER BY`
clause entirely, and replacing it with `ORDER BY triggered_by DESC` (i.e.
"live_monitor always first, recency ignored"). Both survive because the test
only seeds one arrangement (older `agent_completed`, newer `live_monitor`)
and never the inverse. The test currently proves "the old ordering is gone,"
not "the new ordering is recency-based" — it does not pin the actual contract
this round's fix was supposed to establish.

Fix: add `id` (or another monotonic column) as an explicit secondary
`ORDER BY` key for deterministic tie-break. Strengthen the existing test to
also seed the inverse arrangement (a genuinely newer `agent_completed` row
against an older `live_monitor` row, asserting `agent_completed` sorts
first) so it actually pins recency semantics in both directions.

### Should-fix, not blocking (named residual, carried forward)

test-supervisor's independently-probed AC coverage gaps — none masking a
production bug, each individually verified by direct code tracing or a
throwaway probe against real SQLite/git before being characterized as
test-rigor only:

1. Mixed shared+exclusive multi-worktree reclaim has no test against real
   reference counting (only a fake's call count). test-supervisor's own probe
   against real SQLite confirmed the composition is correct.
2. The golden-path "both commits reachable from `B`" scenario is unasserted;
   only branch resolvability is checked.
3. Both idempotency/downstream-path scenarios (task delete after worktree
   reclaimed; stale `worktree_id` on relaunch) have no direct test — verified
   correct by code tracing (`pruneQuarantinedWorktree`,
   `tryReuseExisting`/`buildWorktreeNames`'s random suffix ruling out branch
   collision).
4. Both Invariant scenarios still have no integration test anywhere.
5. The retry-exhaustion test doesn't assert the warning-level log line half
   of that scenario's AC (only job state + `LastError`).

Codex's advisory finding that the eight-attempt threshold isn't pinned by the
new exhaustion test was checked and found **not** to be a gap: a pre-existing
test (`TestRetryTaskResourceCleanupJobUsesBoundedBackoffAndTerminalState`)
already pins `taskResourceCleanupMaxAttempts` independently.

Everything else re-confirmed clean this round by direct code reading, not
inherited from this file's prior claims: auth-first ordering in `DeleteSession`,
the round-1 lock-order-inversion fix, the round-1 path-vs-ID verification fix,
the round-1 ambiguous-error resolution fix, job-claim CAS atomicity,
snapshot-scoped worker execution (no scope widening), no SQL-injection or XSS
surface in the round-3 diff, and the round-1 P1 confirm-control gating
(`disabled={!isLoaded}`).

## REVIEW-ROUND: 4

Fresh-context review wave (2026-08-08) ran code-reviewer, security-reviewer,
and test-supervisor (ksdd subagents) plus a cross-vendor Codex CLI review over
the commit that was supposed to close round 3 (`31a9706c0`), after
independently re-verifying rounds 1-3's fixes are still correctly in place at
HEAD (round-1 lock ordering, path-vs-ID verification, ambiguous-error
resolution; round-3's archive-row exclusion). **Verdict: production-code
residual, round 4 — back to Build.** One finding, converged across all four
legs and confirmed by test-supervisor's actually-executed mutation testing
(not speculation).

### MUST-FIX — the round-3 tiebreak fix only half-satisfies its own ask: `id` is a random UUID, not a monotonic column

(code-reviewer, security-reviewer, test-supervisor, and Codex CLI all
independently identified the same underlying fact; test-supervisor
additionally proved it by executing real mutants against the production
query.)

Round 3's MUST-FIX 2 asked for "`id` (or another monotonic column) as an
explicit secondary `ORDER BY` key for deterministic tie-break." `31a9706c0`
added `ORDER BY created_at DESC, id DESC`
(`apps/backend/internal/task/repository/sqlite/git_snapshots.go:300`), but
`id` is `uuid.New().String()` — a random UUIDv4 — at every insert site
(`git_snapshots.go:31` in `UpsertLatestLiveGitSnapshot`, `:103` in
`CreateGitSnapshot`), over a plain `id TEXT PRIMARY KEY` column
(`base_schema.go:786`) with no monotonic sequence. On a genuine `created_at`
tie between an `agent_completed` row and a `live_monitor` row for the same
branch, `id DESC` makes the result **deterministic but arbitrary** — a UUID
coin flip decides which row wins, not recency. That reopens (stably, rather
than flakily) the exact failure class round 2's CRITICAL and round 3's
MUST-FIX 2 were about: a stale row could still shadow a genuinely newer one
in the pre-delete warning.

test-supervisor mutation-tested the actual production query and confirmed
this line is currently unguarded: dropping `, id DESC` entirely, or flipping
it to `id ASC`, both leave the full `internal/task/repository` suite green.
No existing test seeds two rows with an identical `created_at`.

**Calibrated severity — legs disagree, and that disagreement is itself
informative.** code-reviewer rated this HIGH and recommended
`REQUEST_CHANGES`. security-reviewer (LOW, APPROVE), Codex CLI
("low-risk/non-blocking hardening issue"), and test-supervisor ("present-day
production impact low, not high") independently converged on low practical
severity. Two independent checks narrow the reachability further: this
review's own inspection confirmed `CreatedAt` is never truncated before
persisting (`time.Now().UTC()` used directly at every call site, no
`.Truncate`), and test-supervisor empirically measured the stored column at
microsecond precision with ~58µs between consecutive inserts under real
load — consistent with mattn/go-sqlite3's nanosecond-precision timestamp
formatting. An exact tie is not attacker-forceable and is very unlikely in
practice. This is why the finding routes back to Build rather than being
escalated as a human-triage question: the code itself settles the severity
disagreement (confirmed negligible reachability), only the test-coverage gap
on a safety-critical query is unambiguous and mutation-proven.

Fix (Build's choice of either direction, both close the gap):
(a) replace `id` with a genuinely monotonic secondary key (an
auto-increment/sequence-backed column, portable across this repository's
SQLite-and-Postgres dialect constraint per `apps/backend/CLAUDE.md`'s schema
rules) and add a test seeding two `status_update` rows with an **identical**
`created_at`, asserting the later-inserted row sorts first; or
(b) keep `id` as a stability-only tiebreak, correct the doc comment at
`git_snapshots.go:288-291` (and this section) to state plainly that it
guarantees repeatable ordering, not recency-correctness, on an exact tie, and
add a test asserting the tie resolves to a stable (repeatable-across-calls)
order.

Either direction should also address test-supervisor's should-fix note:
`getGitSnapshotByOrder` (`git_snapshots.go:126-134`, backing
`GetFirstGitSnapshot`) carries the same untiebroken `ORDER BY created_at ASC
LIMIT 1` pattern. It is off the session-delete path and empirically
non-flaky at the same microsecond precision, so it is not itself a blocking
finding — but note it in the same pass if convenient.

**Resolved (Build round 4, commit pending): direction (b).** This repository
already makes the identical tradeoff, deliberately and documented, for
`UpdateTaskSessionLastReadMessageID`'s `id`-based tiebreak (`session.go:1420
-1428`): "id as a deterministic tiebreaker... Portable across SQLite and
Postgres — no dialect branching needed, unlike the rowid-based approach this
replaced (SQLite's rowid pseudo-column doesn't exist on Postgres)." Direction
(a) would introduce a new, one-off monotonic-column mechanism into a codebase
that has already chosen, and justified, the opposite tradeoff for the same
shape of problem (tie-breaking timestamped inserts) — inconsistent for no
correctness gain, since the code's own severity analysis above already
establishes the reachability gap is negligible. Took direction (b):
- Corrected `git_snapshots.go:288-291`'s doc comment to state plainly that
  the `id` tiebreak guarantees a stable, repeatable order on a tie, not that
  the newer row wins, and cross-referenced the `session.go` precedent.
- Added `TestGetGitSnapshotsBySession_TiebreaksIdenticalTimestampsStably`
  (`git_snapshots_test.go`), seeding two `status_update` rows with an
  identical `created_at` and explicit, orderable IDs. Mutation-verified by
  hand against the production query before landing: reverting to plain
  `ORDER BY created_at DESC`, and flipping to `id ASC`, both fail the new
  test (confirmed by executing each mutant), closing the exact gap
  test-supervisor found.
- `getGitSnapshotByOrder`/`GetFirstGitSnapshot` left as-is per the note above
  (off the session-delete path, empirically non-flaky, not itself a blocking
  finding).

### Everything else re-confirmed clean this round

All four legs independently re-verified, by direct code reading (not
inherited from this file's prior claims): round-3 MUST-FIX 1 (archive-row
exclusion) is correctly and completely closed with a genuine, mutation-proven
regression test; `GetGitSnapshotsBySession`'s only production consumer is
still the pre-delete warning path (traced fresh — `Service.GetGitSnapshots` →
`wsGetGitSnapshots` → `apps/web/hooks/use-session-delete-warning.ts`, used by
both `session-tab-menu.tsx` and `mobile-sessions-section.tsx`); both
production writers of `status_update` rows are unconditional, so the new
`snapshot_type` filter drops nothing currently reachable; round-1's
lock-ordering, path-vs-ID verification, and ambiguous-error-resolution fixes
remain intact; auth-first ordering on both `session.delete` and
`session.git.snapshots`; no command injection, SQL injection, path traversal,
or secrets/PII exposure in the round-4 diff; the durable cleanup worker's
snapshot isolation is unchanged and correct.

The round-1/2/3 "should-fix, not blocking" residual list is unchanged and
still accurate — this round's diff touched only the snapshot query and its
tests, so nothing on that list was disturbed.
