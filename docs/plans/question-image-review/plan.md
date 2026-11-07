---
created: 2026-11-07
status: implemented
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-001
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-002
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-003
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-006
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-007
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-008
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
legacy_specs: []
---

# Implementation plan: Question image review

## Overview

Add an opt-in large review view for pending image-bearing questions, from
[issue #5135](https://github.com/kdlbs/kandev/issues/5135). The stage shows the
active question's images above the existing answer form. The operator compares a
baseline with candidates, answers, and submits once.

Read the [requirements](../../specs/tasks/requirements/question-image-review.md)
and the [system design](../../specs/tasks/system-design/question-image-review.md).
The [question image previews package](../question-image-previews/plan.md) is the
foundation and stays authoritative for references, reads, and limits.

The user confirmed these choices in task 14394d7c: an opt-in Review control, all
five comparison modes, a bundle limit of about 16 images, and annotations
specified now but delivered last. That task's first design package was never
committed and was lost with its worktree. This package recreates it.

## Work orders

| Wave | Work order | Depends on | Result |
| --- | --- | --- | --- |
| 1 | [01 Bundle gallery capacity](task-01-bundle-gallery-capacity.md) | none | Bundles accept 16 distinct images. Schema and docs match. |
| 1 | [02 Review surface](task-02-review-surface.md) | none | Opt-in view with shared form, segments, one-image stage, zoom, and pan. |
| 2 | [03 Comparison modes](task-03-compare-modes.md) | 02 | Toggle, Wipe, Side by side, Onion skin, and Difference. |
| 3 | [04 Annotation contract](task-04-annotation-contract.md) | none technically; scheduled after 03 | Answers carry validated regions to persistence and the agent. |
| 4 | [05 Annotation interface](task-05-annotation-interface.md) | 03, 04 | Draw, edit, list, submit, and view regions in history. |

Wave 1 work orders touch separate files and can run in either order. Work
order 04 is backend-only and could start early, but the user asked for
annotations last.

## ASCII UI preview

Structural requirements: the Review control's position in the panel actions, the
stage above the form, the segment strip, the compare toolbar above the stage,
and the separately scrolling form on phones. Spacing, icons, and exact copy are
illustrative. Use existing `@kandev/ui` primitives and localized copy.

### UI-01: Review control in the compact panel

Entry: a pending bundle with at least one question or choice image. Maps to
AC-001.1 and AC-001.2.

```text
+-------------------------------------------------------------------------+
| Question 1 of 4  · Header       [Review images] [Skip] [Collapse] [^]   |
|-------------------------------------------------------------------------|
| Is the new spacing in the settings header acceptable?                   |
| [thumb: Before]  [thumb: After]                                         |
| (1) Approve   (2) Revise   (3) Revert                                   |
| [custom answer.........................................] [clip] [Send] |
+-------------------------------------------------------------------------+
```

While the view is open, the compact panel body is replaced by one status row:

```text
| Reviewing in the large view.                                            |
```

### UI-02: Desktop review view, Side by side

Fixed: header, toolbar. Resizable: the split between stage and form. Scrolling:
the form panel only. Maps to AC-002.1, AC-002.2, AC-003.1, AC-004.1, AC-005.4.

```text
+--------------------------------------------------------------------------+
| Review images  [1 Header ✓] [2 Spacing] [3 Colors] [4 Copy]      [Close] |
| [Toggle|Wipe|Side|Onion|Diff]  Image A [1|2]  Image B [1|2]  [Swap]      |
|                 [-] 100% [+] [Fit] [Actual size] A 1440×900  B 1440×912    |
|------------------------------------+-------------------------------------|
|  A: Before                         |  B: After                           |
|                                    |                                     |
|   (zoom and pan stay in sync)      |   (zoom and pan stay in sync)       |
|                                    |                                     |
|============================ drag to resize ==============================|
| Question 2 of 4 · Spacing                                                |
| Is the new spacing in the settings header acceptable?                    |
| (1) Approve   (2) Revise  [Image 2]   (3) Revert                          |
| [custom answer.........................................] [clip]          |
| [< Prev]                                         [Next >] / [Submit]     |
+--------------------------------------------------------------------------+
```

### UI-03: Phone review view

Fixed: header and stage, 55% of the dynamic viewport at most. Scrolling: form.
Every control is at least 44px. Maps to AC-002.3, AC-005.4.

```text
+-------------------------------+
| [Close]   Review 2 of 4       |
| [1 ✓][2*][3][4]   (scrolls)   |
| [Toggle|Wipe|Side|Onion|Diff] |
| A [1|2]  B [1|2]  [Swap]      |
| [-] 100% [+] [Fit] [Flip A/B] |
|                               |
|   stage: pinch and drag       |
|   "Showing A: Before"         |
|                               |
|-------------------------------|
| Is the new spacing ...        |  <- scrolls separately
| (1) Approve                   |
| (2) Revise                    |
| [custom answer.............]  |
| [< Prev]            [Next >]  |
+-------------------------------+
```

Side by side on phones stacks A above B inside the stage.

### UI-04: Difference and annotation states

Difference toolbar row and stage, maps to AC-005.6, AC-005.8, AC-005.9:

```text
| [Toggle|Wipe|Side|Onion|Diff*]  Tolerance [====o-----] 16              |
| 3,214 pixels changed (0.25%)  ·  Sizes differ: A 1440×900, B 1440×912  |
|  stage: A with changed pixels highlighted                              |
```

Unavailable difference: `Difference is not available for these images on this device.`

Annotate mode in Toggle, with the region list in the form, maps to AC-007.1,
AC-007.7:

```text
|  stage: B shown   +--[1]-------+                                       |
|                   |            |        [Annotate*] [Delete region]    |
|                   +------------+                                       |
|------------------------------------------------------------------------|
| Regions (1 of 8)                                                       |
|  1  B: After  [Label contrast too low.............] 22/500  [Delete]   |
```

History shows `Region 1 on After: Label contrast too low` under the answer.

## Verification strategy

Each work order lists its exact commands. In summary:

- Backend: `go test` for `./internal/clarification/...`, `./internal/mcp/server`,
  and `./internal/orchestrator` focused tests.
- Web unit: `vitest run` on `lib/question-review` and
  `components/task/chat/question-review`, plus touched overlay tests.
- Web static: `pnpm run typecheck`, Oxlint on changed files,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- E2E: `question-image-review.spec.ts` on `chromium` and
  `mobile-question-image-review.spec.ts` on `mobile-chrome`, through `pnpm e2e:run`.

User-facing requirements get end-to-end evidence in work orders 02, 03, and 05.

## Risks

- `clarification-input-overlay.tsx` is above the 600-line limit. Add only hook
  calls and slot wiring there. Put logic in `question-review/`.
- Shortcut collisions: digits and arrows are carousel keys. Stage keys must stop
  propagation. Tests must prove stage keys never select, navigate, or submit.
- Difference memory: two 16-megapixel images need about 192 MB in the worker.
  Measure on the `mobile-chrome` emulation and a real phone if available. Show the
  unavailable state rather than downscale.
- `react-resizable-panels` inside a dialog: confirm keyboard resizing and that the
  form panel scrolls on short desktop viewports.
- Annotation coordinates depend on the image revision loaded at drawing time.
  The observed size lets the agent detect a later change.

## Exclusions

The review view for answered history, freehand shapes, annotations outside the
single-image and Toggle views, server-side comparison, more than four images per
question, images on parent or native provider questions, and draft persistence
across reloads.
