---
created: 2026-09-17
status: done
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/system-page/system-design/tool-payload-retention.md
legacy_specs: []
---

# Implementation Plan: Vacuum and compaction status recovery

## Overview

Prevent false persistence failures during managed SQLite maintenance. Clear a
recovered compaction status error without hiding a failed user action.
Implement the runtime guard first, then the independent status recovery path.
Both work orders require TDD. Implementation is complete.

## Evidence and requirement conformance

On September 17, the retained backend log recorded these local times:

| Time (UTC+01:00) | Observation |
| --- | --- |
| 11:00:30 | Vacuum accepted with HTTP 202 |
| 11:00:44 | Writer ping exceeded its deadline and persistence became unhealthy |
| 11:00:51 | Compaction status GET returned HTTP 503 |
| 11:02:09 | A second vacuum was accepted |
| 11:02:36 through 11:05:07 | Status GETs returned HTTP 503 |
| 11:05:38 | Status GET recovered to HTTP 200 |

Source: `/root/.kandev/logs/backend-logs-2026-09-17-000011.log` on the
investigated host. This path is evidence, not an implementation dependency.

`database.Service.runVacuum` owns the shared maintenance lease and SQLite's
single writer. `requiredstores.Health` independently pings that writer with
a two-second deadline. `requiredPersistenceMiddleware` then blocks stateful
requests with `persistence_unavailable`. The card maps that response to its
generic error. `loadStatus` records GET errors but never clears them on success.

The existing requirements describe health recovery and visible operation states.
They omit maintenance contention and read-error dismissal. This package adds
criteria to those existing capabilities and amends their technical designs.
Platform owns shared persistence health. System-page owns compaction status
recovery because it owns the policy and its action lifecycle.

The assumption check found no unresolved product choice. Scope follows the
diagnosed failure and preserves strict startup checks, real errors, and policy.

## Scope

### In scope

- Runtime probe admission through the existing SQLite maintenance guard.
- State and timestamp preservation when a periodic probe defers.
- Automatic read-error recovery with separate action-error ownership.
- Deterministic backend, hook, component, and desktop/phone browser regressions.

### Out of scope

- Database migrations, vacuum optimization, automatic vacuum, or payload changes.
- New public APIs, timeouts, feature flags, settings, or translated copy.
- General HTTP retry behavior or a new maintenance banner.
- Production database operations, push, or PR creation in this turn.

## Technical approach

The [platform design](../../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-maintenance-coordination)
and [decision](../../decisions/2026-09-17-maintenance-health-probe-coordination.md)
define runtime admission. Add a narrow periodic-check boundary in `health.go`.
Use `maintenance.ForPool` and `TryAcquire` only for SQLite runtime checks.
Keep `Health.Check` strict for startup. Hold the lease for the whole check.
Keep existing middleware and readiness behavior for actual failures.

The [compaction design](../../specs/system-page/system-design/tool-payload-retention.md#status-error-recovery)
defines error ownership. Split read and action errors inside the existing hook.
Keep the outward `error` field and existing component error translation.
Honor lifetime and generation checks before accepting or clearing any result.

## ASCII UI preview

### UI-01: Compaction error recovery

Entry: Settings > System > Data & Logs > Database.
The preview shows only the affected region. Spacing is illustrative.

```text
Current, after a GET recovers:
Messages compaction
[The operation could not complete. Refresh status and retry.]
[Refresh status]
Tasks inactive for [3] [Months v] [Analyze savings]

Proposed, after a GET recovers:
Messages compaction
Tasks inactive for [3] [Months v] [Analyze savings]
...

Proposed, unresolved action failure:
Messages compaction
[Existing translated action error]
[Refresh status]
Tasks inactive for [3] [Months v] [Analyze savings]
```

On phones, the number and unit remain adjacent. Analyze savings stays on its
own full-width row. The existing page owns scrolling and touch targets remain
44px. Error visibility follows the same hook on both viewports. No overlay,
navigation, focus, breakpoint, or layout change is planned.
The shipped `ToolPayloadRetentionCard` and its mobile E2E are the local exemplar.
The mobile guide's shared-state rule applies. A separate phone component is unnecessary.

Required structure: remove only recovered read errors. Keep failed actions and
persisted failures visible. This maps to `AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.7`
and `.8`, with existing phone parity criterion `.5`.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| Platform 007.6 | `TestRuntimeHealthDefersDuringMaintenance` in `health_test.go` |
| Platform 007.7 | `TestRuntimeHealthDeferralPreservesMixedStoreStates` and `TestStartupHealthDoesNotDefer` |
| Platform 007.8, existing 007.1/.3/.4 | `TestRuntimeHealthResumesAfterMaintenance`, existing missing-table recovery, and middleware regression |
| System-page 003.7 | Hook test `clears a recovered status error on background polling` and rendered card recovery |
| System-page 003.8 | Hook tests for action-error precedence, stale GET rejection, and persisted failure preservation |
| System-page 003.5/.7/.8 | Desktop and mobile recovery scenarios in the existing retention specs |

The full prefixes are in the work-order frontmatter. New test names are planned,
not existing evidence. Use a held real writer connection and guard for a
deterministic backend reproduction. Do not depend on a large database making
vacuum slow. The guard fixture reproduces the proven resource conflict.

## E2E tests

Extend `apps/web/e2e/tests/system/tool-payload-retention.spec.ts` (`chromium`)
and `mobile-tool-payload-retention.spec.ts` (`mobile-chrome`). Add status GET
failure followed by success without Refresh status, and action failure followed
by a successful GET. Keep real route rendering and existing mutation scenarios.
Use controlled route responses and browser clock advancement for polling.
Await the specific HTTP responses before DOM assertions. No fixed sleeps.
This browser evidence covers presentation recovery. Backend tests cover the
maintenance cause without exposing production test controls.

## Work orders

- [x] [Task 01: Coordinate runtime health with maintenance](task-01-runtime-health.md)
- [x] [Task 02: Recover compaction status errors](task-02-status-recovery.md)

Execute sequentially. Task 02 does not technically depend on Task 01, but both
must pass before delivery. No delegation is authorized.

## Documentation and companion packages

The completed [retention package](../tool-payload-retention/plan.md) and
[store parity package](../postgres-domain-store-parity/plan.md) retain their
historical results. This follow-up owns only the new criteria and regressions.
Public docs need no change for this implementation. Existing health/recovery
guidance was checked for claims affected by probe deferral. No labels change.

## Verification results

Implementation completed on 2026-09-17. Artifact validation on 2026-09-17:

- `python3 scripts/list-docs.py validate`: passed (286 decisions, 986 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/vacuum-compaction-status`: passed.
- Backend focused race tests passed for required-store health, maintenance,
  database, and persistence middleware.
- Frontend focused tests passed: 23 tests across the hook and card files.
- `pnpm run typecheck`: passed.
- Desktop retention E2E passed: 4 tests on `chromium`.
- Phone retention E2E passed: 4 tests on `mobile-chrome`, including the
  existing focused phone screenshot capture at 390px.
- `git diff --check`: passed.

Product tests and browser checks from both work orders passed. The worktree
contains the implementation and its permanent regression coverage; no push or
PR was created.

## Risks

- Faults that arise during a maintenance lease remain undetected until its release.
  Prior unhealthy state and completed-check timestamps remain unchanged.
- Restore/reset quiescence must continue to govern process teardown and readiness.
- Clearing the shared error unconditionally would hide failed mutations.
- A guard acquired only around the ping leaves schema checks exposed to the same race.
