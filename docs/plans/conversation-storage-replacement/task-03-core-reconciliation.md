---
id: "03-core-reconciliation"
title: "Migrate core conversation delivery"
status: done
wave: 3
depends_on:
  - "02-plugin-reconciliation"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.11
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.12
system_design:
  - "../../specs/plugins/system-design/conversation-source-reconciliation.md"
---

# Task 03: Migrate core conversation delivery

## Summary

Remove the core dependency on journal replay while preserving ordinary live Chat updates.
Use contiguous change batches for normal delivery and connect discrepancy repair to the existing hydration coordinator and prove recovery in the rendered application.

## In scope

- Own core subscription, ID-based message/turn projection, applied-versus-observed revisions, and discrepancy-only recovery hydration.
- Migrate normal publication to transaction-bound receipts and project each change once. Do not run legacy and v2 payload delivery in parallel.
- Reject stale v1 subscriptions with an explicit error after cutover. Test that a full reload restores current transport.
- Preserve pending local messages, rich DTOs, cached-window invalidation, task/session selection, and terminal deletion.
- Replace journal-specific E2E assertions with visible recovery outcomes while preserving the old regression scenarios.

## Out of scope

- Legacy table/file cleanup and any UI layout, touch, or navigation redesign.

## Acceptance

- Mounted idle Chat and Prompt History recover current messages and turns after a missed event or backend restart.
- Contiguous streaming updates and turn completion apply by ID without broad hydration. Recovery preserves optimistic state and discards covered batches.
- Core and plugin views recover independently without cross-cache mutation or duplicate normal notifications.

## Verification

Run from the repository root. Obtain behavioral RED evidence before production edits.

```bash
(cd apps/backend && go test -race ./internal/gateway/websocket ./internal/task/handlers ./internal/task/service ./internal/orchestrator -count=1)
(cd apps/web && pnpm exec vitest run lib/ws/client.test.ts hooks/domains/session/use-session-recovery.test.tsx lib/ws/handlers/messages.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/conversation-recovery.spec.ts tests/task/prompt-history-panel.spec.ts tests/session/session-stream-overload-isolation.spec.ts)
```

Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Managed E2E rebuilds the application and fixture. Run desktop and mobile sequentially.

## Files likely touched

- `apps/backend/internal/gateway/websocket/ordered_session_events.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_turns.go`
- `apps/web/lib/ws/client.ts`
- `apps/web/lib/ws/ordered-session-events.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-session-turns.ts`
- `apps/web/hooks/domains/session/use-session-turns-hydration.ts`
- `apps/web/hooks/domains/session/use-session-message-fetch.ts`
- `apps/web/hooks/domains/session/use-session-recovery.test.tsx`
- `apps/web/e2e/tests/plugins/conversation-recovery.spec.ts`

## Dependencies

02-plugin-reconciliation.

## Risks

- Journal removal can otherwise suppress the only normal message publisher.
- A later revision check must not certify an unapplied earlier message. Test gaps independently of payload projection.
- Core hydration must not await the same readiness barrier that its own completion satisfies.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and the frontmatter criteria.
- [System design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [ADR](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Existing conversation handler, journal, Host facade, and recovery tests provide the regression patterns.

## Results

- Migrated core conversation delivery to v2 source subscriptions with ID-based rich message/turn projection and revision-gated hydration recovery.
- Retained optimistic messages, terminal removal, bounded pending changes, and existing core panel behavior while removing the old ordered journal runtime from production composition.
- Added client tests for matching updates, gaps, malformed/reset changes, recovery, and v2 transport selection.
- `go test -race ./internal/gateway/websocket`: passed.
- Targeted frontend Vitest and the managed core recovery, prompt-history panel, and stream-isolation desktop checks: passed.
- The follow-up reproduced the reported task-service package condition on the comparison base and current tree; both passed the full race package, with three current-tree repetitions. The gateway and frontend recovery gates also pass. See the [follow-up Task 03 results](../conversation-storage-follow-up/task-03-task-service.md).


## Review remediation

Added the five-second subscribed-session revision worker and delivery authorization rechecks. Core revision observations wait one second for pending changes, and stop after unsubscribe. Gateway race tests and core client regression tests pass.

See [the plan](plan.md#review-remediation-verification) for the historical
remediation commands and results. The follow-up package records final delivery
evidence.
