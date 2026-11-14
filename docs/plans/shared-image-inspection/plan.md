---
created: 2026-10-28
status: implemented
requirements:
  - REQ-AGENTS-SHARED-IMAGE-INSPECTION-001
system_design:
  - ../../specs/agents/system-design/shared-image-inspection.md
legacy_specs: []
---

# Implementation plan: Shared image inspection

## Overview and scope

Add temporary actual-size inspection to the shared image viewer in one vertical
work order. Fit remains the default. Preserve file-card identity, authorization,
preview reads, attachment lifecycle, and the separate clarification viewer.
Pinch/drag zoom, downloads, galleries, editor redesign, formats, and preferences
are excluded. The existing implemented image-expansion and image-defaults
plans remain historical delivery records; this package extends their contract.

## Technical approach

`apps/web/components/task/chat/image-preview-dialog.tsx` supplies both the
ordinary dialog and `ImagePreviewContent`. The latter is also mounted in the
controlled composer dialog in `context-items/image-item.tsx`. Give both paths
the same temporary mode and source lifetime; explicitly handle composer focus
return. Keep sources and callbacks untouched. Use the existing translated
question-image labels and the question viewer's bounded scroll pattern without
changing its behavior. The native file-card's existing target key and read
ownership remain intact. The active file-card design has been reconciled to
allow this proposed opt-in mode while retaining initial fit.

## UI interaction preview

UI-01, desktop and phone: activating an image opens a modal with the fitted
image, visible Fit and Actual size controls, and Close. Selecting Actual size
keeps those controls fixed while the image region scrolls in both axes; Fit
returns the whole image. Escape or Close returns to the opener. On phone the
same single-image inspection fills the available bounded space, respects safe
areas, and has at least 44px touch targets. The question viewer is the closest
shipped inspection precedent; an existing modal is appropriate for this short
temporary task. Its image region is the only inspection scroll owner and the
background is locked. The action order and fixed controls are structural;
spacing is illustrative. This maps to AC .1-.4 and the desktop and mobile
rendered checks in Task 01.

## Tests and E2E

Component tests exercise both caller lifetimes, mode controls, source changes,
exact source, reopening, and noninteractive behavior. Existing rich-output
renderer tests cover v1/v2 and replacement/read boundaries; extend them for
switching mode and unchanged read counts. Desktop and phone browser scenarios
use a wide and tall deterministic image with dimensions larger than the
viewport, measure intrinsic-versus-rendered dimensions and both scroll axes,
and exercise keyboard/touch, containment, and dismissal. Cover the direct
composer path as well as a real rich-output or sent attachment path. Test
actual scrolling, not only CSS classes. The work order details the RED gate,
exact commands, and acceptance mapping.

## Work orders

- [x] [Task 01: Shared inspection](task-01-shared-inspection.md)

## Verification results

Task 01 records the focused RED, component and browser GREEN, documentation,
and localization evidence. Desktop and phone use the same built artifacts after
an input, tool, configuration, and artifact receipt comparison.

## Risks

Dialog content can remain mounted when closed; source and open transitions
must not leak mode. A composer without `DialogTrigger` may not restore focus.
Oversized images must scroll inside the modal without clipping fixed actions or
moving the document. Browser proof must use a fresh production build and a
real nontrivial image. Recheck newly opened PRs for semantic overlap before
publication; locale and documentation-only overlap requires scoped assessment.
