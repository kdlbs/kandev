---
id: "01-classify-absent-generations"
title: "Classify absent generations in session projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
acceptance_criteria:
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.22
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.23
  - AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.24
system_design:
  - ../../specs/platform/system-design/expected-runtime-log-severity.md
---

# Task 01: Classify absent generations in session projection

## Summary

Correct the shared DTO projection's handling of a generation lookup with no
rows. Cover real startup state and preserve diagnostic visibility for unrelated
failures across session readers and notifications.

## In scope

- Begin with `TestFreshSessionRecoveryProjectionWithoutGeneration` using
  package `sqlite_test`, a temporary database, real task/session creation,
  successful session lookup, and `dto.EnrichSessionRecoveryBlocks`.
  Register database cleanup immediately after opening it. The test must fail
  with `task session not found` before changing production code.
- Classify typed absence only from the generation lookup in the shared helper.
  Verify direct and wrapped sentinels according to existing conventions.
- Add DTO failure controls, real current/stale generation fixtures, and
  observer-backed HTTP and orchestrator projections. Repeated HTTP reads
  must include a conditional 304 and prove session lookup still succeeds.
- Include a block-list not-found control to prevent broad error suppression.

## Out of scope

Repository semantics, generation persistence, launch scheduling, frontend code,
worktree optimization, and public documentation changes.

## Acceptance

1. A persisted fresh session projects an empty collection and repeated HTTP
   reads/orchestrator notifications emit no missing-generation warning.
2. Unexpected generation and block-list failures still reach existing warnings;
   missing sessions still receive the existing HTTP not-found response.
3. Real fixtures with both current and stale blocks return only the current
   incarnation/generation's open blocks; admission contracts remain unchanged.

## Verification

Run the first test alone before implementation and retain its expected failure,
then run the complete block after the correction from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/dto ./internal/task/handlers ./internal/orchestrator -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -timeout=20m -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/session_recovery_blocks_test.go`
- `apps/backend/internal/task/repository/sqlite/fresh_session_recovery_projection_test.go` (new)
- `apps/backend/internal/task/handlers/task_session_recovery_projection_test.go` (new)
- `apps/backend/internal/task/handlers/task_session_etag_test.go` (existing pattern)
- `apps/backend/internal/orchestrator/delivery_block_projection_test.go`
- `apps/backend/internal/orchestrator/fresh_session_recovery_projection_test.go` (new)

## Dependencies

None. Read the requirement's planned extension and the design's session recovery
projection section, plus the existing durable harness-boundary ADR.

## Risks

Avoid the import cycle from internal-package SQLite tests importing DTOs;
the integration fixture must use package `sqlite_test`. Preserve strict repository
errors used by admission. Do not compare elapsed-time thresholds to prove this
classification.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/expected-runtime-log-severity.md), criteria `.22` through `.24`.
- [Design](../../specs/platform/system-design/expected-runtime-log-severity.md#session-recovery-projection).
- [Harness boundary ADR](../../decisions/2026-09-10-durable-harness-session-boundaries.md).
- `apps/backend/internal/task/repository/sqlite/session_test.go` for fixture patterns.
- `apps/backend/internal/task/handlers/task_session_etag_test.go` for conditional HTTP reads.
- `apps/backend/internal/orchestrator/delivery_block_projection_test.go` for event projection.

## Results

Completed. The permanent real-repository regression failed before the fix
with `task session not found`. Focused regressions passed after the correction.
The original four-package race command passed DTO, handlers (76.356s), and
orchestrator (219.972s); SQLite exhausted Go's package-wide 10-minute timeout
while starting `TestTaskRepositoryBranchPolicyMigrationPreservesLegacyRows`,
with no prior assertion failures. The verification block now isolates SQLite
with a 20-minute timeout for this runner. Its result is recorded below.

Final validation:

- Focused new/changed regressions: passed.
- DTO, handlers, and orchestrator package race suites: passed in the original combined invocation.
- `go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -timeout=20m -count=1`: passed.
- Specification/catalog validation and documentation coverage preflight: passed.
- `git diff --check`: passed.
