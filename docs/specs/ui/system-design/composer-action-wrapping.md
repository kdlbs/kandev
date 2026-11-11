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

Before Task 03, the phone wrappers forced both actions and utilities to full
width. This prevented status icons from sharing a line with utilities, even
when all targets fit. Task 03 revises containment, not workflow eligibility.

Compose status items, the workflow action, and utilities under one stable layout
owner in `ChatStatusBar`. Keep keyed instances across responsive ordering.
On phones place the workflow action first in its own right-aligned row.
Place status items and utilities in the next wrapping flow. On wider screens
retain status, utilities, and workflow order. Extract a local presentation
component if needed to keep `ChatStatusBar` within repository size limits.

Give the phone status group its natural width, with bounded internal wrapping.
Give the utility group natural width, a trailing auto margin, and bounded
internal wrapping. Remove unconditional phone full-width allocation from these
groups. Let CSS flex wrapping select the shared or wrapped row from available
width and actual content. Do not add a width observer or count-based breakpoint.
Keep existing gaps and target dimensions. Long status content must wrap within
the tray. Empty wrappers must not create a gap or consume a line.
The status wrapper uses a trailing auto margin and hides when its only
remaining children are empty provider wrappers. The utility wrapper uses a
leading auto margin. Both groups retain their natural widths when they fit.

`ChangeRequestStatusChip`, the GitLab MR trigger attributes, and the Azure DevOps
PR link expose the same component slot on their triggers.
Non-standalone `ComposerCIStatus` uses that slot to apply 44px minimum targets
for phone widths and coarse pointers. This fixes the measured 40px CI target
without changing standalone Threads chips or fine-pointer desktop density.

Keep `TranscriptNavGroup`'s flattened phone subgroup and utility order.
The workflow action reserves its own row only when eligible. A moving action
keeps that row. Terminal banners retain their separate full-width leading
region. Use DOM order that matches visible order, including keyboard traversal.
Do not duplicate stateful controls or reparent them on viewport changes.

Above the phone boundary retain the bounded, end-justified wrapping action
group and compact inline order. Existing fine-pointer desktop sizing remains
24px; phone and coarse-pointer minimums remain 44px in both active dimensions.
Keep contained label wrapping and pointer-specific disclosure behavior.

## Mobile contract

- Entry: phone task Chat, in the persistent tray above the composer input.
- Exemplar: the shipped [mobile task action bar](mobile-task-action-bar.md)
  contributes readable notices, complete-control wrapping, and reachable
  utilities. `PassthroughStatusRow` supplies bounded end justification.
- Hierarchy: terminal notices, right-aligned next-step row, shared status and
  utility row, then composer input. Status and utility controls wrap downward
  when their complete touch targets do not fit.
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

For .6 and .7, extend `mobile-chat-status-bar.spec.ts` with an open PR, todo
items, and visible transcript controls. At 393px, choose a sparse set whose
measured widths fit and assert matching vertical centers within one CSS pixel.
At 320px, enable enough controls to exceed the available width and assert
wrapping, containment, non-overlap, and 44px targets. Cover 640px, 767px,
768px, a wide desktop, and a narrow fine pointer. Test without a workflow
action and with empty status or utility groups. Open todo, CI, and usage
disclosures, and exercise transcript navigation after wrapping.

## Compatibility and delivery

Preserve `shouldShowProceed`, props, test IDs, callbacks, preview/options
surfaces, and terminal-notice precedence. Status icons move below the workflow
action on phones to share space with utilities. No copy, API, storage, or
localization changes are needed. The passthrough composer retains its composition.
`ComposerCIStatus` remains standalone when `ComposerDisclosureContext.enabled`
is true. Threads auto-hide visibility is a separate contract.

The separate workflow row was implemented and locally verified through
[Task 02](../../../plans/mobile-composer-action-wrapping/task-02-place-action-above-utilities.md).
Task 01 records the completed earlier alignment correction. Fresh desktop,
mouse-phone, and touch-phone screenshots must replace the previous PR placement
evidence during PR delivery. Rendered validation is recorded in the work order.
The shared status/utility row is implemented and verified through
[Task 03](../../../plans/mobile-composer-action-wrapping/task-03-share-status-utility-row.md).
