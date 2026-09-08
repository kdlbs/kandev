---
created: 2026-09-03
status: complete
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Restore Transcript Scroll Across Environment Switches

## Overview

Make an environment-changing task switch request fresh placement for the
incoming session after its Dockview layout and latest message window settle.
Keep the request session-scoped so enabled transcripts land at the newest
message, disabled transcripts use their own saved position, and the older-
history sentinel cannot paginate from transient top geometry.

## Scope

### In scope

- Arm a tokened incoming-session placement request only for cross-environment
  Dockview switches.
- Keep the absolute `pendingChatScrollTop` restore reserved for same-transcript
  layout rebuilds.
- Expose cached session-entry history refresh readiness to the transcript
  coordinator without changing its loading presentation.
- Re-arm initial placement for a matching request and resolve it after layout
  and history settle.
- Block automatic older-history intersection loading until placement completes.
- Add focused store, hook, and browser regressions for enabled, disabled,
  wrong-session-offset, pagination, and same-environment behavior.
- Run the existing mobile auto-scroll suite because mobile shares the native
  transcript coordinator.

### Out of scope

- Changing the auto-scroll control, persisted preference, saved-offset format,
  or message-pagination contract.
- Replaying the outgoing task's absolute transcript offset.
- Changing Dockview layout persistence, portal ownership, or fast/slow switch
  selection.
- Changing normal live tailing, unread-divider placement, explicit transcript
  navigation, maximize, un-maximize, preset, or custom-layout restoration.
- Adding a mobile Dockview environment-switch lifecycle.

## Confirmed root cause

`buildSwitchToSession` routes a task whose session has a different environment
through `performLayoutSwitch` and `buildEnvSwitchAction`. That action toggles
`isRestoringLayout` around `performEnvSwitch`, but intentionally does not call
`preserveChatScrollDuringLayout`, because its absolute `scrollTop` belongs to
the outgoing session.

The incoming panel remains logically visible, so `useActivationPending` does
not observe a hidden-to-visible transition. `useInitialScrollPosition` can then
consume its one-shot latch against cached or partial rows before the incoming
`message.list` refresh completes. Later rows change the content height without
another placement. The transient near-top geometry exposes the lazy-load
sentinel and can start repeated `top-intersection` pagination.

The cached-history entry path in `useSessionMessages` starts a background
`message.list` refresh without including that operation in the loading state
currently available to the transcript. Layout state alone therefore cannot
prove that the incoming history has settled.

## Technical approach

### Session-scoped environment-switch request

- Extend `DockviewStore` with a transient tokened placement request containing
  the incoming `sessionId`, plus a compare-and-clear completion action.
- Arm it inside `buildEnvSwitchAction` after the same-environment early return
  and before the layout mutation. Do not write `pendingChatScrollTop`.
- Preserve the request through both `performEnvSwitch` paths and default-layout
  fallback. A later switch replaces the token.

### History and placement readiness

- Track the cached-history background refresh in `useSessionMessages` and
  expose a dedicated readiness value through `useChatPanelState` to
  `NativeMessageList`. Do not use it to add a new loading indicator.
- Extend the activation-pending primitive with an optional external token so a
  matching environment-switch request re-arms placement while `isVisible`
  remains true.
- Keep the initial-placement latch pending while the layout is restoring or
  incoming history is refreshing. Once both are ready, run placement through
  `scheduleAfterPanelRestore` and re-check all scroll owners before writing.
- Resolve the position with the existing `resolveNativeInitialScrollTop` logic,
  using only the incoming `sessionId` for the saved offset. Clear the request
  only when the token still matches.

### Pagination isolation

- Treat a matching, unresolved environment-switch placement as a hard block for
  `useLazyLoadSentinel`, alongside initial/refetch loading.
- Release the block only after placement has completed or a higher-priority
  explicit scroll owner has consumed the request.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11` | `dockview-env-switch-action.test.ts` proves only a cross-environment switch arms the incoming session request. `message-list-native.test.tsx` holds layout/history pending, settles with a taller incoming transcript, and requires enabled placement at the current bottom. The desktop Playwright flow switches between two tasks with distinct environments and requires the returned enabled transcript to settle at the bottom. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.12` | Native-scroll regressions save distinct offsets for outgoing and incoming sessions and require the disabled incoming session's value. The desktop flow disables the incoming transcript, records its position, visits the other task, and requires the incoming position after return. |
| `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13` | The native-scroll harness requires the lazy-load sentinel to stay blocked throughout unresolved environment placement and verifies no load starts from transient top geometry. |
| Existing criteria 8-10 | Same-environment store and hook cases remain unchanged. The existing inactive-session desktop scenario and mobile suite continue to pass. |

`dockview-scroll-preserve.test.ts` also proves the absolute layout-preservation
helper does not arm, replace, or clear the environment-switch request.

## E2E tests

- Desktop: extend `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` with a
  two-task fixture whose sessions have different task environments and
  overflowing persisted histories. Cover enabled return-to-bottom and disabled
  incoming-position restoration through sidebar task selection.
- Mobile: the defect path is desktop Dockview-only. Run the existing
  `mobile-auto-scroll-toggle.spec.ts` suite to prove shared initial placement,
  bottom following, disabled freezing, and touch controls remain intact.

## Mobile design contract

- **Desktop outcome:** returning to a task in another environment shows its own
  transcript at the current bottom or its own disabled saved position.
- **Mobile entry point:** the existing selected Chat panel under
  `MobileSessionsPicker`.
- **Nearest shipped exemplar:** `SessionMobileLayout` renders one selected
  `TaskChatPanel`; the existing mobile auto-scroll suite covers the shared
  coordinator.
- **Hierarchy and presentation:** no controls, composition, or scroll ownership
  change. The transcript remains the single vertical scroll owner.
- **Shared state and logic:** desktop and mobile share session auto-scroll state
  and native placement. The transient Dockview request is desktop-only.
- **Mobile coverage decision:** no new mobile scenario can reproduce a Dockview
  environment rebuild. Existing mobile Playwright coverage is the parity proof
  for the shared code path.

## Work orders

- [x] [Task 01: Reconcile environment-switch transcript placement](task-01-reconcile-env-switch-transcript-placement.md)

## Verification results

- Cross-environment Dockview switches now arm a tokened request for the incoming
  session. The native transcript holds placement and pagination until layout
  restoration and the session-entry history refresh settle.
- Enabled transcripts resolve at the latest message. Disabled transcripts use
  only their own saved offset. Compare-and-clear tokens protect later switches,
  and the same-environment path remains a no-op.
- Focused Vitest passed in test and production environment modes (128 tests).
- The complete frontend Vitest run passed (1,756 files, 15,076 tests, 4
  skipped). Typecheck, lint, Prettier, spec lint, and diff checks passed.
- The desktop differing-environment Playwright regression passed (1 test). The
  existing mobile auto-scroll suite passed (5 tests).

## Risks

- React effect order can let initial placement run before a session-entry fetch
  marks itself pending. Arming the store request before the layout mutation and
  reading a dedicated refresh signal prevents that first-render race.
- A stale animation frame can target a session after another task switch. The
  token and live session/owner checks must reject it.
- Releasing the pagination block before the bottom or saved-offset write can
  recreate the `top-intersection` cascade.
- Broadening the existing loading presentation for cached history would be an
  unrelated UI change; readiness stays internal to scroll coordination.

## Decisions

- No ADR is required. The repair preserves the current Dockview, transcript
  scroll-owner, per-session storage, and pagination boundaries.
- No public documentation change is required because the repair restores
  existing behavior and adds no user-facing control or workflow.
