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

This presentation contract is owned by UI; Tasks continues to own movement,
eligibility, preview, and options. The phone screenshot published for PR #4372
showed the workflow action below the transcript utility icons. The 2026-10-14
user correction places it above those icons while retaining right alignment.

`ChatStatusBarActions` in
`apps/web/components/task/chat/chat-status-bar.tsx` previously rendered
`ChatStatusBarRightControls` before `WorkflowMoveProceedButton`. Both wrapping
containers became `display: contents` below 768px, placing their children in
the status tray's flex flow. An auto margin aligned the trailing action right;
it could not place it on a preceding row.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-COMPOSER-ACTION-WRAP-001 | Responsive action group; Mobile contract; Verification |

## Responsive action group

Use `useResponsiveBreakpoint().isMobile`, whose boundary is 768px, to select
the action/control order. Render one keyed workflow button and one keyed utility
group under the same action-group parent. Phones place the button first; wider
viewports retain utilities first. Keep component identity during resize and do
not mount duplicate mobile/desktop workflow buttons or stateful disclosures.

On phones the action group occupies the available tray width and stacks the
workflow button above the utility group. Align the button to the content right
edge and constrain it to the available width. Its row is present whenever it is
eligible, even with sparse controls or a short label. The utility row spans the
available width and permits complete-control wrapping. It must not become a
shrink-wrapped, non-wrapping cluster. Keep `TranscriptNavGroup`'s flattened phone
subgroup and the utilities' existing relative order. Hide an empty utility group
and allocate no placeholder for an ineligible workflow action.

Above the phone boundary retain the bounded, end-justified wrapping action
group and compact inline order. Existing fine-pointer desktop sizing remains
24px; phone and coarse-pointer minimums remain 44px in both active dimensions.
Keep contained label wrapping and pointer-specific disclosure behavior.

## Mobile contract

- Entry: phone task Chat, in the persistent tray above the composer input.
- Exemplar: the shipped [mobile task action bar](mobile-task-action-bar.md)
  contributes readable notices, complete-control wrapping, and reachable
  utilities. `PassthroughStatusRow` supplies bounded end justification.
- Hierarchy: existing notices/status information, right-aligned next-step row,
  transcript utility row, then composer input. Utility rows can wrap downward.
- Surface: inline tray, because progression and transcript navigation are
  frequent actions. No additional overlay is needed to locate the action.
- Scroll and viewport: retain the transcript as the scroll owner and existing
  composer/safe-area placement. Add no fixed positioning or nested scroller.
- Touch and state: preserve shared handlers, gates, options, and disclosures.
  The phone workflow action stays at least 44px with either pointer. Keyboard
  traversal follows the visible order in each responsive composition.
- Desktop: retain compact controls, inline utility/action order when space
  permits, and right alignment when the narrower chat pane wraps.

## Verification

Revise `expectComposerActionLayout` in
`apps/web/e2e/tests/workflow/composer-action-layout-helpers.ts`. Below 768px,
compare the workflow button's bottom with the minimum top of all visible utility
buttons. Require the entire action to precede every utility; checking one icon
is insufficient. Compare its right bound with the toolbar content right bound
within one CSS pixel. Continue measuring actual utility bounds, not just wrapper
boxes, and verify containment, 44px hit dimensions, center-point reachability,
and document overflow. Replace the obsolete phone `display: contents` assertion
with rendered row geometry. Wider wrap cases retain the previous trailing-line
oracle; wide desktop retains single-line order and 24px height.

Cover 360px, 393px, 767px, 768px, and wide desktop using the existing mobile and
fine-pointer workflow move fixtures. Include dense controls, a longer fitting
label, and sparse controls with a short label. Tap/click the phone action and
verify the existing move request and destination. Check keyboard traversal and
resize across the boundary without duplicate actions or lost task/session state.
Retain meaningful component coverage for empty utility content, visibility gates,
and moving state, plus the landed action-bar browser scenarios for terminal
notices and 320px/coarse-tablet containment. Classes alone cannot prove row order.

## Compatibility and delivery

Preserve `shouldShowProceed`, props, test IDs, callbacks, preview/options
surfaces, and status/notice positions. No copy, API, storage, or localization
changes are needed. The passthrough composer retains its composition.

The revised design is implemented and locally verified through
[Task 02](../../../plans/mobile-composer-action-wrapping/task-02-place-action-above-utilities.md).
Task 01 records the completed earlier alignment correction. Fresh desktop,
mouse-phone, and touch-phone screenshots must replace the previous PR placement
evidence during PR delivery. Rendered validation is recorded in the work order.
