---
created: 2026-10-06
status: complete
requirements:
  - REQ-UI-SIDEBAR-TITLE-OVERFLOW-001
system_design:
  - ../../specs/ui/system-design/sidebar-title-overflow.md
legacy_specs: []
---

# Implementation Plan: Sidebar Title Overflow

## Overview

Deliver the title fade and its focused tests in one sequential work order.
UI owns this independent clipping presentation. No task state, API, persistence, or configuration changes are required.
The user's Claude screenshot supplies the visual direction. A 16px fade is the proposed starting value.

## Scope

### In scope

- Conditional fade on overflowing sidebar titles, including nested task rows.
- Existing hover-scroll disclosure and complete DOM text.
- Shared phone rows, themes, resizing, and adjacent badge geometry.

### Out of scope

- New disclosure controls, preferences, dependencies, or backend work.
- Other title surfaces and global changes to `ScrollOnOverflow`.

## Technical approach

Use the existing clipping hook in `TaskItemTitle`, with its forwarded ref on `ScrollOnOverflow`.
Apply a scoped alpha mask in `app/globals.css`, conditional on clipping and disabled during fine-pointer title hover.
Keep title flex sizing, row spacing, and all badges outside the mask.
Reuse the hook's observer rather than introducing a new overflow mechanism.

## ASCII UI preview

```text
UI-01 Desktop sidebar, idle rows
Before: [state] Investigate fresh session failu| [PR] [7s]
After:  [state] Investigate fresh session fa~~~| [PR] [7s]
Short:  [state] Short title [PR]                   [7s]
Hover:  [state] ...remaining title ending      | [PR] [...]

UI-02 Phone task picker, existing inset drawer
+--------------------------------------------------+
| Tasks                                            |
| [state] Investigate fresh sess~~~| [7s] [actions] |
| [state] Short title               [7s] [actions] |
|              vertically scrolling rows           |
+--------------------------------------------------+
```

`~~~` illustrates fading text and is not rendered copy. The pipe marks the title boundary, not a divider.
Required structure: only title text fades, short titles stay opaque, and badges remain outside the mask.
Phone composition retains the existing picker, safe-area handling, list scroll owner, and visible touch actions.
Spacing and text are illustrative. No new localized copy is required.
Both views map to AC-UI-SIDEBAR-TITLE-OVERFLOW-001.1 through .6.

## Tests

Add `components/task/task-item-title-fade.test.tsx` using the existing `TaskItem` render pattern and captured resize callbacks.
Cover clipped/fitting geometry, resize removal, title-change recomputation, and full DOM text (.1/.2/.5).
Run `hooks/use-is-title-truncated.test.tsx` alongside it.
Do not substitute class-string assertions for browser mask and geometry checks.

## E2E tests

- Add `e2e/tests/task/sidebar-title-overflow.spec.ts` in `chromium`: long and short titles, nested rows, resize, hover scrolling and reset, light/dark selected/default rows (.1-.6).
- Add `e2e/tests/task/mobile-sidebar-title-overflow.spec.ts` in `mobile-chrome`: open the task picker, inspect masks, verify visible 44px actions and containment, then tap a long-title row and verify navigation (.1-.3/.5/.6).
- Cover 767px and 768px fine-pointer widths in the desktop spec to verify the phone/desktop entry-point boundary.
- Run existing `sidebar-title-width.spec.ts` to guard adjacent badge and trailing-column geometry (.3).
- Use isolated fixtures, causal waits, computed mask styles, geometry, and screenshots. Compare screenshots with UI-01/UI-02.

## Work orders

- [x] [Task 01: Fade overflowing sidebar titles](task-01-fade-sidebar-titles.md)

## Verification results

Design checks passed on 2026-10-06:

- `python3 scripts/list-docs.py validate`: 357 decisions and 1408 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: planned runtime coverage passed with all four artifact references.
- Catalog discovery found both new specifications. Diff checks passed and status confirmed the untracked design package.

Implementation and rendered desktop/phone checks passed on 2026-10-06:

- Focused component and hook tests: 2 files and 8 tests passed.
- Scoped ESLint and `pnpm run typecheck` passed.
- Managed Chromium run for the new overflow spec and existing title-width spec: 3 tests passed.
- Managed Pixel 5 run for the mobile picker spec: 1 test passed.
- Captured and inspected light/dark default and hover states, plus the phone picker, under `apps/web/.pr-assets/`.

## Documentation impact

Public docs require no change for this design package. No commands, labels, settings, or workflows change.
No architecture decision is needed for this local presentation choice.

## Risks

- The outer span must report the inner title's horizontal overflow in the real flex layout.
- A mask left active during scrolling can hide the ending. Browser coverage must verify mask removal on hover.
- Fade width requires visual inspection at narrow widths and with CJK titles.
