---
created: 2026-10-19
status: implemented
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-EXPANSION-001
  - REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001
system_design:
  - ../../specs/agents/system-design/image-preview-expansion.md
  - ../../specs/agents/system-design/image-preview-defaults.md
legacy_specs: []
---

# Implementation Plan: Image preview expansion

## Overview

Reuse the attachment image dialog in native rich-output file previews. The first
work order delivers the interaction, focused regressions, and public guidance.
Agents owns this package because the file-card contract serves both task and
Office transcripts. Existing disclosure plans remain historical evidence.

## Scope

Include ready image cards, keyboard and touch operation, dialog lifetime,
viewport containment, and attachment-viewer regression coverage if shared code
changes. Exclude file-editor viewers, new image formats, zoom tools, and backend
or catalog changes.

## Technical approach

Replace the plain image in `FilePreviewContent` with `ImagePreviewDialog`.
Preserve the source, label, thumbnail bounds, and drag behavior. Give the shared
dialog an explicit viewport-bounded width so centering cannot shrink it below
the thumbnail width. Keep workspace
loading and retained disclosure independent from temporary dialog state.

Reuse registry target keys for cleanup. Make only necessary shared viewer
adjustments for tiny-image hitboxes, close-button touch sizing, and reduced
motion. Do not change the global dialog primitive. Reuse existing translations.

Current v2 and historical v1 presentations use the same component. Missing
content retains the existing unavailable state. The supported image branch is
unchanged, including existing SVG/text encoding behavior.

## ASCII UI preview

UI-01: Completed rich-output image card, desktop and phone. Maps to AC .1-.5.

```text
Before: [shot.png] [Hide preview] [Open file]
        [ inline image, no expansion action ]
After:  [shot.png] [Hide preview] [Open file]
        [ inline image button: open image   ]

Phone:  [shot.png]
        [Hide preview] [Open file]
        [ inline image button ]

UI-02: Image dialog, shared composition
        +--------------------------+
        |                  [Close] |
        |                          |
        |   larger fitted image    |
        |                          |
        +--------------------------+
        Close -> original image trigger
```

The card remains in the transcript. The dialog locks background scrolling.
Phone actions use at least 44px targets. The dialog and image fit the dynamic
viewport. The structure and interaction are required. Spacing is illustrative.
Loading, unavailable, and text states keep their existing presentation.

UI-03: Settings > Preferences > Appearance, edited state. Covers AC .6.

```text
Appearance
  Color theme                  [selection]
  Animate rich-output charts   [switch]
  Chat animations              [switch]
  Show image previews by default [switch]
  Automatically show image file previews. Off starts them collapsed.
  Saved on this device.

Shared save controls: [Reset] [Save changes]
```

Desktop and phone use the same row order. On phones, description text wraps
within the page and the switch has a 44px touch target. The page owns scrolling;
the existing shared save controls retain their layout. With the preference off, fresh image cards start collapsed. Opening Preview
always reveals a button that opens the larger image. Ordering, labels, and interaction
are required; ASCII spacing is illustrative.

## Tests

Extend `rich-output-renderer-file-targets.test.tsx` for AC .1/.3/.4: both versions,
exact bytes, no extra read/callback, and removal on target replacement.
Use `image-preview-dialog.test.tsx` for AC .2 if shared viewer code changes.
Retain the existing view-state and workspace-preview suites as regression checks.

## E2E tests

Extend `e2e/tests/chat/rich-output.spec.ts` in chromium for AC .1-.4: click,
Enter, Space, Escape, outside dismissal, close, and focus return. Assert the
larger image uses the thumbnail source and the card remains expanded.

Extend `e2e/tests/chat/mobile-rich-output.spec.ts` in mobile-chrome for AC .1/.5:
tap to inspect and close, decoded image, viewport containment, touch hitboxes,
and restored transcript interaction. Capture the open phone dialog. Include
tiny and tall images and a narrow fine-pointer case. Check reduced motion if
shared dialog classes change. Scope selectors to the active card or portal.

## Work orders

- [x] [Task 01: Expand preview images](task-01-expand-preview-images.md)
- [x] [Task 02: Initial preference implementation (superseded)](task-02-configure-image-expansion.md)
- [x] [Task 03: Configure initial image disclosure](task-03-configure-initial-disclosure.md)

## Verification results

Design validation passed on 2026-10-19:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1618 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: prospective implementation coverage passed with all four package files and the planned renderer change.
- `git diff --check`: passed. `git status --short` confirms the four new package files are unstaged.

Implementation, final screenshot inspection, and lifecycle checks passed on
2026-10-20. The [work order](task-01-expand-preview-images.md#results)
records exact commands, logs, and environment setup.

- Targeted Vitest: 37 tests passed.
- Browser checks: six desktop and six phone tests passed after the final production edit.
- Typecheck, catalog drift, focused ESLint, and localization checks passed.
- Public documentation: 63 validator tests passed and 48 pages validated.
- The real phone regression detected a shared dialog-width defect. The corrected dialog enlarges the image within viewport bounds.

Public reference updated: `docs/public/automation-and-mcp.md`. Task 01 required no dependency, backend, catalog, or locale changes.
Task 02 adds translated preference copy and device-local persistence.

## Risks

- The semantic trigger makes its thumbnail decorative. Existing role selectors need updates.
- Shared viewer adjustments also affect attachment callers. Preserve their default behavior.
- Target replacement must remove the portal, not only replace the thumbnail.

## Configuration extension

The user requested a task appearance preference on 2026-10-20 during authorized
implementation. Add a default-on device-local switch to Appearance. Save applies
and persists the choice. Reset discards the draft. Follow task 02 for AC .6.

Configuration completed on 2026-10-20. Task 02 records 42 targeted tests, 27
settings-discovery tests, seven desktop and seven phone browser checks,
localization, static checks, public-doc validation, and the final focused
phone rerun after preserving image drag behavior. The phone settings screenshot
was inspected. Both work orders are done.

## Clarified preference scope

The user clarified that the setting controls initial inline disclosure, not
permission to enlarge an image. Task 03 supersedes the preference behavior in
task 02. Initial disclosure remains on by default, retained manual choices win,
and every visible image always opens the larger dialog. Task 02 results record
the earlier implementation only.

Task 03 completed on 2026-10-20. The final preference controls initial disclosure
only. Targeted unit, desktop, phone, static, translation, and documentation checks
passed. Fresh screenshots show the corrected setting and manual enlargement.
