---
created: 2026-10-22
status: complete
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Invalid Git Ref Diagnostics

## Overview

Classify explicit invalid local ref failures in existing backend diagnostics.
The October 22 investigation found empty refs in a shared source repository.
A disposable fixture reproduced a fetch exit code of 1 with a bad-ref cause.
The current classifier maps that output to `unknown`.

This package addresses the confirmed diagnostic gap under existing AC .14.
The original fetch stderr was not retained. The report therefore distinguishes
the reproduced mechanism from the strongly supported incident hypothesis.
See [the diagnostic report](diagnostic-report.md).

## Scope

### In scope

- One fixed `invalid_local_ref` diagnostic category and explanation.
- Classification and emission tests, including a real local Git fixture.
- Existing required-refresh and best-effort policy compatibility.

### Out of scope

- Repairing the live repository, its refs, worktrees, or task records.
- Changing fallback, retry, session creation, or MCP dispatch behavior.
- Raw stderr persistence, new probes, schema, UI, public APIs, or deployment.

## Technical approach

Extend `classifyRefreshDiagnosticOutput` in
`apps/backend/internal/worktree/refresh_diagnostics.go` with bounded matching of
explicit fatal lines that identify a bad `refs/...` reference. Use the invalid-ref
design section for exact matching boundaries and the fixed explanation.
Keep `reason`, `syncFailureCause`, context precedence, and returned errors intact.

No new requirement or ADR is needed. AC .14 already requires safe explanations
for known causes. Existing refresh ADRs retain their authority. The completed
`recovery-diagnostics` package remains historical. This package extends its
classifier without rewriting its recorded results. The completed
`pathless-worktree-materialization-failure` package remains compatible: failed
preparation still has a failed environment with no workspace path.

The assumption check uses the assignment's explicit diagnostic-only scope.
There is no unresolved product choice. Corruption provenance remains an
investigation limit, not a prerequisite for classifying explicit Git output.

## Tests

AC .14: add `TestInvalidLocalRefDiagnosticClassification` and
`TestInvalidLocalRefRefreshDiagnostic` in a new
`apps/backend/internal/worktree/refresh_invalid_ref_test.go`.
The latter must exercise the real manager fetch path against local fixture
repositories with one valid selected branch and one unrelated empty ref.

Preserve AC .3, .6, and .10 through required/best-effort assertions and the
existing `TestRefreshDiagnosticsPreservePolicy`. Include URL/secret-bearing
output, missing-blob output, the generic missing-objects suffix, and cancellation.
No browser test is needed for backend-only log classification.

## Work orders

- [x] [Task 01: Classify invalid local refs](task-01-classify-invalid-local-refs.md)

## Verification results

Implemented on 2026-10-22 against main `b183b383826245af32f88860ff394abd16c548fc`.
The new classification and real Git tests first failed with `unknown` instead
of `invalid_local_ref`. After the correction, all five targeted test functions
and their subtests passed. Changed-package Go lint reported zero issues.
Catalog validation, full specification lint, documentation coverage, and
`git diff --check` passed. Exact commands and results are in Task 01.
The instance-specific diagnostic report remains local. The committed report
contains only the sanitized reproduction and its evidence limits.

PR review identified that the shared test Git helper removed the configuration
isolation overrides. A fixture-local runner now reapplies those overrides.
Regression coverage verifies rejecting hooks in both home and XDG settings;
the updated targeted suite contains six test functions. See Task 01 for results.

## Risks

- Git output varies by version and locale. Unknown shapes must remain unknown.
- An error mentioning missing objects alone cannot identify the damaged side.
- Better diagnostics do not repair data corruption or establish its writer.
- Deployment is separate from this implementation. The live instance and its
  damaged refs were not modified.
