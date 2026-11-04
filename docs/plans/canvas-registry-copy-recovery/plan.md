---
created: 2026-10-28
status: done
requirements:
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-006
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
legacy_specs: []
---

# Implementation plan: Canvas registry example copy recovery

## Overview

Repair the existing Share canvas registry example action in one sequential work
order. The host component directly awaits the modern clipboard API; rejection
escapes the click handler and bypasses its fallback. A rendered component probe
reported one causal failure and the same unhandled NotAllowedError, with modern
success and missing-API fallback controls passing. That probe used controlled
browser boundaries; it does not establish a live browser permission denial.

## Scope

In scope: recover modern clipboard rejection using the existing shared utility,
and verify actual example text and result feedback on desktop and phone.

Out of scope: registry publication, new UI copy, toast, timeout, layout or
navigation changes, runtime flags, global clipboard changes, and plugin runtime
or protocol changes.

## Technical approach

`apps/web/components/settings/canvas-share-help.tsx` passes its existing example
to `copyToClipboard` and sets copied state from the returned boolean, including
clearing a prior success on failure. The utility's fallback uses the active
dialog and restores focus. Preserve the example byte content, translations,
review metadata, and surrounding Share dialog. No new architecture decision is
needed for reuse of the existing host utility.

The existing CanvasShareDialog is a desktop dialog and phone full-height drawer;
How to share expands inline in its scroll body. The current mobile Canvas sharing
spec is the nearest shipped exemplar. Keep that composition and touch path;
this fix changes no visual geometry. The desktop and phone checks exercise the
same copy outcome through the actual host surface (AC-003.5, AC-006.1).

## Tests

Independently author `apps/web/components/settings/canvas-share-help-clipboard.test.tsx`
with real locales and the actual component. Before production changes, prove
modern rejection plus successful legacy copy fails for the behavior itself.
Cover modern success, missing modern API plus successful fallback, and both
paths failing (including after success); verify exact displayed text and no
unhandled rejection. Existing helper tests are supplementary, not a substitute
for the component regression.

## E2E tests

Extend `apps/web/e2e/tests/canvas/canvas-sharing.spec.ts` (chromium) and
`apps/web/e2e/tests/canvas/mobile-canvas-sharing.spec.ts` (mobile-chrome) for
AC-CANVASES-MARKETPLACE-003.5 and AC-CANVASES-MARKETPLACE-006.1. On an isolated
owned backend use the existing seed, promote, remove, feature-enable and restore
fixture. Open Share canvas, expand How to share, and copy via the real UI. Run a
positive control and inject rejection only at the modern Clipboard API boundary;
keep actual DOM fallback and verify actual clipboard text when supported. If
native clipboard access is unavailable, collect evidence and report the limit
before altering test scope. Controlled rejection is not a live policy denial.

## Work orders

- [x] [Task 01: Recover registry example copy](task-01-registry-copy.md)

## Verification results

The component regression, scoped lint, and managed desktop and phone sharing
checks passed. The work order records the pre-fix failure and test-navigation
correction. Catalog validation, specification lint, and diff checks passed.

## Risks

- Clipboard permissions and native fallback support vary by browser and host;
  distinguish an unavailable native clipboard from product failure.
- A copied label can become stale after a later failed attempt; cover that path.
- Existing public canvas sharing documentation describes the workflow, not the
  clipboard implementation. Reassess it and screenshots before finishing; no
  public copy change is expected for this restored action.
