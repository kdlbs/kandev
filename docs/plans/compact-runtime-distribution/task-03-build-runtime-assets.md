---
id: "03-build-runtime-assets"
title: "Build canonical helpers and staged archive variants"
status: complete
wave: 3
depends_on:
  - "02-resolve-remote-helpers"
plan: "plan.md"
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-001
  - REQ-RELEASE-COMPACT-RUNTIME-003
acceptance_criteria:
  - AC-RELEASE-COMPACT-RUNTIME-001.1
  - AC-RELEASE-COMPACT-RUNTIME-003.1
  - AC-RELEASE-COMPACT-RUNTIME-003.4
  - AC-RELEASE-COMPACT-RUNTIME-003.5
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 03: Build canonical helpers and staged archive variants

## Summary

Produce one validated helper for each remote target and compact/full archive
candidates from the Task 02 manifest schema. Keep the currently published
default TAR and ZIP names complete so this task can merge and release safely
before channel cutover.

## In scope

- Add one required canonical helper build job for Stable and Nightly. Use the
  full commit SHA, commit timestamp, `-trimpath`, executable-format and Darwin
  arm64 signature checks, reproducibility test, and artifact upload retries.
  Stamp host `kandev` and native `agentctl` with the same full SHA.
- Generate the fixed-schema `remote-helpers.json`, four compressed helper
  assets, SHA-256 sidecars, and compact/full TAR and Windows ZIP candidates.
  Stage compact candidates under new `-slim` names; retain complete existing
  default names and their current consumers until Task 04.
- Extend bundle validation and workflow contract tests to reject missing,
  mismatched, duplicate, or unexpected staged assets. Require identical
  helper bytes for the same release ref across a rebuild.
- Keep the local source-build/default `package-bundle.sh` output full.

## Out of scope

- Runtime fetch/cache behavior, package-manager channel cutover, and changes
  to Desktop preparation or Docker inputs.

## Acceptance

- Every Stable host platform has staged compact and full candidates built
  from the same canonical helper bytes and manifest identity. Nightly uses
  that helper producer but retains full packages without a fetch manifest.
- The `-slim` candidates exclude all four helpers; `-full` candidates contain
  and validate all four. The existing default TAR/ZIP names remain complete
  and runnable; Windows ZIP contents match their TAR counterparts. Staged
  candidates cannot match public release upload globs.
- A missing required variant, checksum, or signed helper fails the release
  gate before any channel publishes, and a rebuild of one ref produces the
  same helper bytes.

## Verification

```bash
bash scripts/release/runtime-bundle.test.sh
python3 .github/scripts/release-workflow-contract_test.py
bash scripts/release/updater-manifest.test.sh
```

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `scripts/release/package-bundle.sh`
- `scripts/release/runtime-bundle.test.sh`
- `scripts/release/updater-manifest.test.sh`
- `apps/backend/Makefile`

## Dependencies

Task 02 defines and validates the manifest schema before any producer writes
it. All existing archive consumers remain on the complete default names.

## Risks

- Matrix-local timestamps currently make same-named helpers differ by host.
- The Darwin arm64 helper must remain executable on Apple Silicon after
  compression and packaging.
- A publish wildcard must not accidentally upload a `-slim` candidate as a
  supported default before Task 04.

## Parallelism

`sequential`

## Inputs

- `REQ-RELEASE-COMPACT-RUNTIME-001`, `003`, and artifact-production design.
- Task 02's manifest parser and fixture contract.
- Existing release upload retry and runtime bundle contract tests.

## Results

Implemented the canonical Stable/Nightly remote-helper producer with full-SHA
and commit-time stamping, reproducible cross-target builds, executable format
and Darwin arm64 signature checks, compressed assets, SHA-256 sidecars, and
Stable standard/full manifests. Stable archive candidates are staged under
`-slim` and `-full` names while the existing default archives remain complete;
Windows ZIP candidates are checked against their TAR contents. Existing
same-named helper assets are checked by release API digest before publication.
The local package-bundle default remains full.

Validation passed: `bash scripts/release/runtime-bundle.test.sh`,
`python3 .github/scripts/release-workflow-contract_test.py` (36 tests), and
`bash scripts/release/updater-manifest.test.sh`.
