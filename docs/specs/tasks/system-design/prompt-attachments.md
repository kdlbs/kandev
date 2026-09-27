---
status: current
system: tasks
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
created: 2026-09-01
updated: 2026-09-27
owners:
  - Kandev team
---

# Prompt Attachments System Design

## Purpose and boundaries

The task system owns prompt-attachment staging, claims, delivery descriptors,
and retention. It also owns admission of those descriptors to a task session.

The agent runtime materializes claimed files and sends them with the prompt.
It cannot claim staged files or infer task ownership.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-PROMPT-ATTACHMENTS-001` | Claim admission, materialization and delivery, failure and recovery, security |

## Components and responsibilities

- The attachment service stores staged files and validates the authenticated
  owner, workspace, task, and optional session.
- The task repository changes a complete attachment set from `staged` to
  `claimed` in one transaction.
- The orchestrator admits `LaunchSessionRequest.Attachments` before any agent
  start or prompt-turn creation.
- The lifecycle manager reads claimed files and streams them to agentctl before
  it sends the prompt.
- The orchestrator converts a rejected initial prompt into the durable launch
  failure state.

## Claim admission

`LaunchSessionRequest.Attachments` contains untrusted attachment identifiers.
The orchestrator authorizes the task and optional session before each claim.

The orchestrator uses a narrow attachment-claimer interface. The task service
implements this interface and derives the owner and workspace from server
state.

The launch entry point claims attachments before it calls an intent handler.
This order prevents invalid descriptors from starting an agent or prompt turn.

A launch with an existing session uses that session as the claim scope. A new
session launch can use a task-scoped claim because no session identity exists.

Task creation also creates task-scoped claims before it prepares a session. A
later launch for that task treats the existing claim as idempotent.

A session-scoped claim remains bound to its session. Another session cannot use
that claim, even when both sessions belong to the same task.

## Materialization and delivery

The lifecycle manager accepts only claimed file descriptors. The attachment
reader checks the task and the optional session before it opens a file.

The lifecycle manager streams each file to the active agentctl instance. It
then uses the returned safe name for native-prompt or workspace-path delivery.

The lifecycle manager starts prompt generation before materialization. A
materialization error therefore uses the same terminal prompt-error path as an
ACP submission error.

Delivery into a turn that is already generating uses the same materialization
step. The steer route resolves descriptors before it acquires the prompt
lifecycle lock, because materialization streams file bytes over the network and
the lock is held only for a bounded dispatch. A steer whose materialization
fails does not dispatch and does not fall through to an ordinary prompt
carrying unresolved descriptors.

## Failure and recovery

A claim error returns from `session.launch` before the intent changes runtime
state. The response does not disclose another owner, workspace, task, or path.

A materialization or ACP submission error can occur after the launch response.
The lifecycle manager reports that error with the current execution identity.

The existing agent-failure path settles the current turn and session with a
safe generic error. It also publishes the durable task and session state used
by the chat surface.

A delayed error cannot settle a replacement execution or successor prompt. The
existing execution and prompt evidence checks reject stale terminal events.

Shutdown cancellation remains a stopped execution. It does not become a user
visible launch error.

## Persistence

The attachment registry is the source of truth for claim state. Claim changes
are transactional and survive backend restarts.

Claim admission does not write file bytes to a task message. The initial user
message stores bounded attachment descriptors after the launch succeeds.

## Security

Clients cannot provide an owner, workspace, storage key, or executor path. The
backend derives those values from authenticated and persisted state.

Task-scoped idempotency applies only when the stored task matches. Session
scoping remains strict when the stored claim contains a session identity.

## Initial preview during workspace preparation

This proposed extension covers AC-TASKS-PROMPT-ATTACHMENTS-001.8 through
AC-TASKS-PROMPT-ATTACHMENTS-001.11. The task system owns the submitted
attachment set and its session binding. Transcript history still follows the
[UI history design](../../ui/system-design/task-prompt-transcript-visibility.md).

The task-create handlers currently prepare a session synchronously, then start
it asynchronously. `prepareStartAgentSession` does not forward the submitted
text or attachments. `postLaunchCreated` and `postLaunchStart` record the
initial user message after launch. Until then, `useProcessedMessages` builds a
text-only task-description fallback.

Pass an internal, display-only initial-preview value through the task-create
prepare path, including prepare-only creation. Persist it in the target
session's metadata as `initial_prompt_preview` before publishing the created
session or starting workspace preparation. Use an internal preparation option;
do not make preview data a second agent-dispatch input or change passthrough
prepare upgrades. The value contains `content` and `attachments`, using the
existing display descriptor fields: `attachment_id`, `type`, `name`,
`mime_type`, `size_bytes`, and `delivery_mode`. Only validated file-backed
descriptors from the successful task attachment claim enter this value.
Do not copy inline bytes, storage paths, hidden prompts, or arbitrary metadata.

Use the existing session metadata persistence and session publication path.
No new endpoint or table is needed. An atomic metadata-key update must preserve
other session keys. A failed preview write must stop this preparation path
before background work starts and use its existing failure handling.
Reused sessions must not acquire another initial preview. Legacy inline-only
attachments keep their existing post-launch behavior; this extension targets
the file-backed web submission path.

The snapshot remains session-owned across reload and preparation failure. It
is display data, not evidence of delivery, a turn, a prompt ordinal, or a retry
queue. Retain it with the session so a delayed metadata hydration cannot revive
a deleted key. Existing session deletion removes it. Stored user messages are
always authoritative once loaded. Retries must not copy the snapshot into a
different session or alter attachment claim/delivery semantics.

`useSessionState` already provides the resolved session. Pass its validated
preview through `useSessionData` to `useProcessedMessages`. Validate unknown
metadata before mapping it to `Message.metadata.attachments`; reject malformed
entries independently. Never combine task-wide attachment inventory with the
current session or read the transient `deferred_launch` metadata as display state.

Render one synthetic user row only when history is initialized, no older
history remains, and no stored user row is visible. Prefer the session preview
when it has text or valid attachments; otherwise preserve the existing legacy
task-description fallback. Keep the synthetic row's existing identity and
missing-timestamp behavior. Include the preview in memo dependencies so late
session hydration updates the row. Do not insert it into persisted message
state or enable persisted-message mutations for it.

Reuse `chat-message.tsx` image and resource attachment rendering, including
`attachmentContentUrl` and the existing image dialog. Correct its local
attachment type to represent optional inline bytes and file-backed IDs.
Continue to authorize content reads through the existing attachment service.
An inaccessible image must not remove the surrounding message or siblings.

### Desktop and phone composition

Both surfaces show the initial user row above existing preparation progress.
Images and compact file labels wrap inside that row, above its text. The primary
action is opening an image. No additional navigation or toolbar is introduced.
The phone entry is the existing task Chat view. Reuse the dedicated phone
composition in `components/task/task-layout.tsx`, the attachment controls in
`chat-message.tsx`, and the mixed-attachment mobile E2E exemplar.

The transcript remains the single vertical scroll owner. Existing full-height
layout, safe-area handling, image-dialog dismissal, focus return, and touch
targets remain in effect. Inline attachment content fits this brief review
task without an intermediate picker. Desktop and phone share the preview data
and replacement logic; no responsive preference is persisted.

### Verification boundaries

Backend tests pause before workspace launch and verify persisted preview
metadata, fresh reads, prepare-only behavior, claim rejection, and unrelated
session isolation. Frontend tests cover descriptor mapping, attachment-only
content, malformed metadata, late hydration, history guards, and replacement.
Desktop and mobile browser tests open both attachments during preparation,
reload, then observe one stored initial message after launch. Include failure
and unavailable-content cases without suppressing existing progress errors.

## Composer workspace resolution

This extension covers AC-TASKS-PROMPT-ATTACHMENTS-001.12 through .15.
The task system owns upload scope because it owns attachment authorization.
It reuses the existing attachment API and ADR without changing persistence.

`composer-workspace.ts` currently considers Quick Chat sessions and Kanban
workflow collections. Both `chat-input-area.tsx` and
`passthrough-chat-composer.tsx` call it. Office-only task identities can therefore
resolve to no workspace, even when the Office task has a workspace.

Use one shared composer-scope hook for both consumers. Resolve an exact task ID
from Office task records and existing task collections. Preserve the existing
Quick Chat session mapping. A conflicting task-bound identity must not select
a workspace by collection order. Missing or conflicting scope requires an
authoritative `fetchTask(taskId)` read through the hook. Deduplicate that read
per task, and ignore responses after the requested task changes or unmounts.
A failed read leaves scope unresolved with visible recovery. Do not use the
currently selected workspace as a fallback. Cold direct links must work without
first opening an Office or Kanban list.

`use-chat-input-state.ts` must block submission whenever any browser file lacks
an uploaded attachment ID, independent of workspace availability. Retain draft
text and files while scope resolves. Show localized scope feedback and allow
retry after lookup failure. Start pending uploads when valid scope arrives.
Use existing upload-error, retry, removal, and best-effort deletion behavior.
Never convert a newly selected file into an inline-byte submission because
scope is missing. Keep legacy stored descriptor compatibility separate.

Capture task/session/workspace identity for asynchronous upload work. A late
completion must not update a successor draft. Delete an unclaimed late upload
on a best-effort basis. Draft restoration must not transfer files between tasks.
Submission remains blocked while any selected attachment is pending or failed.

### Surface boundary

Quick Chat, general run transcripts, Office advanced chat, and passthrough chat
already share the editor. Reuse it rather than introducing another upload hook.
Office agent run details and per-agent transcript tabs pass `hideInput` and
remain read-only. The simple Office comment composer is live code. Its
Markdown/body-only comment API is not a task-session message API. File-backed
comment claims, retention, and agent delivery require a separate Office design.
This extension does not claim that comment uploads are fixed.

### Desktop and phone behavior

Keep attachment chips above the editable prompt. Place localized scope or
upload feedback beside the chips, with retry and removal actions. Sending is
unavailable while any file is incomplete. Preserve typed text and ready siblings.
The phone entry remains the existing task Chat view. Reuse the shipped mobile
session layout and attachment controls. Chips wrap, touch controls remain at
least 44px, and the transcript retains its existing scroll owner. Keep existing
safe-area and keyboard handling. No extra drawer or navigation step is needed
for this short inline action. Desktop controls retain their normal density.

### Evidence and observability

Tests must exercise a real PNG File through paste, HTTP upload, descriptor
submission, and transcript display. Cover a cold Office route with an unrelated
active workspace. Deterministic DOM paste proves application handling, not macOS
clipboard integration. Retain unreadable-image and HTML-image fallback tests.
Existing upload HTTP errors and localized composer feedback provide diagnostics.
No new metrics, flags, schema, or backend endpoints are required.

## Implementation plans

- [Composer attachment scope](../../../plans/composer-attachment-scope/plan.md)
- [Preparation attachment previews](../../../plans/preparation-attachment-previews/plan.md)

## Observability

Existing launch request diagnostics correlate claim errors with their task and
session. They do not include attachment bytes or storage paths.

Initial prompt errors retain the execution identity. The terminal event and
durable state provide evidence that the prompt did not remain active.

## Related decisions

- [ADR-2026-08-04-file-backed-prompt-attachments](../../../decisions/2026-08-04-file-backed-prompt-attachments.md)
- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)
