---
id: "01-scope-pending-clear-to-request-message"
title: "Scope pending-action clear to the request's own message"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/bounded-task-status-delivery.md"
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

- `applyPendingMessageLocked`'s terminal-status clear branch only fires when
  the triggering message is itself a request that can arm `pending_action`
  (i.e. gated by the same `isPendingMessage` predicate already used to arm the
  flag: `requests_input` true, or `messageType` is `clarification_request` /
  `permission_request`), for both the `status != "" && status != "pending"`
  case and the `events.MessageDeleted` case.
  See `docs/plans/pending-action-premature-clear/plan.md` for the exact
  before/after diff.
- A new regression test proves an unrelated `tool_execute` message reaching
  `status: "completed"` does not clear an armed `permission` (or
  `clarification`) `pending_action` on the same session.
- All existing tests in the package keep passing unmodified:
  `TestProjectorClearsPendingWhenRequestMessageResolves` (the request's own
  resolution still clears — permission approved/expired/rejected,
  clarification answered/cancelled) and
  `TestProjectorKeepsPendingWhenRequestStaysAnswerable` (a `status: "pending"`
  update on a detached clarification still keeps it armed).
- Do not change `applyPermissionEventLocked`, `clearPendingLocked`, or the
  `events.ClarificationAnswered` / `ClarificationPrimaryAnswered` /
  `ClarificationCancelled` / `ClarificationStaleDismissed` branch in
  `applySourceEventLocked` — those already clear correctly and are out of
  scope.

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
