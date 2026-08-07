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
