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

`ChatStatusBarActions` uses `ml-auto` on a nested wrapping flex container.
That aligns the container in its parent but does not right-align its wrapped
children. Its current default `justify-content` starts each internal line at
the left. The existing `PassthroughStatusRow` in `passthrough-toolbar.tsx`
demonstrates a constrained wrapping action group with `justify-end`.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-COMPOSER-ACTION-WRAP-001 | Action-group layout; Mobile contract; Verification |

## Action-group layout

Give `ChatStatusBarActions` end justification and constrain its maximum width
to its parent, following the passthrough group. Retain its `ml-auto`, `min-w-0`,
wrapping, item alignment, and existing gap. Keep `ChatStatusBarRightControls`
before `WorkflowMoveProceedButton` in DOM order. Each internal flex line then
ends against the action group's right edge, which the outer auto margin aligns
with the toolbar's right content edge.

Use the same end justification at every width. It does not change the
single-line order or control sizes; it also works in a narrow desktop pane
without JavaScript measurement, additional responsive state, or forcing every
phone toolbar into two rows. Leave the workflow button's touch sizing and
visibility rules with their existing owners.

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
- Touch and state: preserve existing phone/coarse-pointer hit targets and shared
  workflow handlers. Width-driven phone alignment also applies to fine pointers.
- Desktop: retain the current compact controls and single-line placement when
  there is enough space.

## Verification

Rendered Playwright geometry is the regression evidence. With real transcript
controls present, first prove that the workflow action occupies a later line,
then compare its right bound to the toolbar's content right bound within one
CSS pixel. Check its full hitbox inside the row, touch dimensions, and document
overflow. Activate it and verify the destination step via the existing fixture.

Cover the configured phone viewport, 360px, and the 767/768px boundary; add a
narrow fine-pointer case and a wide desktop single-line case. Measure computed
end justification as well as geometry. Component tests retain the existing
no-content spacer case and move-gating coverage, but class assertions alone
cannot establish rendered wrapping.

## Compatibility

Preserve `shouldShowProceed`, all props, test IDs, callbacks, preview and options
surfaces, and status-chip positions. No localization work is needed because no
copy is added. The passthrough composer is a layout exemplar and remains outside
the implementation scope.
