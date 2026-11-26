---
created: 2026-11-23
status: implemented
requirements:
  - REQ-UI-WALKTHROUGH-EDITOR-REVEAL-001
system_design:
  - ../../specs/ui/system-design/walkthrough-editor-reveal.md
legacy_specs: []
---

# Fix Plan for First Walkthrough Step Scrolling

## Overview

The user reports that opening a walkthrough on desktop opens the first file
in Monaco and highlights the correct range. The viewport stays elsewhere.
Selecting Next and Previous reveals the range.

## Technical approach

`useMonacoWalkthroughRange` records a reveal as complete when it calls
`revealLinesInCenter`. Its layout callback only refreshes marker geometry.
The generic pending cursor also requests a scroll during file opening.
Verify which initialization event leaves the range offscreen. Then adjust the
Monaco walkthrough reveal in the existing hook. Keep the correction specific
to the active reveal and preserve ordinary editor cursor navigation.

## ASCII UI preview

**UI-01. Desktop walkthrough launcher, first step open**

```text
Reported                       Expected
+ Editor ------------------+   + Editor ------------------+
| 1  File start           |   | 152  Context             |
| ...                     |   | 153  Highlighted line    |
|             [Step 1]    |   | 154  Highlighted line    |
+-------------------------+   |             [Step 1]    |
                              +-------------------------+
```

The editor owns the scroll. The floating card remains available with its
existing controls. The line numbers are illustrative. The repair targets
desktop Monaco. A phone smoke check covers the shared Monaco reveal path in
the existing phone editor and bottom sheet.

## Tests

- Hook regression verifies a reveal that starts before editor readiness and
  confirms it runs once the same model has a usable viewport.
- Chromium regression opens a walkthrough whose first range is below the
  initial viewport. Verify editor geometry before touching the step controls.
  Then check the return to step 1 and ordinary manual scrolling.
- Mobile Chromium smoke check confirms the shared editor still opens the file
  and renders the highlight behind the existing sheet.

## Work orders

- [x] [Task 01. Reveal the first Monaco walkthrough range](task-01-reveal-first-range.md)

## Verification results

The hook regression failed before the fix. A reveal attempted with no usable
viewport did not repeat when Monaco reported its first layout. After the fix,
all 11 hook tests passed. Two desktop Chromium tests and one Mobile Chromium
smoke test passed. Web type checking and focused lint passed.

Earlier browser probes did not reproduce the trigger in the reported session.
The hook regression isolates the layout failure, and the browser tests verify
the first-open outcome.

## Risks

- A later viewport callback can override manual scrolling or reveal a stale
  step. Scope each pending reveal to the current target and complete it once.
- A marker can exist outside the scrollable viewport. Browser assertions must
  check the range against the active editor's bounds.
