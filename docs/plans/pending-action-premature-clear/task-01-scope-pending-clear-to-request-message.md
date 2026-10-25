---
id: "01-scope-pending-clear-to-request-message"
title: "Scope pending-action clear to the request's own message"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 01: Scope pending-action clear to the request's own message

## Root cause

`apps/backend/internal/task/statussummary/projector.go`'s
`applyPendingMessageLocked` clears a session's armed `pending_action`
(`permission` / `clarification`) whenever **any** tracked message on that
session reaches a terminal, non-`"pending"` `metadata.status` — not only the
permission/clarification request's own message. Ordinary `tool_execute` /
`tool_edit` / `tool_read` / `script_execution` messages carry a `status` field
as part of normal streaming, so the very next one after a permission request
clears the flag within a second or two, before the user ever answers it. This
is the sidebar/kanban "shield icon flashes and disappears" bug.

## Acceptance

- `applyPendingMessageLocked`'s terminal-status or deletion clear branch only
  fires when the triggering message is a request that can arm `pending_action`
  and its normalized type plus `metadata.pending_id` exactly match the request
  identity stored when the action was armed.
  See `docs/plans/pending-action-premature-clear/plan.md` for the exact
  before/after diff.
- A regression test proves an unrelated `tool_execute` message reaching
  `status: "completed"`, and a request-shaped message with a different
  `pending_id`, do not clear an armed permission request; the matching request
  still clears it.
- All existing tests in the package keep passing:
  `TestProjectorClearsPendingWhenRequestMessageResolves` (the request's own
  resolution still clears — permission approved/expired/rejected,
  clarification answered/cancelled) and
  `TestProjectorKeepsPendingWhenRequestStaysAnswerable` (a `status: "pending"`
  update on a detached clarification still keeps it armed).
- Keep the `events.ClarificationAnswered` /
  `ClarificationPrimaryAnswered` / `ClarificationCancelled` /
  `ClarificationStaleDismissed` branch in `applySourceEventLocked` unchanged;
  the shared clear helper removes the stored identity as well as the action.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/statussummary/...
```

## Files likely touched

- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Output contract

Report the exact diff to `applyPendingMessageLocked`, the new regression test
and its pre-fix failure (confirming it reproduces the bug) and post-fix pass,
the full-package test result, and updated task/plan status.
