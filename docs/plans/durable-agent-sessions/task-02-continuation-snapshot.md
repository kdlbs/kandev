---
id: "02-continuation-snapshot"
title: "Persist continuation snapshots"
status: complete
wave: 2
depends_on: ["01-restore-contract"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.2
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 02: Persist continuation snapshots

## Summary

Persist harness generations, restore attempts, and bounded continuation snapshots through the task repository.

## In scope

- Add additive SQLite/PostgreSQL tables and repository methods with generation compare-and-swap writes.
- Build snapshots from canonical SQL messages, task context, and saved plan references. Record deterministic cutoff, hash, and truncation metadata.
- Reuse planinjection.Reduce and ContainTags. Count the 12,000-byte handover plan within the total 64 KiB snapshot exactly once.
- Add task-owned recovery blocks and expose their typed repository contract without Office-specific state.
- Persist pending delivery state. Remove full context text from debug logs. Cover required-store conformance and previous-stable upgrades.

## Out of scope

Native process lifecycle changes and agentctl journal implementation.

## Acceptance

- Snapshots stay within 64 KiB and exclude the current request, sibling sessions, credentials, and raw protocol frames.
- Crash recovery retains prepared and uncertain snapshots without automatic reinjection.
- Fresh, replay, upgrade, and PostgreSQL evidence cover the new tables under the existing task schema owner.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/task/repository/... ./internal/agent/runtime/lifecycle ./internal/agent/planinjection -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -tags fts5 -race ./internal/persistence ./internal/persistence/storeconformance -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/task/repository/sqlite/session_continuation_test.go`:

- `TestContinuationSnapshotBoundedCanonicalHistory`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.1`.
- `TestContinuationCheckpointCrashSafety`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.2`.

Run conformance with a provisioned PostgreSQL test database and `KANDEV_TEST_POSTGRES_DSN` set.
A skipped PostgreSQL suite does not satisfy acceptance.
Include fresh, migration replay, and previous-stable upgrade results.

Add `TestContinuationComposedPlanBudget` in the snapshot test file.
Cover separators, omission markers, UTF-8 boundaries, duplicate-plan prevention, and the unchanged 4,000-byte dynamic budget.

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/agent/runtime/lifecycle/session_history.go`
- `apps/backend/internal/agent/planinjection/`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/persistence/requiredstores/`
- `apps/backend/internal/persistence/storeconformance/`
- `apps/backend/internal/task/repository/sqlite/session_continuation_test.go` (new tests or extensions).

## Dependencies

[Task 01](task-01-restore-contract.md).

## Risks

- Message pagination and UTF-8 truncation must remain deterministic. A SQLite-only result is insufficient.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/agents/system-design/harness-session-continuity.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented in the task repository continuation schema and `continuation_snapshot.go`, with bounded canonical context and crash-state tests. SQLite/store-conformance checks pass; PostgreSQL execution remains an environment-dependent final gate.
