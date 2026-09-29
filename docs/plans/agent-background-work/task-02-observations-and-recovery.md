---
id: "02-observations-and-recovery"
title: "Durable observation and recovery path"
status: complete
wave: 2
depends_on:
  - "01-contracts-and-rollout"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-004
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-002.1
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-002.3
  - AC-AGENTS-BACKGROUND-WORK-002.4
  - AC-AGENTS-BACKGROUND-WORK-002.5
  - AC-AGENTS-BACKGROUND-WORK-004.1
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 02: Durable observation and recovery path

## Summary

Deliver a provider-neutral inspection projection from normalized observations through persistence, session reads, and websocket events. Project existing ACP observations read-only and retain existing admission accounting as a separate consumer.

## In scope

- Implement workload/run/binding and bounded-output storage in task models/repository/service, with session cascade, exact run identity, monotonic revisions and migration replay on SQLite/PostgreSQL.
- Route typed observations through agentctl stream, lifecycle forwarding, orchestrator, persistence and authenticated gateway. Add paginated task-session workload/output reads and normalized update/output events.
- Reuse existing ACP subagent/detached-shell recognizers to emit only proven observation state; launch-call completion must not falsely settle detached work. Support execution-scoped identity where resume identity is absent.
- Preserve normalized message references, early-child limits, source cursors, retained finished history, generation fencing, startup unknown-state reconciliation, session deletion/reset tombstones and counts deduplicated by work/run.
- Enforce output byte/chunk/subscriber bounds and gap markers. Existing final shell output is a labelled snapshot; child content remains in the existing message writer.
- Read/gateway auth and disabled-feature tests belong here. Live native discovery belongs to Task 03; mutating actions and receipts belong to Task 04.

## Out of scope

New native providers, generic scheduling, host PID control, and production changes owned by later work orders. Preserve the design's explicit exclusions.

## Acceptance

1. `TestBackgroundWorkProjectionReplayAndSuccessor` and `TestBackgroundWorkSnapshotRace` use barriers to prove old observations/snapshots cannot reopen a terminal run or mutate a successor; unknown origin stays unknown.
2. `TestBackgroundWorkRepositoryRoundTrip` and `TestPostgresBackgroundWorkRepositoryRoundTrip` exercise production migrations twice, write/read nonzero domain records, preserve legacy history, cascade deletion and retain output truncation offsets.
3. `TestACPBackgroundWorkObservationOnly`, `TestBackgroundWorkOutputBounds`, and `TestBackgroundWorkReadAuthorization` prove the real normalized event path, lifecycle independence, bounded streams, and session isolation.

## TDD and verification

Write the named behavioral tests first and observe the relevant failure before implementation. Proposed file/test names below are implementation targets, not existing coverage. Run from repository root; fresh worktrees first install with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -race ./internal/agentctl/types/streams ./internal/agentctl/server/adapter/transport/acp ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/service ./internal/task/handlers ./internal/gateway/websocket)
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'BackgroundWork|SubagentContext')
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test ./internal/task/repository/sqlite -run '^TestPostgresBackgroundWork' -count=1)
git diff --check
```

PostgreSQL requires an isolated disposable database via `KANDEV_TEST_POSTGRES_DSN`; use the existing OpenIsolatedPostgres harness. A skipped PostgreSQL test does not satisfy acceptance.

## Files likely touched

- Existing: `apps/backend/internal/agentctl/server/adapter/transport/acp/subagent.go`, background recognizers; lifecycle event transport and orchestrator streaming handlers.
- Proposed: `apps/backend/internal/task/models/background_work.go`; `internal/task/repository/sqlite/background_work*.go`; `internal/task/service/background_work*.go`; `internal/task/handlers/background_work*.go` and matching tests. Extend repository interfaces and migrations.
- Existing: `apps/backend/internal/gateway/websocket/`, `internal/events/types.go`, normalized message metadata and session deletion/reset integration.
- Proposed: `apps/backend/internal/orchestrator/background_work_projection_test.go`, ACP `background_work_projection_test.go`.

## Dependencies

Task 01: Normalized contract and rollout gate.

## Risks

Existing invocation terminality and admission counts differ from live workload state. Do not backfill live ownership or introduce a second transcript writer.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/background-work.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/background-work.md), corresponding mapped sections.
- [ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Plan baseline and existing test patterns](plan.md#baseline-and-integration-points).

## Results

Implemented durable observation and recovery projection for normalized background work:
1. SQLite/PostgreSQL schema and repository operations for `task_session_background_work` and `task_session_background_runs` with session cascade and monotonic revision updates (`internal/task/repository/sqlite/background_work.go`).
2. Task service methods for recording observations and output chunk stream appending with size limits (200KB bounds) and session authorization (`internal/task/service/background_work.go`).
3. HTTP and WebSocket endpoints for listing and fetching session background workloads (`internal/task/handlers/background_work_handlers.go`).
4. Orchestrator stream event routing and session event bus fan-out (`internal/orchestrator/event_handlers_streaming.go`).
5. All acceptance tests passing:
   - `TestBackgroundWorkProjectionReplayAndSuccessor` & `TestBackgroundWorkSnapshotRace`
   - `TestBackgroundWorkRepositoryRoundTrip` & `TestPostgresBackgroundWorkRepositoryRoundTrip`
   - `TestACPBackgroundWorkObservationOnly`, `TestBackgroundWorkOutputBounds`, and `TestBackgroundWorkReadAuthorization`
