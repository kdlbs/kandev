---
id: "01-skip-persisted-reclaim-admission"
title: "Skip persisted archive reclaim writer admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
---

# Task 01: Skip persisted archive reclaim writer admission

## Summary

Keep already-persisted archive-reclaim operations out of writer admission while
preserving authority for genuinely new intents. This work order records the
implemented runtime change in [PR #4849](https://github.com/kdlbs/kandev/pull/4849).
The maintainer follow-up adds permanent repository regressions without changing
production runtime behavior. Neither follow-up claims merge, deployment or fresh
hosted coverage success.

## In scope

An archive-only reader preflight by unique operation ID, after incoming envelope
validation and before BEGIN. A persisted hit returns no insertion; an error is
returned. Missing operations keep the original recovery, task-lock, managed-owner,
archive-generation and ON CONFLICT checks. Existing states are not revived.

## Out of scope

New deletion authority or policy, physical filesystem operations, generic cleanup
changes, schema/index changes, health/probe timeout changes, UI/configuration,
production-data repair, runtime deployment and permanent test infrastructure.

## Acceptance

1. Repeating a persisted operation does not borrow the writer or mutate the job,
   including while the writer is reserved. Incoming managed envelopes and reader
   failures remain rejected; the fast path grants no cleanup authority.
2. A new intent retains the exact archive generation, recovery and exclusive
   owner guards. Concurrent misses remain idempotent, and unarchive or ownership
   changes cannot authorize delayed destruction.
3. Existing bounded reconciliation, job state/retry handling and audited resource
   removal remain intact. Permanent repository tests protect the admission path;
   the delivery documents preserve traceability without changing coverage rules.

## Files likely touched

Runtime implementation:

- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`

Documentation-only follow-up:

- `docs/plans/archive-reclaim-writer-admission/plan.md`
- `docs/plans/archive-reclaim-writer-admission/task-01-skip-persisted-reclaim-admission.md`

Maintainer regression follow-up:

- `apps/backend/internal/task/repository/sqlite/resource_cleanup_archive_reclaim_test.go`

## Requirement-to-evidence mapping

| Existing criterion | Preservation evidence |
| --- | --- |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3 | Stale-generation and single/cascade unarchive-fence checks preserve delayed destructive-work exclusion |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5 | Existing restart/idempotent bounded-cursor check and exact persisted-operation contention case preserve reconciliation identity and cancellation |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7 | Foreign managed-owner and incoming-envelope cases preserve existing authority; generic admission and physical removal are unchanged |

## Dependencies and inputs

No implementation predecessor. Reuse the current
[requirements](../../specs/tasks/requirements/dirty-worktree-archive.md) and
[system design](../../specs/tasks/system-design/dirty-worktree-archive.md).
The runtime head above is the input to this documentation follow-up.
No new product requirement or architectural boundary is introduced.

## Verification

Original existing focused runtime commands from `apps/backend`:

```text
go test -trimpath -tags fts5 -p 2 ./internal/task/service -run '^Test(ArchiveReclaimBackfillUsesBoundedIdempotentCursorAcrossRestart|UnarchiveRefusesRunningArchiveReclaim|CascadeUnarchiveRefusesRunningArchiveReclaim|ArchiveReclaimBackfillAndCascadeUnarchiveShareArchiveFence)$' -count=1
go test -trimpath -tags fts5 -p 2 -race ./internal/persistence/storeconformance -count=1
```

Additional one-off Go overlays exercised the real repository boundary outside
automatic test collection: exact duplicate batch, writer-held progress, new
admission, stale generation, foreign managed owner, invalid duplicate envelope,
reader failure, cancellation and independent concurrent creation. Existing
managed-deletion and task-cleanup controls were selected in that run. The
temporary cases are evidence, not new standing test files.

Run only these repository-root checks for the documentation follow-up, plus the
existing coverage module with the actual combined PR paths and linked document
contents. Require `ok: true`, `status: covered`, and no errors; retain normal hooks.

```text
python -X utf8 scripts/list-docs.py validate
python -X utf8 scripts/lint-spec-files.py --all
git diff --check
```

## Results

The original implementation's consistent-copy case had 40 exact persisted
operations: writer transactions fell from 40 to zero, measured batch time from
979.326 ms to 0.931 ms. The writer-held duplicate case failed before correction
and all 40 operations completed after it. Timing depends on cache/load; zero
writer admission is the deterministic result.

Focused admission/protection checks passed in 35.945 seconds; independent
concurrent creation persisted exactly one operation and passed in 0.511 seconds.
Existing service controls passed in 17.783 seconds, and persistence race
conformance passed in 122.284 seconds. Normal commit hooks and independent review
passed for the source correction. These are original validation facts.

The minute-scale native SQLite latency was not independently reproduced.
PostgreSQL was not exercised because no DSN was available. A broad SQL guard
reported ten existing findings in three unchanged baseline files; it was not a
global pass. No checker, exemption or unrelated source was changed.

### Maintainer regression tests (2026-10-27)

The permanent repository tests use a file-backed SQLite database with separate
writer and read-only reader pools. They cover persisted-operation progress while
the writer is held, unchanged persisted job state, current and stale generations,
reader errors, cancellation, incoming managed envelopes, existing recovery and
managed-deletion barriers, and both guards on new operation misses.

The focused command passed:

```text
go test -trimpath -tags fts5 -p 2 ./internal/task/repository/sqlite -run '^TestCreateArchiveReclaimTaskResourceCleanupJob' -count=1
```

Changed-file Go lint passed with `0 issues`. Hosted checks for the updated PR
head must be read separately from this local result.

Documentation follow-up: catalog validation passed (403 decisions and 1,747
specifications), as did specification lint and whitespace checks. The existing
trusted evaluator accepted the actual runtime path plus the two changed delivery
documents with `covered`, `ok: true`, `errors: []`, and the declared archive
requirement/design. No validator or exemption changed. Fresh same-PR hosted
coverage remains pending; the initial failed run is not a pass.

## Risks

A read hit creates nothing; it must not be interpreted as a new runnable job.
A miss is advisory until the canonical writer transaction completes. Preserve
the original task/owner/generation and idempotency fences across both paths.
