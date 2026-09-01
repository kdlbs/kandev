---
status: draft
system: tasks
created: 2026-08-04
owners:
  - Kandev team
---
# Prompt attachments Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-PROMPT-ATTACHMENTS-001: Prompt attachments

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-PROMPT-ATTACHMENTS-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

Decision: [ADR-2026-08-04-file-backed-prompt-attachments](../../../decisions/2026-08-04-file-backed-prompt-attachments.md)

## Why

Users need to give an agent diagnostic bundles, archives, media, and other
working files that are larger than the current 10 MiB picker limit. A 22.8 MiB
diagnostic ZIP is a normal support artifact, and rejecting it forces users to
move files into a workspace manually or strip useful evidence.

## What

- Task creation, subtask creation, new-session prompts, direct chat messages,
  and queued messages accept up to ten attachments.
- One attachment may contain at most 100 MiB (104,857,600 raw bytes), and all
  attachments in one submission may contain at most 100 MiB in aggregate. An
  attachment exactly at either boundary is accepted; one byte above is
  rejected.
- Limits are measured from raw uploaded bytes, not base64 or JSON length. All
  browser and server feedback presents the same limit.
- The browser uploads files before submitting the task or message. It shows
  each attachment's pending, uploading, ready, or failed state and does not
  submit while any attachment is incomplete.
- A failed upload can be retried or removed without losing the prompt text or
  other ready attachments. Removing an unsubmitted attachment releases its
  staged upload on a best-effort basis.
- Ready attachment descriptors survive a same-browser reload while their
  staging records remain valid. The browser does not persist attachment bytes
  in web storage.
- Submitted user messages and queue entries retain attachment name, MIME type,
  raw size, kind, and delivery mode for transcript display and later authorized
  download. They do not retain base64 data or expose host filesystem paths.
- Queued image previews resolve file-backed attachment IDs through the
  authorized content endpoint and retain the legacy inline-data fallback. The
  queue UI does not construct an image data URL when an attachment has no
  inline bytes.
- Path-delivered files are materialized in the active execution beneath the
  session's `.kandev/attachments/<session-id>/` directory before the agent is
  prompted. Existing native-prompt and path delivery choices remain available.
- Desktop and mobile expose the same upload, retry, removal, submission, and
  size-error outcomes through their existing prompt composers.

## Data model

`task_message_attachments`

| Field | Type | Constraint |
|---|---|---|
| `id` | string | Primary key; opaque client reference |
| `owner_id` | string | Authenticated user that uploaded the file |
| `workspace_id` | string | Required staging/authorization scope |
| `task_id` | string | Nullable until claimed; task FK after claim |
| `session_id` | string | Nullable until claimed; session FK after claim |
| `message_id` | string | Nullable until claimed; message FK after claim |
| `queue_entry_id` | string | Nullable; queued-message owner while queued |
| `name` | string | Original display name, never used as a storage path |
| `mime_type` | string | Client-reported media type |
| `kind` | enum string | `image`, `audio`, or `resource` |
| `delivery_mode` | enum string | `prompt` or `path` |
| `size_bytes` | integer | Raw bytes, `1..104857600` |
| `storage_key` | string | Backend-generated relative storage key |
| `state` | enum string | `staged`, `claimed`, `expired`, or `deleted` |
| `expires_at` | timestamp | Required for staged uploads; cleared on claim |
| `created_at` | timestamp | Upload creation time |
| `updated_at` | timestamp | Last lifecycle change |

Files live under the resolved Kandev home attachment root with private
directory/file permissions. Database ownership is authoritative; filenames and
request paths never determine ownership.

## API surface

- `POST /api/v1/attachments` accepts `multipart/form-data` with `file`,
  `workspace_id`, `kind`, and `delivery_mode`, plus optional `task_id` and
  `session_id`. It returns `201` with `{id, name, mime_type, kind,
  delivery_mode, size_bytes, state, expires_at}`.
- `GET /api/v1/attachments/:id/content` streams the bytes only to a caller
  authorized for the staged upload or claimed task.
- `DELETE /api/v1/attachments/:id` deletes an owned staged upload. A claimed
  attachment cannot be removed through this route.
- Task-create HTTP requests and the `session.message.add`, `message.queue`, and
  `message.update` WebSocket actions carry attachment descriptors with
  `attachment_id`, `name`, `mime_type`, `kind`, `delivery_mode`, and
  `size_bytes`; they do not carry file bytes.
- Existing inline `data` descriptors remain accepted only within the legacy
  bounded compatibility limit. The 100 MiB contract applies to file-backed
  attachment IDs.
- Oversized multipart uploads return `413`. Invalid, expired, mismatched, or
  unauthorized attachment references return a validation, not-found, or
  authorization response before a task/message/queue mutation is committed.

## State machine

- `staged` -> `claimed`: a task, message, or queue mutation atomically adopts
  the upload after owner, workspace, task/session, count, and aggregate-size
  validation.
- `staged` -> `deleted`: the owner removes the attachment before submission.
- `staged` -> `expired`: the 24-hour staging deadline passes.
- `claimed` -> `deleted`: the owning task/message lifecycle deletes the durable
  attachment.
- A queued attachment remains claimed while queued. Editing or deleting the
  queue entry releases no file still referenced by the replacement entry, and
  draining the queue transfers the same attachment ownership to the persisted
  user message without re-uploading it.

## Permissions

- Upload requires access to the declared workspace. Supplying a task or session
  additionally requires access to that resource and a matching task/session
  pair.
- Only the staging owner may read or delete an unclaimed upload.
- After claim, any caller allowed to read the owning task transcript may read
  the attachment. Cross-workspace, cross-task, and guessed-ID access is denied
  without revealing a host path.
- Agent materialization uses the server-resolved task/session execution. The
  client cannot select an executor destination path.

## Failure modes

- If the request crosses 100 MiB, the server stops reading, removes the private
  temporary file, records no usable staging row, and returns `413`.
- If storage is unavailable or the streamed write/rename fails, the upload is
  failed and no task or message is submitted with a dangling descriptor.
- If a staged upload expires before submission, the composer marks it failed
  and asks the user to reattach it; prompt text and other attachments remain.
- If claim validation fails, the enclosing task/message/queue mutation has no
  partial attachment claims.
- If executor materialization fails, the prompt is not sent as though the file
  were present. The user sees an attachment-delivery error and may retry.
- If an unsubmitted attachment cleanup request fails, expiry and storage
  maintenance remain the recovery path.

## Persistence guarantees

- Staged uploads and their descriptors survive backend and browser reloads for
  24 hours unless the owner removes them.
- Claimed uploads survive backend restarts and remain readable with their
  message/task until the owning data is deleted.
- Browser storage contains descriptors only. File bytes are held by the
  backend, and executor materializations remain session-scoped working copies.
- Storage maintenance inventories owned attachment files and fails closed when
  database ownership cannot be established.

## Scenarios

- **GIVEN** a 22.8 MiB diagnostic ZIP, **WHEN** the user attaches it to a new
  task and submits, **THEN** the upload becomes ready, the task is created, and
  the agent receives a workspace path to the ZIP.
- **GIVEN** one file exactly 100 MiB, **WHEN** its upload completes and the user
  submits, **THEN** the submission is accepted.
- **GIVEN** one file of 100 MiB plus one byte, **WHEN** upload begins, **THEN**
  the server returns an oversized error and no ready attachment is created.
- **GIVEN** multiple ready files totaling exactly 100 MiB, **WHEN** the user
  submits them, **THEN** the submission is accepted; a one-byte aggregate
  excess is rejected without a partial message.
- **GIVEN** a ready staged attachment and unsent prompt text, **WHEN** the page
  reloads within 24 hours, **THEN** both return without storing the file bytes
  in browser storage.
- **GIVEN** a staged attachment older than 24 hours, **WHEN** the composer is
  restored or submitted, **THEN** it is shown as expired and must be reattached.
- **GIVEN** an attachment owned by another user or workspace, **WHEN** a caller
  guesses its ID for download or submission, **THEN** access is denied and no
  task/message mutation occurs.
- **GIVEN** a message with a ready attachment is queued while an agent is busy,
  **WHEN** the queue entry drains, **THEN** the same file is attached to the
  persisted message and delivered once without re-upload.
- **GIVEN** a queued image has an `attachment_id` and no inline `data`, **WHEN**
  the queue panel renders it, **THEN** the thumbnail and full-size preview load
  from the authorized attachment content endpoint; a legacy inline image still
  loads from its MIME-qualified data URL.
- **GIVEN** a phone viewport, **WHEN** an upload fails or exceeds the limit,
  **THEN** the existing composer shows reachable retry/remove actions and a
  contained localized error with the 100 MiB limit.

## Out of scope

- Files larger than 100 MiB or aggregate submissions larger than 100 MiB.
- Resumable, chunked, or cross-device draft uploads.
- Per-user or per-workspace configurable attachment limits.
- External object storage or direct browser-to-executor uploads.
- Changing the separate 10 MiB task-document attachment contract.
- Changing the queue's existing compact thumbnail dimensions or crop policy.