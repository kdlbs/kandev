---
id: "01-preserve-terminal-lifecycle"
title: "Preserve terminal resources and fence stale events"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.8
  - AC-UI-QUICK-TERMINAL-001.13
  - AC-UI-QUICK-TERMINAL-001.14
  - AC-UI-QUICK-TERMINAL-001.15
  - AC-UI-QUICK-TERMINAL-001.16
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
  - ../../specs/ui/system-design/quick-terminal-render-lifecycle.md
---

# Task 01: Preserve terminal resources and fence stale events

## Summary

Keep one Quick Terminal xterm and socket through unrelated parent renders.
Ignore disposed stream events while preserving existing startup, stop, detach, and late-result ownership.

## In scope

- Add the real-adapter/view integration regressions listed in the plan before production changes.
- Model a CONNECTING socket that emits an error during close, with a bounded loop guard in the test.
- Use `onStateChangeRef.current` in the stable Quick Terminal `startSession` callback.
- Resolve a deferred start after a callback replacement and assert the latest notification recipient.
- Pass the mount cancellation token to stream handlers and guard every side effect after cleanup.
- Retain old handlers in tests and invoke them after actual cleanup and a replacement mount.
- Preserve late detached-start identity publication, pending explicit close, StrictMode, and initial-input behavior.

The existing wrapper suite mocks the PTY view, so it cannot prove resource continuity.
Create `quick-terminal-render-lifecycle.test.tsx` with both real components and boundary mocks.
The temporary investigation reproduced six terminals after five callback-only renders.
Its desired-behavior assertion failed with two terminals instead of one after the first replacement.

## Out of scope

- Removing `startSession` or `initialInput` from the shared mount-effect dependencies.
- Modal subscription changes, descriptor-write deduplication, retry, reconnect, and keepalive.
- Backend APIs, process lifetime, descriptor persistence, or task passthrough terminals.
- New controls, layout, localized copy, or browser-test implementation owned by Task 02.

## Acceptance

1. Five ordinary callback-only renders and state-persistence renders retain one xterm, socket, and host-shell start, with no disposal before real cleanup.
2. Disposed error, close, binary output, exit, and initial-open callbacks have no effects, including on replacement resize or state.
3. The latest status handler receives deferred startup results, and all existing lifecycle and phone-input regressions pass.

## Implementation notes

Read the [stream-ownership design](../../specs/ui/system-design/quick-terminal-render-lifecycle.md#stream-ownership).
Set cancellation before socket close.
Avoid a blanket guard around `args.report`, because detached late starts must retain their identity notification.
Keep active stream error and exit controls so cancellation cannot disable normal status reporting.
If another parameter exceeds the five-parameter function limit, group related callbacks.

Use `/tdd` after the explicit implementation request.
Run the callback regression before the adapter correction.
Run stale-event regressions before their cancellation correction.
Retain both failure results as behavioral RED evidence.

## ASCII UI preview

Excerpt of [UI-01 and UI-02](plan.md#ascii-ui-preview), covering .15 and .16 with compatibility for .8 and .13:

```text
UI-01 desktop: floating shared dialog
[ Quick Chat                         Close ]
[ + ] [ Terminal 1* ]
[ existing output                          ]
[ $ partly typed command_                  ]

UI-02 phone: full-height shared dialog
[ Quick Chat                 Close ]
[ + | Terminal 1* | tabs ->         ]
[ existing output                  ]
[ $ partly typed command_          ]
[ Ctrl Shift ^C ^D Esc Tab ... ->   ]
[ on-screen keyboard               ]
```

The header and tabs remain fixed. Output retains its vertical scroll owner and input focus.
Phone controls retain horizontal scrolling, touch sizes, and existing keyboard clearance.
The correction removes render-driven clears and false connection errors.
Real connection errors retain their current status presentation.
Spacing and text are illustrative.

## Verification

Run from the repository root.
In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)` once.

```bash
(cd apps/web && pnpm exec vitest run components/quick-chat/quick-terminal-render-lifecycle.test.tsx components/quick-chat/quick-terminal-tab-view.test.tsx components/settings/pty-terminal-view.test.tsx components/settings/agent-login-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/quick-chat/quick-terminal-tab-view.tsx components/quick-chat/quick-terminal-tab-view.test.tsx components/quick-chat/quick-terminal-render-lifecycle.test.tsx components/settings/pty-terminal-view.tsx components/settings/pty-terminal-view.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check -- apps/web/components/quick-chat apps/web/components/settings docs/plans/quick-terminal-render-continuity
```

Task 02 owns browser proof, including screenshots of the retained phone surface.
These component tests establish deterministic browser-like cleanup errors without requiring Safari automation.

## Files likely touched

- `apps/web/components/quick-chat/quick-terminal-tab-view.tsx`
- `apps/web/components/quick-chat/quick-terminal-tab-view.test.tsx`
- `apps/web/components/quick-chat/quick-terminal-render-lifecycle.test.tsx` (new)
- `apps/web/components/settings/pty-terminal-view.tsx`
- `apps/web/components/settings/pty-terminal-view.test.tsx`
- This work order and `plan.md` for status and results.

## Dependencies

None. Existing backend client-key idempotency and descriptor APIs are sufficient.

## Risks

- A stale close callback can clear a newer `sessionIdRef` and silently disable resize.
- Nulling handler properties alone does not guard a callback already queued or captured.
- Suppressing detached late-start notifications can prevent an explicit close from stopping the correct PTY.
- Development StrictMode replay has deliberate extra setup and must retain its current ownership assertions.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/quick-terminal.md), especially .8 and .13 through .16.
- [Shared-tab design](../../specs/ui/system-design/quick-terminal.md).
- [Render lifecycle design](../../specs/ui/system-design/quick-terminal-render-lifecycle.md).
- Existing shared PTY mocks and lifecycle tests.
- `quick-chat-modal.tsx` and `use-quick-chat-modal.ts` for the callback/persistence chain.
- `pty-terminal-lifecycle.ts`, `PtyTerminalDialog`, and `AgentLoginDialog` for compatibility.
- `apps/web/AGENTS.md` and `/tdd`.

## Results

The regressions were run before the repair. The callback-latest-handler test failed because the deferred result went to the old handler. The disposed-stream test observed old error/output/exit effects after cleanup. The real-component continuity test observed six starts/resources across five callback-only renders, and the persisted-state plus CONNECTING-close case observed repeated starts.

The implementation stabilizes the Quick Terminal `startSession` adapter and stores the mount cancellation token in the shared PTY stream context. The four focused component suites passed: 29 tests total. This includes deferred-start latest-handler behavior, one xterm/socket/start over five renders, bounded CONNECTING-close behavior, stale callback fencing after replacement, live error/exit controls, detached late-start identity, existing cleanup, and agent-login compatibility.

Review follow-up strengthened the stale-stream case to rerender one mounted `PtyTerminalView` with a changed `startSession` callback. It captures the old generation's handlers, attaches `session-2` on the same component instance, then invokes the old error, close, binary output, exit, and open handlers. The replacement resize must still target `session-2`. The updated PTY suite passed 16/16 tests. As a mutation check, removing only the old `onclose` cancellation guard caused the replacement resize assertion to fail with zero resize calls.

Verification passed:

- `pnpm exec vitest run components/quick-chat/quick-terminal-render-lifecycle.test.tsx components/quick-chat/quick-terminal-tab-view.test.tsx components/settings/pty-terminal-view.test.tsx components/settings/agent-login-dialog.test.tsx` (29 tests).
- `pnpm run typecheck`.
- Focused Oxlint for the changed component source and test files.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
- `git diff --check` for the component and plan paths.

Task 02 adds fresh-build desktop and phone browser proof and is recorded separately.
