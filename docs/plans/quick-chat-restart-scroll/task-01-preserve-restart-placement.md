---
id: "01-preserve-restart-placement"
title: "Preserve Quick Chat placement through restart"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.1
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.10
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.16
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.17
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.19
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.20
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 01: Preserve Quick Chat Placement Through Restart

## Summary

Keep Quick Chat at the newest messages through history refresh, delayed layout,
and agent resumption. Preserve deliberate reader positions through the same events.
The [plan](plan.md) records two controlled failures and their reproduction sequence.

## In scope

- Forward the existing history-refresh state from Quick Chat to the native list.
- Preserve follow intent through resize-first native restoration and guard release.
- Add focused component/hook regressions before correcting production code.
- Add desktop and phone E2E evidence through a disposable backend restart.
- Keep the plan and work-order results accurate after implementation.

## Out of scope

Agent lifecycle policy, backend storage, prompt replay behavior, new controls,
new preferences, and transcript rendering redesign.

## Acceptance

1. A cached refresh remains provisional until history settles. Both diagnosed
   regressions fail before the fix and pass afterward.
2. Desktop floating, expanded desktop, and phone Quick Chat show the latest reply
   and active resume status after restart. The same conversation is retained.
3. Upward reader input, disabled auto-scroll, unread targets, explicit navigation,
   and session replacement preserve their existing scroll ownership.

## UI preview

See [the full previews](plan.md#ui-preview), views UI-01 and UI-02.
These descriptions follow the task's rich-output preview instruction.

- **UI-01, desktop:** tab strip, one transcript scroller, then fixed composer.
  The scroller ends at the latest reply and visible resume status after entry.
  Floating and expanded views share this behavior.
- **UI-02, phone:** app navigation opens the full-height Quick Chat dialog.
  Its tab strip and safe-area-aware composer surround one transcript scroller.
  The latest reply and status clear the composer. Upward touch pauses following.

The required structure maps to criteria `.16`, `.17`, `.19`, and `.20`.
Existing localized status text and control sizes remain authoritative.

## Implementation notes

Read `AGENTS.md`, `apps/web/AGENTS.md`, and the chat-specific guidance first.
Use `/tdd`, `/mobile-parity`, and `/e2e` in the current session.

The first regression extends the existing Quick Chat component test.
Set `historyRefreshPending` true while `messagesLoading` is false. Capture
`MessageList` props, then settle the refresh and assert both states propagate.

The second regression extends the existing native scroll harness.
Use a clamped 400px viewport with content growing from 1000px to 1300px.
Start at the old bottom, 600px, while automatic placement is blocked.
Deliver content resize callbacks, then simulate native restoration to 500px.
Deliver the scroll event without reader input, then release the blocker.
The expected final bottom is 900px. Current source leaves the view at 500px.
Include the opposite notification order and reader-input controls.

Update `useFollowIntent` without introducing another scroll controller.
Preserve reflow evidence until scroll reconciliation. Keep the write-only append
path and stale-session cancellation. Forward `historyRefreshPending` through
`QuickChatMessageSurface` without adding a new fetch or resume-specific effect.

Follow the plan's E2E matrix. Use `BackendContext.restart()` on the worker-owned
database. Hold and release actual recovery/history responses when needed to
observe pending states. Assert visible geometry before any manual scroll action.
Restore modified preferences and release all gates in `finally` or teardown.

## Verification

Run from the repository root. On a fresh worktree, install dependencies once:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run the focused unit/component suites after the red and green phases:

```bash
(cd apps/web && pnpm exec vitest run components/quick-chat/quick-chat-content.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-native-scroll.test.ts components/task/chat/use-chat-scroll-motion.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
```

Run desktop and phone E2E sequentially. The first command rebuilds current
assets and fixtures. The second reuses that unchanged build:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/chat/quick-chat-restart-scroll.spec.ts e2e/tests/chat/quick-chat-resume-recovery.spec.ts --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-quick-chat-restart-scroll.spec.ts e2e/tests/chat/mobile-quick-chat-resume-recovery.spec.ts --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The existing resume-recovery tests protect task/session identity and explicit
recovery alongside the new viewport tests. A missing or blocked browser test
leaves this work order incomplete. Record the exact command, result, and blocker.

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-content.tsx`
- `apps/web/components/quick-chat/quick-chat-content.test.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/use-chat-scroll-motion.ts` only if required by the shared correction.
- `apps/web/components/task/chat/use-chat-scroll-motion.test.tsx` if that hook changes.
- `apps/web/e2e/tests/chat/quick-chat-restart-scroll-helpers.ts`
- `apps/web/e2e/tests/chat/quick-chat-restart-scroll.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-restart-scroll.spec.ts`

## Dependencies

None. Reuse the existing native scroll controller and isolated backend fixtures.

## Risks

Geometry reconciliation must preserve genuine reader input. Late notifications
must not write to another session. Browser restart tests must observe pending
recovery through transport gates rather than timing assumptions.

## Parallelism

`sequential`

## Inputs

- [Active requirements](../../specs/ui/requirements/transcript-auto-scroll.md).
- [Quick Chat history and resize settlement](../../specs/ui/system-design/transcript-auto-scroll.md#quick-chat-history-and-resize-settlement).
- [Tasks resumption policy](../../specs/tasks/system-design/quick-chat-session-resumption.md).
- Existing `quick-chat-content.test.tsx`, `message-list-native.test.tsx`,
  `quick-chat-resume-recovery-helpers.ts`, and `e2e/fixtures/backend.ts`.

## Results

Implemented. `QuickChatMessageSurface` forwards `panelState.historyRefreshPending`
to `MessageList`. `useFollowIntent` tracks the latest measured content geometry
separately from bounded pre-reflow evidence (the baseline, interval peak, and
recent peaks). Reconciliation accepts movement explained by any retained
measurement, so resize-first restoration and growth followed by shrinkage
(with the shrink measurement arriving before or after native movement) keep
follow intent, including long resize sequences. A connection change settles an
in-flight cached refresh so provisional placement cannot remain pending.
Reader-input checks, disabled preferences, session cancellation, and the
guarded pending-resize correction are unchanged.

Regression tests added before the production repair:

- `quick-chat-content.test.tsx`: cached refresh readiness reaches the list with
  `messagesLoading` false.
- `message-list-native.test.tsx`: resize-first native restoration, growth then
  native shrink in both notification orders, repeated resizes beyond the recent
  retention window, scroll-first, reader cancellation, and disabled controls
  while placement is blocked.
- `use-session-messages.test.ts`: a connection change clears cached-entry
  history pending without losing the later stale response guard.
- E2E reply and status containment checks retry fresh geometry instead of
  asserting one transient frame.

Verification:

- `pnpm exec vitest run components/quick-chat/quick-chat-content.test.tsx
  components/task/chat/message-list-native.test.tsx
  components/task/chat/message-list-native-scroll.test.ts
  components/task/chat/use-chat-scroll-motion.test.tsx
  hooks/domains/session/use-session-messages.test.ts`: 195 passed.
- `pnpm run typecheck` and `pnpm run lint`: passed.
- `quick-chat-restart-scroll.spec.ts` on `chromium`: 3 passed.
- `mobile-quick-chat-restart-scroll.spec.ts` on `mobile-chrome`: 1 passed.
- `quick-chat-resume-recovery.spec.ts` and
  `mobile-quick-chat-resume-recovery.spec.ts`: passed.
- Shared scroll regressions `auto-scroll-toggle.spec.ts`,
  `native-upward-intent.spec.ts`, and `mobile-auto-scroll-toggle.spec.ts`:
  passed.
- Specification validation and lint, Playwright discovery, and
  `git diff --check`: passed.

The E2E specs prove the held latest-window response was applied before checking
reader geometry: the transcript carries the overflow-anchor override only while
`historyRefreshPending` is set, and the tests wait for that override to clear
plus two layout frames before asserting preservation on desktop and phone.
