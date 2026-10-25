---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001
created: 2026-10-13
owners:
  - kandev
---

# GitHub CI PR freshness System Design

## Purpose and boundaries

The integration system owns reuse of provider status during GitHub automation.
This design refines the synchronization step used by the existing
[task PR automation design](../../ui/system-design/ci-pr-automation-02.md).
It retains the established freshness predicate and does not change review policy.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001` | [Refresh admission](#refresh-admission), [Automation continuation](#automation-continuation), [Failure behavior](#failure-behavior) |

## Refresh admission

`handleTaskPRCIAutomationWithRefresh` loads the task and PR-scoped options, then
computes `freshlySynced` with `ciAutomationHasFreshPRStatus`. Admit
`refreshTaskPRForCIAutomation` only when `refresh` is true, `freshlySynced` is
false, and auto-fix or auto-merge is enabled for that PR. The smallest correction
adds `!freshlySynced` to the existing conditional.

`ciAutomationHasFreshPRStatusAt` already uses `TaskPR.LastSyncedAt` and
`github.PRSyncFreshnessWindow` (30 seconds, inclusive boundary). Preserve that
predicate and its boundary tests. Do not add a timer, throttle, cache, or another
freshness definition.

On an admitted refresh, keep `TriggerPRSyncAll`, matching by task/repository/PR,
and the existing handling of a matching result from a partial task-wide sync.
Replace the evaluation input with the matching returned PR and recompute
freshness as today. Fresh input skips this entire refresh branch.

## Automation continuation

`handleTaskPRUpdated` and `handlePRFeedback` still enqueue refresh-eligible
work. The per-PR `ciAutomationCoordinator` remains the worker owner, with one
coalesced follow-up. Each evaluation applies freshness admission to its own
input; a queued follow-up containing fresh status cannot bypass the guard.

Auto-fix still calls `GetPRFeedbackForAutomation` when eligible. Preserve
feedback write-through via `persistPRFeedbackState` and `SyncTaskPR`, caches,
checkpoint deduplication, round budgets, and session routing. Auto-merge retains
`ciAutomationReadyToMerge`, the freshness check, and expected-head admission.
`handleTaskCIOptionsUpdated` retains its initial task-wide sync and subsequent
`startTaskPRCIAutomationWithoutRefresh` evaluations.

## Failure behavior

Stale or missing timestamp input continues through the existing refresh error
path. A matching PR in partial sync results remains usable; an unmatched result
never establishes freshness. Lifecycle evaluation and CI error publication keep
their existing ordering. Archived or missing tasks remain excluded.

No schema, public transport, configuration, or permission changes are needed.
Existing semantic-change and coalesced-follow-up logs remain available.

## Partial feedback and provider events

`persistPRFeedbackState` marks its derived `PRStatus` with the private
`feedbackOnly` flag. `SyncTaskPR` computes the timestamp before mutating the
stored head and base: feedback-only writes retain `LastSyncedAt` when both
identities match, otherwise clear it. Ordinary status-sync callers continue
stamping their observation as before. The partial feedback write still updates
observed fields and publishes through the same canonical persistence path.
No new wire field or database column is introduced.

`handlePRFeedback` invalidates only the affected automation feedback cache
through `InvalidatePRFeedbackForAutomation` before enqueueing evaluation.
The service resolves the existing workspace automation credential scope and
invalidates that PR's scoped feedback key. `handleTaskPRUpdated` does not
invalidate feedback: write-through events can reuse cached feedback without
recreating the forced-refresh loop. Cache TTLs and singleflight remain intact.

## Verification design

A new focused orchestrator test file must count `TriggerPRSyncAll` calls at the
handler boundary, not merely test the freshness helper. Fresh cases must also
assert automation continues: an actionable comment is queued for auto-fix,
and a ready auto-merge uses the supplied head. Stale and nil timestamps must
exercise refresh and verify use of the returned PR. Cover both switches,
PR-scoped eligibility, and the no-refresh path with the existing test mocks.
Producer-level tests enter `GetPRFeedbackForAutomation` and assert stored and
published status freshness for stale, fresh, absent, changed-head, and
changed-base observations. Event-level coverage asserts that a provider
feedback event queues newly observed feedback without forcing status sync.
A real cache test proves that invalidation refreshes the targeted PR and
preserves a sibling PR cache entry. Cache-hit persistence coverage uses a
stored title sentinel because partial feedback no longer renews the timestamp.

Retain freshness-boundary, failed-sync, stale-merge, option-save, and partial-sync
coverage. A partial-sync test must enter with stale input so it actually tests
refresh after the guard is added. Synchronize asynchronous worker tests through
observable completion and clean up workers; do not use sleep-based assertions.

This is backend behavior with no rendered or interaction change. Handler-level
provider call counts and prompt/merge effects provide the end-to-end automation
evidence; no new browser scenario is required.

## Limitations

REST feedback and GraphQL status still disagree on comment-only reviews.
Reusing fresh status removes the unbounded forced-sync/cache-invalidation cycle,
but a normal poll can still cause a semantic update and extra feedback cycle.
No exact requests-per-minute guarantee is established by this change.
