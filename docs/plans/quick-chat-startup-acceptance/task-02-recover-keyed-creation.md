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
Expose exact, authorized creation state without another allocation or launch.

## In scope

- Extend the ordinary Quick Chat request, response, route registration, and status lookup.
- Add the reserved internal external-ID namespace and protect it from public create/release.
- Resolve found outcomes before mutable-field validation or other side effects.
- Project creation identity through authorized list, boot, and lifecycle paths where applicable.
- Keep absent-token legacy requests and older rows compatible.

## Out of scope

The draft aggregate-creation coordinator, new tables, post-deletion server tombstones,
automatic repair of incomplete creations, browser rendering, and configuration creation.

## Acceptance

1. Concurrent or repeated same-token requests return one retained task/session. Found outcomes perform no preparation, claims, mutation, or dispatch.
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

A found task can be unsettled or can lack a session.
Do not repair it from a lookup or a duplicate create.
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
