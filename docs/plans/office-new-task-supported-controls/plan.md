---
created: 2026-10-27
status: complete
requirements:
  - REQ-TASKS-CREATION-ATTACHMENTS-001
  - REQ-OFFICE-TASK-CREATION-001
system_design:
  - ../../specs/tasks/system-design/task-creation-attachments.md
  - ../../specs/office/system-design/overview-01.md
legacy_specs: []
---

# Implementation Plan: Office Creation Attachment Parity

## Overview

Give Office creation the Kanban attachment experience: file selection, pasted
images, drag/drop, thumbnails, image preview, file metadata, retry, and removal.
Persist accepted files with the task and deliver them through Office scheduling.
Remove the empty More options control.

This package resolves [issue #4636](https://github.com/kdlbs/kandev/issues/4636),
assigned to `carlosflorencio`. It replaces the earlier removal-only draft at
the user's direction. Upload must become a working attachment flow.
The directory remains unchanged so existing package links remain valid.

## Evidence and repair scope

At source baseline `dad660099681cfa64db91dfe31b603d45581e9df`,
`NewTaskBottomBar` renders Upload and dots without actions.
`IssueDraft` has no attachment state, and `NewTaskDialog` submits no descriptors.
The earlier temporary reproduction confirmed inert clicks and was removed.
Its two-control absence assertion is superseded by the attachment requirement.

Kanban already supplies `TaskFormInputs`, `FileAttachment`, clipboard parsing,
file uploads, context chips, and preview components. Reuse that implementation.
The screenshot supplied by the user identifies its paperclip Attach files action.

The backend create contract already accepts attachment descriptors.
However, the handler claims them after `CreateTask` publishes its creation event.
Office uses scheduled launches, and their `LaunchContext` currently has no
creation attachments. The revised package closes both gaps.
Source trace established these gaps. Implementation tests now cover atomic claims, delivery, the shared composer, and the Office browser flows.

The selected owner is Tasks: it owns attachment admission, claims, and delivery.
Office retains its existing control inventory, assignment policy, and navigation.
The new task requirement/design pair defines this complete feature.
The Office pair references it and defines the empty-overflow correction.
The existing file-backed attachment ADR remains applicable. The typed association
is an implementation detail within that boundary, so no additional ADR is needed.

## Scope

### In scope

- Reuse the Kanban composer and its attachment components in Office.
- Select multiple files, paste images, and drop files through one upload flow.
- Show image thumbnails, larger previews, and file metadata previews.
- Retain draft attachments, guard incomplete uploads, and handle late results.
- Claim creation files atomically and preserve their explicit ordered association.
- Carry those files into task-bound Office launches, including delayed launches.
- Remove empty More options and refresh public creation instructions/screenshots.

### Out of scope

- Blocker selection from [#4635](https://github.com/kdlbs/kandev/issues/4635).
- Office comment attachments, new overflow actions, and new supported formats.
- A generic PDF/archive/content renderer beyond current Kanban previews.
- Scheduling-policy, provider-capability, or unrelated transcript changes.
- New upload endpoints, blob stores, or runtime toggles.

## Technical approach

### 1. Atomic creation files and scheduled delivery

Extend the existing typed task-create service request and insertion transaction.
Add the proposed `task_creation_attachments(task_id, attachment_id, position)`
association in `internal/task/repository/sqlite/base_schema.go`.
The existing attachment registry remains authoritative for canonical metadata
and bytes. Store no bytes or user-controlled paths in the association.

The transaction inserts the task, claims the full validated set, and records
order before commit. All task insertion/admission paths share this boundary.
An invalid sibling rolls back the entire task and set. Preserve idempotent
Found outcomes and the existing legacy inline-client behavior.
The handler's post-create file-backed claim cannot remain the admission boundary.

Add a narrow task-service reader and inject it into `SchedulerIntegration`.
Fill its existing `LaunchContext.Attachments` for task-bound launches before
the starter runs. Keep taskless runs empty and retain all existing launch gates.
Treat these files as recurring task context, like the description.
Select only the explicit creation association, never all task attachment rows.
Preserve task scope so a later run/session can use the same files.

Update cleanup reference checks so session/queue removal cannot erase these
files. Task deletion removes associations and registry files through the
existing owned cleanup flow. Include SQLite and PostgreSQL behavior.

### 2. Shared composer and Office draft

Extract the existing `useFileAttachments` logic into
`hooks/domains/task/use-task-create-attachments.ts`.
Give `TaskFormInputs` an optional external controller. Its default Kanban
controller remains internal. Office owns its controller above `DialogContent`,
so closing the content preserves pending files without duplicate uploads.
Only one controller processes an active composer.

Replace the Office description Textarea with a thin `TaskFormInputs` adapter.
Retain title, assignee, project, parent, status, priority, and existing feedback.
The composer's paperclip replaces the inactive Upload control.
Keep the property row and remove the empty dots control.

Reuse `ContextZone`, `ImageItem`, `FileAttachmentItem`, and the image viewer.
Ready images without a local preview use `attachmentContentUrl`, including
large and restored images. Other files expose the existing metadata preview.
Image preview does not imply a generic document renderer.

Store ready descriptors in the existing workspace/parent Office draft record.
Use shared serialization/preview restoration. Keep bytes in memory/backend only.
Guard decoding/processing, uploading, failed, expired, and unresolved files in
both the button and handler. Preserve valid siblings after rejection.
Success clears the draft without deleting claimed files. Discard cleans staged
files and rejects late results. A workspace/parent change isolates its successor.

### Compatibility matrix

| Path | Identity/capability | Intended behavior | Evidence/fallback |
| --- | --- | --- | --- |
| Kanban creation | Default composer controller | Existing file, paste, retry, and preview behavior | Existing composer and attachment lifecycle suites |
| Office creation | Exact workspace/parent draft owner | Same composer UI, ready IDs in `createTask` | Office component and desktop/phone browser cases |
| Direct Office launch | Task-bound `LaunchContext` | Explicit ordered creation set | Service starter capture and backendapp lifecycle proof |
| Routed Office launch | Same context through routing adapter | Preserve the same IDs and modes | Routed starter capture. Read/claim failure blocks launch |
| Native images / path files | Existing adapter capabilities | Existing materialization and delivery behavior | PNG + resource-byte proof. Unsupported modes retain visible existing failure |
| Taskless / old text-only tasks | No creation association | No added attachments or policy changes | Empty-set and taskless controls |

## ASCII UI preview

### UI-01: Desktop creation, ready attachments

Entry: Office Tasks > New Task. The shared composer contains its attachment
strip above the description, matching Kanban.

```text
+------------------------------------------------------------------+
| New issue                                                   [X]  |
| Task title                                                       |
| Verify account recovery                                          |
| For [Assignee v] in [Customer portal v]                           |
| +--------------------------------------------------------------+ |
| | [IMG] screenshot.png [x]   [FILE] logs.zip [x]                  | |
| |                                                              | |
| | Describe the issue...                                         | |
| |                                                              | |
| | [paperclip: Attach files]                                     | |
| +--------------------------------------------------------------+ |
| [Todo v] [Medium v]                                               |
|                                                                  |
| [Discard Draft]                                    [Create Task]  |
+------------------------------------------------------------------+
```

`[IMG]` represents the actual thumbnail. `[FILE]` represents a file icon.
The paperclip is the same accessible action as Kanban, with its existing tooltip.
There is one attachment entry point and no empty dots button.
Desktop hover/focus shows details. Clicking an image opens the larger viewer.
Image and file details include their available metadata and upload state.

### UI-02: Phone creation and attachment details

```text
+----------------------------------+
| New issue                    [X] |  fixed header
|----------------------------------|
| Verify account recovery          |
| For [Assignee v]                 |
| in  [Customer portal v]          |
| +------------------------------+ |
| | [IMG] screenshot.png     [x]  | |
| | [FILE] logs.zip          [x]  | |  scrolling form body
| |                              | |
| | Describe the issue...        | |
| |                              | |
| | [paperclip]                  | |
| +------------------------------+ |
| [Todo v] [Medium v]               |
|----------------------------------|
| [Discard Draft]    [Create Task]  |  fixed, safe-area-aware footer
+----------------------------------+

Tap screenshot chip:
         +------------------------+
         | screenshot.png         |
         | +--------------------+ |
         | |    image preview   | |
         | +--------------------+ |
         | PNG / size / status    |
         | [Open]                 |
         +------------------------+
                 inset details drawer
```

The touch Drawer follows the current `ContextChip` pattern. Open expands an
image into the existing larger viewer. Closing returns to the draft trigger.
File chips open their filename/size details, without an invented archive viewer.
Primary and standalone touch actions retain at least 44px targets.
The form has one vertical scroll owner. The document has no horizontal overflow.

### UI-03: Upload and preview states

```text
Processing/uploading: [IMG screenshot.png  Uploading...] [x]
                     [Create Task] disabled

Failed:              [FILE logs.zip  Upload failed] [Retry] [x]
                     [Create Task] disabled until ready or removed

Image viewer:        +--------------------------------------+
                     | screenshot.png                  [X]  |
                     |                                      |
                     |       larger image, fit to view      |
                     |                                      |
                     +--------------------------------------+
```

All text in the drawings is illustrative English. Reuse localized copy.
Control order, thumbnail placement, focus return, upload gates, and responsive
containment are required. Exact spacing and example filenames are illustrative.
These views cover `AC-TASKS-CREATION-ATTACHMENTS-001.1` through `.9` and
`AC-OFFICE-TASK-CREATION-001.1` through `.3`.

## Tests

| Criteria | Required evidence |
| --- | --- |
| Task `.1`-.3, `.9` | Shared controller/composer tests and real Office picker/paste/preview/retry browser flows |
| Task `.4`-.5 | Ready descriptor reload, same-draft close/reopen, context change, discard, late-upload cleanup, success transfer |
| Task `.6` | Atomic insert/claim/order, mixed-invalid rollback, create event ordering, WIP variants, external-id no-side-effect retries |
| Task `.7`-.8 | Direct/routed scheduling, delayed assignment/restart, missing bytes, unrelated-attachment exclusion, actual agent materialization |
| Office `.1`-.3 | Empty dots absent, real Attach files present, existing property/create/error/assignee/source regressions retained |

New exact test names and files appear in the work orders.
Execution results are recorded in "Verification results" below.

## E2E tests

Add `office/new-task-attachments.spec.ts` and
`office/mobile-new-task-attachments.spec.ts` for the full attachment journey.
Share only the fixture and interaction helpers needed by both suites.
Use real PNG/resource bytes, the real upload endpoint, and a real create request.
Use causal waits and deterministic injected failure/latency for error cases.
The OS clipboard is outside DOM paste simulation evidence.

Run the existing Office creation suites, Kanban size-warning case, and desktop/
phone preparation-attachment suites after shared composer changes.
The backendapp test is the delivery proof, including bytes at materialization.
A visible thumbnail alone is not proof of delivery.

## Work orders

- [x] [Task 01: Claim creation attachments and deliver Office context](task-01-claim-creation-attachments.md)
- [x] [Task 02: Reuse the attachment composer in Office](task-02-composer-parity.md)
- [x] [Task 03: Update public guidance and the screenshot](task-03-public-docs.md)

Run 01, then 02, then 03. All work orders completed sequentially.

Companion records are the existing `prompt-attachments`,
`composer-attachment-scope`, `preparation-attachment-previews`,
`office-beta-create-contract`, and `office-project-repository-context` plans.
Keep their existing scenario coverage and completed historical results.
Their prior results do not certify the changed shared composer or this package.

## Package validation

Run from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/office-new-task-supported-controls
git status --short --untracked-files=all -- docs/specs docs/plans/office-new-task-supported-controls
```

Run the local documentation coverage preflight with both planned production paths.
It validates every pending work order and its references:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/tasks/requirements/task-creation-attachments.md',
  'docs/specs/tasks/system-design/task-creation-attachments.md',
  'docs/specs/office/requirements/overview.md',
  'docs/specs/office/system-design/overview-01.md',
  'docs/plans/office-new-task-supported-controls/plan.md',
  'docs/plans/office-new-task-supported-controls/task-01-claim-creation-attachments.md',
  'docs/plans/office-new-task-supported-controls/task-02-composer-parity.md',
  'docs/plans/office-new-task-supported-controls/task-03-public-docs.md',
];
const result = validateCoverage({
  changedFiles: [
    'apps/web/app/office/components/new-task-dialog.tsx',
    'apps/backend/internal/task/service/service_tasks.go',
    ...docs,
  ],
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
JS
```

## Verification results

- Issue assignment: reverified as `carlosflorencio` on 2026-10-27.
- Source trace: shared attachment UI existed. Office composer, creation
  admission, explicit task association, and scheduler delivery are implemented.
- Catalog validation: passed, with 409 decisions and 1763 specifications.
- Specification linter tests: passed, with 36 tests. Full specification lint passed.
- Documentation coverage preflight: `covered`, with three accepted work orders
  and no reference errors.
- Relative links and whitespace: passed. Obsolete removal-only work orders
  were removed.
- Production changes: complete. Atomic creation claims, ordered association,
  scheduled Office delivery, shared composer reuse, and public guidance shipped.
- Unit verification: focused repository, service, handler, scheduler, lifecycle,
  hook, composer, and Office dialog suites pass. Backend changed-Go lint,
  sqlguard, storeconformance, and frontend eslint/i18n checks pass.
- Browser verification: `office/new-task-attachments.spec.ts` 4/4 and
  `office/mobile-new-task-attachments.spec.ts` 1/1 pass. Existing Office
  creation suites pass on desktop (3/3) and phone (2/2).
- Screenshot: `docs/screenshots/office-new-task.png` recaptured from disposable
  Office data at 896x440. The temporary capture spec was removed.
- Review remediation: creation claims hold the attachment deletion fence (both
  race orderings tested); file preprocessing is a synchronous submission
  blocker with generation/accepting guards; every Office collection mutation
  updates the saved draft; submission freezes file admission, discard, and
  close; real lifecycle delivery is proven by agentctl upload-byte and prompt
  descriptor assertions plus a missing-byte no-prompt case.
- PostgreSQL: the new env-gated dialect test was added; the local PostgreSQL
  DSN was unavailable, so that run remains untested here.

## Risks

- A file picker can appear successful while Office receives no attachment bytes.
- Claims after `task.created` permit a scheduler race.
- Enumerating all task claims leaks later conversation files into creation context.
- Session cleanup can delete task context unless association references protect it.
- Keeping controller state inside Radix content loses pending files on close.
- Late uploads or rejected creates can corrupt a successor draft without ownership guards.
- Changed shared composer behavior requires Kanban regression evidence.
- Concurrent #4635 work can add a real overflow action. Reconcile it before
  preserving a stale assertion that every dots control must be absent.
