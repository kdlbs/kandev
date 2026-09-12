---
id: "02-owned-temporary-cleanup-policy"
title: "Owned temporary cleanup policy"
status: completed
wave: 2
depends_on: ["01-temporary-folder-visibility"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-004.8
system_design:
  - ../../specs/system-page/system-design/storage-temporary-folders.md
---

# Task 02: Owned temporary cleanup policy

## Summary

Allow operators to save an opt-in cleanup policy for registered temporary artifacts.
Keep manual cleanup available and preserve the ownership and quarantine checks.

## In scope

- Add the disabled-by-default nested setting and provider settings injection.
- Reconcile ownership per run, reload candidate state, and apply saved retention.
- Reuse explicit cleanup for scheduled/full runs after policy checks.
- Add localized policy controls and accurate quarantine result wording.
- TDD coverage for policy selection, lifecycle protection, recovery, and API compatibility.

## Out of scope

Shared-file cleanup, legacy adoption, configurable age, and cross-filesystem fallback.

## Acceptance

- The policy matrix covers disabled, enabled, explicit, full-manual, scheduled, and unrelated-resource runs.
- Every mutation uses fresh ownership and lifecycle evidence, with protected files unchanged in failure tests.
- Saved policy survives reload and uses the existing authorization, dirty-state, busy, and result flows.

## ASCII UI preview

UI-02 excerpt, [full preview](plan.md#ascii-ui-preview):

```text
Temporary artifacts
Clean stale Kandev temporary artifacts  [off]
Registered inactive artifacts only. Minimum age: 24 hours.
Files move to quarantine. Shared files are excluded.
[Clean stale artifacts]
Result: <GB> moved to quarantine.
```

Map to AC-004.7 and .8. Phone labels wrap and the manual action uses full width.
Keep help inline and actions at least 44 pixels on phones.

## Verification

Run from repository root after Task 01 installed dependencies.

```bash
(cd apps/backend && rtk go test ./internal/system/storage/...)
(cd apps/backend && rtk go test ./internal/backendapp -run 'Test(Storage|OverviewCache)')
(cd apps/web && rtk pnpm exec vitest run components/settings/system/storage/storage-policy-card.test.tsx components/settings/system/storage/storage-maintenance-settings.test.tsx components/settings/system/storage/storage-run-history.test.tsx)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
```

Task 03 owns rendered browser evidence.

## Files likely touched

- `apps/backend/internal/system/storage/types.go`, `settings.go`, and tests.
- `apps/backend/internal/system/storage/tempartifacts/provider.go`, `registry.go`, and tests.
- `apps/backend/internal/backendapp/storage_maintenance.go` and tests.
- `apps/backend/internal/system/storage/operations_test.go` and `scheduler_test.go`.
- `apps/web/lib/types/system.ts` and `hooks/domains/system/use-storage-maintenance.ts`.
- `apps/web/components/settings/system/storage/storage-policy-card.tsx`, `storage-maintenance-settings.tsx`, `storage-run-history.tsx`, and tests.
- `apps/web/src/locales/`.

## Dependencies

Task 01. Preserve its informational source and broad read-only boundary.

## Risks

A snapshot can become stale before mutation.
Saved quarantine retention currently needs explicit wiring into this provider.
The compatibility result field must not cause the UI to describe moved bytes as freed space.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/system-page/requirements/storage-maintenance.md), requirement 004.
[Design](../../specs/system-page/system-design/storage-temporary-folders.md), Cleanup policy through Presentation.
Existing `tempartifacts` tests and storage settings patch tests.

## Results

Implemented. Added the disabled-by-default saved policy, scheduled/full-run opt-in, explicit
cleanup bypass, saved quarantine retention, fresh lifecycle and ownership revalidation before
rename, localized policy controls, and moved-versus-freed result wording. Backend and focused
frontend tests passed. Review remediation now reconciles registry owner liveness before every
scheduled candidate selection; the owner-death and repeated scheduled-cleanup regression passes
without restarting the service.
