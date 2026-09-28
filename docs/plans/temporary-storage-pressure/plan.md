---
created: 2026-09-28
status: complete
requirements:
  - REQ-SYSTEM-PAGE-TEMP-PRESSURE-001
  - REQ-SYSTEM-PAGE-TEMP-PRESSURE-002
system_design:
  - ../../specs/system-page/system-design/temporary-storage-pressure.md
legacy_specs: []
---

# Implementation Plan: Temporary storage pressure

## Overview

Show pressure on the filesystem used by temporary files, then explain its largest directory entries.
Deliver two sequential vertical slices. Each slice includes backend, UI, and focused browser evidence.
Leave tool defaults, operator configuration, and existing cleanup authority unchanged.

- [Requirements](../../specs/system-page/requirements/temporary-storage-pressure.md).
- [System design](../../specs/system-page/system-design/temporary-storage-pressure.md).
- [Ownership decision](../../decisions/2026-09-11-temporary-storage-visibility-policy.md).

## Investigation evidence

On 2026-09-28, the host home filesystem had 181 GiB available while the separate `/tmp` filesystem had none.
The existing capacity API selected only Kandev home. The existing temporary row measured file sizes without filesystem capacity.
Manual cleanup, separately authorized by the user, recovered approximately 120 GiB before this design work.

The saved inventory included multiple custom Go caches, Node and Playwright caches, image archives, and disposable test environments.
Directory names did not prove their originating tasks. Thousands of host-utility directories occupied little space compared with build caches.
The host OS used daily cleanup with a ten-day age policy, not a capacity limit.
These observations motivate the feature. They are not portable product defaults.

## Scope

### In scope

- Temporary capacity with thresholds, timestamps, isolated failure states, and bounded visible-page refresh.
- A top-twenty direct-entry breakdown within the existing read-only scan.
- Honest ownership labels and navigation to existing registered-artifact cleanup.
- Desktop, phone, localization, compatibility, and operations documentation.

### Out of scope

- Emptying arbitrary `/tmp`, new cache cleanup providers, cache relocation, or per-task `TMPDIR`.
- Fixing the existing maintenance busy gate or workspace symlink failure.
- Remote storage, inode pressure, historical growth, or process/transcript inspection.

The user's question about a blanket empty action is answered by a recommendation, not treated as approval for new deletion authority.
This package retains the accepted read-only boundary for shared temporary files.

## Technical approach

Task 01 extends `GET /api/v1/system/storage/disk` without breaking its home fields.
It reuses `metrics.DiskUsage`, the temporary root resolver, and the existing storage hook and store.
Task 02 extends scanner partition results and `tempstore.RootMeasurement`, then renders the breakdown in the existing accordion.
Neither task adds a migration, feature flag, external tool invocation, or cleanup mutation.

| Platform | Capacity and root selection | Evidence | Unsupported behavior |
| --- | --- | --- | --- |
| Linux | Home, effective temp, distinct `/tmp`; native filesystem identity | Injected capacity/mount tests and Linux integration | Unknown sharing identity stays unknown |
| macOS | Same candidate rules; native identity | Platform reader tests and compile validation | Missing capacity is unavailable |
| Windows | Home and effective temp; native volume identity | Platform reader tests and compile validation | No invented `/tmp` path |
| E2E | Existing disposable temporary root override | Real fixture scan and controlled capacity response | Never inspect host `/tmp` |
| Older backend/cache | Existing home fields and optional additions | API, hook, and component fixtures | Missing data is unavailable, never zero |

## ASCII UI preview

All values are illustrative, not current host measurements. User-visible values use GB.
Hierarchy, control order, and states are required. Spacing is not a pixel specification.

UI-01: Storage > Host, capacity visible before recursive analysis. Desktop:

```text
Filesystem capacity                         Updated <time>
Kandev home    <path>       60%       <GB> available
Temporary     /tmp         95%       <GB> available
  Critical: temporary-file operations can fail.
  [View largest entries]

Storage analysis                            [Analyze]
  Measuring folders...  (capacity remains available)
```

UI-01 phone: one page scroll owner, independent inline cards:

```text
Kandev home
<path wraps>
60%   <GB> available

Temporary storage
/tmp
95%   <GB> available
Critical: temporary-file
operations can fail.
Updated <time>
[View largest entries]
```

UI-02: Expanded temporary root. Desktop:

```text
v System temporary folders       Sampled file sizes <time>
  /tmp     Partial: scan timed out
  Largest observed entries       File size      Ownership
  node-compile-cache             <GB>           Not tracked
  playwright-transform-cache-0   <GB>           Not tracked
  <registered directory>         <GB>           Kandev
  Other observed entries (<N>)   <GB>
  Unscanned usage is unknown.
  File sizes can differ from filesystem usage.
  Working in a task folder does not contain all temporary writes.
  [Review Kandev cleanup]
  Cleanup applies only to registered files across this installation.
```

UI-02 phone: stacked entry details and a full-width navigation action:

```text
v System temporary folders
/tmp  Partial
Largest observed entries

node-compile-cache
<GB>  Not tracked by Kandev

playwright-transform-cache-0
<GB>  Not tracked by Kandev

Other observed entries (<N>)
<GB>  Unscanned usage unknown
[Review Kandev cleanup]
Only registered files across
this installation are eligible.
```

Review navigation expands and focuses the existing Kandev artifacts section.
Its existing confirmation remains the only path to a mutation.
No entry selection, arbitrary-delete button, or automatic cleanup follows a warning.

Loading shows capacity and breakdown separately. Unknown values have no percentage or zero-byte placeholder.
A failed refresh keeps the last value with a stale label and timestamp.
A successful empty scan shows no entries and measured zero.
A partial scan says Largest observed entries and keeps sampled bytes.
No eligible artifacts leaves the existing cleanup action disabled with its reason.
Keyboard focus survives refresh. Phone targets measure at least 44 pixels, and paths wrap without horizontal page overflow.

UI-01 covers AC-TEMP-PRESSURE-001.1 through .6.
UI-02 covers AC-TEMP-PRESSURE-002.1 through .7.
Here the abbreviated AC prefix is `AC-SYSTEM-PAGE`.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| 001.1, .2, .4, .5 | `storage/handler_test.go`: distinct/shared filesystems, reserved-space pressure, zero available, failures, legacy response, rejected client paths |
| 001.3, .4 | `use-storage-maintenance-temporary-capacity.test.tsx`: fake timers, visibility/tab changes, coalescing, stale generations, scan independence |
| 001.2, .4, .6 | `storage-disk-capacity-card.test.tsx`: threshold boundaries, invalid values, stale and unavailable labels |
| 002.1, .2, .6 | `filescan/measure_test.go` and `tempstore/provider_test.go`: top twenty, ties, remainder, partial children, cancellation, excluded mounts and symlinks |
| 002.3, .5, .6 | `backendapp/storage_maintenance_test.go`: exact registry ownership, mismatched marker, missing registry, fixture root, unchanged environment |
| 002.1 through .4, .7 | `storage-overview-card.test.tsx` and `storage-overview-resources.test.ts`: breakdown, focus navigation, no mutation, limitations |

All numeric AC references in this table use `AC-SYSTEM-PAGE-TEMP-PRESSURE`.
New test function names are selected during TDD, beside these existing suites.

## E2E tests

Extend `tests/system/storage-temporary-folders.spec.ts` and its `mobile-storage-temporary-folders.spec.ts` counterpart.
Task 01 adds home-healthy/temp-critical, slow analysis, refresh recovery, and visible warning scenarios.
Task 02 adds real disposable-tree breakdown, partial data, ownership, and review navigation without mutation.
Retain the existing policy-save and explicit-cleanup scenarios.
Use `chromium` and `mobile-chrome` sequentially. Include a narrow fine-pointer viewport below 768px.
Inspect captured phone screenshots and assert touch geometry, long-path wrapping, and no horizontal page overflow.

## Companion packages

[Temporary folders](../storage-temporary-folders/plan.md) and
[analysis presentation](../storage-analysis-presentation/plan.md) remain completed historical evidence.
Their active contracts remain valid. This package adds new behavior without reopening their results.
The existing [maintenance package](../storage-maintenance/plan.md) retains mutation and quarantine ownership.

## Work orders

- [x] [Task 01: Temporary capacity warnings](task-01-temporary-capacity.md), wave 1, complete.
- [x] [Task 02: Entry breakdown and cleanup guidance](task-02-entry-breakdown.md), wave 2, complete, depends on Task 01.

## Verification results

Design validation passed on 2026-09-28:

- `python3 scripts/list-docs.py validate`: 321 decisions and 1,222 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- The repository PR-documentation coverage helper accepted the work-order references with a proposed handler change as its coverage trigger.
- Both new specifications appeared in the system-page catalog. All work orders remain pending.

Implementation and rendered verification passed on 2026-09-28:

- Backend storage, filesystem identity, temp-store, scanner, and backend-app tests passed with `go test ./internal/system/metrics ./internal/system/storage/... ./internal/backendapp -count=1`.
- Darwin and Windows temp-store test binaries compiled with `CGO_ENABLED=0`.
- The focused web suite passed: 11 files and 129 tests. TypeScript typecheck and changed-file ESLint passed.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run e2e:sleep-ratchet` passed.
- Desktop temporary-storage E2E passed all 5 tests. Mobile temporary-storage E2E passed all 4 tests. Both runs built the backend and the pseudo-locale Vite production bundle.
- Public-doc tests passed all 62 tests; public-doc validation accepted 47 pages. Specification validation accepted 321 decisions and 1,222 specifications; all specification files passed lint.
- `git diff --check` passed. No custom cache or temporary-directory overrides were added, and existing registered-artifact cleanup remains the only mutation path.
Public operations documentation is assigned to both implementation slices. No unshipped behavior is published during design.

Review remediation passed on 2026-09-28:

- Rejected capacity polls and explicit root-discovery failures keep last-success temporary readings and timestamps, mark them stale, and show a localized failure message. Successful replacement clears stale state; successful authoritative empty results remove old roots.
- Root discovery, filesystem identity, and capacity probes now use per-probe deadlines and a process-wide limit of eight active or still-blocked operations. Unknown identity does not suppress capacity measurement.
- Regression coverage passed for rejection and recovery, discovery failure and authoritative removal, bounded response deadlines, sibling capacity availability, unknown sharing, and the global probe limit.
- `go test ./internal/system/metrics ./internal/system/storage/... ./internal/backendapp -count=1`, targeted Go race tests, the focused frontend tests (12 tests), TypeScript typecheck, ESLint, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- `make -C apps/backend build` and `pnpm run build:vite` passed. Re-run desktop temporary-storage E2E passed 5 tests; mobile E2E passed 4 tests.

## Risks

- Shared temporary directories can change during a scan. Display observed sizes and completeness without claiming a transactional snapshot.
- Apparent bytes can differ from allocated disk usage. Never advertise their sum as reclaimable capacity.
- Registered-artifact cleanup can reclaim little when untracked caches dominate. Keep that limitation visible.
- Shared filesystem identity needs platform-specific evidence. A path prefix or equal capacity does not establish identity.
- Pressure polling must not trigger repeated directory scans or survive an inactive Host tab.
