---
created: 2026-11-10
status: done
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-STARTUP-PROGRESS-002
  - REQ-PLATFORM-STARTUP-PROGRESS-003
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
legacy_specs: []
---

# Implementation plan: readable startup units

## Overview

Show readable backup sizes in the terminal and startup screens.
The screenshot's `15389753344 bytes` becomes `14.3 GB`.
One work order updates the renderers and proves agreement across them.

Platform owns this extension because it owns startup measurements and their consumers.
The existing startup-progress and startup-housekeeping packages remain completed delivery records.
This package does not reopen their work orders.

## Scope

### In scope

- Byte counts in launcher output, initial startup HTML, poll updates, and the restart dialog.
- Byte-rate units in launcher output.
- Complete localization for new progress templates.
- Regression evidence for desktop and phone views.
- A short explanation in the existing startup section of `docs/public/cli.md`.

### Out of scope

- Backup execution, speed, retention, scheduling, and measurement changes.
- New progress endpoints, readiness schemas, configuration, or dependencies.
- A new native desktop shell progress bridge.
- Changes to non-byte counts or the shared web size-formatting policy.

## Technical approach

Add `FormatBytes` and `FormatByteRate` helpers in `apps/backend/internal/startup/bytes.go`.
The helpers match the policy in `apps/web/lib/utils/format-bytes.ts`.
Go initial HTML and terminal output share these helpers.
The standalone page's embedded JavaScript mirrors the same numeric cases without a SPA dependency.
The React formatter imports the existing web helper.

Update `formatStartupLine` in `apps/backend/internal/launcher/health.go` for `UnitBytes`.
Update `countingProgressText`, `applyCountedProgress`, and embedded `progressText` in `startup_page.go`.
Update `formatStepProgress` in `apps/web/lib/startup-progress/format.ts`.
Format done and total independently for a counted byte step.
Progress bars and ETA calculations continue to use raw counts.

Add localized `startup.page.countingBytes` (`{{size}}`) and
`startup.page.countedBytes` (`{{done}} of {{total}}`) templates.
Include both keys in the page's embedded translation map and corresponding web namespace.
Size strings are display data with standard unit symbols.
All prose goes through `i18n.Tf` or `t()`.
Translate all seven real locales and generate Traditional Chinese and pseudo catalogs through repository scripts.

Update the CLI reference's startup paragraph with examples of displayed sizes and rates.
No ADR is necessary because this change reuses the established size policy.

## ASCII UI preview

UI-01: Startup backup progress. Entry: an application route before readiness.
The screenshot establishes the current count. Only the affected region appears here.

```text
Before: Database backup
        15389753344 bytes

After:  Database backup
        14.3 GB
```

Desktop and phone retain the existing single-column composition.
The closest mobile exemplar is the shipped startup page and restart dialog in
`e2e/tests/layout/mobile-startup-progress.spec.ts`.
No navigation, controls, scroll ownership, or breakpoint changes are necessary.
The value and unit must remain visible without document overflow.
The exact sample value and spacing are illustrative.
AC-PLATFORM-STARTUP-PROGRESS-003.14 through 003.16 define the required behavior.

Terminal example for a measured byte rate of `1048576`:

```text
Database backup: 14.3 GB (1.0 MB/s)
```

## Tests

| Acceptance | Evidence |
| --- | --- |
| AC-PLATFORM-STARTUP-PROGRESS-003.14 | New `bytes_test.go`: `TestFormatBytes`, `TestFormatByteRate`. Add `TestFormatStartupLineByteUnits`, `TestBuildStartupPageViewCountingBytes`, `TestBuildStartupPageViewCountedBytesFormatsDoneAndTotalIndependently`, and `TestBuildStartupPageViewByteProgressIsLocalized`. Extend web `formatStepProgress` cases. |
| AC-PLATFORM-STARTUP-PROGRESS-003.15 | `TestFormatStartupLineByteUnits` covers byte rates, zero rates, and omitted rates; `TestFormatStartupLineNonByteUnitsStayRaw` covers non-byte rates. |
| AC-PLATFORM-STARTUP-PROGRESS-003.16 | Desktop and mobile E2E cover the real initial page, poll updates, and React restart progress. |
| AC-PLATFORM-STARTUP-PROGRESS-001.5, 001.6, 002.4, 003.8 | Keep opaque and counting semantics, raw snapshot fields, non-byte plurals, and locale completeness under regression coverage. |

Numeric cases include `0`, `1`, `1023`, `1024`, `1280`, `1048575`,
`1048576`, `15389753344`, `1099511627776`, a value beyond one TB, and
`4504644163416883`, which sits just below a decimal midpoint and must stay
`4096.9 TB` on every surface.
`1280` must show `1.3 KB`, including in Go.
Rates retain one decimal for B/s, including a fractional rate such as `0.5 B/s`.

## E2E tests

Extend `apps/web/e2e/tests/layout/startup-page.spec.ts` in project `chromium`.
Gate the first `/ready` response until the initial HTML assertion completes.
Then advance from `1.0 MB` to `14.3 GB` through the actual poll script.
Finally advance to the large midpoint case and assert `4096.9 TB`, so the
embedded renderer proves the same rounding as Go and the web helper.
Keep the counting step free of a progress bar.

Extend `apps/web/e2e/tests/layout/mobile-startup-progress.spec.ts` in project `mobile-chrome`.
Exercise byte progress in both the startup page and existing restart dialog.
Assert the formatted values, poll updates, and absence of horizontal overflow.
These flows cover AC-PLATFORM-STARTUP-PROGRESS-003.14 and 003.16.

## Work orders

- [x] [Task 01: Format startup byte counts and rates](task-01-format-byte-progress.md)

Execution is sequential in the primary session after an explicit implementation request.
No subagents are authorized.

## Verification results

Design-package checks passed on 2026-11-10:

- `python3 scripts/list-docs.py validate`: 446 decisions and 1963 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` API: references covered, with no errors.
  This preflight used the four package documents and the planned launcher path.
- `git diff --check -- docs/specs docs/plans/startup-readable-units`: passed.
- Plan/work-order whitespace, final newlines, and reference inspection: passed.

Implementation checks passed on 2026-11-10:

- Focused Go tests: `internal/startup`, `internal/launcher -run 'TestFormatStartupLine'`,
  the focused `internal/backendapp` set, and `internal/i18n` all passed.
- `vitest run lib/startup-progress/format.test.ts components/settings/system/restart-progress-dialog.test.tsx`:
  39 tests passed.
- `pnpm run i18n:check`, `pnpm run typecheck`, and focused `oxlint --deny-warnings`: passed.
- `bash scripts/lint-go-changed` over every changed Go file: 0 issues.
- Playwright `chromium` `tests/layout/startup-page.spec.ts` (2 tests) and `mobile-chrome`
  `tests/layout/mobile-startup-progress.spec.ts` (2 tests): passed. A capture run produced
  `restart-byte-progress-mobile` and `startup-byte-progress-mobile`; both match UI-01, keep the
  value and unit visible on a phone viewport, and show no horizontal overflow or progress bar.
- Public-doc and specification gates: 63 doc-validator tests, 55 published pages,
  446 decisions, 1963 specifications, 36 spec-linter tests, and `git diff --check` passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage` covered the final changed paths
  through the task 01 work order.

Review correction (2026-11-10): the first implementation rounded with
`floor(value*10+0.5)/10`, which double-rounds values just below a midpoint. Go now rounds
the exact double through `math/big.Rat` and the embedded script uses `toFixed(1)`, so
`4504644163416883` renders `4096.9 TB` on every surface. Go and the web helper agreed on
5,011 sampled values, and the chromium E2E poll now asserts `4096.9 TB`.

## Risks

- Go and JavaScript rounding can differ at ties. The shared numeric cases prove agreement.
- Updating only initial HTML leaves live updates inconsistent. E2E must exercise the poll script.
- New templates require matching backend, embedded-page, web, and generated locale keys.
