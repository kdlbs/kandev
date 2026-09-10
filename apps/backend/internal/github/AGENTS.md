# GitHub PR status synchronization

Apply these rules to watch identity, PR lifecycle persistence, and batched lookup changes.
Paths and symbols refer to this package unless a path starts with `apps/`.

## Watch identity

A `github_pr_watches` row is unique per session, repository, and branch.
Once a watch finds a PR, it remains that PR's synchronization handle.
Both the poller and on-demand synchronization iterate these watches.

On branch switches, re-point only a searching watch (`pr_number=0`) for the same repository.
Otherwise, add a watch for the new branch.
Apply this rule in `resetPRWatchForBranchSwitch` and `syncPRWatchBranch`.
Retain one searching watch per session/repository and one watch per discovered PR.

## Lifecycle persistence

Watches cannot cover every task PR because sessions can disappear and users can link PRs by URL.
Keep the orphan sweep in `service_pr_unwatched.go`.
It reconciles unwatched `github_task_prs` rows and writes lifecycle fields only:
`state`, `merged_at`, and `closed_at`.
Exclude terminal and detached rows to bound the sweep.
Check and review aggregates belong to the watch-driven path that fetches their evidence.
The REST PR response alone cannot supply those aggregates.

The backend is the only writer of `github_task_prs`.
The feedback read persists through `SyncTaskPR` in `service_pr_feedback_sync.go`.
Its `github.task_pr.updated` event updates every frontend surface.
Do not patch lifecycle fields only in a frontend store copy.

Keep both derivations on `newPRStatus`.
Persist inside the TTL cache's fetch closure so a cache hit causes no write.
Log persistence failures without failing the feedback read.
Use workspace-scoped `ListTaskPRsByPRNumber` to match the authorized credential scope.

`resolveTerminalMergeState` prevents a merged PR from leaving `merged`.
Do not extend this rule to `closed`: GitHub permits `closed -> open`.

## Batched lookup budget

An alias with zero nodes definitively means no open PR on that branch.
Carry this result through `branchBatchResult.ResolvedEmpty` to `applyBatchedSearchingWatch`.
Do not repeat that lookup with `client.FindPRByBranch`.

After a definitive negative result, only the unqueried fork parent needs another lookup.
Cache fork identity with `forkParentRepositoryForLookup` per credential scope, owner, and repository.
Two or more open PRs are ambiguous, not a definitive negative result.
For ambiguity or an unresolved alias, retain the full fork-network fallback.
Throttle this production batched path with `last_checked_at` and `PRSyncFreshnessWindow`, as `triggerPRDetection` does.

For lookup changes, run `service_pr_watch_batched_budget_test.go` coverage.
Retain call-count assertions: a correct final `PRStatus` cannot detect duplicate API requests.
