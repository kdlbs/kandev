---
status: current
system: tasks
requirements:
  - REQ-TASKS-DOCUMENTS-001
  - REQ-TASKS-DOCUMENTS-002
  - REQ-TASKS-DOCUMENTS-003
created: 2026-08-28
updated: 2026-10-08
owners:
  - kandev
---
# Task document persistence lifecycle System Design

## Purpose and boundaries

This design defines the missing-task boundary for backward-compatible task-plan
writes, the attachment publication boundary, and local editor acknowledgement
of successful plan saves. It does not implement the
broader task-documents migration. The existing filename remains for catalog and
plan-reference compatibility.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-DOCUMENTS-001` | [Transactional write](#transactional-write) and [Error contract](#error-contract) |
| `REQ-TASKS-DOCUMENTS-002` | [Attachment publication](#attachment-publication), [Candidate cleanup](#candidate-cleanup), and [Attachment consumers](#attachment-consumers) |
| `REQ-TASKS-DOCUMENTS-003` | [Plan draft acknowledgement](#plan-draft-acknowledgement) and [Plan draft regression strategy](#plan-draft-regression-strategy) |

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

## Plan draft acknowledgement

The task system owns this boundary because the editor submits and observes the
task's backward-compatible plan document. The correction stays inside the
existing local draft/save synchronization. It adds no backend, public API,
store action, event metadata, or version policy. Existing attachment and
missing-task behavior above is independent.

### Existing consumers and responsibilities

- `useTaskPlan` in `hooks/domains/session/use-task-plan.ts` reads the task plan
  from Zustand, saves through `plan-api`, and publishes the returned
  `TaskPlan` through `setTaskPlan` only while its attempt is latest-started for
  that task. It returns a successful result even when an older attempt's
  publication is suppressed. It may publish a legitimate background success
  after task navigation. Preserve both behaviors.
- `usePlanDraft` in `use-plan-draft.ts` owns local content, the editor reset key,
  debounce, and the shared `attemptSave` entry point. Its current content sync
  treats every differing observed plan as external. This is incomplete when
  an own save acknowledges an older snapshot while the live draft has advanced.
- `TaskPlanPanel` passes the hook's live draft into `TipTapPlanEditor`, keyed by
  task ID and `editorKey`. Saving leaves editing enabled: `readOnly` depends on
  loading only. The keyboard shortcut and `PlanPanelHeader`'s save-before-
  implementation path both use `attemptSave`. Keep those consumers intact.
- `TipTapPlanEditor` initializes TipTap from `value`; a changed parent key
  destroys that editor and constructs a replacement. Preventing the own-save
  reset therefore protects content, selection, and focus at the actual consumer.

### Baseline and attempt ownership

Keep two values distinct: the persisted baseline observed from `plan.content`
and the current editable `draftContent`. Observing a successful own snapshot A
advances the baseline, without assigning A to a newer live draft B. Dirty state
continues to compare the live draft with the observed plan, and the existing
debounce can submit B once saving settles. A returned truthy save result by
itself does not advance the baseline: only the published plan does.

Install local own-attempt ownership synchronously before `savePlan` dispatch.
Capture the submitted snapshot, an attempt identity, and the current task-view
identity. Track acknowledgement recognition separately from size-retry
suppression. Recognition must be available when the store publishes, which
occurs inside `savePlan` before the wrapper's promise continuation. Moving a
ref assignment only into `.then()` is insufficient protection for that order.

For this bounded flow, correlate a changed observed plan with an active own
submitted snapshot or an unconsumed successful own receipt in the same task
view. Consume that recognition once. Use the returned result's identity when
available to narrow correlation; do not introduce a permanent content whitelist.
Retire failed/null attempts, consumed receipts, superseded unpublished attempts,
and obsolete task-view records. A callback can update only its own still-owned
record; it cannot clear a newer record merely because their content is equal.
If publication is observed before the wrapper settles, remember consumption
until that attempt settles rather than reviving it in the completion callback.
If settlement happens first, retain only the receipt needed for its pending
publication observation. Equal-content success needs no editor reset or future
receipt exemption.

This local correlation does not establish global source provenance. An external
writer publishing the identical snapshot during an own request is outside the
arbitrary concurrent-writer reconciliation excluded by the requirement. Keep
the current transport/event ordering; do not invent revision comparisons,
merge text, serialize requests, or add global bookkeeping to resolve it.

### Synchronization and recovery

1. On a task-view change, clear local ownership and suppression, adopt that
   task's current plan or empty content, and retain the existing external-sync
   autosave guard. Track a view generation as well as the task ID so A-to-B-to-A
   cannot re-adopt an old callback as ownership of the new A view. A guard must
   read current identity, rather than compare values captured by one closure.
2. On matching same-view own publication, update the observed baseline and
   consume the receipt. Preserve the live draft and `editorKey`; do not mark
   this as an external replacement or suppress B's next debounce. If the draft
   equals the new baseline, dirty state clears without another save.
3. On a genuine external change, keep the existing replacement/remount and
   one-cycle autosave guard. This still applies with a dirty local draft and
   for deletion. Clear obsolete acknowledgement recognition so a later external
   update cannot be hidden by a past submitted string.
4. On failure, leave draft/baseline unchanged. Keep size-only unchanged retry
   suppression, changed-content eligibility, explicit retry, generic retry,
   and save-error task scoping from the [size-limit design](plan-content-size-limit.md).
   A successful old callback must not clear the latest rejected attempt's
   suppression, including when both submissions used identical text.

The existing per-task latest-started admission and boolean saving state in
`useTaskPlan` remain authoritative. Do not infer request order from `isSaving`
or redesign the boolean as a refcount. Only a result actually observed in the
store may be treated as persisted; a suppressed stale success cannot clean the
live draft. Any local helper extraction remains inside this draft lifecycle.

### Responsive and documentation impact

Desktop and phone use the same draft/save hooks and Plan editor. This is state
handling within an existing surface, with no changes to markup, controls,
scrolling, focus interaction, navigation, or breakpoint behavior. Focused real
hook/store tests and a real panel/editor integration case satisfy the
`mobile-parity` state/data exception; no new Playwright coverage or ASCII layout
preview is needed. Exercise both pointer modes in the component case where
practical. Browser/build/full suites are excluded from the design turn.

The public [task plan guide](../../../public/tasks-and-workflows.md#use-the-task-plan)
already describes direct editing with a 1.5-second autosave. The correction
restores that behavior and introduces no new user steps, terms, or API fields.
Internal specification and delivery records suffice. No ADR is required for a
local correction within the established draft/store boundary.

## Plan draft regression strategy

Use `StateProvider`/`createAppStore`, the real `useTaskPlan` and `usePlanDraft`,
and deferred promises only at the plan transport. Seed baseline state through
real store actions, observe the production store publication, then assert the
live draft, dirty state, editor key, and next request arguments. Do not feed
synthetic saved props through rerender or mock either hook or the provider.
Task-ID rerender is appropriate to exercise real task navigation.

`hooks/domains/session/use-plan-draft.save-ack.test.tsx` covers the causal A/B
failure, unchanged success, subsequent edit, create/update, genuine external
updates with clean and dirty drafts, deletion, failed save/retry behavior,
overlapping different and identical submissions, and task/null/round-trip
switches with delayed completions. Check both possible completion orders and
the case of earlier success after a later size rejection. Assert exact request
counts beyond multiple debounce intervals to distinguish preservation from a
lost or spurious autosave.

`components/task/task-plan-panel.save-ack.test.tsx` mounts the real panel and
TipTap editor through the real dynamic adapter and store. Use editor
transactions to produce onChange, defer the transport, type again while saving,
resolve A, and verify editor DOM identity, text, selection/focus, unsaved state,
and B's subsequent transport submission. Mock only transport and unrelated
chrome/services needed for the fixture; never replace the editor with a
textarea or mock the internal draft/save hooks. Existing editor test setup is
the source for narrow browser-environment shims. If a real integration fixture
cannot run without new infrastructure, checkpoint the exact limitation instead
of claiming a mocked editor proves continuity.

Keep the existing draft-size-suppression, use-task-plan, header wiring, session
switch, and TipTap editor suites in the affected regression run. The single
sequential [work order](../../../plans/preserve-plan-typing/task-01-save-ack-continuity.md)
owns the exact commands and implementation evidence.
