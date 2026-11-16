---
id: "01-remove-expiration"
title: "Remove automatic Quick Chat expiration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-EXPIRATION-003
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.1
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.2
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.3
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.4
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.5
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.6
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.7
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.8
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.9
  - AC-TASKS-QUICK-CHAT-EXPIRATION-003.10
system_design:
  - ../../specs/tasks/system-design/quick-chat-expiration-policy.md
---

# Task 01: Remove Automatic Quick Chat Expiration

## Summary

Remove automatic Quick Chat expiration and its unused entry points.
Keep Stop, confirmed tab deletion, and conversation restoration unchanged.
Retain compatibility with durable resource-cleanup jobs created before this removal.

## In scope

- Remove startup registration and the daily expiration worker.
- Remove the service helper, repository expiry queries, and interface entries.
- Remove obsolete expiry assertions and preserve independent manual-delete coverage.
- Add a behavioral startup/restart regression for an aged ordinary Quick Chat.
- Exercise committed and uncommitted legacy cleanup intents.
- Preserve ordinary Quick Chat data during agent-profile cleanup while continuing configuration-chat cleanup.
- Reconcile affected specifications and public documentation.

## Out of scope

- Settings, frontend controls, new endpoints, feature flags, and localization.
- Live database edits, restoration, deployment, commit, push, or PR creation.
- Changes to host storage, tracked tasks, task sessions, Office, automations, or integration policies.
- Changing Stop into deletion or removing existing closure confirmation.

## Acceptance

1. An aged Quick Chat survives isolated startup, maintenance, and backend restart with its conversation and owned resources.
2. Manual stop preserves history. Confirmed deletion still removes only the selected chat and its owned resources.
3. Agent-profile cleanup preserves ordinary Quick Chats and deletes configuration chats using the `config_mode` discriminator.
4. Legacy committed cleanup can finish, while an undeleted chat's uncommitted intent cannot cause resource removal.

## Verification

Use TDD. First add `TestQuickChatManualLifetimeAcrossStartupAndRestart` under `internal/backendapp`.
Seed an ordinary Quick Chat older than seven days in an owned test database before real startup.
Assert persisted task/session/message identity and owned workspace/history survival after startup and restart.
The current startup expiration must cause a behavioral failure before its removal.
Do not substitute an assertion about source strings or a missing method for this regression.

Add `TestQuickChatLegacyCleanupCompatibility` cases under the task service.
Preserve the stored trigger format and existing preparation reconciliation.
Retain manual-delete assertions when expiry-specific test callers disappear.

Add a repository regression proving profile cleanup preserves ordinary Quick Chat task/session data
while continuing to delete a configuration chat.

From `apps/backend`:

```bash
go test ./internal/backendapp -run 'TestQuickChatManualLifetime'
go test ./internal/task/service -run 'Test(QuickChatManualLifetime|QuickChatLegacyCleanup|DeleteQuickChat|DeleteRepositoryQuickChat)'
go test ./internal/task/repository/sqlite -run 'Test(DeleteTask|DeleteEphemeralTask|DeleteEphemeralTasksByAgentProfile|DeliveryRetentionDeletionRetirement|TaskMetadataWatcher)'
```

From `apps/web`:

```bash
pnpm e2e:run --project chromium tests/chat/quick-chat.spec.ts tests/chat/quick-chat-cross-device.spec.ts -- --grep 'resumes a restored session after backend restart|closing a chat on one device removes it from the other'
pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-repository.spec.ts tests/chat/mobile-quick-chat-resume-recovery.spec.ts -- --grep 'deletes dirty repository quick chat with consent|existing Resume recovers interrupted Quick Chat on a phone'
```

Run managed E2E commands sequentially with fresh builds.
If dependencies are absent, run `pnpm install --frozen-lockfile` once from `apps`.
Do not use the developer's live database or instance.

From the repository root:

```bash
bash scripts/lint-go-changed apps/backend/internal/backendapp/main.go apps/backend/internal/backendapp/quick_chat_manual_lifetime_test.go apps/backend/internal/task/service/service_tasks.go apps/backend/internal/task/service/service_test.go apps/backend/internal/task/service/quick_chat_agent_history_test.go apps/backend/internal/task/repository/interface.go apps/backend/internal/task/repository/sqlite/task.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go` and new `quick_chat_manual_lifetime_test.go`.
- Remove `apps/backend/internal/task/service/quick_chat_expiration.go`.
- `apps/backend/internal/task/service/service_tasks.go`, `service_test.go`, and `quick_chat_agent_history_test.go`.
- Existing task-resource preparation/recovery tests for legacy job compatibility.
- `apps/backend/internal/task/repository/interface.go`, `sqlite/task.go`, and `sqlite/session.go`.
- `sqlite/task_selection_coverage_test.go`, `prompt_history_test.go`, `prompt_history_cleanup_test.go`, and `agent_delivery_retirement_test.go`.
- Keep `task/models/resource_cleanup.go` and the worker's stored-trigger handling compatible.
- `docs/specs/tasks/requirements/quick-chat-expiration.md` and the manual lifetime pair.
- Quick Chat native-history, coordinator, startup, and cleanup specifications that name removed symbols or an active sweep.
- `docs/public/developer-tools.md`, `feature-status.md`, and other lifetime wording identified in the plan.

## Dependencies

None. Execute sequentially in the primary conversation.

## Risks

Keep legacy trigger decoding even after its producer disappears.
Preserve manual deletion coverage while removing obsolete expiration tests.
Do not change other workers as part of this removal.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/quick-chat-expiration-policy.md)
- [System design](../../specs/tasks/system-design/quick-chat-expiration-policy.md)
- Existing Quick Chat close actions and manual deletion tests.
- Existing prepared cleanup reconciliation and stored job trigger format.

## Results

Behavioral RED before removal: the aged Quick Chat startup regression failed because the old
startup expiration deleted the chat. After removal, the same isolated startup/restart test passes
and preserves the task, session, message, workspace resource, and Claude history file.

The legacy cleanup tests pass for both a committed deletion and an undeleted chat with a prepared
legacy intent. The committed cleanup completes; the live chat and its resource remain intact.

PR review follow-up keeps ordinary Quick Chats out of agent-profile cleanup while preserving
configuration-chat cleanup. Existing Quick Chat recovery documentation now distinguishes failures
before and after session allocation and describes reconciliation of superseded starts.

Checks passed:

- `go test -trimpath ./internal/backendapp -run 'TestQuickChatManualLifetime' -count=1`
- `go test -trimpath ./internal/task/service -run 'Test(QuickChatManualLifetime|QuickChatLegacyCleanup|DeleteQuickChat|DeleteRepositoryQuickChat)' -count=1`
- `go test -trimpath ./internal/task/repository/sqlite -run 'Test(DeleteTask|DeleteEphemeralTask|DeliveryRetentionDeletionRetirement|TaskMetadataWatcher)' -count=1`
- `make build`
- Desktop Chromium Quick Chat E2E: 2 passed.
- Mobile Chrome Quick Chat E2E: 2 passed.
- Changed Go package lint: 0 issues.
- Specification catalog and full specification lint passed.
- Public-doc tests: 63 passed; public-doc validator accepted 55 pages.
- `git diff --check` passed.
- `go test -trimpath ./internal/task/repository/sqlite -run 'Test(DeleteTask|DeleteEphemeralTask|DeleteEphemeralTasksByAgentProfile|DeliveryRetentionDeletionRetirement|TaskMetadataWatcher)' -count=1` passed.
- `make build` passed after the profile-cleanup selector change.
- Changed Go package lint reported 0 issues; specification catalog and full specification lint passed again.
