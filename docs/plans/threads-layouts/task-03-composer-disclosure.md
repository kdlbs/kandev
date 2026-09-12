---
id: "03-composer-disclosure"
title: "Disclose the existing composer"
status: done
wave: 3
depends_on:
  - "02-render-grid"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-005
  - REQ-UI-THREADS-DECK-004
acceptance_criteria:
  - AC-UI-THREADS-DECK-005.1
  - AC-UI-THREADS-DECK-005.2
  - AC-UI-THREADS-DECK-005.3
  - AC-UI-THREADS-DECK-005.4
  - AC-UI-THREADS-DECK-005.5
  - AC-UI-THREADS-DECK-005.6
  - AC-UI-THREADS-DECK-005.7
  - AC-UI-THREADS-DECK-005.8
  - AC-UI-THREADS-DECK-005.9
  - AC-UI-THREADS-DECK-005.10
  - AC-UI-THREADS-DECK-004.5
  - AC-UI-THREADS-DECK-004.6
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 03: Disclose the existing composer

## Summary

This completed order records the initial implementation. The user's later
animation refinement is implemented and verified separately in
[Task 06](task-06-presentation-polish.md).

Reclaim routine composer space in Threads while preserving normal editing,
submission, recovery, and cancellation. Use one session-scoped controller for
pointer/keyboard disclosure; touch keeps the normal composer visible.

## In scope

- Add an optional Threads-only context, disclosure hook/wrapper, CI-only
  collapsed footer, and explicit Collapse inside the revealed composer.
- Report draft/focus/upload/send and owned native overlay activity from its
  existing owner. Keep the editor mounted while collapsed; make hidden content
  inert and prevent hidden autofocus. Reset timers/holds on session changes.
- Force visible required actions/recovery; preserve cancellation pending,
  draft persistence, model/mode, queue/steering, and send-failure behavior.
- Preserve native transcript follow/reading position during footer allocation
  changes and bound long footer content in grid/phone heights.

## Out of scope

New draft storage, new send/cancel paths, changing other chat hosts' default
behavior, pinning offscreen session subscriptions, and Display preference UI.

## Acceptance

1. Pointer dwell/exit, keyboard tile reveal, and a visible touch composer satisfy
   the disclosure state table, including drafts, portals, failures, operations,
   recovery, required questions/permissions, and cancellation feedback.
2. Revealing/hiding preserves the same editor/session state and transcript
   anchor without changing sibling bounds; offscreen release/restoration works.
3. Users can reply, select existing composer controls, and stop a running
   agent from desktop and phone, with normal full-task-page behavior preserved.

## ASCII UI preview

UI-03 excerpt; see [full tile states](plan.md#ui-03-grid-tile-hovered-or-actively-composing).

```text
COLLAPSED                  REVEALED
+----------------------+   +----------------------+
| Task B       [Open]  |   | Task B       [Open]  |
| transcript scrolls   |   | transcript scrolls   |
|                      |   | [status / queue]     |
|                      |   | [draft editor]       |
|                      |   | [tools]       [Send] |
| [CI]*                |   | [Collapse]           |
+----------------------+   +----------------------+
```

Outer tile bounds remain equal. A required action forces the right-hand
surface open; long footer content scrolls within its bounded allocation.
Only an applicable CI popover remains collapsed, with no other footer controls
or reserved space. UI-05 phone always uses the inline revealed region above
the keyboard, while the deck still has one conversation and 44px touch actions.
Explicit desktop collapse preserves the draft and focuses the tile; keyboard
Enter or a new pointer entry reopens it. These previews cover
all assigned composer criteria; exact spacing is illustrative.

## Verification

Use /tdd. Pure timing tests use fake timers. Seed long transcripts for browser
anchoring; prove actual viewport positions, not DOM visibility alone.
New hook/component/E2E files below are planned.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/use-composer-disclosure.test.ts components/task/chat/composer-disclosure.test.tsx components/task/chat/chat-input-container.test.tsx components/task/chat/use-chat-input-container.test.ts components/task/chat/transcript-auto-scroll.test.ts components/task/chat/clamped-scroll-restore.test.ts components/threads/threads-board.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/use-composer-disclosure.ts components/task/chat/composer-disclosure.tsx components/task/chat/chat-input-area.tsx components/task/chat/chat-input-container.tsx components/task/chat/use-chat-input-container.ts components/task/chat/use-chat-input-state.ts components/task/task-chat-panel.tsx components/threads/thread-conversation.tsx components/threads/thread-column.tsx components/threads/threads-board.tsx --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-composer-disclosure.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-composer-disclosure.spec.ts -- --retries=0)
git diff --check
```

Run focused existing tests for any additional modified overlay/scroll owner,
recording those commands in Results. Required browser cases include a
portaled control after pointer exit, attachment-only draft, failed send,
pending action while collapsed, revealed Stop pending, visible touch composer,
offscreen re-entry, and a full task page with the default composer.
Record emulator versus physical keyboard evidence separately.

## Files likely touched

- `apps/web/components/threads/{threads-board,thread-column,thread-conversation}.tsx`
- `apps/web/components/task/task-chat-panel.tsx` and its native footer if needed.
- New `apps/web/components/task/chat/use-composer-disclosure.ts`,
  `composer-disclosure.tsx`, and their tests.
- `chat-input-area.tsx`, `chat-input-container.tsx`,
  `use-chat-input-container.ts`, `use-chat-input-state.ts`,
  `chat-status-bar.tsx`, `queued-ghost-list.tsx`, and controlled native overlay
  owners needed for their existing open-change callbacks.
- Native transcript scroll modules only if their existing resize path fails
  the focused anchor test; retain their shared ownership.
- `apps/web/src/locales/*/threads.json` (shared chat catalog only if copy is
  actually shared).
- New `apps/web/e2e/tests/task/threads-composer-disclosure.spec.ts` and
  `mobile-threads-composer-disclosure.spec.ts`; shared setup from Task 02.

## Dependencies

Task 02, which supplies stable tile geometry and effective view fields.

## Risks

Portals are not DOM descendants of the tile. File pickers and native uploads
can outlive pointer/focus changes. Plugin instances must stay mounted, but
their opaque internal activity is not a disclosure hold. Required actions use selected-session
identity, not a sibling's task-wide summary. Manual-collapse override must
preserve drafts without allowing a pointer compatibility event to reopen it.

## Parallelism

sequential

## Inputs

- [Disclosure requirements](../../specs/ui/requirements/threads-conversation-deck.md#req-ui-threads-deck-005-composer-disclosure)
- [Disclosure and geometry design](../../specs/ui/system-design/threads-conversation-deck.md#composer-disclosure)
- Current `ChatFooter`, `ChatInputArea`, `ChatInputContainer`, draft hooks,
  cancellation UI, transcript scroll helpers, and their nearby tests.

## Results

Done, 2026-09-11. The plugin question is resolved.

The earlier recording/owned-plugin hold could not be reported by its
owner: voice recording lives in the separate `kdlbs/kandev-plugin-voice`
repository. `PluginComposerCapability` in `apps/web/lib/plugins/types.ts`
exposes only `insertText`, `focus`, and `submit`; `PluginComposerSlotProps`
has no plugin activity or overlay callback. `ChatInputPluginActions` and
`PluginSlot` forward/render the plugin without observing its internal state.
The [voice extraction contract](../../specs/plugins/requirements/voice-extraction.md)
forbids putting voice-specific ownership back into core.

The user answered: “the whole composer should go. we should leave only the CI
popover”. Hide plugin buttons along with all non-CI composer controls; retain
the mounted editor/plugin tree and its capability so hiding does not cancel
operations. Do not infer recording state or add an activity API. The earlier
Reply/Stop row is removed. Keyboard focus on the tile reveals the composer.

Phone/coarse-pointer layouts keep the normal composer visible without changing
the saved preference. This fallback was communicated as an implementation
assumption to preserve access without hover or a persistent Reply control.
Requirements, system designs, and previews are synchronized before code edits.

Implemented the optional session-scoped disclosure context and native activity
reporting. All non-CI footer regions become hidden and inert without unmounting
the editor/plugins. Existing native focus and plugin focus reveal before focus.
The bounded footer retains 80px for the transcript. The existing native scroll
owner now observes height-only viewport changes; it preserves bottom-follow
without moving history or overriding a frozen transcript.

RED/GREEN evidence: timing, explicit focus/inert, native draft/recovery/upload
holds, and viewport-follow tests failed before their implementations. Browser
tests exposed missing cancellation-pending ownership and an entry timer that
could undo explicit collapse. Both were fixed at their native owners; the
timer regression failed before the fix and passes afterward. Touch test setup
now waits for drawer closure before resizing and respects immediate submission
of a single-choice answer.

Validation passed:

- The seven listed unit targets plus `transcript-viewport-resize.test.ts`,
  both `chat-input-area.test.ts`/`.tsx`, `use-chat-input-state.test.ts`,
  `queued-ghost-list.test.tsx`, `queued-ghost-pin.test.tsx`,
  `dynamic-route-recovery.test.tsx`, and both
  `lib/plugins/composer-capability.test.ts`/`.tsx`: 218 tests across 16 files.
  After the final timer fix, the two disclosure test files passed 14 tests,
  including the new explicit-collapse race regression.
- `pnpm run typecheck`; scoped ESLint across all changed composer/scroll,
  task-panel, and thread-column/board modules; `pnpm run i18n:check`;
  `pnpm run i18n:ratchet`; `git diff --check`.
- `pnpm run build:e2e`, then the listed managed browser commands with
  `--no-build` and retries disabled: 6 desktop tests and 2 mobile tests passed.
  Coverage includes CI-only/zero-height collapse, keyboard, drafts, native
  menus, attachments, required questions/permissions, send rejection/retry,
  pending cancellation, history/follow/frozen scroll, and full-task defaults.
- Rendered desktop CI-only/revealed captures were inspected. Phone/tablet
  captures cover visible touch composers, navigation, and a 393x500 viewport.
  This is browser emulation, not physical-device keyboard verification.
