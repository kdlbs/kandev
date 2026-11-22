---
id: "02-canonical-storage"
title: "Incremental canonical output storage"
status: done
wave: 2
depends_on:
  - "01-output-codec"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INCREMENTAL-SHELL-001
acceptance_criteria:
  - AC-PLATFORM-INCREMENTAL-SHELL-001.3
  - AC-PLATFORM-INCREMENTAL-SHELL-001.4
system_design:
  - ../../specs/platform/system-design/incremental-shell-delivery.md
---

# Task 02: Incremental canonical output storage

## Summary

Implement dormant SQL head/chunk storage and a transactional tool-output projector, plus existing output-detail hydration. Keep the existing full-snapshot path readable.

## In scope

- Add idempotent head/chunk schema, bounded append/drop/replace storage and dedicated `agent_delivery_shell_projection.go`; commit tool message summary, chunks, effect and cursor together.
- Route explicit output-detail reads through the storage reference; preserve legacy digest hydration. Provide exact committed projection markers for later lifecycle integration.
- Integrate deletion/reset, required-store descriptor/upgrade fixtures, payload-removal guards and retention typed proof. Cover SQLite and isolated real PostgreSQL concurrent projection.

## Out of scope

Producer activation and negotiation are owned by Task 03. No existing payload grace or historical data rewrite.

## Acceptance

- `TestIncrementalShellProjectionCrashReplay` proves rollback/commit boundaries, one canonical append for duplicate replay, changed duplicate rejection, missing base recovery and no cursor advance on failure.
- `TestIncrementalShellDetailSnapshot` and `TestIncrementalShellRetentionRemoval` prove current output, consistent reads, legacy hydration, post-pruning recovery, removal-marker replay and bounded per-event chunk writes.
- Fresh/replayed schema and required-store tests pass on SQLite and configured PostgreSQL; no runtime advertises v2 yet.

## Verification

Run from the repository root. Add the regression tests and show their expected
red result before making production changes. Commands below define the required checks; actual results are recorded below. Run PostgreSQL only against an isolated configured test database.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'IncrementalShell|MessagePayload|AgentDelivery' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/service -run 'ShellOutput|ToolCallMessage' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race ./internal/task/repository/sqlite -run 'IncrementalShell.*Postgres' -count=1)
```

Run `bash scripts/lint-go-changed` with every changed Go file in this task before
completion, plus `git diff --check`. Record those exact paths/results here.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go`, `base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_shell_projection.go` (new) and focused tests
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`, `agent_delivery_retention_proof.go`
- `apps/backend/internal/task/repository/sqlite/message_shell_chunks.go` (new) and focused tests
- `apps/backend/internal/task/repository/sqlite/message_payload.go`, `message_payload_replay.go`
- `apps/backend/internal/task/models/message_shell_output.go`
- `apps/backend/internal/task/service/service_messages.go`, `service_events.go`
- `apps/backend/internal/persistence/requiredstores`, `storeconformance` upgrade fixtures
- `apps/backend/cmd/kandev/e2e_reset.go`
- `apps/backend/internal/system/toolretention` storage removal integration

## Dependencies

01-output-codec.

## Risks

SQLite and PostgreSQL need equivalent writer/owner fences. A materialized whole tail per patch would defeat the task.

## Parallelism

`sequential`. The primary session owns integration and shared status updates.

## Inputs

- [Requirement](../../specs/platform/requirements/incremental-shell-delivery.md).
- [System design](../../specs/platform/system-design/incremental-shell-delivery.md).
- [Plan](plan.md), including diagnosis, compatibility matrix and test names.
- [Decision record](../../decisions/2026-10-19-incremental-shell-delivery.md).
- Existing `adapter_shell_snapshot_test.go`, `shell_output_test.go`,
  `agent_delivery_test.go`, `message_payload_test.go` and `durable_delivery_stream_test.go`
  patterns in their respective implementation packages.

## Results

Implemented bounded current-tail chunks, atomic shell projection and effect/cursor
commit, consistent detail hydration, storage-reference hiding, and canonical
payload removal. Schema replay/required-store upgrade metadata includes both tables.

RED: the detail regression returned empty output; the HTTP regression returned
404 for a stored compact reference; the removal regression retained its head.
GREEN: exact output, rollback, duplicate replay, missing-base failure, authorized
hydration and canonical cleanup pass. PostgreSQL used an owned disposable Docker
container and independent pooled connections (distinct backend PIDs).

- `go test -trimpath -race ./internal/task/repository/sqlite ./internal/task/models ./internal/task/handlers ./internal/system/toolretention -run 'IncrementalShell|MessagePayload|AgentDelivery|ShellOutput|ToolCallMessage|Cleanup' -count=1`: PASS.
- `go test -trimpath -race ./internal/task/service -run 'ShellOutput|ToolCallMessage' -count=1`: PASS.
- `go run ./cmd/sqlguard ./internal`: PASS.
- `KANDEV_TEST_POSTGRES_DSN=<owned fixture> go test -trimpath -race ./internal/task/repository/sqlite -run 'IncrementalShell.*Postgres' -count=1`: PASS.
- `KANDEV_TEST_POSTGRES_DSN=<owned fixture> go test -trimpath -race ./internal/persistence/storeconformance -count=1`: PASS (184.181s).
- Changed-Go lint: PASS, zero issues after extracting the chunk-commit helper.
- `git diff --check`: PASS.

No v2 producer is activated yet. Typed retention acceptance is wired with v2 in
Task 03; unsupported compact event bodies remain protected until then.
