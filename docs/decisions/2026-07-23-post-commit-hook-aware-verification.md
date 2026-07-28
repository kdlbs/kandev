# ADR-2026-07-23-post-commit-hook-aware-verification: Post-Commit Hook-Aware Verification

**Status:** superseded by [Single-Session Model Switching](2026-07-26-single-session-model-switching.md)
**Date:** 2026-07-23
**Area:** workflow

## Context

Kandev's pre-commit hooks already format changed Go and TypeScript files and
lint changed backend, web, and harness files. Running the same checks again in
local verification adds latency and agent cost, while tests, typechecks, and
specialized validators still need an independent gate.

## Decision

Final local verification runs after commit and before push. The commit workflow
returns a hook receipt containing the commit SHA, active-hook status, absence
of bypass flags or environment variables, successful commit result, and
post-commit worktree state.

In changed-scope mode with authoritative PR CI, `verify` must omit checks whose
changed paths and behavior are exactly covered by that receipt, and may omit no
others. It first
confirms that the receipt matches current `HEAD`, the verification delta, and a
clean worktree. Tests, typechecks, generated-metadata checks, script/docs/
workflow validators, TOML parsing, and other uncovered checks still run.

The commit handoff captures the normal hook stream so it can identify individual
successful hooks, rather than inferring results from a condensed launcher
summary. A verifier may use a narrow helper row only when the changed web source
and its colocated test are demonstrably deterministic and isolated; that row
runs the package-local typecheck, the helper test, and any uncovered changed-file
format/lint checks. Any extra production file, side effect, integration path, or
uncertainty falls back to the normal web package matrix.

For a silent final verifier, the planner performs two normal status waits, then
checks worker and owned-process liveness. When no command remains, it interrupts
the same worker once to recover a buffered completion report. It never starts a
second verifier until the first is confirmed stopped and failed to yield a
usable exact-artifact report.

Missing, stale, bypassed, partial, or ambiguous hook evidence disables the
optimization. Full verification and delivery without PR CI never omit checks
because of hook evidence. A later edit or formatter change invalidates the
receipt and requires a new commit and verification.

No product spec applies because this is an internal delivery convention.

## Consequences

Routine work avoids repeating changed-file formatting and linting while
retaining post-commit verification of the exact artifact that will be pushed.
The planner must preserve a small hook receipt and last verified SHA between
commit, verification, and push. Verification remains fail-closed when that
handoff cannot be proven.

Isolated pure helpers avoid a package-wide unit suite locally, while the PR CI
matrix still covers package-level integration. Buffered worker completion is
recovered promptly instead of wasting time or quota on an overlapping rerun.

## Alternatives Considered

- **Verify before commit:** rejected because hooks repeat checks and may change
  files after verification.
- **Always rerun hook-covered checks:** rejected because PR CI already supplies
  the authoritative full matrix and the duplicate local work has low value.
- **Rely only on hooks and PR CI:** rejected because hooks do not cover tests,
  typechecks, or several specialized validators.
- **Always run the full web unit suite for a pure helper:** rejected because a
  direct helper test plus package typecheck gives proportional local evidence,
  while PR CI remains the authoritative broader check.
- **Replace a silent verifier immediately:** rejected because concurrent suites
  contend for one checkout and may duplicate cost while the first report is
  already buffered.
