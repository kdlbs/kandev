---
id: "02-idempotent-polling-events"
title: "Reconcile and publish canonical PR state"
status: done
wave: 1
depends_on: ["01-canonical-watch-migration"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 02: Reconcile and publish canonical PR state

## Intent

Stop session-driven branch oscillation and duplicate GitHub/event work after canonical identity is available.

## Acceptance

- One branch result per task/repository drives one atomic searching-watch transition; unchanged reconciliation has no inserts or updates.
- Multiple sessions on one task/branch result in one lookup per cycle; multi-repository and multi-branch behavior remains correct.
- `GitHubPRFeedback` and `GitHubTaskPRUpdated` publish only for durable relevant state changes, coalesced by task/repository/PR/head SHA/status.

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_test.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/github/service_pr_watch_multi_branch_test.go`
- `apps/backend/internal/github/service_pr_sync_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. It consumes the task-owned store contract and event semantics used by projection.

## Verification

```bash
cd apps/backend && go test ./internal/github -run 'Test.*(Poller|Reconcile|Branch|PRWatch|Sync|Feedback|MultiRepo).*' -count=1 -v
cd apps/backend && go test ./internal/github -count=1
git diff --check
```

## Output contract

Record red/green call and event counts for repeated resume, branch switch, unchanged cycles, and multi-repository cases. Update task and plan status.

## Results

Commit: `e7cf5e8184c1df4e03efe56298afa2bf176ba269` (branch
`feature/fix-pr-watch-amplifi-u6z`, pushed to origin).

**Fixed (code changed):**

- `TaskBranchProvider.ResolveBranchForSession(taskID, sessionID)` replaced
  with `ResolveBranchForRepository(taskID, repositoryID)`. The old resolver
  ignored `watch.RepositoryID` and always resolved the task's PRIMARY
  repository's `checkout_branch`; on multi-repo tasks this could silently
  overwrite a still-searching secondary-repo watch's branch with the primary
  repo's branch. New behavior resolves the specific repository's
  `checkout_branch` first, falling back to any active session's worktree
  branch for that repository only as a last resort. Locked in by
  `TestRefreshStaleBranches_ResolvesByRepositoryNotSession` (two watches, same
  session, different repos, provider returns different per-repo branches —
  both update to their own branch) and `TestResolveBranchForRepository`
  (checkout_branch precedence, per-repo scoping, worktree fallback, "" when
  unresolvable).
- `buildTaskBranchList`/watch dedup rekeyed from
  `(session_id, repository_id, branch)` to `(task_id, repository_id, branch)`
  (`watchedTaskRepoBranchKey`, `buildWatchedTaskRepoBranchSet`), plus an
  in-pass `emitted` set inside `buildTaskBranchList`. N active sessions
  resolving to the same (task, repository, branch) target now collapse to
  one `TaskBranchInfo` emission and therefore one
  `EnsurePRWatchForWorkspace` call per cycle. Locked in by
  `TestListTasksNeedingPRWatch/dedupes_multiple_sessions_on_the_same_task/repo/branch`
  (two sessions, one shared task environment/worktree, one repo/branch ->
  exactly 1 emitted entry).

**Verified already-satisfied (no code change needed):**

- "One branch result per task/repository drives one atomic searching-watch
  transition; unchanged reconciliation has no inserts or updates" — already
  covered by Task 01's `UpdatePRWatchBranchIfSearching`/`EnsurePRWatchForWorkspace`
  idempotency (existing `TestUpdatePRWatchBranchIfSearching_*` tests pass
  unchanged).
- "`GitHubPRFeedback` and `GitHubTaskPRUpdated` publish only for durable
  relevant state changes, coalesced by task/repository/PR/head SHA/status" —
  `checkSinglePRWatch`/`checkPRWatchWithClient` already gate `GitHubPRFeedback`
  on `hasNew` (checks_state/review_state diff or PR `updated_at` advancing
  past the watch's watermark); the batched path
  (`tryBatchedPRWatchCheck`) already gates on `PRWatchSyncResult.Changed` or a
  terminal merged/closed transition. `GitHubTaskPRUpdated` is already gated by
  `taskPRChangedFields` (Task 01). Existing tests
  (`TestCheckSinglePRWatch_OpenPR_NoChange_NoSync`,
  `TestSyncTaskPR_NoEventWhenUnchanged`,
  `TestSyncTaskPR_SecondIdenticalSyncNoEvent`,
  `TestSyncTaskPR_EquivalentRESTAndGraphQLStatusNoSecondEvent`) already cover
  this and pass unmodified. No literal `head_sha` column exists on `PRWatch`;
  head-SHA-level changes are already captured transitively through
  `checks_state` (a new commit triggers a fresh CI run) and PR `updated_at`.

**Deferred, out of scope for this narrow slice (not silently dropped):**

- Acceptance criterion #4 from the overall plan ("switching branch A -> B
  performs one transactionally visible transition ... merges into an
  existing canonical row for B") is only partially covered — Task 01's
  migration-time merge logic handles this at migration time, but live
  reconciliation's branch-switch-and-merge path was not re-verified/hardened
  in this task. Left for a later wave if still needed.

**Test evidence:**

```
cd apps/backend && go test ./internal/github -run 'Test.*(Poller|Reconcile|Branch|PRWatch|Sync|Feedback|MultiRepo).*' -count=1 -v
... all PASS (no failures)

cd apps/backend && go test ./internal/github -count=1
ok  	github.com/kandev/kandev/internal/github	11.277s

cd apps/backend && go test ./internal/orchestrator/... -count=1
ok  	github.com/kandev/kandev/internal/orchestrator	56.896s
ok  	github.com/kandev/kandev/internal/orchestrator/executor	1.163s
ok  	github.com/kandev/kandev/internal/orchestrator/handlers	0.075s
ok  	github.com/kandev/kandev/internal/orchestrator/messagequeue	0.337s
ok  	github.com/kandev/kandev/internal/orchestrator/queue	0.003s
ok  	github.com/kandev/kandev/internal/orchestrator/scheduler	0.469s
ok  	github.com/kandev/kandev/internal/orchestrator/watcher	0.017s

git diff --check   # exit 0, no whitespace issues
go build ./...     # exit 0
gofmt -l <changed files>   # no output
golangci-lint run ./internal/github/... ./internal/orchestrator/...   # 0 issues
```
