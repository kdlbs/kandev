---
id: "02-recover-keyed-creation"
title: "Recover a keyed Quick Chat creation"
status: done
wave: 2
depends_on: ["01-accept-before-preparation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-COMPOSER-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.13
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.14
  - AC-TASKS-QUICK-CHAT-COMPOSER-001.15
system_design:
  - ../../specs/tasks/system-design/quick-chat-opening-composer.md
---

# Task 02: Recover a keyed Quick Chat creation

## Summary

Bind an optional creation UUID to the authenticated caller and workspace.
Reuse task external-ID uniqueness and settlement.
Expose exact, authorized creation state, and resume an explicit keyed POST retry
for the same retained Quick Chat without another task or session allocation.

## In scope

- Extend the ordinary Quick Chat request, response, route registration, and status lookup.
- Add the reserved internal external-ID namespace and protect it from public create/release.
- Serialize same-token POSTs. Resume an exact unsettled task found by the keyed
  Quick Chat preflight; keep a `CreateTaskOutcomeFoundUnsettled` loser read-only.
- Resolve settled and archived outcomes before mutable-field validation or other side effects.
- Project creation identity through authorized list, boot, and lifecycle paths where applicable.
- Keep absent-token legacy requests and older rows compatible.

## Out of scope

The draft aggregate-creation coordinator, new tables, post-deletion server tombstones,
status-GET repair, browser rendering, and configuration creation.

## Acceptance

1. Concurrent same-process POSTs with one token serialize and return one retained
   task/session. An explicit same-token POST that resolves an unsettled Quick Chat
   in its preflight resumes missing work, reuses any existing session, settles
   before dispatch, and dispatches only a `CREATED` session once. A `CreateTask`
   `FoundUnsettled` outcome remains read-only.
2. A read-only status lookup returns only the exact authorized ordinary-chat creation. Foreign, ambiguous, incomplete, deleted, and lookup-error outcomes remain explicit.
3. Settlement precedes dispatch. Legacy fields remain compatible. Public routes cannot create or release the reserved namespace.

## Verification

Add `TestQuickChatCreationIdentity` cases in a focused new handler test file.
Use a real SQLite repository for concurrent uniqueness, not only an in-memory fake.
Count task, session, claim, and dispatch effects separately.
Include a changed-payload retry, an unsettled task without a session, settlement
failure, explicit deletion before dispatch, and mixed healthy/failed chats.
Add service prefix and list/boot visibility cases beside their existing suites.

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/backendapp -run 'TestQuickChatCreationIdentity|TestQuickChatAcceptance|TestQuickChatFailureRetention|TestHTTPListQuickChatSessions|Test.*ReservedExternalID|Test.*QuickChat.*Authz' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_handlers.go`
- New `apps/backend/internal/task/handlers/quick_chat_creation_identity_test.go`
- `apps/backend/internal/task/handlers/quick_chat_list_handlers_test.go`
- `apps/backend/internal/task/service/external_id.go` and reserved-prefix tests
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/quick_chat_sessions.go` and focused tests
- Existing task DTO/lifecycle projection and boot Quick Chat projection files
- `apps/backend/internal/backendapp/boot_state_quick_chat_authz_test.go`

## Dependencies

Task 01. Re-read the current `CreateTaskResult` and `SettleExternalID` APIs.
Do not treat the draft creation coordinator as implemented.

## Risks

A found task can be unsettled or can lack a session. Status lookup never repairs
it. Only the reserved Quick Chat POST adapter may resume an exact unsettled
creation it resolved before `CreateTask`, under the per-creation handler lock.
Do not resume the `CreateTask` `FoundUnsettled` loser path.
Detect conflicting preexisting namespace holders without adopting their state.
The token lifetime ends with the retained task and browser submission.

## Parallelism

`sequential`

## Inputs

- [Stable creation identity](../../specs/tasks/system-design/quick-chat-opening-composer.md#stable-creation-identity)
- `service/external_id.go`, `CreateTaskResult`, and current task-create found-response behavior
- Existing SQLite external-ID concurrency tests and Quick Chat authorization tests

## Results

Implemented keyed ordinary Quick Chat creation using the existing external-ID
uniqueness and settlement contract. Added exact authorized status lookup,
same-token retries, and creation identity projection through list, boot, and
lifecycle DTOs. Public create/release paths reject the reserved namespace.

Verification passed:

- The final focused backend regression run passed across handlers, orchestrator, executor, SQLite, service, and backendapp, including concurrent identity reuse, route registration, reserved namespace, caller scoping, list projection, lifecycle projection, and Quick Chat authorization cases.
- `bash scripts/lint-go-changed` passed for the changed Go files with zero issues.
- `git diff --check` passed.

PR review fixup revalidation on 2026-10-27:

- From `apps/backend`, `go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/backendapp -run 'TestQuickChatCreationIdentity|TestQuickChatAcceptance|TestQuickChatFailureRetention|TestHTTPListQuickChatSessions|Test.*ReservedExternalID|Test.*QuickChat.*Authz' -count=1` passed, including interrupted preparation recovery, settled-session reuse, and concurrent same-token serialization.
- From `apps/backend`, `go test -race -trimpath -tags fts5 ./internal/task/handlers ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestQuickChatCreationIdentity|TestQuickChatStartFailureIgnoresCompetingStartingSession|TestQuickChatStartFailureDoesNotOverwriteReplacementWaitingSession|TestCommitQuickChatStartFailureRequiresCurrentOwner|TestPostgresCommitQuickChatStartFailureIfCurrentAttempt' -count=1` passed.
- From `apps/backend`, `go test -race ./internal/task/handlers -run 'TestHTTPStartQuickChat(ForwardsAutoTitleAndKeepsProvisionalTitle|WithoutAutoTitleDoesNotMarkPendingTitle)$' -count=1` passed after the full CI shard exposed unlocked async-test reads.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/quick_chat_launch_failure_test.go apps/backend/internal/task/handlers/quick_chat_creation.go apps/backend/internal/task/handlers/quick_chat_creation_identity_test.go apps/backend/internal/task/handlers/task_handlers.go apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/task_http_handlers_test.go apps/backend/internal/task/repository/sqlite/quick_chat_start_failure_cas_postgres_test.go` passed with zero issues; `make -C apps/backend build` succeeded.
- The PostgreSQL CAS regression is environment-gated. No local PostgreSQL DSN was configured, so its live database execution remains covered by CI.
- `git diff --check` passed.
