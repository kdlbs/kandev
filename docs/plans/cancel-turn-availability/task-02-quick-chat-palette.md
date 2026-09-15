---
id: "02-quick-chat-palette"
title: "Scope Quick Chat palette cancellation"
status: pending
wave: 2
depends_on: ['01-composer-availability']
plan: "plan.md"
requirements:
  - REQ-UI-CANCEL-TURN-PROGRESS-001
acceptance_criteria:
  - AC-UI-CANCEL-TURN-PROGRESS-001.4
  - AC-UI-CANCEL-TURN-PROGRESS-001.9
  - AC-UI-CANCEL-TURN-PROGRESS-001.10
  - AC-UI-CANCEL-TURN-PROGRESS-001.11
  - AC-UI-CANCEL-TURN-PROGRESS-001.12
system_design:
  - ../../specs/ui/system-design/cancel-turn-availability.md
---

# Task 02: Scope Quick Chat palette cancellation

## Summary

Expose a localized cancel-only command for the active Quick Chat conversation.
Ensure the command cannot target an underlying task or an inactive chat.

## In scope

- Add `quick-chat-cancel-commands.tsx`, mounted by `QuickChatContent`, with explicit
  session ID, eligibility, and the existing `handleCancelTurn` callback.
- Reuse `buildSessionCommands` and `useRegisterCommands`. Scope registration to
  open Quick Chat, conversation kind, and matching active session ID.
- Suppress the task `session-cancel` entry while Quick Chat is open. Restore it
  on close without suppressing unrelated task commands.
- Apply the composer eligibility rule and guard dispatch while backend or
  optimistic cancellation is pending. Preserve cleanup across tab switching.
- Prove exact callback/request identity and no duplicate cancellation entries.

## Out of scope

Registering task/git/panel actions inside Quick Chat; terminal cancellation or
new palette state architecture; changing the command registry globally.

## Acceptance

1. Add a failing registry integration test named `targets only the active Quick
   Chat session over a running task`. Exercise actual source registration under
   `CommandRegistryProvider`, assert one cancel entry, and inspect its action.
2. Cover A-to-B chat switching, closing Quick Chat, unmount, terminal/setup/idle
   content, disconnected clarification, and pending cancellation. No stale
   callback, duplicate entry, or underlying task cancel may remain active.
3. Desktop and phone E2E open the command panel through their existing entry,
   invoke cancellation, and verify the exact session settles while the underlying
   task keeps running. Preserve the phone's visible composer cancel fallback.

## ASCII UI preview

UI-02, desktop and phone command result
([full preview](plan.md#ascii-ui-preview), criterion .11):

```text
Search: cancel
Agent
  Cancel turn -> active Quick Chat session
```

Idle/setup/terminal foreground content shows no session-cancel result.
Use the existing responsive command-panel composition and touch controls.
UI-01's composer remains the direct phone action; no keyboard is required.

## Verification

Run from the repository root, after Task 01's dependency installation.

```bash
(cd apps/web && pnpm exec vitest run components/quick-chat/quick-chat-cancel-commands.test.tsx components/session-commands.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/quick-chat/quick-chat-cancel-commands.tsx components/quick-chat/quick-chat-content.tsx components/session-commands.tsx e2e/tests/chat/quick-chat-cancel-palette.spec.ts e2e/tests/chat/mobile-quick-chat-cancel-palette.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/quick-chat-cancel-palette.spec.ts tests/chat/cancel-turn-availability.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-cancel-palette.spec.ts tests/chat/mobile-cancel-turn-availability.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

Existing files:

- `apps/web/components/quick-chat/quick-chat-content.tsx`
- `apps/web/components/session-commands.tsx`
- `apps/web/components/session-commands.test.tsx`

New files:

- `apps/web/components/quick-chat/quick-chat-cancel-commands.tsx`
- `apps/web/components/quick-chat/quick-chat-cancel-commands.test.tsx`
- `apps/web/e2e/tests/chat/quick-chat-cancel-palette.spec.ts`
- `apps/web/e2e/tests/chat/mobile-quick-chat-cancel-palette.spec.ts`

## Dependencies

Task 01: composer availability and eligibility wiring.

## Risks

Preserve clarification suppression, pending-state session identity, and direct
steering delivery. Palette sources do not deduplicate matching command IDs.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/cancel-turn-progress.md).
- [Design](../../specs/ui/system-design/cancel-turn-availability.md).
- [Plan and source-trace evidence](plan.md).

## Results

Pending. Run the regression before production changes and record its expected
failure. Then record each verification command and actual result after the fix.
