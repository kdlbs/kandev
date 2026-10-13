---
id: "01-sign-windows-runtimes"
title: "Sign Windows runtimes and preserve fallback integrity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-WINDOWS-SIGNING-001
acceptance_criteria:
  - AC-RELEASE-WINDOWS-SIGNING-001.1
  - AC-RELEASE-WINDOWS-SIGNING-001.2
  - AC-RELEASE-WINDOWS-SIGNING-001.3
  - AC-RELEASE-WINDOWS-SIGNING-001.4
  - AC-RELEASE-WINDOWS-SIGNING-001.5
system_design:
  - ../../specs/release/system-design/windows-runtime-signing.md
---

# Task 01: Sign Windows Runtimes and Preserve Fallback Integrity

## Summary

Make optional SignPath signing reach Windows runtime archives while keeping
original binaries intact on service or extraction failure.

## In scope and owned files

- `.github/workflows/release.yml`: eligibility, upload, signing, staged adoption.
- `scripts/release/signpath-signing-ready.sh` and its shell test: purpose and
  completeness checks.
- `.github/scripts/release-workflow-contract_test.py`: ordering, fallback,
  modes, and current archive contracts.
- `.github/workflows/lint-action-pinning.yml` and `Makefile`: test entry points.
- `docs/public/release-process.md`: repository-scoped configuration and fallback.
- This release requirement/design pair and delivery record.

## Exclusions

No real signing or publication, credentials changes, additional release
channels, installer certificate changes, or unrelated repository work.

## Acceptance

- Eligibility and fallback conform to the linked requirement, with both original
  binaries preserved until a complete signed pair is ready.
- Successful adoption restores executable modes before the current Stable/full
  and Nightly archive validation and checksumming paths.
- Maintainer docs and CI/local test entry points describe the same behavior.

## Verification

Focused checks run only in the audited offline sandbox:

```bash
python3 .github/scripts/release-workflow-contract_test.py
bash scripts/release/signpath-signing-ready.test.sh
```

Hosted CI owns catalog/specification validation and broad verification:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Dependencies and risks

No predecessor work order. The SignPath project must match the uploaded ZIP
shape and configured policy; manual approval may exceed the one-hour wait.

## Results

Implementation is complete. Both focused commands passed (61 Python contracts
and the readiness shell cases). The pinned extractor's CRC-failure proof and
Red/Green regression evidence are retained in the PR review workspace. Current
main integration preserves archive variants, helper manifests, and upload
retry gates. Hosted validation remains pending at this documentation commit.
