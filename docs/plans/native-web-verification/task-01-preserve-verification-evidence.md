---
id: "01-preserve-verification-evidence"
title: "Preserve native verification evidence"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-009
  - REQ-PLATFORM-CI-PERFORMANCE-010
  - REQ-PLATFORM-CI-PERFORMANCE-011
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-009.1
  - AC-PLATFORM-CI-PERFORMANCE-010.1
  - AC-PLATFORM-CI-PERFORMANCE-010.2
  - AC-PLATFORM-CI-PERFORMANCE-010.5
  - AC-PLATFORM-CI-PERFORMANCE-011.3
system_design:
  - ../../specs/platform/system-design/web-native-verification.md
---

# Task 01: Preserve native verification evidence

## Summary

Create a reproducible comparison harness before the required commands change.
Preserve source inventories, complete diagnostics, command failures, and measurement provenance.

## In scope

- Implement the planned Python helper and standard-library tests through TDD.
- Provide `compare-types`, `compare-lint`, and `benchmark` operations with explicit version and executable records.
- Use exact temporary reference installations that cannot modify the workspace compiler binary.
- Bound report input sizes, file inventories, and process budgets. Reject incomplete or malformed evidence.
- Capture raw stdout, stderr, original exits, elapsed time, CPU, and peak process RSS where available.
- Keep unavailable resource evidence unknown. Use argv arrays and owned temporary directories.
- Join started processes on success, errors, and interruption. Preserve failed attempts.
- Capture identical non-dependency inventories while identifying different bundled library versions explicitly.
- Preserve the prior command recipes for comparisons against the repaired current source. Old application snapshots are not equivalent baselines.
- Add fixture support for the eight initial type errors and positive counterparts.
- Add relevant recursive, generic, conditional, inference, and fast-mode regression cases.
- Reuse existing CI measurement infrastructure for hosted job intervals rather than adding another collector.

## Out of scope

- Package-command cutover, a permanent native reference compiler alias, hosted dispatch, or source compatibility repairs.
- Reuse of successful typecheck or lint results across source revisions.
- A general task runner, persistent service, or new CI job.

## Acceptance

1. Real legacy compiler fixtures and parser fixtures prove inventory and diagnostic validation, including duplicate, missing, malformed, and stale evidence.
2. Missing binaries, invalid settings, empty selections, crashes, and child errors cannot produce successful comparisons.
3. A saved report binds source hashes, tool pins, checker settings, cache state, resource scope, individual attempts, and original exits.

## Verification

These are planned new files. Run commands from the repository root.

```bash
python3 .github/scripts/web-toolchain-evidence_test.py
python3 .github/scripts/ci-efficiency_test.py
python3 -m py_compile .github/scripts/web-toolchain-evidence.py
git diff --check
```

The helper tests use isolated fixtures and the installed legacy compiler.
Native candidate integration runs begin after Tasks 02 and 03 install their exact dependencies.
Test names must cover missing diagnostics, input-set mismatch, parser failures, stale incremental success, and interrupted-child cleanup.

## Files likely touched

- `.github/scripts/web-toolchain-evidence.py` (new).
- `.github/scripts/web-toolchain-evidence_test.py` (new).
- `.github/scripts/fixtures/web-toolchain/` (new, valid and invalid corpus).
- This package's evidence and baseline record.

## Dependencies

None.
Read the paired design's Compiler boundary, Verification corpus, and Measurement and adoption sections.
Use `.github/scripts/ci-efficiency.py` and its tests as the existing evidence pattern.

## Risks

- Bundled library path differences can hide an actual application-input mismatch.
- Process RSS is not aggregate job memory.
- Missing generated JSON or external fixtures can create invalid snapshot failures.

## Parallelism

`sequential`

## Results

Completed on November 3, 2026.

- Added `.github/scripts/web-toolchain-evidence.py` with `compare-types`,
  `compare-lint`, and `benchmark`. Every child process is joined on success,
  timeout, error, and interruption; output, inventories, and timeouts are
  bounded; missing binaries, malformed output, empty selections, and
  cross-tool inventory differences fail the comparison.
- Added `.github/scripts/web-toolchain-evidence_test.py`. All 40 tests pass,
  including the installed legacy compiler over the curated corpus, missing and
  extra diagnostics, stale incremental success, parser failure, timeout cleanup,
  duplicate lint findings, and malformed lint reports.
- Added the type corpus: the valid project compiles clean; the invalid project
  produces exactly the eight injected diagnostics; the instantiation project
  produces the same TS2589 diagnostic in every compiler while its bounded
  counterpart stays clean.
- `compare-types` against the current project provisioned TypeScript
  `7.1.0-beta` in an isolated temporary installation and matched the workspace
  `tsc` 5.9.3, tsrs default, and `tsrs --fast` in cold and fresh incremental
  runs. Matching `--checkers 4` arguments reached the reference and tsrs while
  classic TypeScript kept its unsupported CLI. The required corpus manifest
  asserts expected clean and error outcomes: the invalid project reported eight
  diagnostics in every compiler and mode, the valid project zero, and the
  instantiation project the expected TS2589. A per-tool cold, warm, and
  changed-invalid probe rejected stale cached success, and all three tools
  reported identical non-dependency input inventories.
- `compare-lint` reported equal 9,810-file inventories and zero findings from
  both the retained ESLint host and the native candidate.
- `python3 .github/scripts/ci-efficiency_test.py` still passes all 7 tests.
