---
id: "03-runtime-identity-projection"
title: "Project observed runtime identity"
status: done
wave: 3
depends_on:
  - "02-retained-runtime-admission"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-RUNTIME-SELECTION-003
acceptance_criteria:
  - AC-EXECUTORS-RUNTIME-SELECTION-003.1
  - AC-EXECUTORS-RUNTIME-SELECTION-003.2
  - AC-EXECUTORS-RUNTIME-SELECTION-003.3
  - AC-EXECUTORS-RUNTIME-SELECTION-003.4
system_design:
  - ../../specs/executors/system-design/runtime-selection.md
---

# Task 03: Project observed runtime identity

## Summary

Expose nullable `execution_runtime` on session detail and summary read surfaces.
Use existing lifecycle inventory as the only durable authority.

## In scope

- Add read-projection fields to session and summary-observation models, DTOs, conversions, and direct event maps.
- Select runtime from the same `executors_running` row as execution and container IDs.
  Update both full session scan helpers and bounded summary scans.
- Cover HTTP/WS detail/list, boot/session summary, and session update conversion paths without adding unbounded task overview data.
- Preserve existing `runtime` event and `runtime_name` status naming. They already describe actual execution.
- Keep `container_id` omission compatibility. Unknown `execution_runtime` serializes as null, never configured Docker.
- Add optional nullable runtime to the handwritten web session type. Verify normalization does not overwrite known runtime with configured identity.
- Preserve observed standalone for historical mismatches, including stopped rows. Do not manufacture liveness or rewrite old inventory.

## Out of scope

New SQL columns/migrations, a new writer for inventory, API renaming, UI badges, new diagnostics screens, or agent executable identity changes.

## Acceptance

1. Full sessions, summaries, direct session events, and boot payloads expose actual runtime correlated with the same execution/container identity.
   Missing inventory produces null, even when configuration selects Docker.
2. Healthy Docker and historical standalone mismatch fixtures retain truthful values through refresh/restart and typed failure projection.
   A stopped row is not presented as proof of a live worker.
3. Existing clients and auth scoping remain compatible. Projection never reveals credentials, secret references, or environment contents.

## TDD and verification

Create `TestSessionExecutionRuntimeProjection` in the SQLite package and `TestSessionDTOExecutionRuntime` in the DTO package.
Drive full and summary queries from real temporary inventory rows. Include no inventory, Docker, standalone mismatch, and rotated execution cases.
Add DTO/event/boot coverage through their actual conversion seams. Reuse existing producer contracts for runtime events and persistence.
Add a Postgres variant using the existing `KANDEV_TEST_POSTGRES_DSN` convention if dialect-sensitive SQL changes are needed.
No PostgreSQL behavior pass can be claimed from a skipped variant.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestSessionExecutionRuntimeProjection' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/dto ./internal/task/service ./internal/backendapp -run 'SessionDTOExecutionRuntime|SessionExecutionRuntime|ExecutionRuntimeBoot' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/repository/sqlite ./internal/task/dto ./internal/agent/runtime/lifecycle -run 'SessionExecutionRuntime|SessionDTOExecutionRuntime|BuildRunningFromExecution|ToAgentExecution|ExecutorRuntime' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
```

The install command is needed once in a fresh worktree. It must not alter the lockfile.
Run `bash scripts/lint-go-changed` with changed Go files from the repository root.
If a web normalization helper changes, add its focused Vitest command to Results and run it with the existing worker budget.
No rendered UI changes are planned, so no new mobile composition or ASCII preview is required.

## Files likely touched

- `apps/backend/internal/task/models/models.go` and `session_summary.go`.
- `apps/backend/internal/task/repository/sqlite/session.go`, `session_summary.go`, and focused projection tests.
- `apps/backend/internal/task/dto/dto.go` and focused DTO tests.
- Session event/boot converters reached from those models, only if they need explicit projection.
- `apps/web/lib/types/http.ts`.
- Lifecycle persistence/runtime event tests as compatibility controls.

## Dependencies

Task 02. All actual-runtime sources and refusal semantics must be fixed before their public projection is finalized.

## Risks

Full/summary SQL scan order can drift. Cover both query shapes.
The stored selection is not runtime evidence. A null value must not default to the selected runtime.
Avoid a second source of truth on `task_sessions` or `task_environments`.
Keep the bounded task-status summary contract intact.

## Parallelism

`sequential`

## Inputs

- Runtime-selection design: Identity projection.
- ADR 0003 and lifecycle `buildRunningFromExecution`.
- `session.go`, `session_summary.go`, existing `TaskSessionSummaryObservation` conversions, and DTO tests.

## Results

Implemented. Session detail and bounded summary reads expose observed execution runtime.

- Added `ExecutionRuntime` to `models.TaskSession` and
  `models.TaskSessionSummaryObservation`, populated from `COALESCE(er.runtime, '')` on the same
  `executors_running` join as `agent_execution_id` and `container_id` in both full-session and
  summary scans.
- Added nullable `execution_runtime` to both session DTOs and the legacy `ToAPI` map. Unknown
  serializes as `null`; it is never inferred from the selected executor.
- Updated the handwritten web `TaskSession` type. No SQL migration or second writer was added.

Verification:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestSessionExecutionRuntimeProjection$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/dto -run '^TestSessionDTOExecutionRuntime$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'Test.*Session|TestTaskSession|TestSessionSummary' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/dto ./internal/task/service ./internal/backendapp -count=1)
(cd apps/web && pnpm run typecheck)
```

All commands passed. No PostgreSQL-specific SQL was introduced; the added expression is portable.
