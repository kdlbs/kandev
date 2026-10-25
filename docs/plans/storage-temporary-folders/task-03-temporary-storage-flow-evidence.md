---
id: "03-temporary-storage-flow-evidence"
title: "Temporary storage flow evidence"
status: completed
wave: 3
depends_on: ["02-owned-temporary-cleanup-policy"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.8
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.9
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.8
system_design:
  - ../../specs/system-page/system-design/storage-temporary-folders.md
---

# Task 03: Temporary storage flow evidence

## Summary

Prove desktop and phone behavior against isolated data.
Reconcile current specifications and public operations guidance with the implemented behavior.

## In scope

- Desktop and phone analysis, refresh, error, policy-save, explicit-cleanup, and result scenarios.
- Fixture-only roots for real backend scans and deterministic registered-artifact fixtures for cleanup.
- Public operations documentation and current-design/manual-only wording.
- Final requirement, design, ADR, and package status reconciliation after evidence passes.

## Out of scope

Production cleanup, new provider behavior, broad QA, and unrelated UI changes.

## Acceptance

- Both viewports complete the same operator flows, including disabled schedule and manual cleanup.
- An Analyze refresh observes added disposable temp data without changing it.
- Documentation describes actual scope and limits, with all required commands recorded.

## ASCII UI preview

UI-01/UI-02 completion states, [full preview](plan.md#ascii-ui-preview):

```text
System temporary folders   <GB>  Partial
Some entries could not be measured.
[Analyze]

Temporary artifacts       [on]
[Clean stale artifacts]
<GB> moved to quarantine.
```

Match the phone wrapping, one-scroll layout, and touch geometry in the plan.
Inspect a rendered phone screenshot, not only element visibility.

## Verification

Run sequentially from repository root.
The guarded E2E runner rebuilds the web application and backend.

```bash
(cd apps/web && rtk pnpm e2e:run --host --project chromium -- tests/system/storage-temporary-folders.spec.ts)
(cd apps/web && rtk pnpm e2e:run --host --project mobile-chrome -- tests/system/mobile-storage-temporary-folders.spec.ts)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.test.py
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Keep all setup and teardown inside disposable fixture ownership.
A controlled response can prove rendering but cannot substitute for backend cleanup safety evidence.

## Files likely touched

- New `apps/web/e2e/tests/system/storage-temporary-folders.spec.ts`.
- New `apps/web/e2e/tests/system/mobile-storage-temporary-folders.spec.ts`.
- `apps/web/e2e/helpers/storage-maintenance.ts` and fixture setup for injected temp roots.
- `docs/public/operations.md`, principally its temporary-artifact explanation.
- `docs/specs/system-page/system-design/storage-maintenance-01.md`, `storage-maintenance-02.md`, `storage-maintenance-03.md`.
- The paired requirements, new design, proposed ADR, decision index, and this package.
- Companion plan manifests named in `plan.md`, adding successor links only.

## Dependencies

Tasks 01 and 02.

## Risks

Do not point browser fixtures at the host temporary folder.
Only advertise scheduled cleanup after backend policy tests and browser persistence checks pass.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/system-page/requirements/storage-maintenance.md), extensions 003 and 004.
[Design](../../specs/system-page/system-design/storage-temporary-folders.md).
The mobile storage and database-footprint E2E patterns.
Use the repository e2e, mobile-parity, and docs-maintainer skills during execution.

## Results

Implemented. Added isolated desktop and mobile browser coverage, reconciled the current designs,
accepted the storage visibility policy decision, and updated public operations guidance. Chromium
temporary-storage E2E passed with 3 desktop and 2 mobile tests; documentation and specification
validation passed. The final backend remediation suite also passed 1,139 tests across 9
packages; the repository-wide backend command retains its documented unrelated existing failures.
