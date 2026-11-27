---
created: 2026-10-13
status: implemented
requirements:
  - REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001
system_design:
  - ../../specs/platform/system-design/expected-runtime-log-severity.md
legacy_specs: []
---

# Implementation Plan: Fresh-session recovery projection

## Overview

Treat an absent harness generation as an empty recovery projection for an
existing task session. One sequential work order corrects the common DTO
boundary and verifies its callers. Platform owns this repair because it owns
runtime diagnostic severity; task and harness storage authority stays intact.

## Confirmed cause and evidence

On 2026-10-13, session `41451f9b-9401-4881-9634-9ce98f2bd243` existed at
22:25:39.935 Europe/Lisbon. HTTP projection emitted 14 warnings between
22:25:41.082 and 22:25:51.730, approximately every 750 ms. Orchestrator
projection emitted the same missing-generation error at 22:25:49.554.
The first generation committed at 22:25:51.735569843, confirmed by a read-only
database query. The warning means the generation query has no rows, despite
the task session existing.

`EnrichSessionRecoveryBlocks` accepts a nil generation with no error, but the
real repository returns `models.ErrTaskSessionNotFound`. The existing DTO
test for missing generations uses a nil-success fake and misses that mismatch.

Startup completed independently: prepared launch began at 22:25:42.353,
worktree creation was logged at 22:25:49.470, agent launch at 22:25:49.556,
and prompt acceptance at 22:25:51.769. The 7.117-second preparation interval
contains worktree materialization and potentially its prerequisites. Retained
logs do not split remote refresh, lock waiting, checkout, or setup costs.
The warning path has no launch retry or wait; no causal latency claim is made.

## Scope

### In scope

- Classify only the generation lookup's typed absence at the shared DTO boundary.
- Cover repeated fresh-session reads, HTTP validators, and orchestrator projection.
- Preserve unexpected-error diagnostics and current-generation filtering.

### Out of scope

- Worktree performance changes or permanent timing instrumentation.
- Changes to frontend polling, session existence, repository error contracts,
  generation creation, recovery admission, or public response shape.
- UI markup, copy, localization, feature flags, migrations, and public docs.

## Technical approach

In `apps/backend/internal/task/dto/dto.go`, handle the typed absence returned
by `GetCurrentHarnessSessionGeneration` using the repository's established
sentinel conventions. Keep `GetCurrentHarnessSessionGeneration` strict and
continue propagating unexpected generation failures and all block-list errors.
Verify the shared helper through a real fresh SQLite fixture, the HTTP handler,
and the orchestrator's session-state projection.

| State | Projection | Evidence |
| --- | --- | --- |
| Existing session, no generation | Empty collection, no warning | Real SQLite fixture and repeated HTTP reads |
| Current generation with open blocks | Current owner's blocks only | DTO and orchestrator tests |
| Earlier incarnation or generation | No inherited blocks | Mixed current/stale fixture |
| Unexpected read failure | Existing warning path | Observer-backed failure controls |
| Missing task session | Existing not-found response | HTTP regression |

## Tests

- `.22`: `TestFreshSessionRecoveryProjectionWithoutGeneration` in a new
  external-package repository test; existing session lookup succeeds before
  the projection assertion. This regression fails before the fix.
- `.22` and `.23`: handler tests for repeated fresh-session reads, 200/304
  validators, no warning, and a genuine missing session.
- `.23`: DTO tests for unexpected generation errors and all block-list errors,
  paired with observer-backed caller warnings.
- `.24`: DTO and real-repository current/stale block fixtures plus
  orchestrator projection tests.

These are backend integration and diagnostic contracts. No browser interaction
or rendered UI changes are planned, so no Playwright work is required.

## Work orders

- [x] [Task 01: Classify absent generations in session projection](task-01-classify-absent-generations.md)

## Verification results

Implementation completed. Design-stage evidence:

- `go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestDebugFreshSessionRecoveryProjection$' -count=1`: expected failure at DTO enrichment with `task session not found`, after successful persisted-session lookup. Temporary test removed.
- `python3 scripts/list-docs.py validate`: passed (381 decisions, 1589 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Local `pr-docs.cjs.validateCoverage` preflight: actual documentation-only diff
  exempt; proposed DTO implementation plus this package covered, no errors.

Implementation validation:

- New real-repository regression failed before the fix and passed afterward.
- Focused DTO, HTTP, repository, and orchestrator regressions: passed.
- DTO, handlers, and orchestrator race suites passed in the original combined
  command. SQLite reached the package-wide 10-minute limit without assertion
  failures; a separate `-timeout=20m` SQLite race run passed.
- Specification/catalog validation, documentation coverage preflight, and
  `git diff --check`: passed.
- Task 01 is complete. Repository and recovery-admission semantics remain strict.

## Risks

- Broad suppression of the sentinel could hide block-list errors. Handle it
  only at the generation-read boundary.
- Changing the repository result could weaken recovery admission. Keep its
  existing semantics and mutations unchanged.
- This repair removes misleading warnings; it does not promise a shorter
  worktree preparation interval.
