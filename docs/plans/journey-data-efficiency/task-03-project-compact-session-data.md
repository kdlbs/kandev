---
id: "03-project-compact-session-data"
title: "Read narrow session projections for navigation"
status: done
wave: 3
depends_on:
  - "02-batch-completion-summary-reads"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-006
  - REQ-PLATFORM-JOURNEY-LOADING-001
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-006.3
  - AC-PLATFORM-INTERACTIVE-READS-006.4
  - AC-PLATFORM-INTERACTIVE-READS-006.5
  - AC-PLATFORM-JOURNEY-LOADING-001.2
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 03: Read narrow session projections for navigation

## Summary

Padding unrelated session metadata from 4 KiB to 1 MiB does not increase projected bytes or trigger full-metadata decode in navigation reads. Repository tests inspect actual projection shape.

## In scope

Introduce typed narrow summary observations and compact task-session list projections. Migrate boot and HTTP workflow/task-list enrichment together.
Replace full session metadata/configuration/worktree reads where only aggregate status and compact labels are needed. Combine primary-session information with the new batch observations.
Preserve every existing compact DTO field, pending-action revision, primary fallback, deleted-session error behavior, usage/command count, and runner mutability. Keep full-model methods for their rich and mutation callers.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- Padding unrelated session metadata from 4 KiB to 1 MiB does not increase projected bytes or trigger full-metadata decode in navigation reads. Repository tests inspect actual projection shape.
- Boot, workflow snapshots, and compact session lists retain status and labels for mixed states and missing primaries. Full selected-session responses remain complete.
- No session or criterion N+1 path returns; all affected repository, service, and DTO tests pass on both engines.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/backendapp -run 'Test(SessionSummaryProjection|TaskSummaryProjection|TaskStatusSummary|.*PendingAction|.*RunnerMutability|.*PrimarySession|.*StatusSummary.*Boot|JourneyRead)' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestSessionSummaryProjectionPostgres$' -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use a disposable PostgreSQL database with `KANDEV_TEST_POSTGRES_DSN`. Never use the live database. A missing server or skipped test is an incomplete engine gate.

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/session_summary_projection_test.go (new)`
- `apps/backend/internal/task/service/service_sessions.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_status_summary_session_order_test.go (handler projection coverage)`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/status_summary_boot_test.go`

## Dependencies

[Task 02](task-02-batch-completion-summary-reads.md)

## Risks

Partial compact rows must not clear rich fields. Error and permission precedence cannot be inferred from coarse lifecycle state.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Done. The first projection-size check caught mismatched control-fixture labels; after normalizing the expected identity/state fields and aligning the unrelated labels, the 4 KiB and 1 MiB padding cases produced equal serialized projections. Both remain below the 16 KiB summary bound. The repository-shape test verifies that the query extracts only required labels and `last_agent_error`, never selects full metadata/configuration snapshots, and the full selected-session read still retains those rich fields. Service fallback and DTO tests preserve compact fields and explicit `last_agent_error: null` clearing. Handler and boot tests cover mixed session states, missing primary sessions, and combined primary/list observations without full session or primary rereads.

The required checks passed on 2026-10-09:

```text
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers ./internal/backendapp -run 'Test(SessionSummaryProjection|TaskSummaryProjection|TaskStatusSummary|.*PendingAction|.*RunnerMutability|.*PrimarySession|.*StatusSummary.*Boot|JourneyRead)' -count=1)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<disposable PostgreSQL 17 DSN>' go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^TestSessionSummaryProjectionPostgres$' -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

No engine gap remains for this order. The test-only PostgreSQL container is disposable and will be removed after all database work orders finish.

### Review correction

Compact observations authorize the task before narrow or fallback repository reads. New service and HTTP/WS regressions cover owner access, foreign account/workspace denial, unscoped internal callers, and missing tasks. Compact boot siblings use the same runtime summary enrichers as list rows. Tests retain foreground activity, cancellation revisions, and parked epochs/revisions without rich metadata. See the [review correction results](implementation-evidence.md#review-remediation).


## PR #4404 browser CI remediation

Compact summary observations now retain the selected model ID and matching display name. SQLite and PostgreSQL extract two scalar values. The full-model fallback uses the same bounded projection. Provider-restored model identity remains authoritative over stale local overrides; unrelated configuration growth does not enlarge compact responses. Boot siblings receive one display choice, without configuration options or rich metadata. E2E fixtures request a full session explicitly when inspecting preparation, delivery recovery, or persisted configuration.
