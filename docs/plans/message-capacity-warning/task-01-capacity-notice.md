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
(cd apps/backend && go test -trimpath ./internal/task/handlers ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers)
(cd apps && pnpm --filter @kandev/web exec vitest run components/task/chat)
(cd apps/web && e2e/scripts/run-e2e.sh --host --project chromium -- tests/session/message-turn-capacity.spec.ts)
(cd apps/web && e2e/scripts/run-e2e.sh --host --project mobile-chrome -- tests/session/mobile-message-turn-capacity.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Acceptance evidence

- **AC-AGENTS-SESSION-CEILING-003.9:** In both `message-turn-capacity.spec.ts`
  and `mobile-message-turn-capacity.spec.ts`, “keeps one accepted follow-up
  pending, then delivers it after release” checks the collapsed warning and
  delivery. “Keeps the accepted follow-up pending while Auto-run is off”
  checks the alternate delivery instructions. In
  `message_capacity_admission_test.go`,
  `TestMessageCapacityAdmission_QueuesOrdinaryMessageAtFullCapacity` and
  `TestMessageCapacityAdmission_SteerFallbackQueuesInsteadOfFailing` cover
  both refusal paths.
- **AC-AGENTS-SESSION-CEILING-003.10:** The delivery scenario in both browser
  specs checks reload and disappearance. In `queued-ghost-list.test.tsx`,
  “shows one polite notice above the collapsed queue and clears it on session
  switch” checks session isolation. In `service_capacity_admission_test.go`,
  `TestSnapshotProjectsKnownCapacityAdmissionReason` verifies the persisted
  marker and omission for ordinary entries.

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
- Review fixup covers paused managed conversations and restores the automatic
  delivery hint when the conversation resumes. The focused chat suite passed.

| Command listed above | Result |
| --- | --- |
| `go test -trimpath` for the three affected packages | Passed all three packages. |
| `vitest run components/task/chat` | Passed 2,464 tests before review. |
| `run-e2e.sh --project chromium` for message-turn capacity | Passed both cases. |
| `run-e2e.sh --project mobile-chrome` for message-turn capacity | Passed both cases. |
| `python3 scripts/list-docs.py validate` | Passed. |
| `python3 scripts/lint-spec-files.py --all` | Passed. |
| `git diff --check` | Passed. |

The notice joins only non-empty, fully translated sentences. Queue handler
tests verify that client-supplied admission reasons are ignored. The shared
capacity query refreshes every ten seconds while a warning is visible so an
administrator's limit change can appear without reloading chat.
