---
id: "05-metadata-cas-identity"
title: "Preserve metadata member identity in compare-and-set"
status: complete
wave: 4
depends_on:
  - 03-browser-evidence
plan: "plan.md"
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
acceptance_criteria:
  - AC-TASKS-PROMPT-ATTACHMENTS-002.3
  - AC-TASKS-PROMPT-ATTACHMENTS-002.5
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
---

# Task 05: Preserve metadata member identity in compare-and-set

## Summary

Make session metadata JSON compare-and-set distinguish object members, so values
cannot be exchanged or duplicated under different keys while satisfying the
expected-value guard.

## In scope

- Include full object-member identity (or an equivalent complete key/index-aware
  representation) in both SQLite JSON structural comparisons.
- Prove that swapped `execution_id` and `attempt_id`, renamed/removed members,
  and duplicate values at different keys do not match the expected JSON value.
- Preserve equality when only object serialization order changes.
- Add environment-gated PostgreSQL coverage for the same CAS method and cases.

## Out of scope

Changing metadata ownership, CAS API shape, or unrelated JSON operations.

## Acceptance

1. Wrong object member identity never satisfies the expected-value CAS, even
   when the same scalar values appear in both documents.
2. Reordered object serialization remains structurally equal.
3. SQLite tests pass locally. PostgreSQL coverage skips unless
   `KANDEV_TEST_POSTGRES_DSN` is configured and exercises this method when set.

## Verification

From the repository root, run:

```bash
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'TestSetSessionMetadataKeyIfJSONValue' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'TestSetSessionMetadataKeyIfJSONValuePostgres' -count=1)
```

The PostgreSQL test is required even when the DSN is unavailable; in that case
record the environment-gated skip explicitly. Demonstrate RED for the SQLite
member-identity regressions before changing the SQL.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/metadata_initial_prompt_submission_cas_test.go`
- `apps/backend/internal/task/repository/sqlite/metadata_initial_prompt_submission_cas_postgres_test.go` (new)

## Dependencies

Task 03 and the existing JSON CAS implementation in `session.go`.

## Risks

SQLite JSON paths and PostgreSQL JSONB operators use different dialects. Keep
both implementations covered through the repository method rather than testing
only a standalone SQL expression.

## Parallelism

Owns only the SQLite session repository CAS method and its direct tests. Task 04
owns all orchestrator and receipt-provenance behavior.

## Results

Complete on 2026-10-06. SQLite JSON CAS now includes `fullkey` in both structural
comparisons, distinguishing object members while keeping serialization order
irrelevant. Regressions cover swapped ownership values, renamed and removed
fields, and reordered object serialization.

Validation passed:

- `go test -trimpath ./internal/task/repository/sqlite -run 'TestSetSessionMetadataKeyIfJSONValue' -count=1`
- `go test -trimpath -v ./internal/task/repository/sqlite -run '^TestSetSessionMetadataKeyIfJSONValuePostgres$' -count=1`
  was invoked and skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
- The full SQLite repository package and complete backend test run passed. See
  `plan.md` for the full-suite command and remaining validation.
