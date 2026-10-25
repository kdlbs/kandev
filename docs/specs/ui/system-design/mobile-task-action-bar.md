---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-TASK-ACTION-BAR-001
---

# Mobile task action bar design

## Purpose and boundaries

Repair the responsive composer tray without duplicating feature state or
handlers. This is a presentation change. No backend, schema, API, configuration,
or persistence migration is required. The independent composer geometry is
excluded from the [panel toolbar](panel-toolbars.md) fixed-height contract.

## Requirement mapping

| Requirement | Design boundary |
| --- | --- |
| REQ-UI-MOBILE-TASK-ACTION-BAR-001 | Phone composition, shared notice, controls, and verification below |

## Source evidence

`ChatStatusBar` in `apps/web/components/task/chat/chat-status-bar.tsx` renders
`ChatStatusBarArchiveBanners` inside a wrapping flex row alongside indicators
and `ChatStatusBarActions`. `ArchiveDismissBanner` in `pr-archive-banners.tsx`
uses a flexible text span and tiny Archive/dismiss buttons. The screenshot
shows the message wrapping into six lines while the action cluster retains
its width. The terminal messages are currently constructed as English template
strings and require localization when changed.

`PassthroughStatusRow` in `apps/web/components/task/passthrough-toolbar.tsx` also hosts
`PRMergedBanner`; it must adopt the same full-width phone notice placement.
`TranscriptNavGroup` has a non-wrapping navigation subgroup, and the right-control
cluster is non-wrapping. These groups must permit complete-control wrapping.

## Phone composition

- Entry: open a task's chat or passthrough conversation; no new navigation step.
- Hierarchy: terminal notice first, then the existing status/control region and
  composer. Normal chat's revised workflow-versus-utility order is owned by
  [composer action placement](composer-action-wrapping.md); it places a
  right-aligned workflow row above the utility row on phones. Passthrough
  retains its inline/trailing composition.
- Surface: inline composer tray. A short task notice should remain visible
  beside its action, so opening an additional drawer is unnecessary. Frequently
  used utility controls remain direct targets; whole controls wrap at high density.
- Exemplars: retain the dedicated phone composition in `task-layout.tsx` and
  the compact action grouping/safe-area handling in
  `mobile/session-mobile-bottom-nav.tsx`. Reuse `ConversationUsageDisplay`'s
  existing touch `Drawer` and `TaskArchiveConfirmation`'s phone adapter. The
  bar does not replace those surfaces or add another overlay.
- Scroll owner: the existing transcript/composer layout remains authoritative.
  The notice is natural height, outside the transcript scroller. No fixed
  viewport height or nested scroller is added.

Use `useResponsiveBreakpoint().isMobile` for phone composition below 768px.
Existing desktop/tablet inline composition remains available. A shared terminal
banner receives enough allocated width in both hosts: a full-width block or
flex basis cannot be constrained by a shrink-wrapped action parent.

## Shared terminal notice

Keep `PRMergedBanner` and `PRClosedBanner` responsible for all-terminal eligibility
and per-task dismissal state. Change only their presentation and translated text.
Use concise phone messages such as `PR #3948 merged` and `PR #3948 closed without
merging`, plus count-aware variants for multiple PRs. Desktop retains the existing
message meaning with localized strings. Illustrative copy is not a key contract.

Lay out a nonshrinking status icon, `min-w-0` text, Archive, and dismiss. The
message can wrap at natural word boundaries; do not ellipsize status meaning or
line-clamp translations. Archive and dismiss remain nonshrinking touch targets.
At standard English text size the common merged notice should fit one text line
at 393px; longer closed/plural/translations can occupy additional lines.

Keep task keys, dismissal storage functions, `useTaskArchiveConfirm`, its anchor
ref, and `TaskArchiveConfirmation`. Do not dismiss a notice merely because
Archive opens or is cancelled. Existing preference-aware immediate archive and
navigation recovery remain owned by Tasks.

## Controls and responsive sizing

### Narrow desktop notice allocation

`ThreadConversation` embeds `TaskChatPanel` in a tile whose width can be 360px
while the browser uses desktop breakpoints. The phone-only `basis-full` rule
does not protect this host. A zero flex basis and `min-w-0` let the shared
notice collapse around its nonshrinking buttons. The message's
`overflow-wrap:anywhere` then permits single-character lines.

Give `ArchiveDismissBanner` an intrinsic allocation before sibling controls
consume the row. Use a 20rem preferred flex basis and a minimum width bounded
by the smaller of 20rem and the available parent width. Retain flex growth and
cap the banner at the parent width. The wrapping host then moves complete
utility/action groups to another row when necessary. Retain the full-width
phone override, phone ordering, and existing pointer-specific hit sizes.
Keep emergency wrapping for long unbroken text; removing it alone does not
repair the allocation.

The passthrough host must allocate this minimum through its nested action
wrapper. If its intrinsic wrapper constrains the notice, give that wrapper the
same bounded flex allocation while a terminal notice is present. Do not reserve
notice space when the banner is absent. No Threads-specific state or duplicate
banner is required. Archive and dismissal owners remain unchanged.

Use the existing inline phone notice as the mobile exemplar. The change adds
no navigation, overlay, motion, or scroll owner. The Threads footer allocation
continues to preserve its transcript floor. Preserve the wide desktop inline
contract in [composer action wrapping](composer-action-wrapping.md).

Prove allocation with browser geometry in a desktop viewport containing narrow
tiles. Measure the message span as well as banner and button bounds. A visible
notice or a class assertion cannot detect a one-character text column.
Include merged and closed notices, Columns and Grid, and a long translation.
Keep the existing phone, coarse-pointer, archive, and dismissal regressions.

Implementation and verification are recorded in the
[narrow notice repair plan](../../../plans/threads-pr-notice-layout/plan.md).

### Existing control behavior

Retain existing feature components and callbacks. Make both right controls and
the transcript-navigation subgroup able to wrap complete controls. Keep Proceed
at the right content edge, apply the linked normal-chat phone row contract,
allow a long step label to wrap inside a contained button,
and preserve its full accessible name. Override nowrap for both phone widths
and coarse pointers, including tablets at or above 768px, and verify that the
label stays inside its button. Do not squeeze icons or make a
nonshrinking group wider than its parent.

Apply the existing [control sizing design](control-sizing.md) contract to affected
standalone controls. Phone width OR coarse pointer determines target sizing;
pointer-based disclosure choice remains independent. Audit Archive, dismiss,
TodoIndicator, ShareButton, optional AutoScrollToggleButton, usage, transcript
navigation, eligible Threads link, and Proceed. Prefer the existing shared sizing
helpers when suitable; avoid broad descendant selectors or global button rules.
Allow a caller sizing prop when necessary to isolate composer presentation.

Keep fine-pointer compact desktop controls at 24px. `WorkflowMoveProceedButton`
already uses touch minimum height on coarse pointers; fix its phone caller sizing
without changing hover, direct move, long-press options, or in-flight handling.
`ConversationUsageDisplay` already owns its disclosure and loading/error states;
only its phone fine-pointer trigger target needs adjustment if required.

Localize touched notice and control labels in all seven real locales, generate
Traditional Chinese through `i18n:zh-hant`, and use count plural forms. Do not
translate identifiers or saved dismissal keys.

## Verification and compatibility

Use focused component tests for terminal eligibility, localized count messages,
dismissal, shared handler wiring, and resize preservation. Playwright uses
isolated fixture databases and mock PRs. Cover the common screenshot state,
both terminal outcomes, optional dense controls, long translations/step names,
320/393/640/767/768px, desktop fine pointer, coarse tablet, and narrow fine pointer.
Measure actual target bounds and containment; classes alone are insufficient.
Assert computed phone versus desktop composition and zero document overflow.

Exercise usage, sharing, archive cancel/confirm, persistent dismissal, and workflow
movement through existing fixtures/page objects. The passthrough smoke scenario
must verify its shared merged notice fits without disrupting the composer toggle.
Capture reviewed desktop and phone screenshots from disposable E2E fixtures for
the PR. Store PR media on the skill's orphan media ref, never in the merge diff.

## Related contracts

- [Archive confirmation](../../tasks/system-design/archive-confirmation.md)
- [CI automation tray wrapping](ci-pr-automation-01.md)
- [Control sizing](control-sizing.md)
- [Implementation plan](../../../plans/mobile-task-action-bar/plan.md)
