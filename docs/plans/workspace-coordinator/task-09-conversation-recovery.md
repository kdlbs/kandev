---
id: "09-conversation-recovery"
title: "Conversation recovery"
status: pending
wave: 4
depends_on:
  - "06-copilot-wired"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-004
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-001.10
  - AC-COORDINATOR-COPILOT-004.6
system_design:
  - ../../specs/coordinator/system-design/copilot.md
---

# Task 09: Conversation Recovery (WP-4c)

## Summary

A coordinator whose agent fails to start must be recoverable from the popover.
The real-agent check of the integrated build found two gaps: the conversation
route reused a task whose session had ended, which rejects every message, and
the popover's recovery feedback was unreachable because `useSessionResumption`
sets its error only on the automatic path the copilot turns off.

## In scope

- Backend: the conversation route's reuse step treats a task whose primary
  session is `FAILED`, `CANCELLED` or `COMPLETED` as not reusable, archives it
  and creates a fresh task
  ([copilot design](../../specs/coordinator/system-design/copilot.md#conversation-task),
  step 2).
- Frontend: a coordinator-local recovery state in the popover. It reads the
  session state from the store; when the session is terminal or its start
  failed, it shows the session recovery feedback with an action that re-runs
  the conversation open. The shared `useSessionResumption` hook is unchanged.
- The e2e row for `AC-COORDINATOR-COPILOT-004.6` that task 06 left skipped runs
  and passes.

## Out of scope

- Any change to `useSessionResumption`, the task page, mobile or Settings chat
  recovery.

## Acceptance

- An open of a coordinator whose session failed returns a new task and
  session; the old task is archived; the open starts no agent.
- In the popover, a failed start shows the recovery feedback, its action opens
  a fresh conversation, and the Needs you and Queue lists keep working.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/...
cd apps/web && pnpm test -- app/coordinator/copilot
cd apps/web && pnpm e2e:run tests/coordinator
```

## Likely files

- `apps/backend/internal/coordinator/conversation.go` and test
- `apps/web/app/coordinator/copilot/` (body, controller and tests)
- `apps/web/e2e/tests/coordinator/`

## Dependencies

- Task 06 has passed Review; the branch stacks on task 06's branch while G0 is
  open.

## Risks

- Archiving on every open of an ended session could discard a transcript the
  manager wanted; the archived task keeps it, as after a context change.
