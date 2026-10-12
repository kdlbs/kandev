---
created: 2026-09-22
status: implemented
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-005
system_design:
  - ../../specs/system-page/system-design/storage-analysis-presentation.md
legacy_specs: []
---

# Implementation Plan: Storage analysis presentation

## Overview

Deliver concise temporary-folder timeout feedback and relative usage bars inside the existing Storage accordion.
The work orders were sequential because both changed the same resource projection and component tests.
The implementation and verification evidence are recorded below.

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md): 003.10 and requirement 005.
- [System design](../../specs/system-page/system-design/storage-analysis-presentation.md).
- [Existing temporary-storage decision](../../decisions/2026-09-11-temporary-storage-visibility-policy.md).

Confirmed intent comes from the accepted preview: descending size order, bars, existing collapsible behavior, and concise timeout feedback.
No material question remains open. Routine choices use stable ties, a shared maximum, and one fill color.

## Scope

### In scope

- All existing storage resource rows, sorted by numeric measurements.
- Proportional bars, explicit partial state, accessible headers, and phone composition.
- Deadline warning normalization, retained partial samples, and preserved unrelated diagnostic examples.
- Localized copy, focused browser evidence, and operations guidance during implementation.

### Out of scope

- New categories, sorting controls, saved preferences, drilldown, or a Grafana dependency.
- Scan performance changes, longer deadlines, cleanup changes, or host-wide disk accounting.
- New APIs, persisted state, release flags, metrics, or changes to access restrictions.
- Commit, push, or PR creation.

## Technical approach

Task 01 changes `tempstore.measurementFromResult` and `summarize` to normalize joined warnings.
The frontend `systemTemporaryResource` turns the existing deadline reason into one localized explanation.
It also handles repeated deadline lines in older snapshots without hiding unrelated diagnostics.
The shared `filescan.Limiter` and scan cancellation lifecycle remain unchanged.

Task 02 adds numeric measurements to `StorageResource` and orders the canonical list from `storageResources`.
The local pure helper owns stable ordering and scale derivation.
`ResourceRow` renders the bar within `AccordionTrigger`.
Keep stable IDs, `Accordion type="multiple"`, detail actions, and the current snapshot preference.
The existing `storage-disk-capacity-card.tsx` provides the nearby track/token precedent.
Use decorative spans for these relative bars instead of its capacity progress semantics.

Use TDD for changed logic and focused browser tests for each visible outcome.
Install dependencies once from `apps/` before any pnpm command in a fresh worktree.
Reuse existing test fixtures and their isolated temporary roots.

### Companion packages

The completed [temporary-folder package](../storage-temporary-folders/plan.md) remains historical evidence.
Its manifest links this successor for the presentation changes.
The completed [analysis-progress package](../storage-analysis-progress/plan.md),
[database footprint package](../storage-database-footprint/plan.md), and
[progressive-loading package](../storage-progressive-loading/plan.md) define unchanged compatibility behavior.
Do not reopen their tasks or rewrite their result counts.
Run the affected existing component and Storage browser suites as specified in the work orders.

## ASCII UI preview

UI-00: Current desktop Storage analysis, based on the source and supplied screenshot.
Headers have labels and sizes, without bars. Categories use fixed order.
The expanded temporary-folder warning can contain many repeated deadline errors.

```text
Task workspaces                         68.71 GB  v
Database                                 7.09 GB  v
Database backups                        19.99 GB  v
Quarantined resources                   47.26 GB  v
System temporary folders                51.08 GB  ^
  /tmp: 51.08 GB | Partial | 1,015 entries skipped
  context deadline exceeded context deadline exceeded ...
```

UI-01: Proposed desktop, Settings > System > Storage, measured rows with one expanded.
Only the categories from the screenshot are shown here. Implementation includes every existing row.

```text
Storage analysis
Total counted: 242.71 GB
Bars compare category sizes. Categories can overlap.

Task workspaces          [####################]  68.71 GB  v
-----------------------------------------------------------
System temporary folders [Partial] [###############.....]  51.08 GB  ^
  Read-only. This footprint can overlap counted categories.
  /tmp: 51.08 GB | Partial | 1,015 entries skipped
  Scan timed out. Showing partial usage.
  <up to ten distinct non-timeout diagnostic examples>
-----------------------------------------------------------
Quarantined resources    [##############......]  47.26 GB  v
-----------------------------------------------------------
Database backups         [######..............]  19.99 GB  v
-----------------------------------------------------------
Database                 [##..................]   7.09 GB  v
```

The Partial label belongs to the header beside the size and can wrap beneath it.
The total is retained from the screenshot. It is not the sum of these example rows.

UI-02: Proposed phone, same entry and expanded state.

```text
Storage analysis
Total counted: 242.71 GB
Bars compare category sizes.
Categories can overlap.

Task workspaces          68.71 GB  v
[##############################]
-----------------------------------
System temporary folders [Partial] ^
51.08 GB
[######################........]
  Read-only. This footprint can
  overlap counted categories.
  /tmp: 51.08 GB
  Partial | 1,015 entries skipped
  Scan timed out.
  Showing partial usage.
-----------------------------------
Quarantined resources             v
47.26 GB
[#####################.........]
```

UI-03: First-scan and unavailable states, shared semantic order.
Phone bars occupy the line beneath each measured header.

```text
Database backups         [##########..........]  10.00 GB  v
Database                 [....................]   0.00 GB  v
Task workspaces                                  Scanning v
System temporary folders                     Unavailable v
```

The first track is illustrative. A larger measured row can precede this excerpt.
Unmeasured rows have no measured track. All-zero snapshots show only empty measured tracks.
Existing failed-refresh feedback stays visible above the retained snapshot.

Required structure: whole-row activation, descending measured order, stable row identity,
inline expanded details, common bar scale, and a two-line phone header.
There is one page scroll owner and no fixed region or new overlay.
Phone triggers have at least 44-pixel hit targets. Labels and paths wrap.
Spacing, example sizes, and ASCII fill density are illustrative, not pixel specifications.

UI-01 and UI-02 map to AC-003.10 and AC-005.1–.7.
UI-03 maps to AC-005.3 and .8.
Here, short AC references use the prefix `AC-SYSTEM-PAGE-STORAGE-MAINTENANCE`.

## Tests

Proposed test names are implementation targets, not claims of existing coverage.

| Criteria | Test file and proposed case |
| --- | --- |
| 003.10 | `tempstore/provider_test.go`: `TestTemporaryDeadlineWarningsAreBounded`, mixed error leaves, distinct roots, retained sample/counts |
| 003.6, .10 | Existing `TestAnalyzeTemporaryDeadlineReturnsPartialSample` and cancellation tests |
| 003.10 | `storage-overview-card.test.tsx`: `shows one localized timeout and preserves unrelated diagnostics`, including older joined warnings |
| 005.1, .2, .3 | New `storage-overview-resources.test.ts`: `maps every resource measurement`, `sorts numeric sizes with stable ties`, `handles zero and missing sizes` |
| 005.2, .3 | Same new test file: `scales against the largest measured footprint`, including all-zero and partial data |
| 005.4, .8 | `storage-overview-card.test.tsx`: `preserves expanded and focused rows after refresh reordering`, first-scan and stale-snapshot cases |
| 005.5 | Existing `storage-totals.test.ts`: informational and subset exclusions remain unchanged |
| 005.6, .7 | Component assertions for label, size, partial status, decorative bars, and localized descriptions |

Frontend test paths are relative to `apps/web/components/settings/system/storage/`.
Backend test paths are relative to `apps/backend/internal/system/storage/`.

## E2E tests

| Work order | Files and projects | Required evidence |
| --- | --- | --- |
| 01 | Existing `tests/system/storage-temporary-folders.spec.ts`, chromium; `mobile-storage-temporary-folders.spec.ts`, mobile-chrome | Open partial row, one timeout explanation, retained size/counts and unrelated warning, long-path wrapping, no horizontal overflow |
| 02 | New `tests/system/storage-analysis-bars.spec.ts`, chromium; `mobile-storage-analysis-bars.spec.ts`, mobile-chrome | Descending order, proportional fills, zero/unknown states, partial header, independent expansion, reorder/refresh preservation, no cleanup request |
| 02 | Existing Storage maintenance and temporary-folder suites in both projects | Existing actions, snapshots, and disclosures remain usable |

New files live in `apps/web/e2e/tests/system/`.
Fixtures use `test-base` and controlled overview responses through `helpers/storage-maintenance.ts`.
Never measure the developer's host temporary directory for browser assertions.
Use the configured Pixel 5 project. Add breakpoint geometry cases at 767 and 768 pixels.
Assert actual trigger height, bar placement, header containment, and no document horizontal overflow.
Capture and inspect desktop and phone screenshots through `prCapture`.
Exact commands are in each work order. The managed runner builds current assets and cleans its own instance.
Run desktop and mobile commands separately. Do not overlap suites or override worker limits.

## Work orders

- [x] [Task 01: Concise temporary-folder timeout feedback](task-01-timeout-feedback.md)
- [x] [Task 02: Sorted storage bars](task-02-sorted-storage-bars.md)

## Verification results

Implementation verification on 2026-09-22:

- `go test ./internal/system/storage/tempstore ./internal/system/storage/filescan`: passed.
- Focused Storage component suite: passed, 4 files and 38 tests.
- `pnpm exec tsc --noEmit --pretty false`: passed.
- Storage and new browser-file ESLint: passed with six existing test-only warnings and no errors.
- `pnpm --filter @kandev/web build:vite`: passed.
- `make -C apps/backend build`: passed.
- Managed desktop Storage browser command: passed, 13 tests.
- Managed mobile Storage browser command: passed, 11 tests.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`: passed, 62 tests and 47 published pages.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed, 299 decisions and 1109 specifications.
- `pnpm run i18n:ratchet`: passed; the scoped system namespace Traditional Chinese generation also passed.
- `git diff --check`: passed.
- Review follow-up: the serialized empty distinct Go cache now retains `unmanaged_size_bytes: 0`,
  and the storage card preserves focused descendants, including row cleanup actions, during reorder.
- Focused desktop bars E2E after the review fix: passed, including held-refresh focus timing and
  expanded-content assertions.
- Focused mobile bars E2E after the review fix: passed, 3 tests.

Design validation on 2026-09-22:

- `python3 scripts/list-docs.py validate`: passed, 299 decisions and 1109 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans`: passed.
- `git status --short -- docs/specs docs/plans`: confirmed the expected package and companion changes.
- Local document audit: all package links, requirement IDs, and work-order design paths resolve.
- Catalog output includes the new design under system-page.

The full `pnpm run i18n:check` command remains blocked by 32 pre-existing missing Japanese
catalog keys in the unrelated `executors` and `task` namespaces; the new storage keys are
complete and the new-code ratchet is clean. The global `pnpm run i18n:zh-hant` command has the
same pre-existing Traditional Chinese catalog gaps outside the `system` namespace.

No commit or publication occurred. Public operations guidance now documents relative bars and
temporary-folder timeout behavior. Delivery state belongs to this plan and its work orders.

## Risks

- Missing byte fields currently default to zero in some formatted rows. Sorting must not inherit that ambiguity.
- Mixed joined errors can contain a deadline and a real I/O error. Deadline filtering must preserve the latter.
- Bars compare overlapping and subset footprints. The total and capacity card must retain their separate meanings.
- First-scan completion can move rows. Stable keys must preserve focus and open details.
- Long localized labels can squeeze desktop bars or phone values. Rendered geometry checks cover both boundaries.
