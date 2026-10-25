---
created: 2026-09-02
status: done
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Restore Session Transcript Scroll on Activation

## Overview

Make transcript placement wait for a persistent desktop session panel to be
visible and measurable. Preserve bottom-follow intent through inactive tabs,
while retaining reader-owned positions when auto-scroll is disabled or the
reader has moved away from the newest message.

## Scope

### In scope

- Defer one-time transcript placement until an inactive Dockview panel becomes
  visible.
- Reconcile bottom-follow intent after `SessionPanelContent` restores its saved
  absolute offset.
- Suppress bottom writes and near-bottom geometry updates while a transcript is
  hidden.
- Preserve unread-divider, explicit navigation, layout-restore, pagination,
  disabled-position, and manual-reader ownership.
- Add focused hook regressions and desktop browser coverage for refresh, session
  switching, hidden message delivery, and disabled-position preservation.
- Run the existing mobile auto-scroll suite against the shared scroll logic.

### Out of scope

- Changing session tabs, transcript controls, labels, layout, or persistence.
- Replacing Dockview persistent portals or the generic panel scroll restorer.
- Changing unread-cursor semantics, message pagination, or smooth-navigation
  behavior.
- Adding a mobile persistent-panel lifecycle that the product does not use.

## Confirmed root cause

Dockview panel content is rendered through persistent portals. After a refresh,
an inactive session transcript can mount with its full message list while its
scroll container has zero geometry. Both `useInitialScrollPosition` and
`useScrollToDividerOrBottom` mark their one-time work complete in that state, so
activating the tab does not retry placement.

Live updates have a second failure path. `useAutoScroll` writes a bottom offset
while the inactive container is not measurable. When the tab becomes visible,
`SessionPanelContent` restores the last visible absolute `scrollTop`, which no
longer represents the bottom after hidden content growth.

The reproduced inactive transcript had 88 rows and a 3,712 px bottom gap after
refresh and activation. A transcript that received hidden agent output reopened
603 px above the bottom even though auto-scroll remained enabled.

## Technical approach

### Visibility-aware placement

- Pass the existing `isVisible` signal through the native initial-placement,
  divider-placement, and auto-scroll hooks.
- Do not consume an initial-placement latch while `isVisible` is false.
- When an inactive panel becomes visible, defer placement by two animation
  frames so the existing one-frame `SessionPanelContent` restore finishes
  first.
- Keep `SessionPanelContent` generic, but apply its queued absolute-offset write
  only when the panel's `scrollTop` is unchanged since scheduling. A newer
  transcript or user scroll therefore remains the active owner.
- Re-check the live visibility and ownership guards before the deferred write,
  and cancel the scheduled work if the panel becomes inactive again.

### Bottom-follow intent

- Keep the existing near-bottom ref as the transient bottom-follow intent.
- Do not write bottom offsets or derive near-bottom state from hidden geometry.
- On activation, place an enabled bottom-following transcript at the native
  maximum offset through `scrollNativeToBottom`.
- Leave the restored absolute offset untouched when auto-scroll is disabled or
  the reader moved away from the bottom before hiding.

### Competing scroll owners

- Keep pending Dockview layout restoration and explicit message navigation
  ahead of automatic placement.
- Keep the visit-scoped unread divider ahead of bottom placement and mark that
  placement as away from the bottom.
- Run the existing pagination recheck after visibility restoration without
  allowing it to consume or override the placement decision.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.8` | A native-scroll hook regression mounts an overflowing transcript hidden, proves no initial latch/write is consumed, then activates it after a simulated panel restore and requires bottom placement. A desktop Playwright scenario refreshes a task with two long session histories, activates the initially inactive tab, and requires a near-zero bottom gap. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.9` | A hook regression appends while hidden and requires one guarded catch-up on activation. The desktop browser scenario seeds a live message into an inactive session, reactivates it, and requires the marker and transcript bottom to be visible. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.10` | Hook tests cover disabled and manually-away states. A desktop browser scenario disables auto-scroll, switches tabs, and requires the prior absolute position after returning. |

## E2E tests

- Desktop: extend `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` with a
  deterministic two-session fixture. Use the existing E2E session/message
  harness so both histories overflow before refresh and an inactive session can
  receive a live `message.added` event without a timing delay.
- Mobile: no new scenario models the desktop-only persistent hidden portal.
  Run the complete `mobile-auto-scroll-toggle.spec.ts` suite to prove the shared
  native coordinator retains visible initial placement, enabled following,
  disabled freezing, and touch control behavior.

## Mobile design contract

- **Desktop outcome:** switching to an enabled, bottom-following session tab
  shows its newest message after refresh and hidden updates.
- **Mobile entry point:** the existing selected Chat panel under
  `MobileSessionsPicker`.
- **Nearest shipped exemplar:** `SessionMobileLayout` renders one selected
  `TaskChatPanel`; the existing mobile auto-scroll Playwright suite exercises
  that shared native transcript.
- **Hierarchy and presentation:** no controls or composition change. The
  transcript remains the only vertical scroll owner.
- **Shared state and logic:** desktop and mobile keep the same auto-scroll
  preference, near-bottom intent, and bottom helper. Only desktop supplies an
  inactive `isVisible` lifecycle.
- **Mobile coverage decision:** the defect state is unavailable on mobile
  because inactive session panels are not kept mounted. The existing
  mobile-specific Playwright suite is the narrow parity proof for the shared
  logic changed by this repair.

## Work orders

- [x] [Task 01: Reconcile transcript scroll on session activation](task-01-reconcile-transcript-scroll-on-activation.md) (done)

## Verification strategy

- TDD regressions were added for hidden activation, hidden updates, competing
  scroll owners, bounded-frame cancellation, and the activation-scoped
  unread-divider settling window before the implementation was completed.
- A focused `SessionPanelContent` component test controls `ResizeObserver` and
  `requestAnimationFrame` to prove both guarded restore outcomes: unchanged
  `scrollTop` is restored, while a newer scroll remains authoritative.
- Focused native transcript unit tests, TypeScript typecheck, and targeted
  ESLint passed.
- The desktop multi-session scenario and the existing mobile auto-scroll suite
  passed with retries disabled.
- Specification tests, full specification lint, and diff checks passed.

## Risks

- A visibility correction can race the generic panel restore and be overwritten
  if it runs too early. The two-frame schedule and stale-offset guard bound this
  race without making the generic panel component chat-aware.
- A deferred correction can override an explicit jump, unread divider, or
  reader-owned position if ownership is not re-checked at execution time.
- Hidden message updates can incorrectly turn a manual reading position back
  into bottom-follow intent if geometry is sampled while inactive.
- Animation-frame tests can pass in jsdom while missing Dockview's detached DOM;
  the real two-session browser regression is required evidence.

## Decisions

- No ADR is required. The repair preserves the existing Dockview portal,
  transcript scroll-owner, and persisted-setting boundaries documented by the
  current UI system design.
- No public documentation change is required because the fix restores existing
  user-facing behavior and adds no control, setting, or workflow.

## Results

Implemented visibility-aware transcript placement, hidden-update catch-up,
reader-position preservation, and the cooperative generic panel restore guard.
The fixup pass centralized the shared activation-transition lifecycle, retained
the last visible disabled-reader offset through hidden cleanup, and covered
immediate activation-frame cancellation. The divider settling deadline now
starts at first visibility and resets on each hidden-to-visible activation.
Focused unit tests, the direct shared-panel restore regression, typecheck,
targeted ESLint, desktop and mobile Playwright coverage, specification lint,
and diff checks all passed.
