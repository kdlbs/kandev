---
status: current
system: agents
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
---

# Workflow review with no changed files

## Requirement mapping

| Criterion | Design section |
| --- | --- |
| `AC-AGENTS-NATIVE-CODE-REVIEW-001.5` | Automatic producer and durable notice; preserved review boundaries |

## Automatic producer and durable notice

`review.Runner` continues to reject an empty changed-file set before
`CreateRun`. Its internal `NoChangesError` retains the session whose workspace
was inspected and unwraps to the existing `ErrNoChanges`. The existing error
text and `CodeFor` result stay unchanged.

Only `runCodeReviewCallback` records the automatic skip. It uses the existing
task-message service and `status` renderer. A caller-owned message ID consists
of the existing task and workflow-entry IDs; a callback without an entry gets
an ordinary random ID. No content fingerprint, new table or API is involved.
Committed task-step writers format the existing ledger ID and skip dispatch
when it is empty; the backend adapter and step-entry engine pass it unchanged.
The generic engine callback also permits an empty input ID. That compatibility
path's random fallback has no replay-deduplication guarantee and adds no new
required field or admission condition.
The metadata carries `kind=workflow_review_skipped`, the `review_no_changes`
reason, `status=skipped`, and the existing workflow step and entry IDs. This
uses the existing neutral status row rather than a turn-complete separator.
The selected session
comes from the runner's error context, with the callback session as the
compatibility fallback.

The backend adapter passes the existing `CompletedTurn` request flag through
`CreateMessageIdempotent`. Its shared message builder now honors that flag just
as ordinary message creation already does. The existing lifecycle-only,
already-completed turn is never an active foreground turn; re-delivery returns
the original message before creating a turn. This is a skipped-review notice,
not a successful review or inference result.

The task chat's existing `StatusMessage` selects the existing localized
`review:noChangesToReviewYet` copy for this marker. Reload/backfill uses the
same persisted message as live publication. No new view, polling or settings
are needed.

## Preserved review boundaries

- Manual Review and MCP calls keep their existing immediate error and create
  no run or automatic notice.
- A nonempty diff still follows the normal run/inference/finding path.
- Other launch errors keep their existing handling. Skip-notice persistence
  failure is logged and does not block workflow entry or invent a result.
- Existing review runs, findings, message history, active turns and session
  state are not rewritten.
- This path is independent of inference admission and provider ownership
  extensions. It does not reopen deferred-entry case 1 or change model policy.

## Confirmation scope

Temporary explicit overlays exercised the actual automatic callback with
SQLite message readback, duplicate entry, resolved session and foreground-turn
isolation. The original callback, runner and message builder failed the
corresponding three checks. The candidate passed the affected three Go packages
with the race detector, including existing manual/no-run, normal-diff and other
error controls and the actual backend adapter.

The callback's stored API message supplied the UI fixture. The original status
renderer showed its fallback text; the candidate passed both fresh-backfill and
remount assertions using the existing localized copy, plus the unrelated-status
control. The outside-project fixture needed shared React package resolution in
its temporary Vite config; that collection-only failure was not a product
failure. No runtime task, model call or permanent test was created.

## Rejected message writes

If persistence rejects a message after creating its completed lifecycle turn,
the service attempts bounded compensation for that newly owned turn only.
It reuses RollbackReservedTurn and the repository's session-write lock plus
DeleteTurnIfUnreferenced guard. A committed message reference prevents deletion
even when its response was lost; a failed ownership/read/delete leaves the
uncertain row intact and is logged. No blind cascading delete or new schema is
introduced.

The caller's explicit TurnID and ordinary foreground turns are never candidates.
Compensation preserves the original write error and uses a bounded context
independent of caller cancellation. The existing turn-removed publication keeps
readers in step with a removed empty lifecycle row. Healthy creation/replay,
manual review and the active session/foreground state keep their prior path.
