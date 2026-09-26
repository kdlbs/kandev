---
id: "03-extend-diagnostic-bundle-discovery"
title: "Extend diagnostic bundle discovery"
status: done
wave: 2
depends_on:
  - "02-implement-bounded-log-segments"
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 03: Extend Diagnostic Bundle Discovery

## Intent

Include the active file, numbered segments, and legacy singleton files in
diagnostic bundles, with the newest evidence selected first.

## Acceptance

- Backend candidate discovery recognizes only valid retained log files.
- Candidates are ordered newest-first across active, numbered, and legacy
  files before the existing archive budget is applied.
- Existing symlink exclusion, tail truncation, manifest byte ranges, and
  source-budget behavior remain intact.

## Files Likely Touched

- `apps/backend/internal/system/logbundle/archive.go`
- `apps/backend/internal/system/logbundle/service_test.go`

## Dependencies

Task 02 defines the final segment parser, filename, sequence, and retention
contract. Reuse that parser through a narrow exported helper only if doing so
does not couple bundle policy to writer state. Otherwise, mirror the strict
filename grammar in the bundle package and cover it with table tests.

## Parallelism

`sequential` after Task 02. Task 04 depends on the final collection behavior.

## Inputs

- The bundle requirements in the diagnostic-logging specification.
- The accepted bounded-log ADR.
- The segment naming and ordering behavior delivered by Task 02.
- The current archive candidate and safe-file-copy implementation.

## Implementation

1. Add failing bundle tests with active, numbered, legacy, malformed, and
   symlink candidates across retained and expired UTC days.
2. Replace the fixed three-file candidate list with strict directory discovery.
3. Sort valid candidates by UTC day and segment chronology from newest to
   oldest. Treat the active file as the newest current-day candidate.
4. Feed the sorted candidates through the existing regular-file checks, tail
   truncation, source budget, archive naming, and manifest reporting.
5. Verify that a late marker remains in a constrained bundle when older
   segments exceed the archive budget.

## Verification

```bash
cd apps/backend && go test ./internal/system/logbundle -run 'TestBackendOnlyBundle|TestBackendCandidates' -count=1
```

## Output Contract

Report the ordering rule, changed files, exact test result, blockers, and
remaining risks. Update this task and `plan.md` in the same conversation.

## Results

- Changed `archive.go` and added `backend_candidates.go` plus focused candidate
  tests to discover valid active, numbered, and legacy files from the log
  directory.
- Candidates are restricted to the current UTC day and two preceding days,
  sorted newest-first by day and segment sequence, and filtered with `Lstat`
  so symlinks and unrelated names are excluded.
- Existing archive source budgets, tail copying, truncation warnings, and
  manifest byte ranges remain in the archive path. Each candidate opens once
  without following the final path component. The collector uses the opened
  handle for both `Fstat` and copying. A candidate that disappears before
  opening produces a partial bundle warning.
- Added discovery-order, future-date, newest-within-budget, and deterministic
  rotation-during-collection bundle tests.
- Verification: `cd apps/backend && go test ./internal/system/logbundle -run
  'TestBackendOnlyBundle|TestBackendCandidates' -count=1` passed.
- Blockers: none.
- Remaining risk: bundle collection still depends on the existing archive
  budget, so a constrained archive can be partial by design.
