---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
created: 2026-09-10
owners:
  - kandev
---

# Mobile Action Confirmation System Design

## Purpose and boundaries

This design owns reusable phone confirmation presentation. Domain adapters
continue to supply the target, current eligibility, localized consequences,
options, and mutation callbacks. No backend, store schema, plugin SDK, archive
policy, or recovery boundary changes.

The accepted surface choice is recorded in
[Mobile confirmation surfaces](../../../decisions/2026-09-10-mobile-confirmation-surfaces.md).
The [task archive design](../../tasks/system-design/archive-confirmation.md)
remains authoritative for preference and navigation;
[task cleanup hierarchy](confirmation-warning-hierarchy.md) remains
authoritative for cleanup copy and discard consent. Delivery is tracked in the
[plan](../../../plans/mobile-action-confirmations/plan.md). The content-sized
host correction is tracked in the
[sheet sizing plan](../../../plans/mobile-confirmation-sheet-sizing/plan.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-MOBILE-CONFIRMATION-001 | Routing, hosted content, adoption boundaries |
| REQ-UI-MOBILE-CONFIRMATION-002 | Mobile composition, accessibility, localization |
| REQ-UI-MOBILE-CONFIRMATION-003 | Request lifetime, domain adapters, failure behavior |

## Current implementation

`InlineConfirmActions` inserts a `basis-full` description/action region into
rows and can remove itself before invoking the owning callback.
`TaskArchiveConfirmation` routes fine pointers to `ActionConfirmPopover` and
coarse pointers to inline copy, with `TaskArchiveConfirmDialog` for cascade,
bulk, or forced-dialog requests. Phone Kanban currently forces that alert.

`SessionTaskSwitcherSheet` already owns an inset `Drawer`, its list state, and
its scroll container. `MobilePickerSheet` owns the corresponding session and
terminal picker shell. Their current rows keep their own confirmation state.
Several settings and integration adapters share the same inline primitive.

## Components and responsibilities

Add application-owned components under `apps/web/components/confirmation/`:

- `MobileConfirmationContent`: title, subject, accessible description,
  optional domain content, and full-width footer. It receives action variant,
  disabled state, test IDs, and callbacks; it knows no domain IDs or stores.
- `MobileActionConfirmation`: controlled phone surface adapter. It uses the
  nearest explicit host when available, otherwise the existing `@kandev/ui`
  `Drawer`. It does not infer host ownership from DOM selectors.
- `MobileConfirmationHost`: one locally scoped confirmation outlet inside an
  existing drawer or centered form dialog. It preserves the origin subtree,
  shows one active confirmation, and owns step cancellation and focus return.

Names are implementation targets, not public APIs. Keep these small and
composable; do not introduce an application-wide modal queue or persist React
nodes in Zustand. Retain `InlineConfirmActions` and `ActionConfirmPopover` for
existing non-phone consumers rather than changing their global semantics.

### Routing

Use `useResponsiveBreakpoint().isMobile` before pointer-specific routing.
Phones remain phones with a mouse attached; 640-767px must not fall through the
global menu CSS breakpoint. At 768px and wider, preserve each caller's existing
fine/coarse-pointer behavior.

| Origin | Phone destination | Cancellation destination |
| --- | --- | --- |
| Tasks drawer | Confirmation step in that drawer | Same task list, filters and scroll |
| Session/terminal or saved-view drawer | Confirmation step in that drawer | Same picker/editor state |
| Page or its transient menu/popover | Standalone compact Drawer after menu dismissal | Page and surviving trigger |
| Inline action in a centered form dialog | Confirmation step in that dialog | Same unsubmitted form |
| Existing full delete/discard/type-to-confirm alert | Existing alert | Existing behavior |

For a menu inside a drawer, close only the menu and use the drawer host. For a
page menu, move confirmation ownership outside its portaled children before
closing it. An open confirmation must never be owned solely by an unmounted
menu item. Use controlled menu dismissal and its close-autofocus completion,
not a new fixed timeout, to transfer focus.

### Hosted content

The host belongs inside the existing `Drawer`/`Dialog` root and outside the
origin content it may hide. Keep the list/form subtree mounted, hidden and
inert while the confirmation outlet is active; the outlet is a sibling, never
a descendant of that hidden subtree. Its source controls remain mounted, so
row-local state and scroll offsets survive. A source-keyed registration plus
a portal into the outlet can preserve the confirmation's owning React context.
Register and clean up after commit, never update host state during render.

The registration has one stable request token. Re-rendering descriptions or
disabled state updates the same request rather than reopening it. Releasing an
older request cannot remove a newer one. Unmount clears only its own token.
There is one active request per host and no request queue.

Update the host's accessible title/description references to the active step.
Use one dialog/focus-trap boundary; do not insert another nested `role=dialog`
or `Drawer` for hosted content. Keep a semantic labelled group inside it.
Origin content contributes no active scroll region while hidden.

Drawer hosts use content-sized confirmation geometry, including the Tasks,
sidebar-filter, Threads, session, terminal, GitHub, and GitLab surfaces. The
origin's regular height is restored on Cancel; it does not set a minimum height
for the decision. Centered form/command dialog hosts retain their existing
geometry. An explicit host surface kind distinguishes these compositions;
never infer them by querying ancestor roles or change every `Drawer` globally.

For an active drawer request, the host supplies content properties that override
the origin's fixed height and the primitive's direction-specific height cap.
Use intrinsic height, a `calc(100dvh - 1rem)` maximum and the existing bottom
inset. Keep the header and action footer outside the body's internal scroller.
The confirmation outlet alone participates in the active layout.
The drawer root uses `overflow: clip` in both steps. Unlike `overflow: hidden`,
this prevents focus or scroll-into-view from scrolling the shell itself and
clipping the header while its bounds still appear correct. Inner origin/body
scrollers retain normal scrolling. Initial Cancel focus uses `preventScroll`.

Before hiding the origin, capture its rendered dimensions for that request.
Keep it mounted, invisible and inert in an out-of-flow container with those
dimensions while the compact step is active. Simply using `visibility:hidden`
inside the shared grid still reserves the editor's height; removing its layout
without preserving its dimensions can clamp nested scroll offsets. Release the
temporary sizing with the request, restore normal flow, then return focus.
Rotation recomputes visible containment without recreating the request.

Reuse Vaul's bottom entry, swipe and dismissal behavior. Do not close and reopen
the root merely to replay an entrance animation. Any hosted content transition
is scoped, bottom-up and reduced-motion-aware; it must not compete with Vaul's
outer transform, restart when copy changes, or delay action availability. No
new detents, gesture navigation, global animation rules or dependencies are
introduced.

### Request lifetime and focus

Capture a stable target key and initiating control before leaving a menu.
Adapters retain existing domain validation, including target disappearance and
pending eligibility changes. A disconnected menu node is not sufficient proof
that a page-owned target disappeared; use the stable owner/target plus a
surviving focus-return control.

Cancel or header Back releases the active step, reveals the origin and then
focuses its surviving trigger. If live data removed the trigger, focus the
host's visible title or nearest valid control. Escape is handled in capture
phase and stops propagation so it cannot also close the outer drawer. Swipe
or backdrop dismissal closes the entire surface and invalidates its request.
Route/context changes and crossing the phone breakpoint cancel unsubmitted
requests. Rotation within phone widths only recomputes containment.

Add a synchronous submit guard before closing any confirmation or invoking
its callback. The event which opened the decision never submits it. A late
classification result, rejected callback, or effect cleanup must not reopen
an invalidated request. Reopening explicitly creates a fresh request token.

## Domain adapters

### Archive

Use one archive state/content boundary for preference mode, descendant
classification, cascade state, in-flight warnings, and cleanup consequences.
Extract it from `TaskArchiveConfirmDialog` only as far as required for the
existing alert and new mobile renderer to share it. Do not duplicate
`useArchiveConfirmationMode`, `useSubtaskCountState`, or cleanup policy.

Phone `TaskArchiveConfirmation` selects the mobile surface even when an older
caller supplies `inline` or `forceDialog`. Direct full-dialog callers and
`TaskArchiveConfirmFlow` also reach the same mobile renderer. The archive
surface is known before classification, so it may show localized loading copy
with the action disabled until the result settles. An error keeps the existing
conservative full-content decision and does not pretend there are zero children.
Desktop classification gating and tablet routing remain unchanged.

Mount task-row confirmation beside the row, using the existing fine-pointer
sibling pattern on phones too. In `SessionTaskSwitcherSheet`, host the request
above the hidden list. Preserve `useSheetActions` navigation and
`surfaceAction` close-on-confirm behavior: opening or cancelling a step must
not call that wrapper. Single/bulk/cascade submissions keep the same values.

The command panel's `CommandPanelConfirmation` is an existing owned
confirmation region inside a dialog. On phones, use that dialog's focused
step host; keep the command source mounted and restore its query on Cancel.
Do not open a drawer under an already active command dialog. Its desktop
explicit-inline behavior remains unchanged.

`getCleanupSummary`, `getBulkCleanupSummary`, `TaskCleanupConsequences`, and
`StillWorkingWarning` remain shared. Render the current domain-provided
consequences; the screenshot is not an authority for branch-deletion policy.
Archive's phone action uses the existing full-dialog default variant. Task
delete and its discard selection remain in their existing centered alert.

### Other inline consumers

Adopt the adapter at each actual phone call site, supply a visible localized
title and target, and keep the initiating row/trigger mounted. Do not promote
an `aria-label` into visible copy by scraping the DOM.

Session/terminal pickers use the `MobilePickerSheet` host. Saved-view filters,
Threads, GitHub/GitLab filter sheets, and settings forms opt into the host at
their existing shell. File rows and ordinary settings cards open standalone
drawers; action menus hand off to a stable owner first. Walkthrough and plan
history surfaces host a step when already inside a mobile drawer/dialog.

Saved-view eligibility, exact captured IDs, default-marker isolation, and
optimistic rollback stay in their current hooks. Session warnings remain
shared with desktop. Full system alerts and explicit discard inputs are not
replaced by this adapter.

The filter shells in `app/github/github-page-client.tsx` and
`app/gitlab/gitlab-page-client.tsx` share the presentation-only
`components/integrations/integration-filters-sheet.tsx`. On phones it uses
`Drawer`, so the picker and compact confirmation share a bottom surface with
a fixed header and one scrollable list. Above the phone boundary it retains
the right-hand `Sheet`. Existing selection, default, save and delete handlers
stay with each provider; retain their close timing and test IDs.

## Mobile composition and accessibility

Reuse the inset rounded geometry of `SessionTaskSwitcherSheet` and the fixed
header/internal scroll pattern of `MobilePickerSheet`. Use existing theme
tokens with a neutral surface, a 16-18px action title, and 14px readable body
copy. Consequences precede quieter supporting notes; required warnings and
options remain visible in the body. Avoid reusing selection color as warning
background. Restrained transitions follow existing primitives and reduced
motion; no stagger delays, new animation dependency, or decorative bounce.

Use a `min-h-0 flex-1 overflow-y-auto overscroll-contain` body, a fixed header,
and an action footer with `env(safe-area-inset-bottom)` clearance. Cap the
surface using dynamic viewport units and preserve top/side insets. The footer
orders the semantic primary action above Cancel, both full-width with a 48px
target height. Apply these sizes only to phone composition.

Cancel receives initial focus. Centered form/command `DialogContent` hosts
disable their existing `enterConfirms` option while a phone confirmation step
is active. Phone destructive actions have no implicit Enter default; focused
buttons retain normal Enter/Space activation. Archive
also uses explicit button activation on the new phone surface. Existing
desktop alert Enter behavior remains in the base UI package. Label controls,
announce the active step, and keep the hidden origin outside the accessibility
tree. Give active title/description references unique IDs even when the hidden
origin includes a primitive title; never mount two elements with the same ID.
Verify touch hit-testing, not just CSS dimensions.

All new copy belongs to existing domain namespaces or `common`, uses `t()` or
`Trans`, and ships in `en`, `pt-pt`, `zh-cn`, generated `zh-hk`/`zh-tw`, and
generated `pseudo`. Keep comparison tokens untranslated. Prefer existing
localized titles/labels and current cleanup copy to unnecessary wording changes.

## Failure, persistence, and security

Presentation does not take ownership of domain transport. Preserve each
caller's close-before-dispatch, awaited, or optimistic behavior explicitly in
its adapter. In particular terminal close still removes local state before
teardown and shows only its existing error notification on failure; it does
not restore the terminal or display a shared spinner. A caller that keeps its
form open for retry continues to do so, scoped to its current request token.
The adapter defaults to `completionPolicy="close-before-dispatch"`. Workflow
sync removal opts into `"await-with-retry"`: a fulfilled callback dismisses the
step, while rejection releases the submit guard without discarding the form.
The controller retains its existing error feedback. Unmount, target changes,
or parent closure invalidate completion handling for the old request.

The shared shell neither invents success/rollback behavior nor suppresses a
domain error. Existing pending state disables confirmation; ordinary Cancel
remains available unless the owner already prevents dismissal during execution.
The submit guard supplements, not replaces, domain in-flight guards.

All presentation state is ephemeral. No database, user-setting, remote API,
permission, plugin contract, or logging changes. Existing archive preference,
task navigation, worktree discard consent, and terminal teardown contracts
remain authoritative.

## Verification

Component tests cover host lifetime, preserved origin state, focus, captured
targets, route/breakpoint cancellation, stale registration cleanup, callback
ordering, disabled/duplicate submission, and rejection after dismissal.

Each adoption work order owns mobile Playwright scenarios for its real entry
points and the existing desktop regression command. The shared geometry checks
cover 320px, configured Pixel 5, 767px, and landscape phone widths; a separate
coarse-pointer 768px case proves the unchanged tablet boundary. Long names,
Portuguese/pseudo copy, dark/light themes, safe-area padding, finite animation
settlement, one active scroll owner, focus return, and horizontal overflow are
tested on the active surface. Desktop and phone projects run separately through
the managed runner with a fresh production build.

Hosted geometry checks must assert the short confirmation's actual bounds and
copy-to-footer spacing, not merely dialog presence or a height equal to the
editor. Use the saved Threads view deletion as the compact regression fixture;
also assert restoration of the original dimensions, draft, scroll and focus.
Exercise the fixed-height Tasks/filter hosts and the intrinsically sized
session/terminal pickers. GitHub/GitLab cases must assert bottom direction in
addition to deletion and selection/default isolation.

Capture and inspect the Tasks confirmation, standalone archive, and one
non-task sheet using the focused E2E fixtures. Public instructions change with
implementation, not in this design-only turn.
