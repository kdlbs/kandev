---
id: "01-claim-creation-attachments"
title: "Claim creation attachments and deliver Office context"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CREATION-ATTACHMENTS-001
acceptance_criteria:
  - AC-TASKS-CREATION-ATTACHMENTS-001.6
  - AC-TASKS-CREATION-ATTACHMENTS-001.7
  - AC-TASKS-CREATION-ATTACHMENTS-001.8
system_design:
  - ../../specs/tasks/system-design/task-creation-attachments.md
---

# Task 01: Claim Creation Attachments and Deliver Office Context

## Summary

Persist an explicit creation attachment set before any Office wake can launch.
Deliver that set through existing task-bound Office launch contexts and retain
it through individual session cleanup.

## In scope

- Typed task-create attachment admission, derived from authenticated uploader identity.
- Atomic task insertion, registry claim, and ordered creation association.
- Common normal/WIP insertion paths, duplicate-create semantics, and rollback.
- Task-service reader, scheduler injection, direct/routed launch delivery, and taskless controls.
- Cleanup/reference protection and schema compatibility for SQLite/PostgreSQL.
- Handler, repository, scheduler, and real lifecycle integration proof.

## Out of scope

- Browser/UI implementation and Office scheduling-policy changes.
- New upload endpoints, blob stores, provider formats, or implicit file selection.
- Session initial-submission recovery and automatic replay changes.

## Acceptance

1. Every successful file-backed create commits the exact ordered set and claims
   before the task is visible. Any invalid sibling rolls back all new claims
   and the task. Duplicate Found outcomes leave retry uploads untouched.
2. Every task-bound Office launch carries only the task's creation files.
   Direct/routed, delayed assignment, pause/restart, and taskless cases follow
   the design. Missing bytes fail visibly without text-only replacement.
3. Real PNG/resource files reach lifecycle materialization with correct bytes.
   Session/queue cleanup preserves them, and task deletion removes them.

## Implementation notes

Start with `TestHTTPCreateTaskOfficeAttachmentsBeforeWake` against the real
create handler and task service. At `task.created`, inspect attachment claim
state and the explicit set. Current post-create claims must fail this assertion.
Include a mixed batch with one valid row and one foreign/expired/missing row.

Follow [atomic creation ownership](../../specs/tasks/system-design/task-creation-attachments.md#atomic-creation-ownership).
Use typed internal input instead of public metadata. The proposed association
contains only task ID, attachment ID, and position.
Factor existing claim logic for the caller's insertion transaction.
Do not weaken locking or count total bytes from client declarations.
Use registry canonical fields and preserve the bounded inline compatibility path.

Keep `external_id` admission order and Found outcomes unchanged.
Use the association as a live reference in every applicable cleanup path.
Remove associations at task deletion before registry deletion under its owned
cleanup transaction/job. A restrictive foreign key alone cannot replace safe
cleanup selection.

Inject a narrow creation-attachment reader into `SchedulerIntegration` in
`internal/backendapp`. Fill the existing launch attachment field.
Retain all existing launch guards and correlated failure handling.
Task creation files remain task-scoped and cannot be rebound to one session.

Add these test groups:

- `task/repository/sqlite/task_creation_attachments_test.go`:
  `TestTaskCreationAttachmentsAtomic`, `TestTaskCreationAttachmentsCleanup`,
  `TestTaskCreationAttachmentsRestartAndOrder`, and WIP/duplicate subcases.
- `task/repository/sqlite/task_creation_attachments_postgres_test.go`:
  `TestTaskCreationAttachmentsPostgres`, using existing PostgreSQL test setup.
- `task/handlers/task_http_creation_attachments_test.go`:
  `TestHTTPCreateTaskOfficeAttachmentsBeforeWake` and rejection subcases.
- `office/service/task_creation_attachments_test.go`:
  `TestSchedulerTickCreationAttachments` for direct/routed, empty/taskless,
  delayed assignment, later task runs, and missing/foreign association cases.
- `backendapp/office_creation_attachments_test.go`:
  `TestOfficeCreationAttachmentsMaterialized`. Reuse the production starter
  and lifecycle-adapter pattern from `office_project_workspace_test.go`.
  Inspect native image descriptors and installed resource bytes.

## Verification

Run from the repository root. First run the new handler regression before the
correction and record the expected red claim/order failure.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers -run 'TestTaskCreationAttachments|TestHTTPCreateTaskOfficeAttachments|TestCreateTaskWithExternalID' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/office/service -run 'TestSchedulerTickCreationAttachments|TestSchedulerTick_LaunchesAgent|TestSchedulerTick_StartTaskError_TriggersRetry' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestOfficeCreationAttachmentsMaterialized$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-Go lint with `bash scripts/lint-go-changed <exact changed Go paths>`.
Use the final path list rather than linting the whole backend.
Run the documented PostgreSQL test setup and the new PostgreSQL test without
skipping it before recording cross-database evidence. Report an unavailable
database as untested, not passed. Run the plan's reference preflight.

## Files likely touched

All paths below start with `apps/backend/internal/`:

- `task/handlers/task_http_handlers.go` and new handler regression file.
- `task/service/service_requests.go`, `service_tasks.go`, `service_attachments.go`.
- `task/models/models.go` for internal typed input, if required by insertion.
- `task/repository/interface.go`, `sqlite/base_schema.go`, `sqlite/task.go`.
- `task/repository/sqlite/attachment.go` and new association/cleanup helpers/tests.
- `office/service/scheduler_integration.go` and new attachment scheduler tests.
- `backendapp/main.go` or its extracted Office wiring file, plus integration test.

## Dependencies

None. Task 02 uses this accepted creation and delivery path.

## Risks

- An insertion path or cleanup path can bypass the new invariant.
- A task-wide inventory read can include unrelated session attachments.
- A descriptor capture without actual byte materialization can hide delivery loss.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/task-creation-attachments.md).
- [Design](../../specs/tasks/system-design/task-creation-attachments.md).
- Existing file-backed attachment ADR and repository claim helpers.
- Scoped backend/Office guidance and `/tdd`.

## Results

Complete. File-backed creation descriptors are validated in the task service, claimed and associated in the task insert transaction, and read through a narrow task-owned reader injected into the Office scheduler. The insert claim holds the attachment service's lifecycle fence, so staged deletion/expiry cannot unlink bytes under an uncommitted create; a deterministic race test covers deletion-first and create-first orderings. Session and workspace cleanup preserve or explicitly remove the creation set before attachment rows; task deletion removes associations and registry rows through the existing owned cleanup path. A later create-step rollback also releases the new claims and bytes.

Verification passed:

- `go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers -run 'TestTaskCreationAttachments|TestCreationAttachmentClaimFencedAgainstDelete|TestHTTPCreateTaskOfficeAttachments|TestHTTPCreateTaskRejectsInvalidOfficeAttachment|TestCreateTaskWithExternalID' -count=1`
- `go test -trimpath -tags fts5 -race ./internal/office/service -run 'TestSchedulerTickCreationAttachments' -count=1`
- `go test -trimpath -tags fts5 ./internal/backendapp -run '^TestOfficeCreationAttachmentsLaunchContext$' -count=1`
- `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestSessionPromptMaterializesClaimedAttachments$' -count=1`
- `go run ./cmd/sqlguard ./internal`
- `go test -trimpath -race ./internal/persistence/storeconformance -count=1`
- `bash scripts/lint-go-changed <changed Go paths>`: 0 issues.

The lifecycle test streams real claimed bytes to the agentctl materialization endpoint, asserts the resulting prompt descriptors contain no inline data, and proves missing bytes fail before an attachment-free prompt is sent. `TestOfficeCreationAttachmentsLaunchContext` separately proves the production Office starter forwards the ordered creation set. The PostgreSQL test file was added, but `KANDEV_TEST_POSTGRES_DSN` was unavailable locally, so the PostgreSQL run is untested here rather than reported as passed.
