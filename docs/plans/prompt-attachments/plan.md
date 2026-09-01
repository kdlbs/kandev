---
spec: docs/specs/tasks/requirements/prompt-attachments.md
created: 2026-08-04
status: in_progress
---

# Implementation Plan: 100 MiB prompt attachments

## Overview

Replace browser-to-agent base64 attachment payloads with the staged,
file-backed contract from
[ADR-2026-08-04-file-backed-prompt-attachments](../../decisions/2026-08-04-file-backed-prompt-attachments.md).
Build the durable registry and authenticated upload boundary first, then stream
claimed files into agentctl, migrate every web composer to attachment IDs, and
prove the shared desktop/mobile behavior with focused E2E coverage. The shared
application WebSocket remains capped at 32 MiB.

---

## Backend

### Attachment registry and private storage

- Add a `task_message_attachments` model and repository contract under
  `apps/backend/internal/task/models/`,
  `apps/backend/internal/task/repository/interface.go`, and
  `apps/backend/internal/task/repository/sqlite/`.
- Extend replayable SQLite/Postgres schema creation and migrations with the
  fields and indexes from the spec. Index staged expiry, task/message ownership,
  and queue-entry ownership; keep `storage_key` backend-generated and relative.
- Add `AttachmentService` in
  `apps/backend/internal/task/service/attachment_service.go`. It owns bounded
  stream-to-temp upload, `0600` files/`0700` directories, atomic rename, exact
  raw-byte count/aggregate validation, authorization, staged deletion, claim,
  queue ownership transfer, and 24-hour expiry.
- Register attachment storage under `<ResolvedHomeDir>/attachments` in backend
  composition. Add a typed storage-maintenance inventory/cleanup provider so
  orphan classification fails closed when the repository cannot be read.

### HTTP and submission contracts

- Add authenticated handlers in
  `apps/backend/internal/task/handlers/attachment_handlers.go` for:
  `POST /api/v1/attachments`,
  `GET /api/v1/attachments/:id/content`, and
  `DELETE /api/v1/attachments/:id`.
- Stream multipart bodies with `io.LimitReader`/`io.CopyBuffer`; do not call
  `io.ReadAll`, `ParseMultipartForm` with a 100 MiB memory budget, or trust the
  original filename as a path. Return `413` at 100 MiB + 1 byte and remove the
  temporary file on every failure.
- Extend `pkg/api/v1.MessageAttachment` with the opaque attachment ID and raw
  size while preserving the current inline `data` compatibility shape at its
  old limit. Client-supplied paths are never accepted.
- Update task creation, `session.message.add`, `message.queue`, and
  `message.update` validation to claim staged IDs atomically with the enclosing
  mutation and enforce ten files/100 MiB aggregate from repository sizes.
- Change message and queue metadata to persist descriptors only. Queue edit,
  delete, merge, and drain must preserve or release claims deterministically;
  idempotent message replay must return the already-claimed descriptors without
  re-claiming or re-uploading.

### Agentctl materialization and prompt delivery

- Add an authenticated, bounded agentctl attachment-materialization endpoint
  under `apps/backend/internal/agentctl/server/api/` using the same private temp,
  exact byte bound, filename sanitization, and atomic write pattern as the
  existing diagnostic materializer.
- Add a streaming client in
  `apps/backend/internal/agent/runtime/agentctl/client_attachments.go`; it opens
  the authoritative backend file and streams it to the active execution without
  loading the whole attachment or base64 encoding it.
- Update lifecycle launch, resume, direct prompt, and queued prompt paths to
  resolve authorized descriptors and materialize them before agent prompt
  dispatch. A failed materialization aborts prompt delivery and produces the
  existing user-visible prompt error path.
- Refactor
  `apps/backend/internal/agentctl/server/adapter/transport/shared/attachments.go`
  so path-delivered files reuse the materialized session-scoped file instead of
  decoding `data`. Retain the bounded inline compatibility path for older
  clients and native prompt content where still required.
- Keep `apps/backend/internal/gateway/websocket/client.go:maxMessageSize` at
  32 MiB and add a regression proving attachment-ID messages stay below that
  transport boundary independently of file size.

---

## Frontend

### Upload API and attachment state

- Add `apps/web/lib/api/domains/attachment-api.ts` with typed upload, content
  URL, and staged-delete operations. Use authenticated multipart HTTP and map
  `413`, expiry, authorization, and storage failures to stable error kinds.
- Refactor `apps/web/components/task/chat/file-attachment.ts` so a selected
  `File` becomes an upload state (`pending`, `uploading`, `ready`, `failed`) and
  then a server descriptor. Keep preview bytes in an object URL only while the
  page needs them; revoke URLs on removal/unmount.
- Set `MAX_FILE_SIZE` and `MAX_TOTAL_SIZE` to 100 MiB and retain `MAX_FILES = 10`.
  Client checks are fast feedback only; the server is authoritative.
- Update `use-chat-input-state.ts`, task-create selectors/submit helpers,
  new-session/subtask shared prompt flows, and queue hooks so they submit only
  ready attachment IDs and never JSON/base64 bytes.
- Change `lib/local-storage.ts` draft persistence to store descriptors and
  staging expiry only. Restore ready items after reload, detect an expired or
  missing staged item, and preserve prompt text when retry/removal is needed.
- Update transcript attachment rendering to use the authorized content endpoint
  for previews/downloads and to render descriptor metadata without embedding
  a data URL.
- Localize all changed attachment status, limit, retry, expiry, and failure copy
  in `src/locales/en/chat.json` and the generated pseudo locale. Do not leave the
  currently hardcoded feedback strings on touched lines.

### Mobile design contract

- **Outcome and entry:** desktop and phone retain the existing paperclip,
  paste, and drop entry points in their prompt composers; both can upload,
  retry/remove, and submit the same 100 MiB attachment.
- **Nearest exemplar:** preserve the existing inline attachment chips in
  `file-attachment-preview.tsx`; reuse the task mobile composer's current
  single-column composition rather than introducing a dialog or drawer.
- **Hierarchy and action:** the prompt editor remains primary. Each chip shows
  name/size plus upload state; failed items expose visible Retry and Remove
  actions, and incomplete uploads disable the composer submit action with an
  accessible explanation.
- **Geometry:** the composer remains the scroll owner. Status/actions wrap
  inside its width, add no document horizontal overflow, and expose touch
  targets at least 44px in the active dimension on coarse pointers.
- **Shared logic:** upload state, validation, descriptor conversion, and retry
  handlers are shared. Mobile-specific work is limited to presentation classes
  and E2E assertions.

---

## Tests

- **Exact size/count/aggregate and atomic claim:** table-driven service tests in
  `apps/backend/internal/task/service/attachment_service_test.go` use a
  configurable small test ceiling/synthetic readers to prove exact-boundary,
  +1-byte, ten-file, aggregate, expired, owner mismatch, and no-partial-claim
  behavior without allocating 100 MiB in every test.
- **Durable descriptors without bytes:** repository tests in
  `apps/backend/internal/task/repository/sqlite/attachment_test.go` and message
  metadata tests prove claimed rows survive reopen, cascades/cleanup are scoped,
  and neither message nor queue JSON contains `data` or `storage_key`.
- **HTTP upload/download/delete:** handler tests stream multipart bodies through
  the full handler -> service -> repository path and cover `201`, `413`, staged
  delete, range/content headers if supported, auth denial, mismatched task/session,
  and cleanup after interrupted writes.
- **Queue and idempotency:** focused message/queue tests cover edit, remove,
  merge, drain, replay, and aggregate validation with attachment IDs.
- **Agent delivery:** lifecycle/agentctl tests prove a claimed file is streamed
  into the intended session directory, traversal names cannot escape, failed
  materialization prevents prompt dispatch, and inline legacy attachments
  retain the old bounded behavior.
- **Frontend state:** Vitest coverage proves 100 MiB acceptance, 100 MiB + 1
  rejection, aggregate checks, upload-state transitions, retry/removal,
  descriptor-only draft persistence, expiry restoration, object-URL cleanup,
  and task/chat/queue payloads with IDs and no base64 data.

---

## E2E Tests

- **Large task attachment:** update
  `apps/web/e2e/tests/task/create-task-attachment-warning.spec.ts` (rename if the
  final scenarios read more clearly) to attach a file above the former 10 MiB
  ceiling, create the task, and verify the agent-visible workspace path and
  transcript descriptor.
- **Mobile upload parity:** update
  `apps/web/e2e/tests/chat/mobile-attachment-size-warning.spec.ts` to prove a
  file above 10 MiB reaches ready/submitted state on `mobile-chrome`, and that a
  synthetic 100 MiB + 1 file shows contained localized feedback with reachable
  retry/remove behavior and no horizontal overflow.
- **Expiry/failure recovery:** add one focused desktop scenario that forces an
  expired or failed staged descriptor, preserves prompt text, and completes
  after retry or reattachment.

---

## Public documentation

- Update `docs/public/tasks-and-workflows.md` from the old raw/base64 mismatch to
  the ten-file, 100 MiB per-file/aggregate staged-upload contract and recovery
  behavior.
- Update `docs/public/websocket-api.md` to keep the 32 MiB transport limit while
  explaining that current web-client attachment bytes use HTTP staging and only
  descriptors cross the socket; legacy inline callers must still obey the
  lower compatibility limit.
- Reconcile `docs/specs/office/requirements/live-updates.md` where it currently attributes
  the 32 MiB socket ceiling to base64 image traffic.

---

## Verification Results

Backend and frontend production paths are implemented and the public docs are
updated. Full backend verification passed (`go test -tags fts5 ./...`, 9,495
tests in 189 packages); the full web suite passed (1,095 files, 8,439 tests,
4 skipped), with typecheck, lint, i18n checks, and public-doc validators also
passing. Dedicated successful-large-file and expiry-recovery Playwright
coverage remains pending.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-attachment-storage-api](task-01-attachment-storage-api.md) — sequential

Wave 2:

- [x] [task-02-agent-delivery-and-queue](task-02-agent-delivery-and-queue.md) — sequential; depends on task 01

Wave 3:

- [x] [task-03-frontend-staged-uploads](task-03-frontend-staged-uploads.md) — sequential; depends on tasks 01-02

Wave 4 (parallel candidates after task 03; user authorization required for subagents):

- [ ] [task-04-attachment-e2e](task-04-attachment-e2e.md) — parallel-safe with task 05; owns E2E files only
- [x] [task-05-public-docs](task-05-public-docs.md) — parallel-safe with task 04; owns documentation only

Execution remains sequential in the primary conversation unless the user
explicitly authorizes subagents.

## Open risks

- Native prompt providers may impose limits below 100 MiB even after Kandev
  stages the file safely. Path delivery must remain the reliable route for large
  diagnostic/resource files, and provider rejection must stay user-visible.
- A 100 MiB HTTP upload can be slow on remote browser connections. Resumable or
  chunked uploads are explicitly out of scope, so interrupted uploads restart.
- Claimed attachments add durable disk usage. Correct task/message ownership,
  storage inventory, expiry, and fail-closed cleanup are required before the
  limit can ship.
