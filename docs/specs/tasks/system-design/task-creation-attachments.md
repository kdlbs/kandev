---
status: current
system: tasks
requirements:
  - REQ-TASKS-CREATION-ATTACHMENTS-001
created: 2026-10-27
owners:
  - Kandev team
---

# Task Creation Attachments System Design

## Purpose and boundaries

Connect the existing creation composer and file-backed attachment pipeline to
Office creation and its scheduled launches. This is one task-owned vertical
contract across the browser, task repository, and Office scheduler adapter.

The [prompt attachment design](prompt-attachments.md) and
[file-backed attachment ADR](../../../decisions/2026-08-04-file-backed-prompt-attachments.md)
retain authority for streaming, authorization, limits, and runtime materialization.
Office retains assignment, pause, budget, workflow, and provider-routing policy.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-CREATION-ATTACHMENTS-001` | Composer reuse, draft lifetime, atomic creation ownership, scheduled delivery, responsive composition, verification |

## Source evidence

`NewTaskDialog` uses a plain description Textarea. Its Upload button has no
handler, and `IssueDraft` has no attachments. Kanban's `TaskFormInputs` already
owns file selection, clipboard parsing, drag/drop, upload, and context chips.
`ImageItem` supplies thumbnail/details and a larger image dialog.
`FileAttachmentItem` supplies the existing metadata preview.

The shared HTTP create request accepts `attachments`. The handler currently
calls `CreateTask`, then claims attachments. `finalizeCreatedTask` publishes
`task.created` before that claim. Office creation uses neither `start_agent`
nor `prepare_session`, so its descriptors do not enter the deferred launch map.
`SchedulerIntegration.prepareAndLaunch` builds `LaunchContext` without setting
`Attachments`, although the launch interfaces already carry that field.

## Composer reuse

Compose the existing `TaskFormInputs` from
`apps/web/components/task-create-dialog-selectors.tsx` into the Office dialog.
Use a small Office adapter for its description callbacks, ref, and draft owner.
Supply the exact Office workspace, existing description placeholder, initial
attachments, pending-state callback, and accepted attachment-change callback.
Keep provider selectors and optional enhancement/launch-preview actions outside
this adapter. Extract the existing `useFileAttachments` logic into
`hooks/domains/task/use-task-create-attachments.ts` for both consumers.
Do not copy the upload implementation or introduce a second attachment model.

Give `TaskFormInputs` an optional shared attachment controller. Kanban retains
its default internal controller. Office supplies one owned above `DialogContent`,
so closing the Radix content does not erase pending files or restart uploads.
Only one controller may process uploads for a composer. The unused default
controller must be disabled when an external controller is supplied.

Reuse `FileAttachment`, `readClipboardAttachments`, `processFile`,
`acceptAttachmentsWithinLimits`, `uploadAttachment`, and `toMessageAttachments`.
The composer owns one real file input for picker, paste, and drop entry points.
The property row retains Status and Priority. Remove the obsolete Upload button
and empty dots trigger after the composer's Attach files action exists.

`ContextZone` places attachment chips above the description, as in Kanban.
Image chips use `ImageItem` and its existing image dialog. Non-image chips
use `FileAttachmentItem`. Ready/restored previews use `attachmentContentUrl`.
Add that URL fallback for uploaded images without an inline preview, including
images beyond the local preview threshold. Do not persist data URLs or bytes.
A file chip exposes metadata, not a new document-content renderer.

## Draft lifetime and submission

`useIssueDraft` keeps the existing Office fields. Store only ready
`StoredFileAttachment` descriptors in its workspace/parent draft record.
Reuse `toStoredChatDraftAttachments` and `restoreAttachmentPreview`.
Legacy records without an attachment array restore as empty.
In-flight `File` objects and local previews remain in controller memory.
After reload, unrecoverable pending bytes require reattachment.

Use a keyed draft body and an owner/generation guard for workspace, parent,
discard, and successful creation. Closing and reopening the same draft retains
its composer state. A context switch loads only that context's saved descriptor
set. Late callbacks return rejection to the existing upload lifecycle, which
deletes unclaimed late results. Discard releases known staged descriptors.
Success resets the accepted draft without deleting its now-claimed files.

Guard both the button and `handleCreate` with processing, ready-upload, and
expiry checks. The processing guard starts before asynchronous image decoding.
Preserve existing title/project validation and all Office payload fields.
Add the ready descriptors to the existing `createTask.attachments` field.
A create rejection keeps text and valid attachments. Show localized feedback
for the rejected attachment and offer remove/reattach or retry as applicable.
Do not fall back to inline bytes or make description text mandatory.

## Atomic creation ownership

Use an explicit ordered association instead of enumerating all task claims.
Add `task_creation_attachments` in the task repository's `base_schema.go`:

- `task_id`, with task foreign-key deletion.
- `attachment_id`, referencing the existing attachment registry.
- `position`, preserving submitted order.
- Unique `(task_id, attachment_id)` and `(task_id, position)` pairs.

This stores identity and order only. Canonical names, kinds, sizes, delivery
modes, ownership, and bytes remain in the existing registry.
The association distinguishes creation files from later chat and queued files.
It introduces no separate blob storage or independent attachment lifetime.

Add typed creation-attachment admission to `CreateTaskRequest` and the internal
task insertion input. Derive uploader identity from authenticated context.
Validate canonical descriptor fields through the attachment service. The task
insert transaction claims the complete set and writes the association before
commit. Every normal, WIP-admitted, and queued task insertion uses this boundary.
Factor the existing claim selection/update logic into transaction-usable helpers.
Retain row locking, count/size checks, expiry checks, and cross-owner denial.

Move file-backed create admission from the handler's post-create claim into
this boundary. Preserve the existing bounded inline compatibility path.
No creation event, runner wake, or successful task becomes visible with an
unclaimed or partial creation set. Duplicate `external_id` Found outcomes
retain their existing no-side-effect semantics and do not claim retry uploads.

Use `TaskCreationAttachmentClaim` as a proposed internal typed input, with
`json:"-"` if carried on the task model. It contains uploader identity and
validated descriptors only. Public metadata cannot forge this input.
It is not a new public task field or a generic transaction callback mechanism.

Repository cleanup treats the creation association as a durable reference.
Queue removal, draft cleanup, session deletion, and claim rollback cannot erase
these accepted task files. Workspace deletion removes associations before it
deletes the workspace's attachment registry rows. Task deletion uses the
existing attachment cleanup job and removes the association and bytes through
that owned lifecycle. Apply the schema and reference checks to both SQLite and
PostgreSQL.

## Scheduled delivery

Add a narrow task-service reader, proposed as
`ReadTaskCreationAttachments(ctx, taskID)`, and inject it into
`SchedulerIntegration` from `internal/backendapp`.
The reader joins the explicit association to canonical claimed rows.
Validate each row's task ownership and permitted claim scope.
Validate descriptor metadata and byte availability. Empty results remain a
valid text-only task. A partial or invalid set returns an error.

Before each task-bound Office launch, populate `LaunchContext.Attachments` with
this ordered set. Preserve the existing Office prompt and all launch guards.
Taskless runs retain no creation attachments. Direct and routed launches use
the same reader and existing attachment-carrying launch interfaces.

Creation files are recurring task context, like the task description. They
remain available to later task-bound agents and runs. They are not consumed
by the first run or discovered from task-wide chat history.
Keep them task-scoped during message admission so later sessions can use them.
Preserve independent session-scoped claims for ordinary chat attachments.

The orchestrator and lifecycle keep their existing materialization path.
Path resources install before prompt dispatch, and image native-prompt/path
modes keep their current behavior. Do not synthesize an initial user message
from the creation inventory or change session submission recovery semantics.
A scheduler read failure uses existing run-failure handling and releases its
checkout. Materialization failure uses the existing correlated prompt-error path.
Neither path silently drops an attachment or starts a text-only replacement.

## Responsive composition

The phone entry remains Office Tasks > New Task. The nearest composer exemplar
is `TaskFormInputs`; `ContextChip` supplies its touch Drawer, and
`image-preview-dialog.tsx` supplies the larger image viewer.

Desktop retains a centered, bounded dialog. Below 768px, phone uses a full-height
creation composition in the existing dialog shell, sized to the dynamic viewport.
Use a fixed header/footer and one `min-h-0` scrolling form body.
Keep safe-area clearance at the footer and controls at least 44px on phones
and coarse pointers. Desktop ordinary buttons retain 28px density.
The fixed regions must not inherit shared grid stretching or oversized minimums.
Reset the shell's desktop translation/width constraints only in the phone branch.
Retain touch targets at phone widths even with a fine pointer.

Chips wrap or scroll within the composer's existing context region.
They never widen the dialog or the document. On touch, the chip opens the
existing inset details Drawer. Open shows the larger image viewer when an image
is available. Closing either layer restores the corresponding trigger.
The draft remains intact underneath. Touch users need no hover interaction.

See `UI-01` through `UI-03` in the
[implementation plan](../../../plans/office-new-task-supported-controls/plan.md#ascii-ui-preview).

## Verification

Repository tests cover atomic claims, ordering, rollback, mixed valid/invalid
sets, WIP paths, external-id retries, restart reads, and cleanup references.
Handler tests cover the actual Office create request with no direct start.
Office service tests cover direct/routed and taskless runs with a capturing starter.
Backendapp tests use the production starter and real lifecycle attachment
materialization to prove PNG/resource descriptors and actual bytes reach the agent.

Browser tests exercise real file selection and a readable clipboard PNG.
Cover thumbnail, larger preview, non-image metadata, text paste, pending/failed
upload guards, retry/remove, ready-draft reload, owner changes, and creation.
Use a deliberately slow upload for late-completion/discard coverage.
Run desktop and phone Office cases plus affected Kanban composer regressions.
Clipboard event tests prove application handling, not operating-system clipboard integration.

## Implementation plans

- [Office creation attachment parity](../../../plans/office-new-task-supported-controls/plan.md).
