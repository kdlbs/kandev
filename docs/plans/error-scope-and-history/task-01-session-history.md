---
id: "01-session-history"
title: "Retain session failure entries"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.9
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.8
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Retain session failure entries

## Summary

Persist and render each accepted session failure as one chronological message. Preserve its body and details after recovery and later output.

## In scope

- Reuse failure admission, stable message identity, and stamp/execution fences. Persist bounded marker metadata before terminal publication.
- Make duplicate same-stamp delivery idempotent. Failed successor attempts receive separate entries.
- Remove message deletion after later activity and state-based body hiding. Separate pending, resolved, and superseded controls from history.
- Migrate bootstrap cards into ordinary rows. Remove only error-specific prepend/reveal branches from detail, preview, simple Chat, and Quick Chat.
- Preserve archived-task guards, provider-specific remediation, confirmation, cancellation, and workspace-only semantics.
- Add localized historical/pending labels in all five catalogs. Generate Traditional Chinese with `pnpm run i18n:zh-hant`.

## Out of scope

Shared task projection, provider hang repair, and timeout changes.

## Acceptance

1. Same-stamp replay creates one persisted entry. Recovery and later output preserve it after reload, with no stale actions.
2. Older history insertion and new messages obey ordinary scroll rules. No recovery-specific forced placement or duplicate banner remains.
3. Desktop and phone preserve details and valid recovery actions, including automatic recovery followed by output.

## ASCII UI preview

UI-01: Desktop task, shared failure plus an independently recovered session error.

```text
+--------------------------------------------------------+
| Task title / workflow                                  | fixed
| ! Workspace preparation failed. [Recovery details]      | shared
+--------------------------------------------------------+
| Session A | Session B | Plan | Pull request | Files      | tabs
+--------------------------------------------------------+
| Earlier conversation                                 ^ |
| ! Session resume failed. 10:18                        | |
|   Loading timed out. [Recovery details]                | | scrolls
|   Recovered.                                          | |
| Agent: I resumed work...                              v |
+--------------------------------------------------------+
| Message input                                          | fixed
+--------------------------------------------------------+
```

UI-02: Phone task Chat, same two independent failures.

```text
+--------------------------------+
| < Task title       Session A v | fixed task chrome
| ! Workspace preparation failed |
| [Recovery details]             | shared on every view
+--------------------------------+
| Earlier messages             ^ |
| ! Session resume failed      | |
| Loading timed out.           | | transcript scrolls
| Recovered. [Details]         | |
| Agent: I resumed work...     v |
+--------------------------------+
| Message input                  | safe-area clearance
| Chat | Plan | Files | More     | existing navigation
+--------------------------------+
```

UI-03: Session entry states, inside either transcript.

```text
Unresolved: ! Resume failed. [Resume] [Recovery details]
Pending:    ! Resume failed. Resuming... [Details]
Recovered:  ! Resume failed. Recovered. [Details]
Older:      ! Resume failed. [Details]   (no stale actions)
```

Expanded session details wrap inline. Phone actions stack with 44-pixel targets.
A failed new attempt appends a separate error entry. Same-stamp updates retain position.

UI-04: Shared details, desktop dialog / phone inset bottom drawer.

```text
+--------------------------------+
| Workspace preparation       X  |
| Affected repository: owner/repo|
| Safe cause and bounded details |
| [Valid recovery action]        |
+--------------------------------+
```

The phone drawer has safe-area clearance, an internally scrolling body, and 44-pixel controls.
The desktop dialog uses compact controls. Closing details does not clear the shared alert.
Views map to recovery criteria 006.4/6/7/8/9 and task criteria 002.4/5/6/7.
Fixed versus scrolling regions, scope, order, and retained history are requirements.
Copy and spacing are illustrative and must use localized strings and existing tokens.
The two errors coexist only because they represent different failures.

See the [combined plan](plan.md#ascii-ui-preview). Task 01 implements the views within its scope.

## Verification

Run from the repository root. Install workspace dependencies once before the first package command if this checkout lacks them.
Use `/tdd` for changed logic and `/e2e` for browser work. First establish the regression against current behavior.

```bash
(cd apps/backend && go test ./internal/orchestrator ./internal/orchestrator/executor ./internal/task/repository/sqlite)
(cd apps/web && pnpm exec vitest run hooks/processed-message-filtering.test.ts components/task/chat/messages/action-message.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/message-list-native-scroll.test.ts components/task/task-chat-panel.launch-error.test.tsx components/quick-chat/quick-chat-session-view.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/launch-failure-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts)
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go` and existing failure/resolution tests.
- `apps/backend/internal/orchestrator/executor/executor_launch_failure*` and `apps/backend/internal/task/repository/sqlite/` fenced metadata/message operations.
- `apps/web/hooks/processed-message-filtering.ts` and its test.
- `apps/web/components/task/chat/messages/action-message*`, `session-bootstrap-recovery-card.tsx`, and `message-list-{shared,native,native-scroll}*`.
- `apps/web/components/task/task-chat-panel.tsx`, `simple/task-chat.tsx`, and `components/quick-chat/quick-chat-session-view.tsx`.
- Existing tests named in verification. Add `action-message.test.tsx` if absent.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`.
- Existing desktop/mobile launch-failure E2E files.

## Dependencies

None.

## Inputs

Read both owning specs linked from the plan and the September 14 scope decision.
Use the existing launch-recovery tests as the fixture pattern.

## Risks

Preserve failure identity, sanitized details, existing recovery guards, and message ordering under reversed delivery.
The shared repository can contain other edits. Do not revert unrelated changes.

## Parallelism

`sequential`

## Results

Complete on September 14, 2026. Session failures are persisted as chronological, stamp-scoped messages. Same-stamp writes are idempotent, resolved entries retain their message and details, and later agent output no longer removes or hides them. The accepted bootstrap terminal path now produces the durable entry with safe bootstrap details and existing recovery choices, and Office failures use the same session-history producer. Profile-specific pre-agent failures retain their session scope. Bootstrap state admission requires an execution-fenced repository commit; when that commit succeeds but the transcript write fails, the executor retries the same idempotent message identity. Recovery-specific transcript reveal and prepend behavior was removed while ordinary older-history anchor restoration remains.

Validation passed:

- Backend orchestrator, executor, SQLite, and status-summary tests passed.
- Focused frontend tests for processed-message filtering, action messages, native scrolling, task chat, Quick Chat, session recovery, and mobile layout passed.
- Frontend typecheck, lint, i18n checks, and Vite build passed.
- Real bootstrap admission, Office failure production, stale retirement fencing, and reload-retained history regressions passed.
- Chromium and Mobile Chrome launch-recovery suites passed with 4 and 3 tests respectively.
- The bootstrap repair regression and the profile-specific session ownership regression passed in the full orchestrator suite.
