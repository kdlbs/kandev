---
id: "01-create-tasks-in-projects"
title: "Create plugin tasks in an Office project"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.17
  - AC-PLUGINS-PLUGINS-001.18
  - AC-PLUGINS-PLUGINS-001.19
  - AC-PLUGINS-PLUGINS-001.20
  - AC-PLUGINS-PLUGINS-001.21
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
---

# Task 01 Create plugin tasks in an Office project

## Summary

Carry an optional project ID through the shared task create request. Reuse
the existing permissions, task creation, and exact retry behavior.

## In scope

- Add optional field 12 to `CreateTaskRequest` and regenerate the Go
  bindings. Carry it on `CreateTaskInput.ProjectID` and forward it through
  `taskReader.prepareCreate` and `pluginsTaskWriterAdapter.createTask`.
- Use the existing Office workspace lookup for an explicit project. Check
  before identifier and repository work even when the caller supplies a
  parent or repositories. Recheck in `insertTaskTx` as part of the task
  write. Use the existing invalid and conflict result types.
- Omit a nil field from the exact payload digest. Keep project lookup on
  the new-task path so existing exact receipts can replay.
- Document the create field and minimum host requirement in the plugin
  authoring guide.

## Out of scope

- Changes to task update, project administration, browser UI, and release publication.

## Acceptance

- Both SDK methods can create a root task in the selected project using
  their existing permissions.
- Reject an empty, missing, or foreign project before creating a task.
- Omission preserves parent inheritance and old exact retries. Changing
  the project with the same exact idempotency key returns a conflict.

## Regression evidence

1. Extend `TestCreateTaskInputRichProtoRoundTrip` with a failing descriptor
   assertion first. After adding the field, cover omitted, set, and empty values.
2. Extend `TestPluginHostData_Wire_Writes` and `TestExactTaskCommands` to create
   new tasks with projects over gRPC and reject an explicit empty ID. In the
   exact test, check the conflict for a changed project. Keep the original
   request as the digest fixture. Its
   digest on `8aab9447b35f` is
   `sha256:c2f23e35215828fd8af686096239277eafe8163493c91565632b10dce853041c`.
3. Use the existing Office fixture for `TestPluginsTaskWriter_CreateTaskInProject`.
   Assert a stored root task with the selected project and reject missing and
   foreign projects with explicit repositories. Cover the insert recheck with
   a project removed after the lookup. Reuse the existing approval, project
   repository, and parent inheritance regressions.

## Verification

```bash
make -C pluginsdk proto
(cd pluginsdk && go test -trimpath -count=1 -run '^(TestCreateTaskInputRichProtoRoundTrip|TestCreateTaskInputProjectIDProtoRoundTrip|TestHostData_TaskWritesAndMessage)$' .)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run '^(TestPluginHostData_Wire_Writes|TestPluginHost_Tasks_CreateDeniedWithoutWriteCapability|TestExactTaskCommands|TestExactTaskCreatePersistsProjectRejections|TestExactTaskCreateRetriesTemporaryWriterFailure|TestExactTaskCreateRecoversDomainCommit|TestExactHostAdmissionSerializesApprovalRevocation)$' ./internal/plugins)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run '^(TestCreateTaskRootProjectRepositories|TestCreateTaskProjectRepositorySourceFailuresPrecedeTaskCreation|TestCreateTask_Subtask_InheritsParentProject|TestPluginsTaskWriter_CreateTaskInProject|TestPluginsTaskWriter_CreateMapsProjectSelection|TestOfficeProjectRepositoryReaderWiring|TestOfficeProjectRepositorySourceAdapterRejectsReadAndDecodeFailures)$' ./internal/task/service ./internal/backendapp)
(cd apps/backend && go test -trimpath -tags fts5 -count=1 -run '^(TestGetProject_NotFound|TestGetProjectWorkspaceID)$' ./internal/office/repository/sqlite)
(cd apps/backend && go build -trimpath -tags fts5 ./internal/plugins ./internal/backendapp ./internal/task/service ./internal/task/repository/sqlite)
node scripts/pluginsdk/check-proto-generation.mjs
node scripts/pluginsdk/consumer-smoke.mjs --mode local --backend-only
node scripts/release/plugin-sdk.mjs --check
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use `KANDEV_TEST_POSTGRES_DSN` with the same writer test command to cover
PostgreSQL.

## Files likely touched

- `pluginsdk/proto/kandev/plugin/v1/plugin.proto` and generated Go bindings
- `pluginsdk/data_types.go` and `pluginsdk/data_types_test.go`
- `apps/backend/internal/plugins/host_write.go`, `host_exact_tasks.go`, and
  the existing Host wire and exact task tests
- `apps/backend/internal/backendapp/services.go`, `adapters_office.go`, and
  `office_project_repositories_test.go`
- `apps/backend/internal/task/service/`,
  `apps/backend/internal/task/repository/admission/`, and
  `apps/backend/internal/task/repository/sqlite/task.go`
- `docs/public/plugins-authoring.md`

## Dependencies

None.

## Risks

- Exact retries depend on the existing request digest when the field is omitted.
- Older hosts disregard the new protobuf field. Use the SDK release's host minimum.

## Parallelism

`sequential`

## Inputs

- [Plugin System Requirements](../../specs/plugins/requirements/plugins.md#req-plugins-plugins-001-plugin-system)
- [Host data API design](../../specs/plugins/system-design/plugins-02.md#office-project-selection-on-task-creation)
- [Capability approval](../../specs/plugins/system-design/capability-approval.md)
- [Project repository context](../../specs/tasks/system-design/project-repository-context.md)
- [Project repository context implementation](../office-project-repository-context/plan.md)
- [ADR 0043](../../decisions/0043-plugin-host-data-api.md)
- [Published SDK decision](../../decisions/2026-10-09-published-plugin-sdk.md)

## Results

- Added and generated optional `CreateTaskRequest.project_id`, preserving nil,
  explicit-empty, and set values in the SDK. Omitted project input retains the
  historical exact-create payload digest.
- Both create methods map project selection through the shared task service.
  Explicit projects are checked in the authorized workspace before task
  preparation, then checked again in the insert transaction. Missing and
  foreign projects return InvalidArgument; membership changes at insert return
  the existing conflict result.
- Updated the public plugin authoring guide with field behavior and the minimum
  host-version requirement.
- Focused SDK, plugin Host, exact-command, task-service, and backendapp tests
  passed. A targeted build of the affected backend packages passed.
- Proto generation check, SDK consumer smoke, SDK release check, public docs
  tests and validation, spec catalog validation, spec lint, changed-Go lint,
  and `git diff --check` passed. PostgreSQL-specific coverage was not run
  because `KANDEV_TEST_POSTGRES_DSN` is unset.
- Review regressions passed for project deletion between the admission and
  repository-source reads and after source loading. The project fixture uses
  the isolated PostgreSQL test database when `KANDEV_TEST_POSTGRES_DSN` is set,
  and its read-back query uses rebound parameters.
- Exact create now completes invalid-project and membership-change receipts;
  tests verify replay returns the same refusal without task data and does not
  call the writer again. A temporary writer failure remains retryable.
- The PostgreSQL test branch was not run because `KANDEV_TEST_POSTGRES_DSN` is
  unset.
