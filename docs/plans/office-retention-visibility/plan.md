---
created: 2026-10-28
status: implemented
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
system_design:
  - ../../specs/system-page/system-design/system-data-storage-pages.md
legacy_specs: []
---

# Implementation Plan: Office retention visibility

## Overview

Hide Office retention when the effective Office feature is disabled.
One sequential work order covers Storage, settings discovery, regression tests, and the operations guide.
The existing system-page requirement/design pair owns this behavior.
The work order is complete.

## Evidence and root cause

Source inspection confirms that `StorageSettings` always includes the retention tab and panel.
It does not read `useFeature("office")`, which existing Office surfaces use.
The retention discovery entry also lacks an availability gate.
`useSettingsTab` accepts a recognized target fragment before validating the query against available tabs.
Removing only the tab would therefore leave a fragment path to unavailable content.

Minimal reproduction: disable Office and visit `/settings/system/storage`.
The current tab list includes Office retention regardless of the feature value.
Then visit `/settings/system/storage?tab=office-retention#setting-system-retention`.
The current composition selects and mounts retention.
These findings come from a read-only source trace. The component regression produced eight expected failures across Storage and discovery before the correction.
The browser suites now verify both availability states.

## Scope

Include the effective Office gate, retained Host access, stale-link fallback, and discovery consistency.
Preserve enabled Office behavior and existing role gates.
Exclude backend retention operations, runtime flag registration/defaults, agent-profile settings, and visual redesign.

## Technical approach

Follow the design's **Office availability** section.
Filter the retention tab, target mapping, and mounted panel together in `StorageSettings`.
Use the existing discovery `requires` mechanism with an Office context value.
Keep the shared tab hook unchanged unless regression evidence proves a separate correction is necessary.
Add the availability note to `docs/public/operations.md` when implementation ships.
The historical `settings-storage-tabs` package remains the record of the original delivery.
This package owns the new availability cases and their validation.

## UI preview

UI-01: Settings > System > Storage, Office disabled.
The table is a structural preview, not a rendered verification result.

| Surface | Before | Proposed |
| --- | --- | --- |
| Desktop header | Host and Office retention | Host only |
| Phone header below description | Host and Office retention | Host only |
| Disabled retention deep link | Retention panel | Host panel |
| Office enabled | Both tabs | Both tabs |

The existing Settings surface owns scrolling and safe-area clearance.
Reuse `settings-tabs.tsx` and `mobile-settings-header-tabs.spec.ts` as the closest shipped phone examples.
Inline tabs remain appropriate because they select the primary page content.
The existing Host actions and floating Save changes control remain the primary actions.
No extra surface, breakpoint, or animation is required.
This preview maps to AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.2/.6/.8.

## Tests

New `storage-settings.test.tsx` cases cover disabled/default, query, fragment, enabled, and enabled-to-disabled states.
Assert the selected Host panel and that the retention child never mounts when disabled.
Use the real tab hook so fragment precedence cannot be hidden by a mock.
The first regression, `hides Office retention when Office is disabled`, must fail before implementation.
Discovery catalog and hook tests cover false, absent, true, and reactive flag values.
These tests map to criteria 002.2/.6/.7/.8.

## E2E tests

Add desktop and phone Office availability cases to the existing settings-header-tabs suites.
Use isolated fixtures with explicit Office on/off configuration or a controlled boot payload.
Cover the default route, retention query, fragment-only target, and legacy retention destination.
Assert Host access, absent retention controls, and no retention requests while disabled.
Assert enabled tab selection and the existing enabled draft round trip still work.
Exercise settings search with Office disabled and enabled.
Phone checks include touch access and no document horizontal overflow.
Capture one disabled desktop view and one disabled phone view.

## Work orders

- [x] [Task 01: Gate Office retention surfaces](task-01-gate-office-retention.md)

## Verification results

Design validation passed on 2026-10-28:

- `python3 scripts/list-docs.py validate`: 414 decisions and 1812 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered, with no errors.
  The local preflight supplied the four package documents and simulated the planned Storage source change.
- `git diff --check`: passed. The two specifications are modified and the plan directory is untracked.

Implementation validation passed on 2026-10-29:

- Work-order Vitest command: 5 files, 33 tests passed.
- Settings-discovery regression audit: 7 files, 49 tests passed, including the catalog suite above.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192` after the default heap was exhausted.
- ESLint passed for all changed TypeScript files, including the shared browser helper.
- The localization ratchet passed.
- Managed Chromium suite: 11 passed. Managed mobile-chrome suite: 7 passed.
- Public-doc validators: 63 tests and 54 published pages passed.
- Specification catalog, specification lint, and diff checks passed.

The browser runner built the current backend, web assets, and fixture plugin.
The first run could not reach localhost because an inherited configuration selected another bind address.
A loopback override corrected that test-process setting.
Chromium then required Nix library directories in `LD_LIBRARY_PATH`.
Final runs used `KANDEV_SERVER_HOST=127.0.0.1` and those library paths, with `--no-build` reusing the fresh build.
No host configuration or application defaults changed.
The initial browser attempt failed in setup, so the component and discovery failures provide the RED evidence.
Screenshots cover disabled desktop and phone surfaces and were visually inspected.
The focused desktop recapture passed one test after the phone suite cleared the earlier artifact.
Local evidence is retained at `/tmp/kandev-office-retention-evidence-b756l24b/{desktop,mobile}.png`.
The final documentation-reference preflight returned `covered` with no errors.

## Risks

- A retention hash can bypass a tab-only filter.
- A visited panel remains mounted unless the composition removes it explicitly.
- Existing discovery tests may assume every catalog entry is always visible.
- Retention E2E cases must set Office availability explicitly instead of relying on profile defaults.
