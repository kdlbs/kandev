---
status: current
system: ui
requirements:
  - REQ-UI-COMPOSER-ACTION-WRAP-001
created: 2026-10-09
owners:
  - Kandev
---

# Composer Action Wrapping System Design

## Boundary and evidence

This is a local responsive presentation adjustment in
`apps/web/components/task/chat/chat-status-bar.tsx`. No API, state, persistence,
or workflow authority changes are needed. The screenshot shows transcript icons
on the first line and Open PR left-aligned on the second line.

Before correction, `ChatStatusBarActions` used `ml-auto` on a nested wrapping flex container.
That aligned the container in its parent but did not right-align its wrapped
children. Default `justify-content` started each internal line at
the left. The existing `PassthroughStatusRow` in `passthrough-toolbar.tsx`
demonstrates a constrained wrapping action group with `justify-end`.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-COMPOSER-ACTION-WRAP-001 | Action-group layout; Mobile contract; Verification |

## Action-group layout

On desktop, `ChatStatusBarActions` uses bounded end justification, following the
passthrough group. Retain `ml-auto`, `min-w-0`, wrapping, item alignment, and the
existing gap. Each internal flex line ends against the action group's right
edge. Keep `ChatStatusBarRightControls` before `WorkflowMoveProceedButton` in DOM
order.

Below the canonical 768px boundary, preserve the landed
[mobile action bar composition](mobile-task-action-bar.md): action and utility
wrappers use `display: contents`, allowing whole controls to wrap directly in
the toolbar. The workflow button's phone auto margin keeps it at the trailing
edge. Do not restore a shrink-wrapped phone utility group. Its label may wrap
inside the contained button, which retains 44px minimum height and width with
either pointer. Fine-pointer desktop retains its 24px height. Coarse-pointer
label wrapping and existing disclosure/visibility rules remain with their owners.

## Mobile contract

- Entry: phone task Chat, in the persistent status row above the composer.
- Exemplar: the shipped passthrough composer action group contributes bounded
  wrapping and end justification. Existing phone task Chat owns overall layout.
- Hierarchy: status information, transcript utilities, then the next-step action.
  The workflow action remains inline and visible when it wraps.
- Surface: inline toolbar, because advancement is a frequent action with no
  additional content needed to locate it. This adjustment needs no overlay.
- Scroll and viewport: retain the transcript as the scroll owner and the existing
  composer/safe-area placement. Add no fixed positioning or nested scroller.
- Touch and state: phone hit targets meet the 44px minimum with either pointer;
  existing coarse-pointer targets and shared workflow handlers are preserved.
- Desktop: retain the current compact controls and single-line placement when
  there is enough space.

## Verification

Rendered Playwright geometry is the regression evidence. With real transcript
controls present, first prove that the workflow action occupies a later line,
then compare its right bound to the toolbar's content right bound within one
CSS pixel. Check its full hitbox inside the row, touch dimensions, and document
overflow. At phone widths, verify the button center is a reachable hit target
with either pointer. Activate it and verify the destination step via the existing fixture.

Cover the configured phone viewport, 360px, and the 767/768px boundary; add a
narrow fine-pointer case with a 44px target and click, and desktop cases with a
24px height and wide single-line order. Measure computed
phone `display: contents` and desktop end justification as well as geometry.
Measure the union of visible utility-control bounds because a phone wrapper
does not generate a box. Component tests retain the existing
no-content spacer case and move-gating coverage, but class assertions alone
cannot establish rendered wrapping.

## Compatibility

Preserve `shouldShowProceed`, all props, test IDs, callbacks, preview and options
surfaces, and status-chip positions. No localization work is needed because no
copy is added. The passthrough composer is a layout exemplar and remains outside
the implementation scope.
