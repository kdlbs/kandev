---
status: building
created: 2026-07-30
owner: kandev
---

# Keep Transcript Scroll Position On Navigation

## Why

Users read earlier transcript history by scrolling up while the session's
auto-scroll preference stays at its default (**enabled**) — no toggle
interaction implies "I still want new output to pull me to the bottom,"
not "reset me to the bottom right now." Navigating to another surface
(kanban board, another task) and back remounts the chat panel via
dockview's layout rebuild. Today that remount always re-opens at the
bottom when auto-scroll is enabled, silently discarding the read position
the user never asked to give up. Users report this as "the transcript
position is not preserved" when they navigate away and back.

The already-shipped auto-scroll toggle (`docs/plans/scoll-toggling-*`,
PR #2039) fixed this for the **disabled** case only: freezing auto-scroll
persists and restores the exact offset across remounts. The enabled case
was left at its pre-existing bottom-anchored behavior, which is the gap
this spec closes.

## What

- Both message-list renderers (`native`, the default, and `virtuoso`,
  selectable via `?renderer=virtuoso` and used for 1000+ message
  transcripts) restore the session's last-captured scroll offset at
  mount time, **regardless of whether auto-scroll is currently enabled
  or disabled** — not just when disabled as today.
- Scroll offset capture is unaffected by this change: both renderers
  already persist the current offset continuously (native: on every
  scroll event, coalesced to one write per animation frame; virtuoso: on
  unmount, and immediately on every disable transition). This spec adds
  no new capture path.
- A pending dockview layout-rebuild restore (maximize / un-maximize)
  continues to take precedence over this offset restore, unchanged.
- Live behavior while a session stays mounted is unchanged: with
  auto-scroll enabled, new messages continue to pull the view to the
  bottom in real time; with it disabled, the view continues to freeze in
  place. This spec only changes what scrollTop/state a remount opens
  with — not any in-session auto-follow logic.
- A session with no captured offset yet (never scrolled, or opened for
  the first time) still opens at the bottom regardless of the toggle
  state — there is nothing to restore.
- Re-enabling auto-scroll after a restored, disabled-and-then-remounted
  session keeps the existing catch-up semantics (`shouldCatchUpOnAutoScrollEnable`):
  it jumps to the bottom only if content genuinely appended while
  disabled and isn't already in view.

## Data model

No new persistence. Reuses the existing per-session scroll-offset state
already written by the shipped auto-scroll toggle feature:

- Native: `transcriptAutoScroll.scrollTopBySessionId[sessionId]` (in-memory
  store) falling back to `getStoredAutoScrollTop(sessionId)`
  (sessionStorage).
- Virtuoso: `transcriptAutoScroll.virtuosoStateBySessionId[sessionId]`
  (in-memory `StateSnapshot`, not sessionStorage-backed).

## Failure modes

- No captured offset for the session (`undefined`): falls back to the
  bottom, same as today's disabled-with-nothing-captured case.
- A pending dockview layout restore always wins over this restore path,
  same precedence as today.

## Persistence guarantees

Unchanged from the shipped toggle feature: the native offset survives a
page reload within the same browser session (sessionStorage); the
virtuoso snapshot is in-memory only and does not survive a full page
reload, only a dockview-level remount.

## Scenarios

- **GIVEN** auto-scroll is enabled (default) and the user scrolled to the
  middle of an overflowing transcript, **WHEN** they navigate to the
  kanban board and back into the same task, **THEN** the chat panel
  reopens at the same offset instead of the bottom.
- **GIVEN** auto-scroll is enabled and the user is genuinely at the
  bottom, **WHEN** they navigate away and back, **THEN** the chat panel
  reopens at the bottom (the captured offset is already the bottom, so
  behavior is unchanged from today).
- **GIVEN** auto-scroll is enabled and the session has never been
  scrolled (no captured offset), **WHEN** it is opened for the first
  time or remounted, **THEN** it opens at the bottom.
- **GIVEN** a session's chat panel stays mounted with auto-scroll
  enabled, **WHEN** new messages arrive, **THEN** the view continues to
  auto-follow to the bottom live, unaffected by this change.
- **GIVEN** a pending dockview layout-rebuild restore (maximize /
  un-maximize), **WHEN** the panel remounts, **THEN** that mechanism
  still determines the scroll position regardless of the auto-scroll
  toggle state or any captured offset.
- **GIVEN** the `virtuoso` renderer is active (`?renderer=virtuoso`) and
  auto-scroll is enabled, **WHEN** the user scrolls up and then navigates
  away and back, **THEN** the panel restores the same virtualized
  position instead of resetting to the last item.

## Out of scope

- Changing the default auto-scroll-enabled state or the toggle UI.
- Changing in-session (mounted, not remounting) auto-follow or freeze
  behavior — both are already correct and covered by
  `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`.
- Making the virtuoso snapshot survive a full page reload (it is
  in-memory only today; unaffected by this fix).
- Cross-tab or cross-device scroll-position sync.

## Implementation plan

- [Keep Transcript Position](../../plans/keep-transcript-position/plan.md)
