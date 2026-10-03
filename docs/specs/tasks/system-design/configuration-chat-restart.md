---
status: current
system: tasks
requirements:
  - REQ-TASKS-CONFIG-CHAT-RESTART-001
created: 2026-09-29
owners:
  - kandev
---

# Configuration Chat Session Restart System Design

## Boundary and mapping

Tasks owns replacement of the repository-less config-mode ephemeral task.
Reuse the typed utility-chat store, runtime lifecycle, and canonical task
cleanup. Do not implement restart as a transcript reset or session-only delete:
the backing ephemeral task would remain restorable.

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-CONFIG-CHAT-RESTART-001 | Backend operation; Failure recovery; Shared client state; Responsive interaction |

The existing `httpStartConfigChat` prepares config-mode tasks, while
`deleteTaskAfterUserAction` obtains a native deletion preview and calls task
deletion. `Service.DeleteTask` commits before runtime cleanup finishes, and
`session.stop` also permits asynchronous teardown. Neither response alone
proves the old process has stopped. The restart route therefore coordinates
stop, canonical deletion, and creation on the backend.

## Backend operation

Add `POST /api/v1/workspaces/:id/config-chat/restart` alongside the existing
creation route. The request contains `task_id` and `session_id` for the captured
conversation and the existing `X-Kandev-Task-Delete-Confirmation` header. It
accepts exactly one JSON document and no prompt, replacement profile, config-mode
override, or cascade option.
The client obtains a fresh `getTaskDeletePreflight` ticket after confirmation.
Success returns the existing `StartConfigChatResponse` identity shape.

Expose transient restart admission through the existing authenticated
`GET /api/v1/workspaces/:id/quick-chats` response as
`config_chat_restart_pending: boolean` and an optional
`config_chat_retiring_session_id`. The handler-owned coordinator retains only
active workspace operations, removes entries on settlement, and is shared with
config-chat creation admission. While it is active, `httpStartConfigChat`
rejects a competing creation with a typed conflict. This closes the temporary
empty-list window even for another browser. The list response must be consistent
with admission: detect an operation change in that workspace across its session read and retry or
report pending rather than declaring an authoritative empty result. These
additive fields reveal no state before workspace authorization.

The `httpRestartConfigChat` handler lives in a focused file. A narrow
restart lifecycle interface, separate from `OrchestratorStarter`, delegates
old-session retirement to the orchestrator. Its implementation belongs in a
focused `config_chat_restart.go`, using existing authorization, session
lifecycle exclusion, transient-retry cancellation, and synchronous stop.
Existing authorization and lifecycle helpers remain integration boundaries.

1. Authorize workspace write using the request identity, then claim the
   workspace's restart admission. Inside that exclusion, authorize task write
   and session control access and validate eligibility. Validate the task/session
   pair with
   `AuthorizeTaskSessionAccess` and the orchestrator's
   `authorizeTaskSessionPair` before runtime access. Read through scoped service
   methods. Require the selected task to belong to the URL workspace, be
   unarchived, ephemeral, workflow-less, repository-less, non-automation, and
   have `metadata.config_mode == true`. Require its current primary session to
   match the request and reject an unexpected additional session.
2. Capture and validate the effective profile and executor from the existing
   task/session. Reuse launch validation; preserve executor-profile selection
   when present. Sessions with neither executor nor executor profile persist the
   implicit host runtime as empty; pin it to the system local executor during
   replacement. Do not fall back to changed workspace defaults. This validates
   known incompatibilities and current provider availability, not future availability.
3. Under the same exclusion, consume the native preview through
   `WithTaskDeleteConfirmation` before any stop mutates the previewed state.
   Reject missing, stale, replayed, or mismatched tickets with existing error
   semantics. This ticket is authorization for the exact deletion, not a new
   persistent restart ledger. After the first request settles, a repeated old
   target is stale and cannot create another replacement.
4. The orchestrator retirement boundary uses `tryAcquireSessionLifecycleLock`
   before cancellation guards and rejects contention. It suppresses
   reset/resume/prompt admission for the
   retiring session, and cancels transient retry ownership. Keep that exclusion
   through the bounded synchronous stop and deletion callback. Call
   `StopSessionSynchronously` only behind the explicit pair authorization;
   that internal method deliberately does not authorize callers itself. Inspect
   retained execution ownership even for terminal session states. Use existing
   confirmed-absence rules, not string matching or an HTTP 404 alone.
5. After stop succeeds, invoke `DeleteTaskWithLifecycle` through the authorized
   deletion callback. Keep the canonical cleanup snapshot, queue removal,
   deletion event, and owned-resource cleanup; never delete rows directly.
   Do not hold a cancellation guard while calling code that reacquires it.
   Remaining environment cleanup may finish asynchronously once the agent is
   stopped. Release the old-session exclusion after deletion or failure.
6. Create a new config-mode ephemeral task using the existing config-chat
   creation fields and captured launch selections. Prepare with
   `IntentPrepare` and `DeferredStart: true`, then explicitly launch using
   `IntentStartCreated`, `LaunchActivationSourceUserAction`, and an empty prompt.
   Set the internal `NoInitialPrompt` and `SkipMessageRecord` launch options:
   startup retains the config MCP mode without composing an instruction-only
   conversation turn. Normal message dispatch supplies configuration context
   with the first real prompt. These options are not accepted on the wire.
   This avoids a double passthrough launch and does not rely on automatic
   open-time resumption. Use the normal launch response/state for admitted or
   queued startup; never invent a conversational turn or a synthetic user message.
   Boot diagnostics without an active prompt use the existing completed
   lifecycle-only turn contract and never acquire current-turn authority.
   Ordinary prompted startup retains its already-created conversational turn.

The accepted operation uses a bounded context derived with
`context.WithoutCancel` so browser dismissal does not strand half the sequence
or discard caller identity. Use existing task-delete/agent-launch timeout
budgets. No detached, unowned goroutine or database migration is required.

## Failure recovery

Return stable frontend-translatable error codes with a closed stage value
(`validate`, `stop`, `delete`, `create`, `start`) and authoritative
`old_deleted`. If a replacement session exists, include its identity using the
success response shape. Do not report ambiguous deletion as `old_deleted=false`;
resolve post-commit errors through canonical lifecycle state first.

Before deletion, preserve the old descriptor and transcript, even if the agent
has stopped. After deletion, remove them and never attempt to recreate the old
history. On prepare failure, inspect the new task's primary session: retain and
return an already-persisted session, or use `DeleteTaskWithLifecycle` for an
unprepared task. Use a fresh bounded cleanup context with the original identity
if preparation exhausted the operation deadline. On startup failure after preparation, retain and return the
new session and its normal launch error for retry. A partial successful create
must not become an unreachable config task.

Mutating requests are not automatically retried. On timeout/disconnect, use
`listQuickChatSessions` to reconcile the workspace. While the response reports
restart pending, retain progress and suppress competing starts and retiring
session effects. After pending clears, adopt a unique replacement when present;
retain the old target if it still exists; show setup with a cleared-conversation
notice only when the list is authoritatively empty. Reopening uses the same
resync path. Poll only while an observed operation is pending, with the existing
bounded request/reconnect behavior; each status read has a ten-second deadline
and settles independently of terminal-tab resync. A failed read keeps Refresh feedback and
does not permit creation. Multiple eligible results are an error to reconcile,
not permission to choose or delete one arbitrarily. After a backend crash,
normal persisted chat restoration and durable cleanup own the surviving state.

## Shared client state

`useConfigChat` owns the restart mutation and error/progress state. Share
`activeConfigChatOperations` admission between start and restart for both mounted
hook consumers. Add the API function and typed result/error parsing to
`lib/api/domains/workspace-api.ts`. Capture workspace/task/session identity
before confirmation and revalidate it at submission.

`ConfigChatPanel` renders a progress body instead of the retiring
`QuickChatSessionView`, preventing its automatic resume and pending prompt
effects. Other mounted representations must apply the same shared retiring
session state. Keep restart state separate from `reset()`'s abandoned-setup
cleanup: closing an accepted restart must not delete its returned replacement.

On success, idempotently remove the old descriptor and session caches through
existing store actions and install the new descriptor with kind `config`, new
task/session IDs, captured workspace, and effective profile. Do not transfer
`initialPrompt`, transcript, queued prompts, clarifications, or editor draft.
Deduplicate a descriptor already received through WebSocket/list hydration.
Register without opening Quick Chat. Closing or navigating away suppresses
focus/activation, while persisted restoration keeps the new chat reachable.
Late responses and old-task deletion events can only affect their captured IDs.
Preserve ordinary chats and another workspace's current selection.

After confirmed retirement, use `removeTaskSession` and `cleanupTaskStorage` for
the captured old identity as well as replacing its descriptor. This also clears
history, questions, queues, and saved drafts if the WebSocket deletion event was
missed; the normal deletion handler remains safe to apply afterward.

## Responsive interaction

The entry point stays the Settings Configuration Chat FAB. Keep its existing
viewport-bounded panel and full-screen expansion: this change adds one recovery
control and does not redesign chat navigation. The closest shipped chat
exemplar is `mobile-configuration-chat.spec.ts` and `QuickChatModal`.

Extract header/restart presentation from `config-chat-panel.tsx` to retain the
component size limits. Render a rotate/restart icon with localized accessible
name Restart session before Expand and Close. Fine-pointer desktop buttons
use 28px; phone and coarse-pointer hitboxes use at least 44px for all three
neighbors, without title overlap. The title may truncate.

Use `ActionConfirmPopover` on desktop and `MobileActionConfirmation` on phone,
the shipped reusable confirmation exemplar. A short destructive choice uses
the existing inset bottom Drawer, with a single internal scroller and safe-area
padding, while preserving the chat underneath. Keep the adapter mounted across
breakpoint changes, bind its target key to the captured session, and cancel
unconfirmed decisions on identity changes. Cancel/Escape returns focus to the
header control. Confirmation closes before dispatch; progress and errors live
in the panel, including when an old session still exists.

Uncertain restart results expose Refresh status in both the floating panel and
expanded chat. In expanded phone chat, keep feedback and its 44px recovery action
inline in the existing full-screen surface, using the same shared admission and
read-only reconciliation. Dirty-worktree preflight tells the user to commit
changes before retrying; restart never discards them.

Use one localized `role="status"` for progress and an accessible error region
with recovery actions. Keep the button's accessible name stable. All new copy
belongs to `configChat` in en, pt-pt, zh-cn, zh-hk, zh-tw, ja, ko and generated pseudo;
generate Traditional Chinese through `i18n:zh-hant`.

## Evidence and existing decisions

Tests must prove physical-stop ordering, missing-runtime recovery, authorization,
duplicate requests, failure after each destructive boundary, and loss of the
response. Browser evidence must prove a usable new conversation, empty old
history, expansion/reload identity, and phone confirmation and control geometry.

Reuse [typed utility-chat ownership](../../../decisions/2026-07-14-typed-utility-chat-sessions.md)
and [task cleanup](runtime-cleanup.md). No separate ADR is needed: the new
endpoint composes those owners without changing their ownership. Structured
logs should name stage and old/new identities; no prompt or credential logging
and no new metrics are needed.

## Delivery

- [Plan and work orders](../../../plans/configuration-chat-restart/plan.md)
