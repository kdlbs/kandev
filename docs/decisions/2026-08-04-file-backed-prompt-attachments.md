# ADR-2026-08-04-file-backed-prompt-attachments: File-backed prompt attachments

**Status:** accepted
**Date:** 2026-08-04
**Area:** backend, frontend, protocol, infra

## Context

Prompt attachments currently travel as base64 inside task-create HTTP JSON and
WebSocket message payloads, are copied into message and queue metadata, and are
persisted in browser session storage. That design has an effective limit below
the advertised 10 MiB raw-file limit and cannot safely support 100 MiB files:
base64 expands one 100 MiB file to about 133.3 MiB before JSON and duplicates
those bytes across browser, backend, database, and agentctl memory.

The product needs to accept diagnostic archives and other resource files up to
100 MiB without raising the shared WebSocket frame limit or storing file bytes
in message metadata.

## Decision

Prompt attachments use a two-phase, file-backed flow. The browser streams each
file over authenticated HTTP multipart upload to a backend-owned staging area
under `<ResolvedHomeDir>/attachments`, and task-create, session-message, and
queue contracts carry opaque attachment IDs plus bounded metadata rather than
base64 data. The backend streams request bodies to a private temporary file,
enforces a 100 MiB raw-file limit while copying, atomically commits successful
uploads, and records ownership and lifecycle state in the task repository.

The backend authorizes attachment operations against the authenticated owner,
workspace, task, and session. A submission atomically claims every referenced
staged attachment for the resulting task message or queued message. Claimed
message metadata contains only display and delivery descriptors; it never
contains attachment bytes or a host filesystem path. Existing inline
base64-based clients remain accepted at the old bounded limit during the
compatibility window, but the Kandev web client uses only attachment IDs.

Agent delivery also remains streaming and file-backed. The lifecycle manager
streams claimed files from backend storage to an authenticated agentctl
materialization endpoint, which writes them beneath the session-scoped
`.kandev/attachments/<session-id>/` directory. Agent-facing prompt content then
uses the selected native-prompt or workspace-path delivery behavior without
returning the file bytes to the frontend or persisting them in task messages.

Unclaimed uploads expire after 24 hours. Claimed uploads follow their owning
message/task lifecycle and remain available to authorized transcript reads and
downloads until that owner is deleted. Attachment storage participates in the
typed storage-maintenance inventory and fail-closed cleanup conventions from
ADR 0045 rather than adding an independent broad filesystem sweeper.

The shared application WebSocket retains its 32 MiB inbound-message limit.
Attachment count remains ten, and both the per-file and aggregate raw-byte
limits are 100 MiB per submission.

## Consequences

- A 100 MiB file no longer becomes a roughly 133.3 MiB WebSocket frame or JSON
  database value, and message/history reads stay proportional to metadata.
- Task creation and message submission become two-phase operations. The UI must
  expose upload progress, block submission while uploads are incomplete, and
  recover cleanly from failed or expired staging records.
- Backend storage gains a durable attachment registry, private files, ownership
  checks, expiry, and cleanup responsibilities.
- Agentctl gains a bounded streaming materialization endpoint, but existing
  session-scoped attachment paths and delivery semantics remain reusable.
- The compatibility path temporarily supports two attachment representations;
  validation must never allow a client-supplied host path or attachment ID to
  bypass ownership checks.

## Alternatives Considered

- **Raise the WebSocket limit to roughly 150 MiB and increase the base64
  constants.** Rejected because it multiplies memory use, bloats SQLite or
  Postgres message metadata, exceeds browser storage quotas, and raises the
  resource ceiling for every WebSocket action.
- **Upload directly from the browser to agentctl.** Rejected because task
  creation can attach files before an executor exists, remote agentctl is not a
  public browser boundary, and browser access would bypass backend ownership
  and task authorization.
- **Reuse task-document attachments.** Rejected because prompt attachments have
  message/queue ownership, delivery-mode semantics, staging before task
  creation, and transcript retention that differ from named task documents and
  their revision model.
- **Store large drafts only in IndexedDB.** Rejected because it addresses one
  browser quota but leaves HTTP/WebSocket, database, backend-agentctl, remote
  executor, and cleanup problems unchanged.
