---
id: "01-capacity-notice"
title: Show the capacity admission reason in chat
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-SESSION-CEILING-003
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-003.9
  - AC-AGENTS-SESSION-CEILING-003.10
system_design:
  - ../../specs/agents/system-design/message-capacity-warning.md
---

# Show the capacity admission reason in chat

Implement AC-AGENTS-SESSION-CEILING-003.9 and .10 under the
[session ceiling design](../../specs/agents/system-design/message-capacity-warning.md).

1. In message_handlers.go, mark both capacity refusal paths. Cover admission,
   rollback and replay in the handler tests. Check ordinary queue admission
   and ensure client metadata cannot assert the reason. Extend the existing
   queue projection and web entry type to expose the optional reason.
2. In the shared queued-ghost-list composer surface, render one localized
   status while the selected queue contains a marked entry. Keep the message
   bubble and delivery behavior intact. Verify Auto-run off, empty queue,
   ordinary queue, and session switch in focused component tests.
3. Extend the desktop and mobile message-turn-capacity Playwright cases. Send
   under the one-session ceiling, read the notice with the panel closed,
   reload, release the filler session, and assert disappearance and one delivery.
   Check phone containment and Auto-run off copy.

## Verification commands

```sh
(cd apps/backend && go test ./internal/task/handlers ./internal/orchestrator/messagequeue)
(cd apps && pnpm --filter @kandev/web exec vitest run components/task/chat)
(cd apps/web && e2e/scripts/run-e2e.sh --host --project chromium -- tests/session/message-turn-capacity.spec.ts)
(cd apps/web && e2e/scripts/run-e2e.sh --host --project mobile-chrome -- tests/session/mobile-message-turn-capacity.spec.ts)
```

## ASCII UI preview

UI-01. Desktop and phone chat after Send, with the queue collapsed.

```text
[clock] Message queued because the session limit was reached.
    The current session limit is 1.
    It will send automatically when capacity is available.
[1 queued]
[Compose a message ...]
```

Use the shared yellow warning surface in the existing composer region and transcript scroll owner. Phone text wraps
above the same queue control. The explanation requires no tap or hover.

## Implementation results

- The server records capacity refusal with the queue write and projects the
  reason in session snapshots and live updates. Merges retain the reason.
- Desktop and mobile chat read the same visible queue entries. The status
  appears above the queue control and keeps the Threads composer visible.
- Frontend verification passed for all 2,464 chat tests. The two touched
  suites passed again after fixture cleanup (73 tests). Type checking,
  localization, formatting and focused lint passed.
- Go tests passed for task handlers, the message queue and orchestrator handlers.
- The desktop browser regression reproduced the missing warning with the old
  backend. Both Chromium cases and both mobile Chrome cases passed with the
  rebuilt backend, including reload, Auto-run off and single delivery.
- The yellow warning and effective limit passed 92 focused frontend tests and
  all four desktop and mobile browser cases. The Tailscale test chat showed
  the enforced limit of 1 while the saved setting was 5.
