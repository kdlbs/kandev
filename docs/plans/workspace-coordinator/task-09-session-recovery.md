---
id: "09-session-recovery"
title: "Coordinator conversation recovers from an ended session"
status: pending
wave: 4
depends_on:
  - "06-copilot-wired"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-004
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-001.10
  - AC-COORDINATOR-COPILOT-004.6
system_design:
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/copilot-popover.md
---

# Task 09: Coordinator Conversation Recovers From An Ended Session (WP-4c)

## Summary

A coordinator whose agent fails to start could never recover, and the manager
could not see it. The conversation route reused a task whose session had
ended, and every message to it is rejected; the popover's recovery feedback
was unreachable because `automaticRecovery={false}` stops
`useSessionResumption` from ever setting an error. This work order makes the
route replace an ended conversation, and gives the popover its own
ended-session display with a Retry that opens the new conversation.

## In scope

- Backend: the conversation route's step 2
  ([copilot design](../../specs/coordinator/system-design/copilot.md#conversation-task)):
  a current task whose ensured session is `FAILED`, `CANCELLED` or
  `COMPLETED` is archived and the route creates a fresh task through steps
  3 to 7. Ported from commit `915c72185` on `coordinator/p1-integration`
  (`internal/coordinator/conversation.go`, with its test).
- Frontend: the popover's [Ended session](../../specs/coordinator/system-design/copilot-popover.md#ended-session)
  display in `ReadyBody` (`apps/web/app/coordinator/copilot/coordinator-copilot-body.tsx`):
  terminal detection from the store's session row, `SessionRecoveryFeedback`
  above the view with the row's `error_message` or the existing fallback,
  Retry wired to the open sequence's `retry`, the empty intro hidden while
  ended, and `QuickChatSessionView` keyed on the session id and `askKey`.
- E2E: replace the skipped `AC .004.6, residual` row in
  `apps/web/e2e/tests/coordinator/copilot.spec.ts` with a passing test.

## Out of scope

- Any change to `useSessionResumption`, `QuickChatSessionView`,
  `SessionRecoveryFeedback` or `EnsureSessionErrorBanner`: all are shared
  with the task page, mobile and the Settings chat.
- New translated copy: the banner's existing title, fallback detail and Retry
  label are reused.
- Showing an ended conversation's transcript after it is replaced, and
  resuming an ended session in place (requirements, Out of scope).

## ASCII UI preview

Desktop popover, session ended after the manager's first message:

```text
+-- Coordinator: Planner ---------------------- [x] +
| [about KAN-418 x]                                 |
| +-----------------------------------------------+ |
| | /!\ Couldn't start a session                  | |
| |     agent process exited: exec: "claude" ...  | |
| |     [Retry]                                   | |
| +-----------------------------------------------+ |
|  You: About KAN-418: Why is KAN-418 here?         |
|                                                   |
| +-----------------------------------------------+ |
| | Ask the coordinator...                  [Send]| |
| +-----------------------------------------------+ |
+---------------------------------------------------+
```

After Retry: the same popover on a new, empty conversation (intro and
"Try asking" suggestion, no banner, chip kept). The 390px layout is the same
stack at full width.

## Acceptance

- Reopening a conversation whose session is `FAILED`, `CANCELLED` or
  `COMPLETED` returns a new task and session and archives the old task;
  a non-terminal session is still reused; two racing opens over one ended
  task converge on one new task.
- While the popover holds an ended session, it shows
  `session-recovery-error` with the session's `error_message` (or the
  fallback) above the transcript; Retry opens the new conversation, the
  banner disappears and the empty state shows; the Coordinator lists stay
  usable throughout; no agent starts and no message is sent by Retry.
- The previously skipped Playwright row passes against a real backend
  session driven to an ended state, not a route interception.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'OpenConversation' -count=1
cd apps/web && pnpm test -- app/coordinator/copilot hooks/domains/coordinator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/copilot.spec.ts
```

Backend tests (`internal/coordinator/conversation_test.go`): one per
terminal state (new task id, old task archived, reference moved); a
`WAITING_FOR_INPUT` and a `CREATED` session are reused; `ArchiveTask`
failing still returns a new task; two concurrent opens over one ended task
return the same new task with exactly one new conversation task left
unarchived.

Component tests (`coordinator-copilot.test.tsx` or a sibling file): for each
of `FAILED`, `CANCELLED`, `COMPLETED` the banner renders with the row's
`error_message`, and with the fallback when `error_message` is empty; for
`CREATED`, `RUNNING`, `WAITING_FOR_INPUT` and a missing row no banner renders;
the empty intro is hidden while ended; Retry calls the open sequence once and
is disabled while it is `loading`; a 200 with a new `session_id` removes the
banner and remounts the view; a Retry that returns 502 shows the
"Could not open the conversation." row.

E2E (`copilot.spec.ts`, replacing the skipped row): a manager opens the
copilot, sends a message the mock agent answers, the test ends that real
session through the API (`apiClient.stopSession({ force: true })`, as
`tests/terminal/terminal-ended-session.spec.ts` does) and waits for a
terminal state; the popover shows `session-recovery-error` while a Needs you
item action stays clickable; Retry leads to a route response with a
different `task_id`, the empty intro, and no banner.

## Risks

- `COMPLETED` or `CANCELLED` replacing a long conversation loses its
  transcript from view; accepted and recorded under Out of scope.
- The store row must be live while the popover is open; the launcher's
  `useSession` subscription provides it. A regression there would hide the
  banner, which the component tests pin through a pushed state change.
