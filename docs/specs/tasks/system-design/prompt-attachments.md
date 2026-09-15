---
status: draft
system: tasks
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
created: 2026-09-01
updated: 2026-09-10
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

## Implementation plans

- [Preparation attachment previews](../../../plans/preparation-attachment-previews/plan.md)

## Observability

Existing launch request diagnostics correlate claim errors with their task and
session. They do not include attachment bytes or storage paths.

Initial prompt errors retain the execution identity. The terminal event and
durable state provide evidence that the prompt did not remain active.

## Related decisions

- [ADR-2026-08-04-file-backed-prompt-attachments](../../../decisions/2026-08-04-file-backed-prompt-attachments.md)
- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)
