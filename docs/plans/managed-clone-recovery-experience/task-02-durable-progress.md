---
id: "02-durable-progress"
title: "Persist and project recovery operation progress"
status: completed
wave: 2
depends_on:
  - "01-private-artifacts"
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-005
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.2
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.5
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.6
  - AC-TASKS-MANAGED-CLONE-RELOCATION-005.8
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
  - ../../specs/tasks/system-design/managed-clone-relocation-experience.md
---

# Task 02: Persist and project recovery operation progress

## Summary

Provide authoritative recovery status across hydration and WebSocket clients.
Keep terminal results after claim release. Fence phase updates and retries by
operation, attempt, ownership generation, and monotonic revision.

## In scope

- Add latest-operation schema, repository API, and runner binding. Establish
  the durable operation after the claim and before heavy mutation inside admission.
- Instrument existing per-slot phases without another tree scan. Add bounded
  heartbeat reporting and separate workspace completion from agent readiness.
- Preserve accepted transfer through browser disconnect, while respecting
  operation deadlines and backend shutdown. Prove ownership of that runner.
- Reconcile older-instance nonterminal rows as interrupted on backend restart.
  Retain claims and require explicit current admission for continuation.
- Guard duplicate repairs and background workspace reconstruction. Return the
  current `in_progress` result without another transfer or misleading error.
- Add rich session DTO, boot/detail/list hydration, read-only status action,
  session notification, web types, and revision-aware store merging.
- Keep unknown phases and older-server omissions compatible. Do not change
  rendered components or erase current errors through progress writes.

## Out of scope

Recovery UI, localization, file labels, new cancel behavior, automatic retries,
claim expiry, new lifecycle states, and autonomous migration workers.

## Acceptance

1. Status is durable, path-free, readable during migration, and consistent across
   boot, task reopening, reconnect, and notifications. Reads never bootstrap workspaces.
2. Later slots and resume failures cannot produce false readiness. Restart marks
   abandoned work interrupted; duplicates cannot transfer again or evict claims.
3. SQLite and configured PostgreSQL prove fences. The web store rejects stale
   attempts, operations, revisions, environment bindings, and hydration responses.

## TDD and regression evidence

Add the planned tests from the plan: `TestRecoveryOperationProjectionCASAndClaimRelease`,
`TestPostgresRecoveryOperationSerializesWriters`,
`TestWorkspaceRecoveryProgressAllSlotsAndResumeOutcome`,
`TestWorkspaceRecoveryInterruptedRunnerDoesNotExpireClaim`,
`TestWorkspaceRecoveryStatusReadDoesNotBootstrap`,
`TestWorkspaceRecoveryBootProjection`, and `TestWorkspaceRecoveryNotificationParity`.

Include a late callback from an earlier attempt of the same operation. Include
claim release before a status read, notification-before-hydration, legacy omitted
fields, publication success followed by resume failure, and persistence failure
at a phase boundary. A mixed inventory must contain healthy and failed slots.
Unit tests for web state normalization need no new rendered mobile composition.
Task 03 owns browser parity evidence for this contract.

## Verification

Run from the repository root. Install dependencies once if this checkout lacks
`apps/node_modules`. Run commands sequentially.

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service ./internal/orchestrator ./internal/backendapp ./internal/gateway/websocket ./internal/agent/handlers -run 'Test.*(WorkspaceRecovery|RecoveryOperation|RecoveryClaim|ManualRecovery)' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/state/slices/session/workspace-recovery-projection.test.ts)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before marking this order done, run the configured PostgreSQL gate:

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated test PostgreSQL DSN before this gate}"
(cd apps/backend && go test ./internal/task/repository/sqlite -run 'TestPostgres(RecoveryOperation|TaskEnvironmentRecoveryClaim)' -count=1)
```

Use the repository isolated-schema fixture. Do not use the live application's
database. If no test DSN is available, record the gate as blocked, not passed.
Record actual test counts so a filter matching no tests cannot count as evidence.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_recovery_operations.go` (new), test, and PostgreSQL test.
- Task repository interfaces and `internal/task/service/workspace_recovery_progress.go` (new).
- `apps/backend/internal/worktree/recovery_admission.go` and phase callback contracts.
- `apps/backend/internal/orchestrator/task_operations.go` and recovery preflight helpers.
- `apps/backend/internal/orchestrator/managed_clone_recovery_progress_test.go` (new).
- `apps/backend/internal/task/dto/dto.go` and session hydration service paths.
- `apps/backend/internal/backendapp/boot_state.go` and dependency wiring.
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/session_notifications.go`
- Existing session recovery/status handler registration.
- `apps/web/lib/types/http.ts`, shared WebSocket types, and notification handlers.
- `apps/web/lib/state/slices/session/session-merge.ts`
- `apps/web/lib/state/slices/session/task-session-projection-actions.ts`
- `apps/web/lib/state/slices/session/workspace-recovery-projection.test.ts` (new).

## Dependencies

Task 01 supplies versioned journal locators and authenticated operation/slot
provenance. Progress must follow the same operation across both storage layouts.

## Risks

Admission already performs heavy recovery before returning. Instrument inside
that path, not after it finishes. Claims prove exclusion, not liveness. A progress
failure cannot undo a published replacement or authorize another operation.

## Parallelism

`sequential`

## Inputs

- Design sections: Durable latest-operation projection; Phases and completion;
  Runner liveness; Hydration and wire contract.
- Existing recovery claim/CAS tests, workspace error projection, session notification
  tests, boot status tests, and revision-aware pending-action merge pattern.
- [Plan compatibility and test matrices](plan.md).

## Results

Implemented durable, fenced recovery operation records and runner liveness,
path-free session and boot projections, read-only status lookup, reconnect
notifications, and revision-aware web state merging. Nested recovery admission
retains the outer runner's claim and progress ownership, and accepted recovery
preserves launch context values and cancellation.

Verification passed: the required filtered Go packages recorded 48 matching
tests passed and 6 skipped; the gateway WebSocket and agent handler packages
had no matching tests. The session recovery projection suite passed 6/6, and
`pnpm run typecheck` passed. Catalog validation and spec lint checks passed.
The PostgreSQL concurrency gate was blocked because
`KANDEV_TEST_POSTGRES_DSN` is unset; no PostgreSQL pass is claimed.
