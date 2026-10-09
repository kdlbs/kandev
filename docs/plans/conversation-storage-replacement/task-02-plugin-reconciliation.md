---
id: "02-plugin-reconciliation"
title: "Switch plugin scopes to source reconciliation"
status: done
wave: 2
depends_on:
  - "01-source-reads"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.14
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.17
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.8
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.9
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.11
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.12
system_design:
  - "../../specs/plugins/system-design/conversation-source-reconciliation.md"
---

# Task 02: Switch plugin scopes to source reconciliation

## Summary

Implement the v2 incremental subscription and migrate the browser Host facade.
Preserve current public shapes and lifecycle outcomes. Core migration follows in Task 03.

## In scope

- Own gateway v2 registration, scoped change batches, coverage-only batches, revision checks, authorization, and slow-subscriber reset.
- Replace durable replay with ID upsert/removal and complete contiguous revision coverage. Use source repair only for discrepancies or lifecycle recovery.
- Preserve loaded range, join loadMore calls, apply message/turn changes atomically, and fence late responses. Pagination continues after fully applied revisions.
- Extend existing fixture recovery E2E with a plugin missed-notification and concurrent-pagination scenario.

## Required regression cases

- Deliver 100 ordinary agent updates to a prompt-only scope. Assert zero history rereads after initial hydration.
- Apply a matching update and turn completion by ID. Assert no bulk refresh and no duplicate records.
- Drop interval 10-to-11 and deliver 11-to-12. A revision check at 12 must not advance applied state past 10.
- Deliver irrelevant changes as empty coverage batches without exposing entity IDs or content.
- Cover duplicate, reordered, partially overlapping, malformed, and oversized batches with bounded recovery.
- Page after fully applied changes without restarting loaded history. Fence events arriving during the page read.
- Cover filter entry/exit and session moves, including removal from the previous selection.

## Out of scope

- Core consumer migration, legacy storage cleanup, plugin extraction, and rendered layout changes.

## Acceptance

- A plugin-only fixture applies ordinary changes by ID with zero history rereads, including unrelated agent streaming and turn completion.
- Authorization, task filters, teardown, stale generations, and terminal state pass existing and new regressions.
- Gap, malformed batch, direct SQL, reconnect, and expiry tests repair state without falsely advancing applied revision. Idle checks read no payloads.

## Verification

Run from the repository root. Obtain behavioral RED evidence before production edits.

```bash
(cd apps/backend && go test -race ./internal/plugins ./internal/gateway/websocket -count=1)
(cd apps/web && pnpm exec vitest run lib/plugins/conversation-host.test.tsx lib/plugins/conversation-host-isolation.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/plugin-sdk typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/conversation-recovery.spec.ts)
```

Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Managed E2E rebuilds the application and fixture. Run desktop and mobile sequentially.

## Files likely touched

- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/conversation_delivery.go (new)`
- `apps/backend/internal/gateway/websocket/conversation_delivery_test.go (new)`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/web/lib/plugins/conversation-host.tsx`
- `apps/web/lib/plugins/conversation-scope.tsx`
- `apps/web/lib/plugins/conversation-event-projection.ts`
- `apps/web/lib/plugins/conversation-host.test.tsx`
- `apps/web/lib/plugins/conversation-host-isolation.test.tsx`
- `apps/web/e2e/tests/plugins/conversation-recovery.spec.ts`

## Dependencies

01-source-reads.

## Risks

- A latest-page refresh alone leaves older loaded deletions stale.
- Do not turn malformed or unauthorized tokens into infinite recovery loops.
- Keep the source revision as consistency authority. Timestamps alone do not order writes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and the frontmatter criteria.
- [System design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [ADR](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Existing conversation handler, journal, Host facade, and recovery tests provide the regression patterns.

## Results

- Migrated plugin scopes to the source-backed v2 subscription with decimal revision reconciliation, bounded buffering, source-page recovery, and terminal removal handling.
- Preserved public Host hook shapes, task/filter semantics, loaded ranges, paging, independent panel scopes, and sanitized DTOs.
- Added source reconciliation tests for matching updates, revision gaps, recovery resubscription, coverage, malformed input, and terminal lifecycle.
- Added a managed desktop fixture that drops one plugin source change, verifies a fresh subscription, restores the missed message, and continues paging without remounting.
- `go test -race ./internal/plugins ./internal/gateway/websocket ./internal/office/testharness`: passed.
- Targeted frontend Vitest, TypeScript, ESLint, and managed desktop/plugin recovery checks: passed.


## Review remediation

Binding renewal, failed-binding retry, complete turn pagination, terminal authorization removal, and revision-observation handling were fixed. The source-scope regressions pass (11 tests).

See [the plan](plan.md#review-remediation-verification) for commands, results, and remaining package gates. No commit was created.
