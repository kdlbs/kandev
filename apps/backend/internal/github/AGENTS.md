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
It reconciles unwatched `github_task_prs` rows and writes lifecycle fields,
the current `head_sha`, and stored `workflow_attention` when already present.
Exclude terminal and detached rows to bound the sweep.
Actions workflow reads belong to watched sync and feedback paths; the orphan
sweep does not collect new workflow evidence. Check and review aggregates belong
to the watch-driven path that fetches their evidence.
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

## Batched phase progress and continuation

`fetchBatchedWatchStatuses` runs a numbered phase and then a searching phase.
Each phase owns its singleflight and progress key (`batched-pr:` /
`batched-branch:`), so a changing search set never discards numbered chunk
progress. The numbered phase is fetched, applied, and published before the
searching read starts; searching latency or its context deadline cannot withhold
a finished known-PR update. `apply*BatchGroups` is the only apply seam; keep
review-thread pagination complete before publication and keep
`applyPRWatchStatusResult` as the writer.

`prWatchSyncOutcome` carries applied results plus the unfinished phase and a
typed `AdmissionDeferredError`. Local deferral must not enter REST fallback or
auth circuits; only unfinished watches may fall back, and the passive per-task
sync path must return the deferral instead of retrying it over REST.
`poller_pr_continuation.go` retains that unfinished work per workspace, keyed by
provider target with every live watch consumer and one deadline per target, and
resumes it from `prMonitorLoop`'s timer at its admission boundary (future
`RetryAt`, then positive `Delay`, then a one-second floor). A retried target
always replaces its own deadline; untouched targets keep theirs. Pruning drops
consumers whose watch lifetime disappeared or whose target changed, and drops a
workspace entry when its credential fingerprint changed, so a new principal or a
retire/resume lifetime is admitted at the next normal attempt. Timer wakes
recheck admission, never run the transport fallback, and do not suppress a
replacement lifetime. Do not add a second scheduler or a per-watch goroutine.
