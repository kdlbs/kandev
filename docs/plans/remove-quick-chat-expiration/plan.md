---
created: 2026-11-16
status: done
requirements:
  - REQ-TASKS-QUICK-CHAT-EXPIRATION-003
system_design:
  - ../../specs/tasks/system-design/quick-chat-expiration-policy.md
legacy_specs: []
---

# Implementation Plan: Remove Automatic Quick Chat Expiration

## Overview

Remove the startup and daily expiration producer and its unused deletion queries.
Quick Chats remain available until explicit deletion.
Stop preserves the conversation. Confirmed tab closure deletes it through the existing lifecycle.

This package replaces the unimplemented opt-in proposal.
There is no new Storage setting, HTTP endpoint, policy store, frontend control, or localization work.

The task system owns the [requirement](../../specs/tasks/requirements/quick-chat-expiration-policy.md)
and [system design](../../specs/tasks/system-design/quick-chat-expiration-policy.md).
One sequential work order delivers removal, regression coverage, and documentation.

## Scope

- Remove automatic expiration for ordinary Quick Chats.
- Preserve existing manual stop, tab closure confirmation, and conversation restoration.
- Preserve canonical deletion and supported local agent-history cleanup.
- Preserve ordinary Quick Chat task data during agent-profile cleanup while retaining configuration-chat cleanup.
- Retain compatibility with already committed cleanup jobs from older versions.
- Leave all other cleanup policies separate.

Restoration, live data changes, deployment, and changes to other cleanup policies are excluded.

## Current cleanup audit


These observations describe the running instance on 16 November 2026, before this package is implemented.

| Scope | Current behavior and evidence |
| --- | --- |
| Ordinary Quick Chat | Unconditional seven-day idle expiry in `task/service/quick_chat_expiration.go`; startup and daily execution. Deletes the backing task, all sessions/messages, owned resources, and supported local agent history. |
| Configuration Chat | Excluded from the expiration query by config-mode metadata. Explicit deletion retains its normal lifecycle. |
| Tracked task | No general age-based deletion. `auto_archive.go` checks workflow-step auto-archive thresholds every five minutes. No current `workflow_steps` row has a positive threshold. Archive keeps task/session history and starts runtime/workspace cleanup. |
| Sessions inside tracked tasks | No general age-based row deletion. Stop, idle suspension, and reconciliation can change runtime state without removing the conversation. Explicit session deletion removes its conversation; parent task/workspace deletion can cascade. |
| Host storage | Live `storage_maintenance` settings enable a six-hour schedule. Workspace grace is seven days; quarantine retention is 24 hours. Go cache and Docker cleanup are enabled. Dependency and temporary-artifact cleanup are off. This policy reclaims resources, not task conversation rows. |
| Tool detail and change history | Live `tool_payload_retention.policy.enabled` is false. Its configured age is three months. Compaction can remove stored tool detail and historical code-change content without deleting task/session identities or user/assistant messages. |
| Office run metadata | Missing `office_run_retention` settings use enabled defaults: 30 days, floor 50 per owner, six-hour interval. `office/retention/store.go` deletes terminal run bookkeeping and satellites, not `tasks` or `task_sessions`. |
| Automation checkouts | `automation/run_retention.go` keeps ten recent eligible task checkouts per automation. It preserves task rows and transcripts. |
| Generated integration tasks | GitHub/GitLab watches have separate cleanup policies for their generated issue/review tasks. Those policies can delete the generated task and its conversations; they are not the ordinary-task inactivity policy. |
| Delivery and diagnostic evidence | Bounded delivery replay/deduplication records and diagnostic logs have their own retention. Those records are distinct from persisted task conversation messages. |

The incident deleted `PRs to Review`, `Issues`, and `Find unassigned GitHub work` at 10:37 UTC on 16 November.
Their cleanup jobs have trigger `quick_chat_expire` and state `succeeded`.
The source log is `/root/.kandev/logs/backend-logs-2026-11-16-000009.log`, beginning at line 37319.
The database backup `kandev-v0.101.0-17-g6a6c83c1477-20261114T103259Z.db` contains their 2,797 inline message records.
Database inspection used read-only connections. No application data was restored or edited.

## Technical approach

Removed `StartQuickChatExpirationLoop` from `backendapp/main.go` and deleted its service file.
Removed `deleteExpiredQuickChatTask` and the repository expiry methods and interface entries.
Deleted expiry-only helpers and tests while preserving independent manual-deletion guarantees.

Keep the stored `quick_chat_expire` trigger and durable worker handling for old jobs.
An undeleted task with an uncommitted prepared intent remains protected by preparation reconciliation.
No newly selected old chat can enter deletion.

The existing Quick Chat close actions already perform confirmed task deletion.
Stop and modal dismissal preserve the conversation. No frontend implementation change is required.
Desktop and phone use the existing chat surfaces and manual actions.

Agent-profile cleanup uses `config_mode` metadata to retain ordinary Quick Chat data while
continuing to remove configuration chats.

Reconciled the native-history, coordinator, startup, cleanup, and cascade specifications.
Retired the sweep-specific criterion without reusing its identifier.
Updated public Quick Chat lifetime wording in `docs/public/developer-tools.md`,
`docs/public/feature-status.md`, `docs/public/tasks-and-workflows.md`, and `docs/public/canvases.md`.
The README, operations guide, and screenshot catalog had no affected claims.

## Tests

All criterion suffixes refer to `AC-TASKS-QUICK-CHAT-EXPIRATION-003`.

| Criteria | Evidence |
| --- | --- |
| `.1`, `.2` | Behavioral startup/restart regression with a Quick Chat older than seven days. Its task, sessions, messages, workspace, and native history remain. |
| `.3`, `.4`, `.8` | Existing manual Stop/resume, restart restoration, confirmed close, and phone consent flows. Canceling closure retains the chat. |
| `.5` | Existing manual deletion tests for native history, prompt-sequence cleanup, resource ownership, and delivery retirement. |
| `.6` | Changes remove the expiration producer and API rather than introduce settings machinery. No new UI or endpoints are present. |
| `.7` | Legacy cleanup worker tests cover committed deletion and an uncommitted intent for a still-live task. |
| `.9` | Existing separate cleanup policies remain intact. `.10` is covered by the repository regression that preserves ordinary Quick Chats while deleting configuration chats during profile cleanup. |
| `.10` | Repository regression proves profile cleanup preserves an ordinary Quick Chat task, session, and message while continuing to delete a configuration chat. |

## E2E tests

Reuse the desktop restart case in `tests/chat/quick-chat.spec.ts`.
Reuse confirmed cross-device closure in `tests/chat/quick-chat-cross-device.spec.ts`.
Reuse phone consent in `tests/chat/mobile-quick-chat-repository.spec.ts`.
Reuse phone resume in `tests/chat/mobile-quick-chat-resume-recovery.spec.ts`.

The backend startup test owns the aged-chat fixture and destructive-expiration regression.
The browser tests prove the unchanged manual lifecycle and restoration.
No seven-day wait, new settings E2E, or rendered UI preview is necessary.

## Work orders

- [x] [Task 01: Remove automatic expiration](task-01-remove-expiration.md) (`done`, sequential).

## Verification results

Implementation and documentation checks passed:

- Go behavioral regression: the old startup loop deleted the aged Quick Chat in RED; the revised startup and restart regression passes.
- Focused Go package tests: backendapp, task service, and SQLite repository passed.
- Backend build: `make build` passed.
- Desktop Quick Chat E2E: 2 passed (restart restoration and cross-device confirmed closure).
- Mobile Quick Chat E2E: 2 passed (dirty repository deletion consent and interrupted-session resume).
- Changed Go package lint: 0 issues.
- Specification catalog: 453 decisions and 2,020 specifications validated.
- Full specification lint: passed.
- Public-doc validator tests: 63 passed; live validator accepted 55 pages.
- `git diff --check`: passed.

The persisted `quick_chat_expire` trigger remains only for committed legacy cleanup-job recovery.
No frontend controls, settings, or new API were added.

PR review follow-up verification passed:

- Focused SQLite deletion, profile-cleanup, and delivery-retirement tests.
- Backend build and changed-package Go lint.
- Specification catalog validation and full specification lint.

## Risks

- Removing a stored trigger breaks retry of cleanup already committed by an older backend.
- Removing expiry tests can accidentally remove independent prompt, delivery, or native-history coverage.
- Stop and delete must retain their distinct meanings.
- Existing host, integration, Office, and automation cleanup continues under separate rules.
