---
id: "01-passthrough-cancellation"
title: "Repair Quick Chat passthrough cancellation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.10
  - AC-UI-QUICK-TERMINAL-001.11
  - AC-UI-QUICK-TERMINAL-001.12
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 01: Repair Quick Chat Passthrough Cancellation

## Summary

Give the existing configurable Quick Chat action opt-in toggle semantics and protect a focused
agent passthrough xterm from Radix's Escape dismissal. Prove that Escape reaches the terminal while
the dialog stays open, and that the configured action remains a reliable close path.

## In scope

- Add and test the toggle option on `useQuickChatLauncher`.
- Enable the option for the global ordinary Quick Chat action only.
- Register a focus-scoped, unmodified Escape predicate from `PassthroughTerminal` only while its
  AttachAddon and terminal WebSocket are connected.
- Add desktop browser evidence for terminal delivery, dialog retention, and shortcut dismissal.
- Re-run the existing mobile touch-close scenario.

## Out of scope

- A new user setting or locale copy.
- Synthetic Escape writes or changes to agent cancellation APIs.
- Terminal behavior outside a guard-enabled dismissible surface.
- UI layout, pointer dismissal, or mobile composition changes.

## Acceptance

- The configured Quick Chat action closes an open shared dialog and keeps its sessions intact; all
  non-toggle launchers retain their current selection behavior.
- Unmodified Escape from a connected xterm helper textarea is forwarded on the agent terminal
  WebSocket and does not close Quick Chat; disconnected terminals and other focus targets retain
  existing escape behavior.
- Existing nested-widget and mobile touch-dismissal paths remain green.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-quick-chat-launcher.test.ts components/global-commands.test.tsx components/task/use-passthrough-terminal.test.ts components/quick-chat/quick-chat-modal.test.tsx
cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "passthrough Escape"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts -- --grep "opens from the home header and closes with the touch control"
```

## Files likely touched

- `apps/web/hooks/use-quick-chat-launcher.ts`
- `apps/web/hooks/use-quick-chat-launcher.test.ts`
- `apps/web/components/global-commands.tsx`
- `apps/web/components/global-commands.test.tsx`
- `apps/web/components/task/passthrough-terminal.tsx`
- `apps/web/components/task/use-passthrough-terminal.test.ts`
- `apps/web/e2e/helpers/ws-capture.ts`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`

## Dependencies

None.

## Risks

- Radix handles Escape in document capture before xterm; the guard must match the terminal's exact
  focus and modifier scope without stopping propagation.
- The E2E WebSocket observer must attach before the terminal socket opens and filter out the main
  JSON gateway plus unrelated terminal frames.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-QUICK-TERMINAL-001`, especially acceptance criteria `.10` through `.12`.
- `docs/specs/ui/system-design/quick-terminal.md`, section "Launcher toggle and terminal Escape
  routing".
- Existing Quick Chat escape-guard tests and terminal WebSocket capture patterns.

## Results

- Added opt-in toggle semantics to the shared Quick Chat launcher and enabled them only for the
  configurable global Quick Chat command. The launcher requests the mounted modal close lifecycle,
  falling back to the store action only when no handler is registered. The capture-phase command
  closes an open dialog before xterm can consume the configured chord.
- Registered a focused, unmodified-Escape guard from `PassthroughTerminal` only while its
  AttachAddon is installed on an open WebSocket. Radix keeps the dialog open while xterm forwards
  the same key as `\x1b` through `AttachAddon`.
- Extended the E2E WebSocket observer to capture AttachAddon's raw text frames in addition to the
  existing JSON gateway and binary terminal frames.
- RED evidence: the focused unit tests initially failed on the missing toggle, shortcut options,
  and terminal predicate; the desktop E2E initially failed because Escape closed the dialog.
- GREEN evidence:
  - `cd apps && pnpm --filter @kandev/web test -- --run hooks/use-quick-chat-launcher.test.ts components/global-commands.test.tsx components/task/use-passthrough-terminal.test.ts components/quick-chat/quick-chat-modal.test.tsx` (4 files, 41 tests passed)
  - `cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "passthrough Escape"` (1 Chromium test passed)
  - `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts -- --grep "opens from the home header and closes with the touch control"` (1 mobile-chrome test passed)
  - Targeted ESLint, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
