---
created: 2026-10-28
status: implemented
requirements:
  - REQ-TASKS-WALKTHROUGH-TIME-001
system_design:
  - ../../specs/tasks/system-design/walkthrough-generation-time.md
legacy_specs: []
---

# Implementation Plan: Walkthrough Generation Time

## Overview

Show the saved generation time in the in-app walkthrough card.
One work order delivers the display, localization, focused tests, and public documentation.
The user confirmed the in-app surface. PR webpages are excluded.

## Scope

### In scope

- The latest saved walkthrough time on desktop, phone, and the shared inline card.
- Localized absolute date and time, timezone on hover, and invalid-value handling.
- Replacement, reload, locale, and phone containment coverage.

### Out of scope

- Backend changes, relative timers, generation duration, and stale-code detection.
- External PR walkthroughs, launcher metadata, and chat result summaries.

## Technical approach

Use `TaskWalkthrough.updated_at`, which changes on replacement while `created_at` remains fixed.
Add `WalkthroughGenerationTime` inside the shared `StepHeader`, below its badge and progress text.
Use the existing `formatDateTime` helper and `diff` locale namespace.
The [system design](../../specs/tasks/system-design/walkthrough-generation-time.md) defines formatting and fallback behavior.
Document the meaning of the timestamp in `docs/public/sessions-and-review.md`, under Generate a walkthrough.
The public page update belongs to implementation, because this package describes planned behavior.

## ASCII UI preview

### UI-01: Open walkthrough metadata

Entry: open the existing walkthrough launcher. State: saved walkthrough with a valid generation time.
Desktop floating card and phone bottom sheet share this content order:

```text
[Walkthrough]  Step 1 of 5, Line 24
 Generated Oct 28, 2026, 10:42 AM      [X]
[Current step                         v]
```

On a narrow phone, the date line wraps within the existing bottom sheet.
The close target spans both text lines in a separate column. The visible date and time require no hover.
Hover text also includes the timezone. The timestamp aligns with the badge text's left edge.
The header and metadata remain above the existing scrolling content and navigation footer.
For invalid or missing timestamps, omit the Generated row without reserving space.

Required structure: metadata beneath progress within the header, before the step picker, fully visible or wrapped.
The example date, English wording, and spacing are illustrative.
Use existing tokens and translated copy, with no new animation or decorative icon.
This preview covers AC-TASKS-WALKTHROUGH-TIME-001.1, .3, .4, and .5.

## Tests

Add `components/diff/walkthrough-generation-time.test.tsx` for:

- `renders the saved time with a semantic datetime`: criterion .1.
- `updates the saved time on rerender`: criterion .2.
- `reformats the same instant after a language change`: criterion .3.
- `omits missing and invalid generation times`: criterion .5.

Use fixed input instants and an explicit test timezone. Check semantic output and the visible label.
For the locale assertion, compare at least English and Portuguese rendering.
Test failure paths without creating separate date parsing infrastructure.

## E2E tests

Extend `e2e/tests/review/walkthrough.spec.ts` in the `chromium` project.
Check that the displayed instant equals the persisted `updated_at`, survives navigation and reload,
and changes after the existing live walkthrough replacement flow. This covers criteria .1 and .2.
Make the original and replacement timestamps distinguishable without arbitrary sleeps.

Extend `e2e/tests/review/mobile-walkthrough.spec.ts` in the `mobile-chrome` project.
Assert the complete timestamp, close control, and navigation remain visible in the bottom sheet.
Check timestamp containment and zero document horizontal overflow, including Portuguese copy.
Capture the rendered phone result for comparison with UI-01. This covers criteria .3 and .4.

## Work orders

- [x] [Task 01: Show walkthrough generation time](task-01-show-generation-time.md)

## Verification results

Implementation complete. All task-defined checks passed on 2026-10-28.
The [work order results](task-01-show-generation-time.md#results) record exact commands and environment corrections.
Final evidence: 10 component tests, 10 desktop E2E tests, 10 phone E2E tests, typecheck, lint, translation checks, and documentation validation.
The phone landscape test drove the compact header composition shown in UI-01.
Desktop and phone captures match the revised preview.
The timezone and alignment refinement passed 10 component tests and three focused desktop/phone E2E tests.
The final badge-text alignment correction passed one desktop and two phone E2E tests on 2026-10-29.
Updated captures show the timezone omitted from the visible label and the timestamp aligned with the badge text, including its padding and border inset.

Design-package checks passed on 2026-10-28:

- `python3 scripts/list-docs.py validate`: 411 decisions and 1777 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/walkthrough-generation-time`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight: covered, with no errors.
  The preflight used the planned card source change to exercise all package references.
  It did not query GitHub or claim that implementation exists.
- Catalog discovery found both new task specifications. Git status shows all four package files as untracked.

## Risks

- `created_at` describes the first saved walkthrough and would misdate later replacements.
- Long localized dates can wrap on phones. Keep them outside the control row.
- A future edit API could change the meaning of `updated_at`. Revisit this contract if such an API is introduced.
