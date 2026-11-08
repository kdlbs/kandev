---
created: 2026-11-08
status: complete
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Restart Scroll

## Overview

Reopening a long Quick Chat after a backend restart must reveal the latest
messages while the agent resumes. One sequential work order repairs history
readiness and follow intent, then proves the result on desktop and phone.

The UI system owns the reusable scroll contract. The existing
[requirements](../../specs/ui/requirements/transcript-auto-scroll.md) already
define this result in `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.16`, `.17`, and `.19`.
Criteria `.1`, `.10`, and `.20` protect disabled and reader-owned positions.
No requirement changes or new ADR are needed. Task recovery policy remains with
the [Tasks design](../../specs/tasks/system-design/quick-chat-session-resumption.md).

## Evidence and root cause

Investigation used checkout `88c81ce1ba2`. The user reports a long Quick Chat
opening in the middle after restart, with the resume spinner below the viewport.
Two temporary tests exercised the current source and existing test harnesses.

`QuickChatMessageSurface` passes `messagesLoading` but omits
`panelState.historyRefreshPending`. `useSessionMessages` exposes these as separate
states because cached refresh does not require a blocking loader. The native
list defaults the omitted property to false. Its coordinator can therefore
consume final placement while the latest-window refresh remains pending.
The ordinary task Chat surface already forwards this property.

The follow-intent observer has a separate notification-order defect.
`useFollowIntent` replaces its geometry baseline during a content resize.
If native restoration moves the viewport afterward, its scroll handler can
mistake that movement for departure from the bottom. The later guarded
correction requires near-bottom intent, so releasing the blocker cannot recover.

The controlled sequence used a 400px viewport and 1000px content at offset
600px. Initial placement blocked automatic following. Content grew to 1300px,
then content observers ran. A simulated native restoration moved the offset to
500px without reader input. A scroll notification followed, then the blocker
released. The transcript remained at 500px instead of reaching 900px.

The temporary Quick Chat test also failed because `historyRefreshPending: true`
was absent from the child props. Six control cases passed. Reversing only the
notification order without native movement passed all seven geometry cases.
These are controlled component/hook reproductions. They do not prove the exact
browser event sequence on the user's instance. The restart browser tests below
must establish the rendered result before this repair is marked complete.

Temporary files were copies of `quick-chat-content.test.tsx` and
`message-list-native.test.tsx`, named `restart-scroll-investigation.test.tsx`
in their respective directories. They are removed after evidence collection.

## Scope

### In scope

- Quick Chat history readiness during ordinary and cached entry.
- Follow intent across delayed content resize and native scroll notification order.
- Visibility of the latest transcript row and active resume status after restart.
- Desktop, expanded desktop, and full-height phone Quick Chat.
- Regression controls for reader input, disabled following, and session replacement.

### Out of scope

- Agent resume policy, native agent identity, prompt replay, or backend persistence changes.
- Auto-scroll preferences, unread-divider semantics, or new navigation controls.
- Transcript virtualization, pagination size changes, or general layout redesign.

This repair restores documented behavior. Public commands, settings, and terminology
remain the same, so public documentation needs no change.

## Technical approach

`QuickChatMessageSurface` in `quick-chat-content.tsx` forwards the existing
history-refresh property. It retains cached rows and the shared initial-placement
controller. It does not start another fetch or add a resume-specific scroll effect.

`useFollowIntent` in `message-list-native-scroll.ts` preserves sufficient reflow
evidence through resize and scroll settlement. The change retains the existing
reader-input guards and the guarded pending-resize correction in
`use-chat-scroll-motion.ts`. Tests exercise both notification orders, including
growth followed by native movement while a placement guard is active.

The shared immediate bottom helper remains write-only. Initial placement,
explicit navigation, disabled positions, unread targets, and prepend compensation
retain their existing priority. Session changes and unmounts invalidate old work.

## UI preview

The current task requests rich-output previews. These structural views use
descriptions instead of ASCII drawings. The conversation preview compares the
same views. Copy and spacing are illustrative. Scroll ownership and visible
latest content are required.

### UI-01: Desktop Quick Chat, reopened long conversation

Entry: open the previous Quick Chat after restarting the disposable backend.
The tab strip stays above the transcript. The composer stays below it.
The floating and expanded desktop views use the same transcript controller.

- Reported state: the transcript viewport shows older middle rows. The latest
  reply and resume spinner remain below it.
- Required state: the latest reply and active resume status occupy the bottom
  of the transcript viewport. Earlier rows remain reachable by scrolling upward.
- Loading state: cached rows remain visible during refresh. Settled history
  receives final placement, unless the reader has claimed another position.

### UI-02: Phone Quick Chat, reopened long conversation

Entry: app navigation, then Quick Chat. Reuse the full-height Quick Chat dialog
in `quick-chat-modal.tsx`, with its existing dynamic viewport and safe-area rules.
The tab strip and composer surround one internal transcript scroller. Dense
conversation content keeps this full-height surface instead of a short drawer.

The required scroll result matches UI-01. The latest row and active status clear
the fixed composer. A deliberate upward touch pauses following. Controls retain
their existing touch targets. The document does not scroll horizontally.

Both views map to criteria `.16`, `.17`, `.19`, and `.20`.

## Tests

- `quick-chat-content.test.tsx`: add `keeps Quick Chat placement pending during
  cached history refresh`. Assert pending and settled readiness reach the list,
  including when `messagesLoading` is false.
- `message-list-native.test.tsx`: add `preserves follow through resize-first
  native restoration while placement is blocked`. Cover both notification
  orders, enabled and disabled following, and actual reader cancellation. Add
  the growth-then-native-shrink sequence with a 700px expected final offset;
  retain disabled and reader-input controls.
- Retain the existing no-measurement append, initial-placement, unread-divider,
  explicit-navigation, session-switch, and delayed-card cases.
- `use-chat-scroll-motion.test.tsx`: retain blocker-release, content resize,
  motion-off, and cancellation coverage. Extend only if its implementation changes.

## E2E tests

Add `quick-chat-restart-scroll.spec.ts`,
`mobile-quick-chat-restart-scroll.spec.ts`, and a shared
`quick-chat-restart-scroll-helpers.ts` under `apps/web/e2e/tests/chat/`.

Use the existing worker-owned backend and `BackendContext.restart()`.
Create an ordinary Quick Chat with enough mixed messages to overflow the viewport
and exceed a history window. Record its task and session IDs. Restart that
backend, reload, and reopen the same conversation through its normal entry point.
Do not use the user's instance or replace restart with a page reload alone.

Prove the latest persisted reply appears at the bottom without scrolling it
into view. While recovery remains pending, prove the specific active resume
status lies fully inside the transcript viewport. Then prove the settled result.
Use a controlled transport gate if recovery is too fast to observe reliably.
Release that gate in cleanup. Wait for the actual history/recovery transport,
then assert geometry with bounded polling. Do not use arbitrary sleeps. Prove
the held refresh was applied before checking reader geometry: wait for the
transcript's pending overflow-anchor state to clear after release, then let two
layout frames settle.

Cover a cached reopen with pending history refresh and delayed content layout.
Cover actual upward wheel/touch input during refresh and a disabled preference
with a saved position. Repeat the enabled reopen in expanded desktop Quick Chat.
Check the session ID stays the same and no user prompt is replayed.

The desktop project is `chromium`. The phone project is `mobile-chrome`.
These flows cover criteria `.16`, `.17`, `.19`, and `.20`, with `.1` and `.10`
as preservation controls. Geometry checks include the status footer and composer,
not only message presence in the DOM.

## Work orders

- [x] [Task 01: Preserve Quick Chat placement through restart](task-01-preserve-restart-placement.md)

The work order is sequential. No delegation is authorized.

## Verification results

Implementation and browser verification passed. `QuickChatMessageSurface` now
forwards `historyRefreshPending`, and `useFollowIntent` tracks the latest
measured content geometry separately from bounded pre-reflow evidence (the
pre-reflow baseline and recent peaks). A single scroll reconciliation can
therefore attribute movement to the immediately prior measurement or to an
earlier reflow state, covering growth followed by shrinkage in either
notification order as well as the original resize-first restoration. The unit
regressions fail on the previous source and pass with the repair.

Implementation checks:

- Focused unit/component suites: 155 tests passed (4 files).
- Desktop `quick-chat-restart-scroll.spec.ts`: 3 tests passed, including the
  active resume status inside the viewport, cached refresh settlement proven by
  the transcript's pending/cleared overflow-anchor state, reader wheel control,
  expanded width, and disabled saved position after restart.
- Mobile `mobile-quick-chat-restart-scroll.spec.ts`: 1 test passed with the
  phone full-height surface, the same causal refresh-settlement signal, touch
  reader control, and composer containment.
- Existing `quick-chat-resume-recovery.spec.ts` and
  `mobile-quick-chat-resume-recovery.spec.ts`: passed, preserving explicit
  recovery and session identity.
- Shared scroll regressions `auto-scroll-toggle.spec.ts`,
  `native-upward-intent.spec.ts`, and `mobile-auto-scroll-toggle.spec.ts`:
  passed.
- `pnpm run typecheck`, `pnpm run lint`, `python3 scripts/list-docs.py validate`
  (437 decisions and 1938 specifications), 36 specification-linter tests,
  `python3 scripts/lint-spec-files.py --all`, Playwright discovery across all
  three projects, and `git diff --check` passed.

Design-turn checks (historical):

- The four existing suites listed in Task 01 passed: 146 tests.
- `python3 scripts/list-docs.py validate` passed: 437 decisions and 1938 specifications.
- `python3 scripts/lint-spec-files.test.py` passed: 36 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `.github/scripts/pr-docs.cjs::validateCoverage` passed with the two planned
  runtime files supplied as prospective changes. No runtime file was edited.
- `git diff --check` passed. Only this plan package and the shared design changed.
- Both temporary test files were removed. No isolated instances were started.
- Browser checks were not run during the design turn; the implementation
  evidence above supplies the restart and spinner assertions.

## Risks

- A reflow correction can steal a deliberate reader position unless input guards remain intact.
- A stale correction can affect another session after tab replacement.
- Real browser notification order can differ from the controlled hook sequence.
- Resume can complete too quickly for a spinner assertion without a causal gate.
- Older pagination can change layout during entry and must retain its placement guard.

Completed companion packages remain historical records. This package links the
same active design without reopening their completed work orders.
