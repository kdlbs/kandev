---
id: "01-accept-before-preparation"
title: "Accept Quick Chat before preparation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.8
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.9
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.13
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-003.3
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Accept Quick Chat before preparation

## Summary

Allocate and return the ephemeral conversation before repository or agent startup.
Keep structured and terminal-backed opening-message ownership separate.
Persist asynchronous failure through existing recovery contracts.

## In scope

- Replace synchronous ordinary `IntentStart` with deferred `IntentPrepare`, then one background `IntentStartCreated`.
- Reuse normal task-create dispatch context and configured startup budget.
- Use `NoInitialPrompt` when no HTTP opening prompt was supplied. Retain backend-owned initial-submission receipts before acceptance.
- Keep legacy HTTP 200 fields, pre-session rollback, retained failures, and ownership guards.
- Return a machine-readable definitive rejection only for typed failures that happen before task allocation.
- Add a real-server regression that blocks preparation behind the accepted response.

## Out of scope

Creation UUIDs, new UI copy, configuration-chat redesign, global timeouts, and a worker queue.

## Acceptance

1. The real POST returns its task/session ID while worktree preparation is blocked. Its response survives a short server write deadline.
2. The background launch retains auth values and a finite startup deadline. Structured and terminal openings each have one delivery owner.
3. Post-allocation failures retain typed recovery and payload ownership. Stale failures, deletion, and capacity deferral preserve their existing guards.

## Verification

Start with `TestQuickChatAcceptance_ReturnsBeforePreparation` in a new
`quick_chat_acceptance_test.go`. Run it red against the synchronous handler.
Use the production handler and real orchestrator seam behind a preparation barrier.
Add `TestQuickChatAcceptance` cases for auth/cancellation, structured no-prompt
startup, terminal receipts, interrupted dispatch, and asynchronous failure.
Include a legacy structured-profile request with an explicit HTTP prompt.
Its prompt must remain backend-owned and must be delivered once.
Update existing retention assertions for the accepted-then-failed flow.
Verify the early failure transition is bound to the owning Quick Chat and
agent-start attempt, and that only typed pre-allocation validation/reference
errors carry the definitive rejection code.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers ./internal/orchestrator ./internal/orchestrator/executor -run 'TestQuickChatAcceptance|TestHTTPQuickChatOpeningPayload|TestHTTPConfigChatOpeningPayload|TestQuickChatFailureRetention|TestQuickChatLaunch' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- New `apps/backend/internal/task/handlers/quick_chat_acceptance_test.go`
- `apps/backend/internal/task/handlers/task_http_quick_chat_opening_test.go`
- `apps/backend/internal/task/handlers/quick_chat_failure_retention_test.go`
- `apps/backend/internal/orchestrator/quick_chat_launch_failure.go` and its tests
- `apps/backend/internal/orchestrator/session_launch.go` and focused tests
- Existing initial-submission and executor failure helpers only where required

## Dependencies

None. Read the startup amendment and the shipped task-create dispatch helpers.

## Risks

The initial-submission receipt excludes inline bytes by design.
Preserve its explicit replay-blocked outcome after an uncertain terminal dispatch.
Do not pass a request-cancelled context or capture `gin.Context` in a goroutine.

## Parallelism

`sequential`

## Inputs

- [Startup amendment](../../specs/tasks/system-design/quick-chat-opening-composer.md#startup-acceptance-amendment)
- [Retained failures](../../specs/tasks/system-design/task-launch-failure-recovery.md)
- `prepareStartAgentSession`, `dispatchTaskSession`, `launchStartCreated`, and `models.NewInitialPromptSubmission`

## Results

Implemented the two-phase Quick Chat launch. The handler returns the task/session identity after deferred session preparation, then dispatches one bounded `IntentStartCreated` after writing the response. Promptless requests set `NoInitialPrompt`; explicit HTTP prompts retain backend delivery and persist a bounded initial-submission receipt during prepare. Deferred failures keep the accepted session and receive typed recovery state. Early failure recording is bound to the owning Quick Chat and agent-start attempt and leaves competing or settled sessions intact. Typed pre-allocation validation and repository-reference failures return a definitive rejection code for the editable setup.

Verification passed:

- The final focused backend regression run passed across handlers, orchestrator, executor, SQLite, service, and backendapp. It includes `TestQuickChatPreparedStartPersistsEarlyFailure`, which verifies that a fresh session can claim its first deferred-start attempt when the metadata key is absent.
- `bash scripts/lint-go-changed` passed for the changed Go files with zero issues.
- `git diff --check`

PR review fixup revalidation on 2026-10-27:

- From `apps/backend`, `go test -trimpath -tags fts5 ./internal/task/handlers ./internal/orchestrator ./internal/orchestrator/executor -run 'TestQuickChatAcceptance|TestHTTPQuickChatOpeningPayload|TestHTTPConfigChatOpeningPayload|TestQuickChatFailureRetention|TestQuickChatLaunch' -count=1` passed.
- From `apps/backend`, `go test -race -trimpath -tags fts5 ./internal/task/handlers ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestQuickChatCreationIdentity|TestQuickChatStartFailureIgnoresCompetingStartingSession|TestQuickChatStartFailureDoesNotOverwriteReplacementWaitingSession|TestCommitQuickChatStartFailureRequiresCurrentOwner|TestPostgresCommitQuickChatStartFailureIfCurrentAttempt' -count=1` passed.
- From `apps/backend`, `go test -race ./internal/task/handlers -run 'TestHTTPStartQuickChat(ForwardsAutoTitleAndKeepsProvisionalTitle|WithoutAutoTitleDoesNotMarkPendingTitle)$' -count=1` passed after the full CI shard exposed unlocked async-test reads.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/quick_chat_launch_failure_test.go apps/backend/internal/task/handlers/quick_chat_creation.go apps/backend/internal/task/handlers/quick_chat_creation_identity_test.go apps/backend/internal/task/handlers/task_handlers.go apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/task_http_handlers_test.go apps/backend/internal/task/repository/sqlite/quick_chat_start_failure_cas_postgres_test.go` passed with zero issues; `make -C apps/backend build` succeeded.
- The PostgreSQL CAS regression is environment-gated. No local PostgreSQL DSN was configured, so its live database execution remains covered by CI.
- `git diff --check` passed.
