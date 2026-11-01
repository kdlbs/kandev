---
id: "09-chat-question-surface"
title: "Answer async questions from the task chat"
status: completed
wave: 7
depends_on:
  - "06-wait-and-authority-recovery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-001
  - REQ-TASKS-ASYNC-QUESTIONS-007
  - REQ-TASKS-ASYNC-QUESTIONS-009
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-001.2
  - AC-TASKS-ASYNC-QUESTIONS-007.2
  - AC-TASKS-ASYNC-QUESTIONS-007.4
  - AC-TASKS-ASYNC-QUESTIONS-007.5
  - AC-TASKS-ASYNC-QUESTIONS-009.1
  - AC-TASKS-ASYNC-QUESTIONS-009.2
  - AC-TASKS-ASYNC-QUESTIONS-009.3
  - AC-TASKS-ASYNC-QUESTIONS-009.5
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 09: Answer async questions from the task chat

## Summary

Add a strip above the composer where an operator can see and answer open async questions, with the Inbox on or off.
This closes the production gap: operators can answer and get notified without the Inbox.

## In scope

- A task-scoped `GET /api/v1/agent-questions` read covering operator requests across sessions, forwarded child requests, and requests this task forwarded.
- Task and parent identity in the `task.question.changed` payload.
- A store slice and `use-task-async-questions` hook with workspace and generation guards.
- The strip, placed in `ComposerFooterAllocation`, using `AsyncQuestionForm`.
- The phone flow, reusing the full-height answer surface.
- Forwarded requests in the parent's chat, naming the child task.
- Question notifications on create and forward.
- Locale strings in all catalogs.

## Out of scope

- Transcript entries and pending-action state (Task 10).
- Answer delivery E2E flows (Task 12).
- Inbox changes, beyond sharing the form.

## Acceptance

1. With the Inbox flag off, an open request is visible and answerable from the author's chat, and task scope never returns an unforwarded parent request.
2. Answering a running author's question does not cover the composer, and a concurrent winner closes the row and discloses its outcome.
3. A create or forward raises one notification, and a retried create raises none.

## ASCII UI preview

The combined views are in [the plan](plan.md#ascii-ui-preview): UI-04, UI-05, and UI-06.

```text
UI-04 collapsed
| ...transcript scrolls...                                 |
|----------------------------------------------------------|
| ? 2 open questions, agent continues work          [Show] |
|----------------------------------------------------------|
| [Composer                                         Send ] |

UI-05 phone
| ? 2 open questions   > | -> full-height answer surface
| [Composer        Send] |
```

## Verification

Run from the repository root. In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./internal/task/questions ./internal/backendapp ./internal/gateway/websocket -count=1)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-task-async-questions.test.tsx components/task/chat/async-question-strip.test.tsx components/needs-you-inbox)
(cd apps/web && pnpm run typecheck && pnpm run lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
bash scripts/lint-go-changed <changed go files>
```

Required new tests: `TestAsyncQuestionsTaskScopeIncludesForwarded`, `TestAsyncQuestionsTaskScopeHidesUnforwarded`, `TestAsyncQuestionNotificationOncePerRequest`, and `TestAsyncQuestionNotificationSkipsUnforwardedParent`.

Use `/mobile-parity` for the phone flow.

## Files likely touched

- **New:** `apps/web/components/task/chat/async-question-strip.tsx`, `async-question-strip.test.tsx`
- **New:** `apps/web/hooks/domains/session/use-task-async-questions.ts`, `use-task-async-questions.test.tsx`
- **New:** a store slice under `apps/web/lib/state/slices/`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/needs-you-inbox/async-question-row.tsx`, `async-question-form.tsx`
- `apps/web/lib/api/domains/agent-questions-api.ts`, `apps/web/lib/types/session-events.ts`
- `apps/web/src/locales/*/`
- `apps/backend/internal/task/questions/http_handlers.go`, `http_handlers_test.go`, `service.go`, `types.go`
- `apps/backend/internal/task/repository/sqlite/async_questions.go`
- `apps/backend/internal/backendapp/async_questions.go`, `async_questions_test.go`, `gateway.go`

## Dependencies

Task 06, because the strip must reflect stale and withdrawn closures.

## Risks

- `ComposerFooterAllocation` also hosts the blocking panel and manages reserved heights. Test the strip alongside an active blocking clarification.
- `async-question-row.tsx` is written for the Inbox. Extract shared pieces rather than duplicating drafts and submit logic. Module-level drafts must keep one question's draft shared across both surfaces.
- `handleSemanticOccurrence` was designed around blocking pending IDs. Confirm that it deduplicates a question-scoped occurrence ID before relying on it.
- The chat strip and Inbox refresh on the same event, so one change can cause two reads. That is acceptable if each stays bounded.

## Parallelism

`sequential`

## Inputs

- [Task chat question surface](../../specs/tasks/system-design/async-agent-questions.md#task-chat-question-surface-remediation).
- [Operator attention](../../specs/tasks/system-design/async-agent-questions.md#operator-attention-remediation).
- `useNeedsYouInboxWsRefresh` for event coalescing, and `task-chat-panel.tsx` for composer placement.

## Results

Implemented the task chat question strip, shared answer form, forwarded-child provenance, live invalidation, and phone answer surface. The strip remains available when the Needs-you Inbox is disabled.

Passed:

- `go test -trimpath ./internal/task/questions ./internal/backendapp ./internal/gateway/websocket -count=1`
- Focused async question and Inbox Vitest suites: 217 tests passed
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`
- Mobile async question E2E: 2 tests passed
- Changed-file Go lint: `0 issues`
