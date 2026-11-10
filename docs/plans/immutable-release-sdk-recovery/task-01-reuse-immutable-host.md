---
id: "01-reuse-immutable-host"
title: "Reuse verified immutable host releases"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-003
acceptance_criteria:
  - AC-RELEASE-COMPACT-RUNTIME-003.4
  - AC-RELEASE-COMPACT-RUNTIME-003.7
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 01: Reuse Verified Immutable Host Releases

## Summary

Make an existing complete immutable host release a verified input to backfill.
Preserve its assets, notes, tag, and updater feed while allowing dependent
publication jobs to continue.

## In scope

- Add `existing-release.mjs` with injectable HTTP/command boundaries and focused
  stateful tests. Extend existing validators or extract their shared inventory
  only when necessary. Do not maintain a second platform/asset allowlist.
- Add an initial `publish-release` classification/verification step using
  helpers from `github.workflow_sha`. Keep the signed tagged source as input.
- Distinguish confirmed absence from authentication, transport, malformed
  metadata, and ambiguous release identities. Preserve the normal mutable path.
- For a published immutable release, verify required files and integrity in a
  separate directory. Cover archives, sidecars, canonical helper identity,
  desktop assets, optional complete updater sets, size report, and browser demo.
- Skip all host release mutations only after verification succeeds. Preserve
  explicit downstream success gates and normal Stable/Nightly exclusions.
- Update the public recovery how-to, including landing token creation,
  repository scope, environment secret placement, and secret-name preflight.

## Out of scope

- SDK draft creation, signing, npm publication, and bootstrap implementation.
- Token generation or copying any credential value into repository files.
- Deleting, replacing, or reopening a published host release.

## Acceptance

1. A complete immutable `v0.99.0` fixture passes with no release writes.
   Absent and mutable controls still exercise their existing publication paths.
2. Missing, duplicate, corrupt, mismatched, or unreadable required evidence fails
   before downstream publication. New rebuild bytes never replace old assets.
3. Workflow tests prove that verified reuse skips notes/updater/release writes
   and preserves all mode, source-authority, and downstream dependency guards.

## Verification

Run from the repository root. Start with a failing immutable-release regression.

```bash
node --test scripts/release/existing-release.test.mjs scripts/release/remote-helper-assets.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
zizmor .github/workflows
git diff --check
```

Capture the existing zizmor baseline before editing. Report pre-existing
findings separately. Introduce no finding in the changed recovery path.

## Files likely touched

- `scripts/release/existing-release.mjs` and `.test.mjs` (new).
- `scripts/release/remote-helper-assets.mjs` and `.test.mjs` when sharing validation.
- `scripts/release/verify-desktop-assets.sh` only if reusable validation needs adjustment.
- `.github/workflows/release.yml` and `.github/scripts/release-workflow-contract_test.py`.
- `Makefile` and `.github/workflows/lint-action-pinning.yml` for regression execution.
- `docs/public/release-process.md`.

## Dependencies

None.

## Risks

Original assets can be irreparable when already frozen. Fail with the exact
missing/conflicting identity. Do not classify a 403, timeout, or bad response
as absence, or compare nondeterministic rebuilt installers as replacement authority.

## Parallelism

`sequential`

## Inputs

- [Host recovery design](../../specs/release/system-design/compact-runtime-distribution.md#published-immutable-release-recovery).
- [Repair plan](plan.md), `.github/AGENTS.md`, and the release skill.
- Existing staged asset verification, canonical helper checks, and desktop verifier.

## Results

Implemented with an initial failing workflow-guard regression. Verification:

- Host asset and canonical helper suites: 24 tests passed. A real archive,
  helper manifest, checksum, desktop, and unsigned updater fixture passed.
- Release workflow contracts: 74 tests passed. Action pinning: 9 tests passed.
- Public docs validator suite: 63 tests passed. All 55 public pages validated.
- `zizmor .github/workflows`: the same 10 baseline findings remain (exit 14).
  Rule, severity, workflow, and primary finding text match the pre-edit report.
  No new finding was introduced. Step indices and line positions shifted.
- Formatting and `git diff --check` passed.
- A read-only invocation against actual `v0.99.0` verified its signed source
  and selected reuse for GitHub release ID `435377048`. No asset was changed.

PR review regressions reproduced JSON rejection, incomplete and unsafe ZIPs,
incorrect size rows, and leftover partial downloads before their fixes. Both
compact archive variants and legacy Windows ZIPs now pass real extraction and
bundle validation. Size reports are checked against published bytes. The suite
runs in the regular local and CI Node test commands; all three Make platform
expansions include it. The combined release-helper suite passed 62 tests,
including the final size-rounding regression.
