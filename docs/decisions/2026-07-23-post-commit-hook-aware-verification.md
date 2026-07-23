# ADR-2026-07-23-post-commit-hook-aware-verification: Post-Commit Hook-Aware Verification

**Status:** accepted
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

In changed-scope mode with authoritative PR CI, `verify` may omit only checks
whose changed paths and behavior are exactly covered by that receipt. It first
confirms that the receipt matches current `HEAD`, the verification delta, and a
clean worktree. Tests, typechecks, generated-metadata checks, script/docs/
workflow validators, TOML parsing, and other uncovered checks still run.

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

## Alternatives Considered

- **Verify before commit:** rejected because hooks repeat checks and may change
  files after verification.
- **Always rerun hook-covered checks:** rejected because PR CI already supplies
  the authoritative full matrix and the duplicate local work has low value.
- **Rely only on hooks and PR CI:** rejected because hooks do not cover tests,
  typechecks, or several specialized validators.
