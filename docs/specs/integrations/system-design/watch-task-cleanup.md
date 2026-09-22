---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-WATCH-CLEANUP-001
  - REQ-INTEGRATIONS-WATCH-CLEANUP-002
created: 2026-09-22
owners:
  - kandev
---

# Watch task cleanup design

## Ownership and mapping

Integration stores select candidates. Integration services preserve cleanup
policies and call the existing task deletion adapters.

| Requirement | Design section |
| --- | --- |
| REQ-INTEGRATIONS-WATCH-CLEANUP-001 | Candidate selection and compatibility |
| REQ-INTEGRATIONS-WATCH-CLEANUP-002 | GitHub batch termination |

## Candidate selection and compatibility

Cleanup queries use this predicate for each deduplication row `d`:

```sql
d.task_id = '' OR EXISTS (
  SELECT 1 FROM tasks t
  WHERE t.id = d.task_id AND t.archived_at IS NULL
)
```

Keep watch, workspace, and generation predicates outside this parenthesized
expression. A plain inner join loses reservations. A left join with only an
archive-null check incorrectly includes deleted tasks.

Apply this filter to GitHub `ListReviewPRTasksByWatch`, `ListAllReviewPRTasks`,
`ListIssueWatchTasksByWatch`, and `ListAllIssueWatchTasks` in `store.go`.
Workspace cleanup already uses the all-record queries and watch ownership.
There are no GitHub workspace-specific list methods in this checkout.

`DeleteReviewWatch` and `DeleteIssueWatch` also consume the by-watch methods.
Move their explicit deletion inventory to the existing unfiltered
`ListReviewPRTaskIDsByWatch` and `ListIssueWatchTaskIDsByWatch` methods.
Preserve their authorization, empty-ID handling, and best-effort deletion.
Reset and deduplication lookups remain unfiltered.

GitLab applies the predicate to all, workspace, and by-watch review and issue
queries in `store_watches.go`. Preserve workspace joins, generation values,
and ordering. Its cleanup currently skips empty reservations and retains that
behavior.

Azure DevOps applies the predicate to `ListWorkItemWatchTasks` and
`ListPullRequestWatchTasks` in `watch_store.go`. Preserve watch generation
scoping. Its cleanup also continues to skip empty reservations.

Archived and missing-task records are ignored, not purged by this change.
Retaining them avoids changing deduplication and allows restored tasks to
become eligible again. Existing task-deletion handlers may still remove them.
No schema migration or index is needed: the existence query uses the task key.

Eligibility reflects committed task state at query time. This change does not
make remote requests atomic with a concurrent archive or deletion.

## GitHub batch termination

Extend the private deletion gates to return errors alongside eligibility and
reason. Extend both batch helpers to return a partial count and error.
Use `errors.As` with `GitHubAPIError` and the existing
`isGitHubRateLimitAPIError` classifier in `errors.go`.

Check cancellation before each row. On a rate-limit error, return immediately,
including on the empty-reservation path. Ordinary fetch errors retain existing
per-row failure tracking and continuation. Do not classify every 403 as a
rate limit. Propagate rate-limit errors from `GetAuthenticatedUser` as well.

Per-watch, global, orphan, and workspace cleanup entry points propagate the
partial count and error. Global sweeps stop before the next workspace batch.
In each review or issue poll cycle, a rate-limit error suppresses subsequent
cleanup calls, including its final orphan sweep. Discovery remains subject to
its existing search-bucket rules. This is a cycle-local stop, not a new global
credential circuit or scheduler.

Keep existing client rate tracking and reset behavior. A later poll retries
normally. Emit one batch-level error through existing caller logging instead
of one new failure for each unvisited row. Unvisited rows do not increment
failure counters.

## Minimal review cleanup lookup

Replace the cleanup gate's `GetPRFeedback` call with `GetPR`. For merged or
closed PRs, apply the existing policy without additional provider reads.
For open PRs, fetch `ListPRReviews` and the authenticated user only when needed
for the existing approval rule. Preserve approval matching and policy behavior.
Never fetch comments, check runs, workflow runs, or workflow jobs for cleanup.
Reuse existing authenticated-user caching. Do not introduce another cleanup TTL.
Propagate rate-limit errors from each required read to the batch stop path.
Thread the resolved automation `RateTracker` into batch admission and honor
core exhaustion before each row. Never substitute another workspace's tracker.

## Test boundaries

Use real store fixtures and instrumented clients. Cover mixed collections,
all cleanup entry points, policy matrices, empty reservations, preserved
deduplication, and explicit watch deletion. Fixtures representing active tasks
must insert actual task rows. Missing-task fixtures must assert zero provider
calls rather than treating an absent task as active.

Rate tests cover wrapped typed errors, ordinary 403 responses, cancellation,
partial counts, an error on the first reservation, tracker exhaustion, and
later-cycle recovery. Poller tests prove the final orphan sweep does not restart
cleanup after a rate-limit stop.

## Related contracts

- [GitHub identity ownership](../../../decisions/0047-github-authentication-ownership.md)
- [PR lifecycle prompt protection](../../../decisions/0051-pr-agent-notifications-extend-task-pr-automation.md)
- [Requirements](../requirements/watch-task-cleanup.md)

Existing authorization and lifecycle protections remain authoritative. This
local candidate filter needs no new architecture decision.
