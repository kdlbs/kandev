---
created: 2026-10-28
status: complete
requirements:
  - REQ-UI-DIALOG-CONTENT-CONTAINMENT-001
system_design:
  - ../../specs/ui/system-design/dialog-content-containment.md
legacy_specs: []
---

# Implementation Plan: Office Project Repository Picker Clipping

## Overview

[Issue #4894](https://github.com/kdlbs/kandev/issues/4894) reports a clipped
repository picker in Office's New Project dialog. Restore the floating picker
while retaining the bounded form body and persistent Cancel/Create Project
footer. One sequential work order owns the browser regressions and correction.

The implementation is complete. The issue is assigned to `carlosflorencio`.

## Requirements and ownership

The existing [dialog containment requirement](../../specs/ui/requirements/dialog-content-containment.md)
owns this reusable interaction contract. Office retains repository selection
semantics and project persistence. Reuse
`REQ-UI-DIALOG-CONTENT-CONTAINMENT-001` and its active acceptance criteria
`.1` through `.8`. This correction needs no new requirement.

The [system design](../../specs/ui/system-design/dialog-content-containment.md#office-create-project)
defines the dialog-local portal and bounded picker. This corrects an incomplete
technical design without changing a public API, persisted value, or ownership
boundary. No ADR is necessary.

## Confirmed root cause

`ProjectRepositoryPicker` sets `portal={false}` on its 420-pixel popover.
`CreateProjectDialog` applies `overflow-clip` to the enclosing dialog shell.
Repository chips move the trigger within its wrapping row. The anchored picker
then extends beyond the shell, which clips both paint and pointer interaction.
The fixed width also exceeds a narrow phone viewport.

The previous containment package deliberately introduced the shell clipping.
Removing form containment would reintroduce displaced footer controls. A
document-body portal alone also needs modal focus and scroll-lock validation.
The shipped Task dialog supplies the safer local pattern: portal inside the
dialog shell, outside its scrolling form, with visible shell overflow.

## Investigation evidence

Source baseline: `a33ff4139ada169271a8177fa181c45b0c0176e7`.
The isolated dev instance used an empty database and mocked providers. Browser
discovery responses supplied 18 repository suggestions to the real picker.

| Probe | Observed result |
| --- | --- |
| Desktop, 1280 x 720, one selected repository | Picker spans x=713..1133. The dialog ends at x=896. The first option's right-side point hits `dialog-overlay`. |
| Temporary desktop dialog-local relocation and visible shell | The same point reaches an option. Wheel input advances the list to 300px. Document horizontal overflow remains zero. |
| Desktop dismissal after restoring temporary DOM | Escape closes the picker, preserves the project dialog, and returns focus to Add repository. |
| Touch phone, 393 x 851 | The dialog is 361px wide. The picker remains 420px wide and extends beyond the viewport. |
| Temporary phone width cap and visible shell | Picker width becomes 361px. Right-side hit testing succeeds. Trigger/search heights are 44px, touch selection adds a chip, and document horizontal overflow remains zero. |

These are diagnostic probes, not permanent TDD or production-build results.
Direct DOM relocation on the phone caused a React `removeChild` error during
an update. Reload cleared that diagnostic artifact. The implementation must
use the actual React portal and verify its complete lifecycle through E2E.

The smallest reliable desktop reproduction adds one custom repository, reopens
the picker with many suggestions, and hit-tests an option beyond the dialog's
right edge. The old visibility assertions pass because clipped nodes still
have layout boxes.

## Scope

### In scope

- Place the picker portal inside the dialog shell and outside its form body.
- Preserve form scrolling, compact intrinsic height, and the persistent footer.
- Bound picker width and height to the available viewport.
- Preserve desktop pointer, keyboard, wheel, and phone touch selection.
- Cover project-detail use, creation persistence, and existing long-chip flows.

### Out of scope

- Project APIs, repository value normalization, discovery ownership, and stores.
- Shared Dialog, Popover, Command, or task-dialog context changes.
- A new phone drawer, global overlay rewrite, new copy, or translation changes.
- Other Office forms and the other four dialog-containment consumers.

## Technical approach

1. Capture `DialogContent` with a callback ref in `CreateProjectDialog`.
2. Pass that element through `ProjectFormBody` and `ReposField` to an optional
   `ProjectRepositoryPicker.portalContainer` prop.
3. Use `PopoverContent`'s existing portal and container support. The standalone
   project-detail picker retains the default document-body target.
4. Change the project shell to visible overflow. Retain its height/width caps,
   grid rows, and the body's `min-h-0`, `min-w-0`, and overflow rules.
5. Retain the 420px preferred width. Add a viewport width cap and 16px Radix
   collision padding. Bound height with Radix's available-height value and
   permit the command list to shrink and scroll.
6. Keep existing callbacks, discovery timing, exclusions, query reset, and
   project submission unchanged. Use a stable picker selector if necessary.

The relevant files are `create-project-dialog.tsx`,
`project-repository-picker.tsx`, both existing Office picker E2E specs, and a
small shared E2E helper. No shared UI package change is required.

## Mobile design contract

- Entry: Office > Projects > New Project > Add repository.
- Presentation: retain the click/search combobox. This short, flat choice fits
  a viewport-bounded popover without an extra navigation step.
- Precedents: `TaskCreateDialog` supplies the modal-local portal.
  `RepositoryOptionsSurface` supplies the bounded desktop popover.
  The curated `mobile-picker-sheet.tsx` supplies the fixed-search/scrolling-list
  hierarchy without importing task-owned state into Office.
- Hierarchy: search first, custom entry and repository groups next. Selecting
  an option adds its chip and returns to the project form.
- Scroll ownership: only the command list scrolls within the picker. The form
  retains its separate scroll body. Opening the picker does not scroll the
  document or create a horizontal scroll container.
- Geometry: use available-height and viewport-width limits, a 16px collision
  inset, and existing phone/coarse-pointer targets of at least 44px.
  A shorter viewport reduces the list area. Existing input anti-zoom rules
  remain effective.
- State: both viewports share the existing query, discovery, filtering,
  exclusion, selection, and form state.
- Verification: Pixel 5 touch selection and creation persistence, plus a short
  320 x 568 viewport. Assert actual hit targets, search reachability, picker
  containment, and zero document horizontal overflow while the picker is open.

## ASCII UI preview

### UI-01: Desktop expanded picker

Entry: New Project, one repository chip selected, picker open.

```text
Before: dialog clips the part after its right border
+------------------------------+
| New Project                  |
| ...                          |
| [repo chip] [Add repository]  |
|             [search ...     X|  clipped region
|             [repository ... X|
| [Cancel]    [Create Project] |
+------------------------------+

After: picker floats beyond the dialog, within the viewport
+------------------------------+
| New Project                  |
| ...                          |
| [repo chip] [Add repository]  |
|             +-------------------------+
|             | Search or paste ...     |
|             | Repository options      |
|             | (list scrolls)          |
| [Cancel]    +-------------------------+
+------------------------------+
```

The header and footer retain their form layout. The picker can temporarily
cover underlying fields. Its border can extend beyond the dialog shell.
These structural rules map to `.1` through `.6` and `.8`. The drawing does not
specify placement above versus below the trigger.

### UI-02: Phone expanded picker

Entry and selection behavior match UI-01. The surface fits the phone width.

```text
+---------------------------+
| New Project               |
| (form body scrolls)       |
| [repo chip]               |
| [Add repository]          |
| +-----------------------+ |
| | Search or paste ...    | |
| | Repository options    | |
| | (list scrolls)        | |
| +-----------------------+ |
| [Cancel]                  |
| [Create Project]          |
+---------------------------+
```

The search control stays reachable. The list shrinks on shorter viewports.
Option and trigger targets remain at least 44px. The form footer remains in
its existing persistent row. These rules map to `.1`, `.3`, `.4`, `.7`, and
`.8`. Spacing, sample copy, and collision placement are illustrative.

## Tests

Browser layout is the regression-test level. A jsdom class or prop assertion
cannot prove clipping, wheel delivery, or touch reachability. No new pure
logic requires a separate unit test.

| Acceptance criteria | Evidence |
| --- | --- |
| `.1`, `.2`, `.3`, `.5`, `.6` | Retain both existing long-chip cases, their body scroll range, and persistent footer checks. |
| `.4` | New desktop clipping case: 420px width, out-of-dialog hit testing, actual pointer selection, and list wheel scrolling. |
| `.7` | New phone case: width/height bounds, 44px targets, touch selection, and no horizontal overflow with the picker open. |
| `.8` | New keyboard case: real opener, search focus, ArrowDown/Enter selection, Escape, outside dismissal, and focus return. |
| `.4`, `.8` | Existing project-detail path/custom/duplicate cases and creation persistence. A reload after creation establishes durable state. |

All abbreviated criteria above use `AC-UI-DIALOG-CONTENT-CONTAINMENT-001`.

## E2E tests

Extend `e2e/tests/office/project-repository-picker.spec.ts` in `chromium`:

- `create-project dialog: repository picker escapes the dialog without clipping`.
- `create-project dialog: repository picker scrolling and keyboard dismissal preserve the form`.

Extend `e2e/tests/office/mobile-project-create-dialog.spec.ts` in `mobile-chrome`:

- `keeps the repository picker within the phone viewport and persists selection`.

Retain every existing test in both files. Share discovery seeding and picker
geometry helpers in `e2e/helpers/project-repository-picker.ts`. Resolve the
active picker from its trigger or unique visible surface. Scope the parent
dialog by `create-project-dialog`, rather than a broad live dialog locator.

## Companion package

The [original plan](../dialog-content-containment/plan.md) and
[Task 05](../dialog-content-containment/task-05-create-project.md) retain their
completed historical results. Their follow-up notes point here. Only the
Office picker scope changes. The other four completed tasks remain unaffected.
The new work order owns all new RED/GREEN counts and final commands.

## Work orders

- [x] [Task 01: Restore the floating repository picker](task-01-unclip-repository-picker.md).

Execution is sequential. This package does not authorize delegation.

## Verification results

Design-package checks passed:

- `python3 scripts/list-docs.py validate`: 415 decisions and 1824 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Local `validateCoverage` preflight: actual documentation changes are exempt.
  projected production paths are covered by both linked work orders, with no
  reference errors.
- `git diff --check -- docs/specs docs/plans` and new-package status inventory.

Implementation results are recorded in
[Task 01](task-01-unclip-repository-picker.md#results). Browser evidence at
1280 x 720, 393 x 851, and 320 x 568 proves the picker paints and hit-tests
beyond the dialog shell, keeps the search control reachable while the list
scrolls, and persists the selected repository after creation and reload.

## Risks

- A body-level portal can conflict with modal focus or wheel delivery. Use the
  existing dialog-local container pattern and exercise both interactions.
- Removing the shell clip without retaining body containment can restore form
  overflow. Keep both original long-chip cases.
- A max-height alone can hide options. Keep the search control outside the
  shrinking list and verify the last option by actual scrolling.
- Direct DOM relocation is only diagnostic evidence. Permanent tests must
  mount and unmount the real React portal without console errors.

## Documentation impact

Internal design and implementation records change. Public instructions, labels,
screenshots, APIs, and configuration remain applicable. No public documentation
change is required for this repair.
