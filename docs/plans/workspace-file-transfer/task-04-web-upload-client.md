---
id: "04-web-upload-client"
title: "Web upload client, selection normalization, and two-phase hook"
status: done
wave: 3
depends_on:
  - "03-backend-upload-route"
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.6
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.5
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 04: Web upload client, selection normalization, and two-phase hook

## Summary

The frontend transport and flow control: normalize any selection into destination-relative paths,
call the preflight, and drive the uploads. No user-visible entry point yet, and no dialog yet; this
is the machinery both depend on.

## Scope

- `apps/web/lib/utils/upload-selection.ts` flattening a selection to `{ file, relativePath }`. A
  picked folder carries `webkitRelativePath`; a flat multi-file pick yields bare names. Both normalize
  to one shape, so callers never branch on how the files were chosen.
- `apps/web/lib/api/domains/workspace-file-api.ts` with `preflightWorkspaceUpload` (JSON) and
  `uploadWorkspaceFile` (`FormData`), mirroring `attachment-api.ts` for `credentials: "include"`, the
  interim settings interlock header, and `ApiError` on a non-OK response.
- One upload request per file, so a rejected file does not fail the rest of a selection.
- `apps/web/hooks/use-file-upload.ts` returning
  `{ uploadFiles(dir, selection), uploads, conflicts, resolveConflicts, cancel }`, with per-file
  status `pending | uploading | ready | failed` plus `blocked` for a conflict awaiting a decision.
- Upload state is owned by `use-file-upload-entry-points.tsx`, which mounts one flow for both
  Files-panel entry points. `use-file-operations.ts` keeps CRUD and download calls.
- Optimistic insertion with `insertNodeInTree`, replaced with the server-reported path on success and
  removed on failure.

## Exclusions

- The conflict dialog component. That is task 05; this task exposes the state it binds to.
- The toolbar menu and context-menu item. Those are task 06.
- Download work.

## Acceptance

- A selection with no conflict uploads without ever entering the blocked state, and a selection with
  conflicts stops before any upload request is sent.
- Cancelling resolution sends no upload request at all, including for files that had no conflict.
- Per-file status transitions are observable, one failing file leaves the others' results intact, and
  a successful file is inserted at the server-reported path rather than the requested one.

## Verification

Write the flattening and flow-control tests first and confirm they fail before the production change.
Then:

```bash
cd apps && pnpm --filter @kandev/web test -- lib/utils/upload-selection.test.ts lib/api/domains/workspace-file-api.test.ts hooks/use-file-upload.test.ts
cd apps/web && node ../node_modules/typescript/bin/tsc --noEmit
```

The typecheck is spelled out because `pnpm run typecheck` exhausts the default heap on this host.

## Files likely touched

- `apps/web/lib/utils/upload-selection.ts` and its test
- `apps/web/lib/api/domains/workspace-file-api.ts` and its test
- `apps/web/hooks/use-file-upload.ts` and its test
- `apps/web/hooks/use-file-operations.ts`

## Dependencies

Task 03.

## Parallelism

Sequential. It calls the routes task 03 introduces.

## Inputs

- Requirements: `REQ-UI-WORKSPACE-FILE-TRANSFER-001` and `-004`.
- System design: `Components and responsibilities > Frontend`, `Control flow`.
- Existing patterns: `uploadAttachment` for the request shape, `file-attachment.ts` for the status
  vocabulary, `handleCreateFileSubmit` for optimistic tree insertion.

## Risks

- Starting non-conflicting uploads while the dialog is open would break the cancel guarantee. Do not
  optimize that way.
- `fetch` has no upload progress event. If per-file progress is wanted rather than a running
  indication, `XMLHttpRequest` is required; decide before building the status model.
- The server-reported path is authoritative after `keep_both`. Inserting the requested name leaves
  the tree wrong.

## Output contract

Report the normalized selection shape, the hook's state machine, files changed, exact commands and
results, then mark this task `done` and update its checkbox in `plan.md`.

## Results

- `lib/utils/upload-selection.ts` normalizes both pickers to `{ file, relativePath }`, rejecting
  absolute paths and any `..` segment per entry rather than failing the batch.
- `lib/api/domains/workspace-file-api.ts` with `preflightWorkspaceUpload` (JSON) and
  `uploadWorkspaceFile` (`FormData`), one request per file, `ApiError` preserving the server message
  and status so a 409 stays distinguishable.
- `hooks/use-file-upload.ts` owns the two-phase flow. The batch is parked in a promise while the
  dialog decides; `resolveConflicts` applies per-file choices and `cancelConflicts` resolves with
  `cancelled: true` having uploaded nothing.
- Upload state is mounted by `use-file-upload-entry-points.tsx`, so both Files-panel entry points
  share one flow while `use-file-operations.ts` remains focused on CRUD and download calls.

**Cancel is proven, not assumed.** `uploadWorkspaceFile` is asserted never-called after a cancel,
including for the file in the selection that had no conflict. That is the assertion that forbids the
tempting optimization of uploading unconflicted files while the dialog is open.

Refactored `performUploads` / `buildUploadItems` / `destinationIndex` out of the hook body to stay
under the 100-line function limit.

### Commands

```
pnpm --filter @kandev/web test -- lib/utils/upload-selection.test.ts lib/api/domains/workspace-file-api.test.ts hooks/use-file-upload.test.ts   26 passed
node ../node_modules/typescript/bin/tsc --noEmit                                                                                               clean
pnpm run lint                                                                                                                                  0 problems
```
