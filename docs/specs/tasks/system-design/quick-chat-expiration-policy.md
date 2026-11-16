---
status: current
system: tasks
requirements:
  - REQ-TASKS-QUICK-CHAT-EXPIRATION-003
created: 2026-11-16
owners:
  - kandev
---

# Quick Chat Manual Lifetime System Design

## Purpose and boundaries

Remove automatic Quick Chat expiration.
The existing task deletion path remains responsible for confirmed tab closure and explicit workspace deletion.
No settings package, preference, HTTP endpoint, frontend control, or feature flag is added.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-QUICK-CHAT-EXPIRATION-003` | Automatic producer removal; Manual lifecycle; Agent-profile cleanup; Upgrade compatibility; Verification |

## Automatic producer removal

Remove the `StartQuickChatExpirationLoop` registration from `internal/backendapp/main.go`.
Delete `internal/task/service/quick_chat_expiration.go`, including its startup sweep, daily timer, and retention constants.
Remove `deleteExpiredQuickChatTask` from `service_tasks.go`.

Remove `ListExpiredQuickChatTasks` and `DeleteExpiredQuickChatTask` from the task repository interface and SQLite implementation.
Remove expiry-only query helpers if no other caller uses them.
Keep shared task deletion, authorization, metadata, and resource-cleanup helpers.

Remove tests whose sole purpose is to prove automatic expiration.
Preserve independent prompt-history, delivery-retirement, and native-history guarantees through manual-delete coverage.
Do not replace the removed worker with a disabled worker or a no-op method.

## Manual lifecycle

`use-quick-chat-close-actions.ts` already separates modal dismissal from confirmed persisted-tab deletion.
Its confirmed close action calls the existing task deletion API.
No frontend production change is required.

Stop ends or cancels agent execution and retains conversation identity and history.
Opening a preserved chat follows the existing resumption preference.
A user can stop the agent and keep the chat, or confirm tab closure to delete it.

Agent-profile cleanup uses the task's `config_mode` metadata to select configuration chats.
It preserves ordinary Quick Chat tasks, sessions, messages, and owned resources so profile removal
does not act as an implicit conversation deletion path.

Confirmed deletion still captures the task-owned resource inventory and native agent-history identities.
Its durable worker retains existing ownership, runtime-stop, source-manifest, and retry protections.
Native history removal remains restricted to the deleted Quick Chat's supported local agent identities.

## Upgrade compatibility

No database migration or new settings document is required.
A new backend stops creating expiration work as soon as it replaces the old process.

Retain the persisted `quick_chat_expire` trigger discriminator in `task/models/resource_cleanup.go`.
Keep its interpretation in the durable cleanup worker for jobs created by older versions.
Do not remove an old job merely because its producer no longer exists.
Only already committed deletion cleanup can resume. A prepared intent for an undeleted chat must not delete or strip that live chat.

This preserves restart recovery without retaining an expiration entry point.
Tests seed both an already-deleted task with pending legacy cleanup and an undeleted task with a prepared legacy intent.
The latter must remain protected by existing preparation reconciliation.

## Documentation

Reconcile [Quick Chat persistence](../requirements/quick-chat-expiration.md) with the manual lifetime requirement.
During implementation, update the native-history requirement/design to describe explicit deletion and legacy cleanup.
Retire their automatic-sweep acceptance criterion without reusing its identifier.

Update public Quick Chat and feature-status documentation to remove unconditional seven-day expiry.
Configuration chats no longer need an explanation based on exclusion from that removed sweeper.
Review related coordinator and startup specifications for references to removed functions.
Keep durable trigger-format documentation where it describes compatibility with stored jobs.
Public docs remain accurate for the running version until the implementation lands.

## Verification

Add a behavioral startup regression with a persisted Quick Chat older than seven days.
Run real isolated startup and maintenance boundaries, then verify its task, session, messages, workspace, and native history remain.
Exercise backend restart and restoration without an inactivity exception.

Keep a distinct manual-close test that proves task deletion still removes the selected chat and owned resources.
Test that Stop retains the conversation and that canceled closure does not delete it.
Use existing desktop and phone chat tests for these unchanged interactions.
No new settings UI, locale work, or settings E2E is required.

Retain legacy cleanup recovery tests independently from tests of the removed expiration producer.
The [work order](../../../plans/remove-quick-chat-expiration/task-01-remove-expiration.md) owns exact commands and implementation evidence.
