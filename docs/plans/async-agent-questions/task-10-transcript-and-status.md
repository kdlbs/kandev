---
id: "10-transcript-and-status"
title: "Add transcript history and accurate status"
status: completed
wave: 8
depends_on:
  - "09-chat-question-surface"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-007
  - REQ-TASKS-ASYNC-QUESTIONS-009
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-002.4
  - AC-TASKS-ASYNC-QUESTIONS-004.2
  - AC-TASKS-ASYNC-QUESTIONS-007.6
  - AC-TASKS-ASYNC-QUESTIONS-007.7
  - AC-TASKS-ASYNC-QUESTIONS-009.4
  - AC-TASKS-ASYNC-QUESTIONS-009.6
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 10: Add transcript history and accurate status

## Summary

Record each async question in the author's transcript, and make status labels accurate on every surface.
A parked session shows as needing input, and a running one does not.

## In scope

- The `agent_question_request` message type, a `transcript` outbox intent, and a deterministic message ID.
- `GET /api/v1/agent-questions/:id`, and a transcript card that reads current state with deduplicated reads.
- `pendingActionsBySessionQuery` returning `clarification` for a `parked` or `release_requested` gate, and `session.pending_action_changed` on gate transitions.
- Per-question waiting state, loaded once per session. This replaces the per-session `HasParkedWait` read, and it replaces the hard-coded `waiting: false` in the MCP view.
- Option labels and distinct answered, rejected, withdrawn, and stale states in history, chat, and the Inbox.
- Agent-facing wording: a distinct heading for rejected outcomes, and correct `parent_question_status` metadata.

## Out of scope

- A task-list indicator for open questions on running tasks.
- Lifecycle E2E flows (Task 12).

## Acceptance

1. Each request has one transcript entry across delivery retries, and it shows the current outcome after a reload.
2. A parked session projects `clarification`, a running session with open requests projects nothing, and blocking clarification behavior is unchanged.
3. In a session waiting on question A, question B does not show as waiting, and history shows labels rather than option IDs.

## ASCII UI preview

The full view is [UI-07 in the plan](plan.md#ascii-ui-preview).

```text
UI-07
? Asked: Which value belongs in the missing API field?  Open
? Asked: Database choice                 Answered: PostgreSQL
? Asked: Heading wording                 Rejected: Out of scope
? Asked: Retry budget                    Withdrawn: Found in the docs
? Asked: Deploy region                   Closed: parent task changed
```

## Verification

Run from the repository root. In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'TestAsyncQuestions|PendingAction' -count=1)
(cd apps/backend && go test -trimpath ./internal/clarification ./internal/mcp/handlers -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'TestAsyncQuestions|Clarification' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/agent-question-message.test.tsx components/needs-you-inbox lib/needs-you-inbox)
(cd apps/web && pnpm run typecheck && pnpm run lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
bash scripts/lint-go-changed <changed go files>
```

Required new tests: `TestAsyncQuestionsTranscriptEntryIsIdempotent`, `TestAsyncQuestionsParkedGateProjectsPendingAction`, `TestAsyncQuestionsOpenRequestProjectsNoPendingAction`, and `TestAsyncQuestionsInboxWaitingIsPerDependency`.

PostgreSQL evidence for the changed pending-action query requires `KANDEV_TEST_POSTGRES_DSN`.

## Files likely touched

- **New:** `apps/web/components/task/chat/messages/agent-question-message.tsx`, `agent-question-message.test.tsx`
- **New:** `apps/backend/internal/task/questions/transcript_delivery_test.go`, `apps/backend/internal/task/repository/sqlite/async_question_pending_action_test.go`
- `apps/backend/internal/task/models/models.go`, `apps/web/lib/types/http.ts`
- `apps/backend/internal/task/questions/delivery.go`, `http_handlers.go`, `service.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/backendapp/async_questions.go`
- `apps/backend/internal/clarification/inbox_handlers.go`
- `apps/backend/internal/mcp/handlers/async_questions.go`
- `apps/web/components/task/chat/message-renderer.tsx`
- `apps/web/components/needs-you-inbox/async-question-row.tsx`
- `apps/web/src/locales/*/`

## Dependencies

Task 09.

## Risks

- `session.pending_action` already affects chat state. `agentDisconnected` uses it in `task-chat-panel.tsx`, and `use-chat-panel-state.ts` reads it. Trace these first. A parked session must not imply a blocking panel that has no messages.
- The pending-action query has a turn-based rule that quarantines old input. The gate branch must also respect terminal sessions.
- A message type added without renderer support would show as an unknown row. Cover the renderer and the transcript's message filters.
- Delivery kinds now include a non-queue intent, so the adapter must dispatch by kind before any queue admission.

## Parallelism

`sequential`

## Inputs

- [Task chat question surface](../../specs/tasks/system-design/async-agent-questions.md#task-chat-question-surface-remediation).
- [Operator attention](../../specs/tasks/system-design/async-agent-questions.md#operator-attention-remediation).
- `pending_action_projection.go` and `publishSessionPendingActionChanged` for revisioned projections.

## Results

Implemented deterministic transcript entries, current-status lookup, parked-gate pending-action projection, per-question waiting state, and terminal outcome labels. Added the transcript message type to the chat visibility filter after E2E exposed that it was being dropped before rendering.

Passed:

- `go test -trimpath -race ./internal/task/questions -count=1`
- SQLite async question and pending-action tests
- Clarification, MCP handler, and focused orchestrator suites
- Focused async question and Inbox Vitest suites: 217 tests passed
- Typecheck, lint, i18n check, and i18n ratchet
- SQL guard and changed-file Go lint: `0 issues`

PostgreSQL pending-action coverage was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.
