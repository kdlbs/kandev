---
id: "01-preserve-initial-placement"
title: "Preserve initial placement until completion"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.5
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.7
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.10
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.11
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.12
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.13
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.14
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.15
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.16
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.17
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.19
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.20
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 01: Preserve initial placement until completion

## Summary

Keep initial placement pending until the correct owner applies it. Reconcile
ordinary cached entry after refresh and content growth without moving a reader
who deliberately navigated elsewhere.

## In scope

- Session-scoped provisional and final phases with and without Dockview tokens.
- Explicit deferred/applied/transferred outcomes and release notifications.
- Session identity for layout restores when required to reject stale ownership.
- Visible geometry, observer cleanup, cancellation, and pagination guards.
- Follow intent across prompt submission, startup, streaming, and footer resize.
- Small upward gestures with animations on, off, and OS reduced motion.

## Out of scope

The latest-message button, backend ordering, and preference changes.

## Acceptance

- Ordinary refresh and temporary-blocker regressions fail before the correction and pass afterward.
- Explicit targets, valid restores, unread dividers, disabled positions, and reader gestures keep ownership.
- Desktop and phone follow a send from bottom, keep the running indicator visible, and pause after any deliberate upward movement.

## TDD evidence

In `message-list-native.test.tsx`, add `reconciles ordinary cached entry after
refresh without an environment token` and `retries deferred initial placement
after a temporary owner releases`. Use the existing NativeScrollManagementHarness.
Use a clamped scroll setter so height growth exposes stale pixel positions.
Add hidden-to-visible, session replacement, failed refresh, delayed row resize,
programmatic lock release, and user-cancellation cases. Do not replace valid
same-transcript restore assertions with unconditional bottom expectations.

Add `pauses after a 30px upward wheel with motion off` and
`does not force a reader to bottom on work start`. Temporary tests of these
cases failed against the current hook. Use clamped metrics in permanent tests.
Also cover a send from bottom, status-only footer growth, composer shrink,
paused readers during late startup, downward return to bottom, touch, keyboard,
and scrollbar gestures. Test motion enabled, disabled, and reduced-motion paths.

## ASCII UI preview

UI-01: Task Chat entry, desktop and phone. The shared transcript is the only
vertical scroll owner. Desktop keeps its Dockview surroundings. Phone keeps the
full-height Chat tab, fixed header, composer, and existing safe-area clearance.

```text
Current failure            Corrected entry         Reader above latest
+--------------------+     +--------------------+  +--------------------+
| Oldest message     |     | Recent conversation|  | Older conversation |
| Earlier history   |     | Latest assistant   |  | Reader's position  |
| ...                |     | reply              |  | ...                |
+--------------------+     +--------------------+  +--------------------+
| Composer           |     | Composer           |  | [Jump to latest]   |
+--------------------+     +--------------------+  | Composer           |
                                                  +--------------------+
```

The action sits outside the scroller above the composer. It disappears at the
bottom and for empty history. During refresh, cached history keeps its intended
position. Cold empty history retains the existing loading state. Phone uses a
44px minimum hit target. Desktop uses the existing compact density. No additional
sheet or navigation step is needed. Exact spacing is illustrative.

Criteria: `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.14`, `.16`, `.17`, and `.18`.

The full preview is in [the plan](plan.md#ascii-ui-preview). This work order owns
the corrected entry column. Task 02 owns the latest-message button.

## Verification

From the repository root, install workspace dependencies once if absent.
Run the regression tests before production changes and record the expected RED.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/message-list-native.test.tsx components/task/chat/message-list-native-scroll.test.ts components/task/chat/transcript-auto-scroll.test.ts components/task/chat/use-chat-scroll-motion.test.tsx components/task/chat/chat-scroll-motion.test.ts lib/state/dockview-scroll-preserve.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/auto-scroll-toggle.spec.ts tests/chat/unread-divider.spec.ts tests/session/session-layout.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on each changed TS/TSX file from `apps/web`. If new helper
files or test suites are extracted, include them in the recorded commands.

## Files likely touched

- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/transcript-auto-scroll.ts`
- `apps/web/components/task/chat/use-chat-scroll-motion.ts`
- `apps/web/components/task/chat/chat-scroll-motion.ts`
- `apps/web/components/task/chat/transcript-viewport-resize.ts`
- `apps/web/components/task/chat/message-list-footer.tsx` and `messages/agent-status.tsx` only if footer integration requires changes.
- `apps/web/lib/state/dockview-scroll-preserve.ts`
- `apps/web/lib/state/dockview-store.ts` (only if restore identity is needed)
- Matching unit tests listed in Verification.
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
- `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`

## Dependencies

None.

## Risks

Preserve the write-only append path, WebKit-safe offset, and existing chat-motion
behavior. Do not add a permanent retry loop or globally disable anchoring.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/transcript-auto-scroll.md), criteria listed in frontmatter.
- [Design](../../specs/ui/system-design/transcript-auto-scroll.md), initial placement and interaction boundaries.
- Existing `NativeScrollManagementHarness`, WS response-hold helper, and completed task-switch continuity package.

## Results

Implemented session-scoped placement completion, deferred retries, session-owned
layout restoration, and follow-intent tracking with motion enabled or disabled.
Added actual desktop wheel and phone touch E2E coverage. The focused unit suite
(74 tests) and web typecheck passed. The required Chromium suite had 22 passes;
its unrelated terminal marker test passed on targeted rerun. All seven mobile
auto-scroll E2E tests passed. The temporary coordinator trace and the prior
investigation remain recorded in the plan.

Review follow-up: a controlled-RAF test joins
`preserveChatScrollDuringLayout` with the native placement coordinator. A
same-session restore that completes before hidden-panel activation keeps its
non-bottom offset. Different-session and unapplied restores still allow normal
initial placement.


### Hosted live-follow observation follow-up, 2026-10-06

Run `37460626206`, shard 11 job `112264886425`, observed a 27-pixel downward
layout change while measuring a 30-pixel upward wheel. Moving the baseline after
hover alone still failed a local zero-retry repetition. The fixture now holds
real incoming notifications after the agent's START marker, keeps request
responses live, and confirms the real tail was held before the wheel action.
It releases every retained frame after observing upward movement, and in cleanup.
The later tail must preserve the reader position within three pixels. This
separates layout changes caused by new output from the actual wheel movement
without editing client state, changing production scroll behavior, increasing
deadlines, or loosening either scroll assertion.

- Initial combined targeted run: the hover-only repair failed one of three scroll cases, confirming it was insufficient.
- Targeted run with notification ordering: all six scroll/continuation cases passed in 1.1 minutes with retries disabled.
- Final full-suite and hosted receipts are recorded below when complete.

- Final exact-source desktop check: `cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/session/transient-turn-runtime-continuity.spec.ts tests/chat/auto-scroll-toggle.spec.ts -- --retries 0 --trace=retain-on-failure`: all sixteen cases passed in 5.4 minutes, including native retry exhaustion and the positive held-tail observation. No retry was exercised. Full web ESLint, focused ESLint, and the web typecheck passed.
