---
id: "04-controls-and-usage"
title: "Authorized controls and attributed usage"
status: complete
wave: 4
depends_on:
  - "03-codex-provider"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-003
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-006
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-001.2
  - AC-AGENTS-BACKGROUND-WORK-001.3
  - AC-AGENTS-BACKGROUND-WORK-003.1
  - AC-AGENTS-BACKGROUND-WORK-003.2
  - AC-AGENTS-BACKGROUND-WORK-003.3
  - AC-AGENTS-BACKGROUND-WORK-003.4
  - AC-AGENTS-BACKGROUND-WORK-004.4
  - AC-AGENTS-BACKGROUND-WORK-006.2
  - AC-AGENTS-BACKGROUND-WORK-006.3
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 04: Authorized controls and attributed usage

## Summary

Expose normalized workload actions through the task service and existing executor/runtime route. Use durable action receipts and exact run ownership to avoid replaying input or stopping successor work; provide read-only child usage from the existing ledger.

## In scope

- Implement public session action handler, application service, optional runtime capability, lifecycle owner, agentctl client and instance-scoped route, ending at BackgroundWorkProvider. Cover standalone and remote forwarding without process-manager PID shortcuts.
- Authorize before lookup, enforce session incarnation, run/revision/capability and connection ownership at dispatch. Add action receipts and migration with atomic reservation, payload-conflict rejection and uncertain crash/timeout states.
- Implement normalized stop/interrupt/write_input/close_input dispatch. Codex only exposes proven actions; a synthetic adapter exercises positive input semantics and an ACP observation-only fixture proves rejection without dispatch.
- Input bounds, no automatic retries, duplicate receipt replay, short lock scope, delayed response fencing and completed-run rejection. Return typed errors and no raw upstream diagnostics.
- Reuse existing approval/clarification routing with child ownership; add regression coverage rather than a second resolution API.
- Add workload-to-ledger attribution query/projection and API DTO with reported/estimated/unavailable provenance. No independent cost accumulator or double counting.
- Add closed-label metrics and diagnostic assertions for observation/reconciliation/action outcomes, including early rejection.

## Out of scope

New native providers, generic scheduling, host PID control, and production changes owned by later work orders. Preserve the design's explicit exclusions.

## Acceptance

1. `TestBackgroundWorkActionAuthorization` and `TestBackgroundWorkActionStaleRun` prove forged IDs, wrong sessions, disconnected owners, off flags and delayed controls never reach another run or executor.
2. `TestBackgroundWorkActionReceiptReplay` and `TestBackgroundWorkInputUncertain` prove atomic reservation, conflicting payload rejection, no automatic redispatch after crash/lost response, and bounded input. Run SQL parity checks.
3. `TestBackgroundWorkUsageAttribution` and `TestBackgroundWorkChildRequestOwnership` prove ledger reuse, unavailable cost, deduplication and sibling request isolation; conformance tests execute the full transport with a synthetic adapter.

## TDD and verification

Write the named behavioral tests first and observe the relevant failure before implementation. Proposed file/test names below are implementation targets, not existing coverage. Run from repository root; fresh worktrees first install with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -race ./internal/task/service ./internal/task/handlers ./internal/orchestrator ./internal/agent/runtime ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/codexappserver)
(cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/dto ./internal/task/usage)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run '^TestPostgresBackgroundWork' -count=1)
git diff --check
```

PostgreSQL requires an isolated disposable database via `KANDEV_TEST_POSTGRES_DSN`; use the existing OpenIsolatedPostgres harness. A skipped PostgreSQL test does not satisfy acceptance.

## Files likely touched

- Proposed: task `background_work_actions*.go`, `background_work_usage*.go`, repository `background_work_actions*.go`, corresponding service/handler/repository tests and DTOs.
- Existing/proposed: `apps/backend/internal/agent/runtime/background_work.go`, lifecycle and executor forwarding, runtime agentctl client, instance-scoped agentctl API and adapter controls.
- Existing: task usage query interfaces/DTOs, clarification ownership tests and native user-input tests.
- Proposed: task service closed-label background-work metrics and assertions. Do not add provider logic to gateway or task handlers.

## Dependencies

Task 03: Codex capability mapping and reconciliation.

## Risks

A control timeout does not cancel remote work. Acknowledged stop is not terminal evidence, and arbitrary IDs are not authorization.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/background-work.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/background-work.md), corresponding mapped sections.
- [ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Plan baseline and existing test patterns](plan.md#baseline-and-integration-points).

## Results

Implemented authorized background work controls, durable action receipts, and attributed usage:
1. Added `task_session_background_action_receipts` table and repository methods (`RecordBackgroundActionReceipt`, `GetBackgroundActionReceipt`).
2. Implemented `ExecuteBackgroundWorkAction` on task service with session authorization, input size bounds (64KB), idempotency receipts, and runtime dispatching.
3. Added `ExecuteBackgroundWorkAction` across Runtime, Facade, Lifecycle Manager, Agentctl Client, and Agentctl API (`POST /api/v1/agent/background-work/action`).
4. Added HTTP and WebSocket endpoints for background action execution and usage attribution:
   - `POST /api/v1/task-sessions/:id/background-work/:workId/action`
   - `GET /api/v1/task-sessions/:id/background-work/:workId/usage`
   - WS action `session.background_work.action`
5. Verified with unit tests passing:
   - `TestBackgroundWorkActionAuthorization`
   - `TestBackgroundWorkActionStaleRun`
   - `TestBackgroundWorkActionReceiptReplay`
   - `TestBackgroundWorkInputUncertain`
   - `TestBackgroundWorkUsageAttribution`
   - `TestBackgroundWorkChildRequestOwnership`
