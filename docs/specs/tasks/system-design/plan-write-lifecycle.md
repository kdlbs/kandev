---
status: current
system: tasks
requirements:
  - REQ-TASKS-DOCUMENTS-001
  - REQ-TASKS-DOCUMENTS-002
created: 2026-08-28
updated: 2026-10-07
owners:
  - kandev
---
# Task document persistence lifecycle System Design

## Purpose and boundaries

This design defines the missing-task boundary for backward-compatible task-plan
writes and the attachment publication boundary. It does not implement the
broader task-documents migration. The existing filename remains for catalog and
plan-reference compatibility.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-DOCUMENTS-001` | [Transactional write](#transactional-write) and [Error contract](#error-contract) |
| `REQ-TASKS-DOCUMENTS-002` | [Attachment publication](#attachment-publication), [Candidate cleanup](#candidate-cleanup), and [Attachment consumers](#attachment-consumers) |

## Components and responsibilities

- `internal/task/repository/sqlite.Repository` owns the transaction and database
  error classification for SQLite and PostgreSQL.
- `internal/task/service.PlanService` owns operational logging and preserves
  typed repository errors.
- `internal/task/planws` owns the shared WebSocket error contract for browser
  handlers and MCP handlers.

## Transactional write

`WritePlanRevision` writes the plan head before it writes or merges a revision.
Both plan tables reference the task row with foreign keys.

If the head write reports a foreign-key violation, the repository returns the
shared `ErrTaskNotFound` sentinel. The transaction then rolls back. No plan head
or revision remains.

The repository uses `internal/db.IsForeignKeyViolation` for both supported
database dialects. It does not inspect a raw database error outside that helper.

## Error contract

The plan service passes `ErrTaskNotFound` to its callers. It records the
expected rejection at debug level. Other write errors remain error-level
entries.

`planws.CreateError` and `planws.UpdateError` map `ErrTaskNotFound` to the
existing `not_found` WebSocket code. The response does not include a database
constraint message.

The browser and MCP surfaces use the same `planws` mapping. No request or
success payload changes.

## Failure and recovery

A concurrent task deletion can occur after access validation and before the
plan transaction. The foreign-key classification closes that race without a
separate task existence query.

An unrelated database error keeps its wrapped diagnostic context. The service
records it at error level and the wire contract returns `internal_error`.

## Test strategy

SQLite and PostgreSQL repository tests cover the sentinel and rollback. Service
tests cover log severity. Shared contract tests cover browser and MCP mappings.

## Attachment publication

`internal/task/service.DocumentService.UploadAttachment` owns preparation and
publication. `internal/task/repository/sqlite.Repository.CreateDocument` and
`UpdateDocument` already persist `TaskDocument.DiskPath` with the attachment
metadata in one statement. No repository, schema, dialect, or public DTO changes
are needed. `DiskPath` is internal (`json:"-"`).

The current canonical `<key>.<ext>` write truncates an already published file
before lookup or metadata persistence can fail. The correction uses one unique,
exclusively created file per upload, in the existing task attachment directory:

1. Preserve task/key/extension validation and the 10 MiB bound. Resolve the
   existing document before allocating a candidate; a lookup error writes no
   bytes. Keep the current `basePath/attachments/<taskID>` service layout.
2. Create a private candidate with `os.CreateTemp` and a fixed safe prefix;
   neither the client key nor filename needs to form its storage basename.
   Keep its restrictive creation permissions. Write all submitted bytes and
   close successfully before attempting metadata publication. Check short
   writes and close errors. Clean preparation failures using that exact owned
   path, after closing any acquired handle.
3. Construct the same attachment HEAD with the candidate's complete path.
   Preserve existing ID and creation time. Call the existing create/update
   method. Its successful row write is the publication boundary; there is no
   rename onto a shared path and no later filesystem step needed for download.
4. Return the existing success shape. On a metadata error preserve its wrapped
   primary error and retain the complete candidate as described below. Do not synthesize a
   successful response or restore a stale row snapshot.

This is local publication ordering, not a filesystem/database transaction.
Complete files may be left unreferenced after a crash. It promises preservation
for an operation rejected before publication; it cannot roll back an independent
successful operation or an ambiguously committed database statement.

## Candidate cleanup

Preparation failure before metadata invocation may remove only this operation's
definitely unpublished candidate after its handle is closed. A lookup failure
allocates no candidate. Once `CreateDocument` or `UpdateDocument` invocation
begins, retain the complete candidate on every returned error and record one
bounded diagnostic while preserving the wrapped primary error. There is no
cleanup verification query: a later row selecting another path, or no row, cannot
prove that the candidate was never published. A download may already have
resolved it before another upload or deletion changed the row.

Paths are unique and never reused or adopted by other uploads. Do not delete a
previous `DiskPath`, construct a canonical path to remove, or glob/scan the
directory. Prepublication removal errors preserve the primary upload error and
receive a diagnostic log; they do not turn a failed upload into success. A
retained complete candidate may be an orphan, consistent with deferred document
file reclamation; no commit-marker or error-classification framework is added.

After a successful upload retain superseded paths, matching the existing lack
of document-file reclamation. This also avoids invalidating a download that has
already resolved an older path. Retained superseded files are unreferenced
storage, not revision history or a second exposed attachment. Reclamation is
outside this bounded correction.

If another upload succeeds while this upload fails, that successful row remains
authoritative. No row rollback is attempted. An error-after-commit followed by
an independent overwrite or deletion still retains that candidate's bytes.
This does not add locking,
compare-and-swap semantics, or universal upload/delete concurrency guarantees.

## Attachment consumers

The source audit at the design base found these boundaries:

- `DocumentService.DownloadAttachment` returns the persisted `DiskPath`, and
  `internal/office/dashboard.DocumentHandler.downloadAttachment` streams it
  with `c.File`. Both accept legacy canonical files and opaque candidate paths.
- `DocumentService.DeleteDocument` and repository `DeleteDocument` remove the
  row/revisions only; they do not unlink binary files. Preserve that policy for
  both legacy and newly published files. A deleted row is no longer downloadable.
- `buildDocHead` preserves attachment fields on an attachment-to-attachment
  text update. Do not change text updates, revert behavior, or document authors.
- `AttachmentService` and `resource_cleanup_jobs.go` manage separate
  `TaskMessageAttachment.StorageKey` descriptors. Their cleanup removes only
  descriptor paths under its own root. It does not sweep task-document files;
  do not route document candidates through prompt-attachment cleanup.
- `office/routes.go` passes `svcs.KandevHome` to `NewDocumentHandler`, which
  passes `basePath` unchanged to the service. The actual service layout is
  `<basePath>/attachments/<taskID>`; correct the handler's inaccurate storage
  comment if touched, without relocating existing files.

No HTTP route, access guard, response field, rendered UI, or localization changes
are required. The registered Office upload route keeps its current 500 mapping
for service failures, 413 for oversized input, and 200 payload for success.
Download continues to use metadata filename/MIME headers.

## Attachment regression strategy

Permanent tests exercise the real `DocumentService`, a real SQLite repository,
and files under `t.TempDir`. Inject only specific repository failures via a
wrapper that otherwise delegates to the real repository. Compare the full
persisted row after rejection and bytes obtained through production download.
Check first upload, same/different extension replacement, preparation failures,
successful replacement, cleanup isolation, and legacy download/delete. An
error-after-real-commit case followed by independent successful replacement or
deletion must retain the candidate, including bytes resolved by an earlier
download. Assert the current, prior and independently published files survive;
do not infer nonpublication from the current pointer. Prepublication write/close
failures must meaningfully prove owned candidate cleanup. Ordinary create/update
rejections must also retain their complete candidates. No universal concurrency
or download-lifetime scheme is introduced.

HTTP tests register `RegisterDocumentRoutes`, submit multipart requests, and
download through the router using the real service/repository/filesystem. Cover
lookup/update rejection and successful replacement with byte and header checks.
Use a narrow per-service file-writing seam only if necessary to drive partial
write/close failure through production upload; avoid a generic filesystem
abstraction or global mutable hooks. Real deterministic filesystem failures
cover directory/candidate creation; permission tests require an enforcement
probe on privileged hosts.

Since publication path creation changes, add a narrow actually executed
`test-windows` native-lane step for portable service attachment regressions.
Use native paths and close handles before removal. A Windows build alone is
not evidence. PostgreSQL fixtures are unnecessary while SQL and repository
contracts remain unchanged.
