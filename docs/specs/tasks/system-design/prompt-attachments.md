---
status: current
system: tasks
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
  - REQ-TASKS-PROMPT-ATTACHMENTS-002
created: 2026-09-01
updated: 2026-10-06
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

| Requirement                        | Design sections                                                               |
| ---------------------------------- | ----------------------------------------------------------------------------- |
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

This extension covers AC-TASKS-PROMPT-ATTACHMENTS-001.12 through .17.
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
authoritative `fetchTask(taskId)` read through the hook. Once that read starts,
its pending, resolved, or failed state takes precedence over cached workspace
records. Deduplicate the read per task, keep a current subscriber through cache
changes, and ignore responses after the requested task changes or unmounts. A
failed read leaves scope unresolved with visible recovery until the user retries.
Do not use the currently selected workspace as a fallback. Cold direct links must
work without first opening an Office or Kanban list.

`use-chat-input-state.ts` must block message and plan-implementation actions
whenever any attachment lacks an uploaded attachment ID, independent of
workspace availability. Retain draft text and files while scope resolves. Show
localized scope feedback and allow retry after lookup failure. Start pending
uploads when valid scope arrives. Recover old inline draft bytes as a file and
upload them before use; an unrecoverable descriptor remains blocked until the
user removes it. Do not persist bytes for a new pending browser file. Use
existing upload-error, retry, removal, and best-effort deletion behavior. Never
convert an incomplete file into an inline-byte submission because scope is
missing. Keep compatibility for already-ready descriptors and legacy message
attachments separate.

Capture task/session/workspace identity for asynchronous upload work. A late
completion must not update a successor draft. Delete an unclaimed late upload
on a best-effort basis. Clear the attachment collection if the task changes
while a session ID is reused, and delete its unclaimed descriptors. Session
changes load only that session's own draft. Draft restoration must not transfer
files between tasks. Message submission and plan implementation remain blocked
while any attachment is pending, failed, or otherwise missing its descriptor.

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
Plan implementation controls use the same incomplete-upload state and remain
disabled on desktop and phone until every attachment is ready or removed. Keep
their handlers guarded so direct invocation cannot bypass that state.
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

## Initial submission recovery

This section maps REQ-TASKS-PROMPT-ATTACHMENTS-002 to recovery admission,
persistence, owned dispatch, and transcript reconciliation. It extends the
submission lifecycle, rather than creating another attachment registry.
Delivery is tracked in [Fresh-start recovery](../../../plans/fresh-start-recovery/plan.md).

### Submission source and compatibility

Persist a bounded `initial_prompt_submission` session metadata record after
attachment claim and before background launch. The proposed record contains
version, raw submitted content, plan-mode intent, validated file-backed descriptors,
and pending/dispatching/accepted delivery state. Store no bytes, paths, hidden workflow
instructions, or saved-prompt expansion. Use the existing metadata-key writer.
A failed write stops preparation before agent launch. Preview and submission
records derive from the same validated request and remain separate consumers.

Before provider dispatch, an owned conditional write marks the record dispatching.
The normal launch acceptance callback marks it accepted with its owned
execution/attempt evidence. A crash in dispatching leaves acceptance uncertain. Task creation must retain this record through failed
startup and restart. A missing receipt does not prove that dispatch never occurred.
For a pending record with ambiguous acceptance, refuse automatic replay and show
a safe recovery error. Never promise exactly-once delivery across an uncertain
provider-acceptance/database-write boundary.

For older sessions, reconstruct a pending record only at explicit fresh recovery.
Require a session-owned initial preview, a matching bootstrap failure before
provider identity/accepted prompt evidence, and valid claims for every descriptor.
Read the attachment registry for canonical fields and bytes availability. The
preview provides candidate IDs and raw content, not an executable prompt or
proof of delivery. Reject malformed entries, foreign claims, missing bytes,
and ambiguous acceptance instead of silently omitting attachments.
If a legacy text-only session has no saved attachments, retain its existing
text fallback. Do not enumerate task-wide attachments to guess the original set.
An existing session that already accepted another retry requires explicit user
resubmission when original-delivery provenance is uncertain.

This compatibility admission is the only recovery exception to the preview's
ordinary display-only role. It does not change ordinary preview rendering,
prepare-only creation, sibling isolation, or legacy inline-byte delivery.
Do not persist inline bytes in the new record. Inline-only legacy submissions
retain their existing bounded compatibility path; unavailable bytes require
explicit resubmission.

### Owned recovery dispatch

`RecoverSessionWithOptions` currently clears the token and launches `IntentResume`
without attachments. `newResumeLaunchRequest` restores only `task.Description`.
Capture the source submission and failure identity before token reset. Validate
all claims under the existing authorization and workspace recovery guards.
Keep the session row and its workspace; clear only provider conversation identity.

For a failed-start fresh recovery, suppress the executor's automatic description
prompt through a narrow internal `ResumeOptions` setting. Start the agent without
a prompt and use the existing owned `resumeTaskSessionWithContinuation` path.
Provider-confirmed readiness resolves the matching startup failure before dispatch.
Rebuild the first-conversation Kandev instruction block from current trusted
server state using the same first-launch policy as a newly created session.
Compose it with the captured raw content for dispatch, while keeping the receipt
and transcript content raw. Then deliver the effective prompt through
`promptTask` with the captured plan-mode intent and canonical attachment
descriptors. Preserve current workflow, completion-signal, question/autopilot,
and title/tool policy. Do not revive old provider conversation context,
re-expand a stored hidden prompt, or copy the task plan.

Keep initial replay ahead of orphan queue draining. Enforce the session-owned
initial-prompt hold at each shared guarded queue reservation/admission boundary,
including manual and enqueue-triggered drains; suppressing only the boot-ready
handler's drain is insufficient. Reuse the owned initial-prompt hold/acceptance
mechanism so queued messages cannot steal the first recovered turn. Release that
hold after acceptance, cancellation, or terminal failure, then allow ordinary
queue draining to resume. A duplicate caller cannot attach another continuation
to the owner's attempt.

Initial submission replay applies only before accepted initial delivery and
before any later conversation work has been accepted. Any ordinary resume or
interactive prompt accepted while the original receipt is still pending must
durably retire that receipt's replay authority. The visible initial user row is
not proof of provider acceptance. If accepted-work provenance cannot be
established after restart, fail closed and require explicit user resubmission.
Fresh recovery after ordinary conversation work keeps its existing prompt
policy. Ordinary resume prompt content, read-only restore, Office scheduling,
dynamic route changes, and unrelated new-session launches remain otherwise
outside this extension.

### Delivery, persistence, and presentation

Use existing attachment materialization before dispatch for both native-prompt
images and path-delivered resources. Preserve descriptor order and delivery mode.
A delivery failure uses the existing correlated prompt-error settlement; it must
create its own current error after startup success. Do not retain the old bootstrap
controls as a substitute for the new delivery error.

Record the original raw user submission and descriptors through existing message
persistence. Update `backfillInitialUserMessageIfMissing` to accept the owned
submission descriptors and avoid a text-only duplicate. An existing user record
wins; synthetic preview replacement remains governed by requirement 001.
Acceptance metadata is server-owned and uses conditional writes for the owned
attempt. Preserve unrelated session keys and user edits. No table migration,
public endpoint, runtime flag, or separate attachment inventory is required.

Desktop uses the current Chat transcript and composer recovery region. Phone
uses `mobile/session-mobile-layout.tsx`, stacked recovery actions, safe-area
clearance, and the transcript's existing scroll region. Reuse current image
preview and file-label controls. Recovery pending disables equivalent actions.
All new visible feedback uses the existing localization contract.

### Verification

Use real attachment claims and persisted sessions in focused backend tests.
Cover PNG plus path-delivered ZIP, attachment-only input, restart, legacy admission,
invalid claims, materialization failure, cancellation, and duplicate requests.
Inspect the captured prompt descriptors and materialized bytes, not preview visibility.
Desktop and phone browser tests must prove recovery, one user row, retained history,
reload/reconnect convergence, and a restored editable composer. Runtime failure
resolution follows the [startup recovery design](../../agents/system-design/session-startup-failure-explanations.md).
