---
id: "12-review-follow-ups"
title: "Maintainer review follow-ups"
status: pending
wave: 5
depends_on:
  - "08-proposals-ui"
  - "09-session-recovery"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-005
  - REQ-COORDINATOR-PROPOSALS-002
  - REQ-COORDINATOR-PROPOSALS-005
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-001.11
  - AC-COORDINATOR-COPILOT-005.6
  - AC-COORDINATOR-PROPOSALS-002.14
  - AC-COORDINATOR-PROPOSALS-002.15
  - AC-COORDINATOR-PROPOSALS-005.10
system_design:
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/proposals.md
---

# Task 12: Maintainer Review Follow-ups (WP-4f)

## Summary

The maintainer's review of the design package (2026-09-28) found a
profile-change race in the conversation open, a stale approval claim that
nothing recovers while the process keeps running, an approval promise the UI
made beyond what the guard enforces, and an Ask about this reference too weak
for a fresh conversation to explain the item. Tasks 01 to 09 are built, so
these land as one follow-up instead of reopening them.

## In scope

- `coordinators.config_revision` (migration, default 0), incremented by the
  PATCH that clears `conversation_task_id`; the conversation open's
  conditional update checks it and returns 409 on a mismatch
  ([copilot design](../../specs/coordinator/system-design/copilot.md#conversation-task),
  step 4).
- The proposal service's once-a-minute stale-claim sweep, and the card's
  "Approval did not finish." state with **Retry**
  ([proposals design](../../specs/coordinator/system-design/proposals.md#recovery)).
- The MCP guard refusing approve and reject from a coordinator principal and
  from an unresolved principal, with the guard's action-table test extended.
- `get_coordinator_item_kandev` on the coordinator surface, scoped to the
  coordinator's own proposals and its workspace's stall records, and the
  standing-instructions line that explains the bracketed reference.
- Changes to criteria owned by built work orders, delivered here:
  `AC-COORDINATOR-COPILOT-003.1` (seventh tool, task 03),
  `AC-COORDINATOR-COPILOT-004.4` (intro copy), and
  `AC-COORDINATOR-COPILOT-005.1` to `005.3` (the chip's `ref` and the
  `About <id> [<kind>:<ref>]: ` prefix, task 06). Copy goes through `t()` in
  every shipped locale; the settings intro reads "propose work that waits for
  your decision".

## Out of scope

- Storing or editing permission settings (decision D17, gate G2).
- The panel swap (task 11).

## Acceptance

- An open racing a saved profile change never attaches the old task.
- A claim that goes stale while the process runs is recovered within a minute;
  a stale card offers Retry.
- A coordinator principal cannot approve or reject; the table test covers it.
- A fresh conversation asked about a proposal or a stall reads its record
  through the new tool, and cannot read another coordinator's proposal.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/mcp/...
cd apps/web && pnpm test -- app/coordinator lib/coordinator components/task/chat/messages
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator
```

## Likely files

- `apps/backend/internal/coordinator/` (store migration, conversation open,
  proposal sweep) and tests
- `apps/backend/internal/mcp/` (tool registration, guard) and tests
- `apps/web/app/coordinator/` (proposal card, copilot store and chip),
  `apps/web/components/task/chat/messages/user-message-body.tsx`
- `apps/web/src/locales/*/coordinator.json`

## Dependencies

- Tasks 08 and 09 have passed Review. The branch stacks on task 08's branch
  with task 09's merged in. It runs beside task 11; both touch the copilot
  store, so whichever lands second rebases. Task 10 follows both.

## Risks

- The sweep must not race the startup pass or an approve into two creates;
  it takes the same stale re-claim `UPDATE`, which admits exactly one winner.
