---
created: 2026-10-10
status: implemented
requirements:
  - REQ-RELEASE-WINDOWS-SIGNING-001
system_design:
  - ../../specs/release/system-design/windows-runtime-signing.md
legacy_specs: []
---

# Implementation Plan: Windows Runtime Signing

## Overview

Record the delivered signing path and PR review remediation for release
archives. Signing eligibility, isolated adoption, packaging contracts, and
public maintainer guidance form one sequential work package.

## Scope

The package covers the release workflow, readiness helper, focused tests,
Makefile/CI wiring, and release guide. It excludes real release or signing
requests, secrets configuration, installer signing changes, and new channels.

## Work orders

- [x] [Task 01: Sign Windows runtimes and preserve fallback integrity](task-01-sign-windows-runtimes.md)

## Verification strategy and results

- Credential-free offline nonroot containment: 61 release-workflow contract
  tests and the SignPath readiness shell tests passed.
- A corrupt ZIP exercised the pinned action's bundled extractor and proved
  partial overwrite before `Invalid CRC`. Regression tests failed the old
  workflow and pass with separate staging and complete-pair adoption.
- The implementation incorporates main `7fb97329b910d749879042850815f961c360e39c`.
  The scoped delta passed `git diff --check`.
- Broad lint, specification/catalog validation, typecheck, unit/race/E2E, and
  local hook equivalents are delegated to hosted CI under this review's host
  execution restriction. These results are pending at this documentation commit;
  no local hook pass or required-check bypass is claimed.
- No release dispatch, signing-service call, publication, or secrets mutation
  was used as verification.

## Risks

Signing can wait one hour for manual approval before falling back. Policy slugs
must identify the intended certificate. Local disk/install errors still stop
packaging; the workflow must not report a complete archive after those errors.
