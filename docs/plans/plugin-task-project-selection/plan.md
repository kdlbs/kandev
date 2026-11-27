---
created: 2026-11-24
status: done
requirements:
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/plugins/system-design/plugins-02.md
legacy_specs: []
---

# Fix plan for plugin task project selection

## Overview

Let a plugin create a root task in an Office project by supplying its project
ID directly. One work order covers both SDK create methods. [GitHub issue #5491](https://github.com/kdlbs/kandev/issues/5491)

## Evidence and root cause

On `8aab9447b35f`, the generated protobuf descriptors expose `Task.project_id`
at tag 28. `CreateTaskRequest`, the same message embedded in the exact create,
and `UpdateTaskRequest` have no such field. The Go SDK and the Host create adapter
carry no project selection. The canonical task service already accepts, persists,
and projects a project ID. Creating a subtask without a project can inherit the
parent's project, which explains the reported anchor-task workaround.

Both SDK creates reach `taskReader.prepareCreate` and
`pluginsTaskWriterAdapter.createTask`. The shared create request is the
smallest place to add the field.

## Scope

### In scope

- An optional project ID on `CreateTaskRequest`, including exact create.
- Same-workspace project validation through the existing task creation path.

### Out of scope

- Project reassignment. The Office operation has its own budget and event handling.
- Project discovery, project CRUD, and new browser or frontend SDK surfaces.

## Technical approach

1. Add optional `project_id` to `CreateTaskRequest`. Exact create already
   embeds that request. Forward the field through the shared SDK and Host
   mapping to the task service.
2. Use each method's existing permissions and the Office project workspace
   lookup. Keep the membership check in the existing create path, including
   its write transaction.
3. Keep an omitted ID out of the exact request digest so old retries still
   match. A supplied ID is part of the request identity.

[Host data API design](../../specs/plugins/system-design/plugins-02.md#office-project-selection-on-task-creation)

## Tests

| Contract | Evidence |
| --- | --- |
| `AC-PLUGINS-PLUGINS-001.17` | Extend the SDK and Host wire tests. Verify the stored root task with the existing Office fixture. |
| `AC-PLUGINS-PLUGINS-001.18` | Run the existing manifest and workspace approval regressions. |
| `AC-PLUGINS-PLUGINS-001.19` | Exercise missing, empty, and foreign IDs, including the explicit repository path. |
| `AC-PLUGINS-PLUGINS-001.20` | Run the existing parent inheritance coverage. |
| `AC-PLUGINS-PLUGINS-001.21` | Extend the exact create test for project payload conflicts and historical digests. |

Start with the missing `project_id` descriptor assertion in
`TestCreateTaskInputRichProtoRoundTrip`.

## Work orders

- [x] [Task 01 Create plugin tasks in an Office project](task-01-create-tasks-in-projects.md)

## Verification results

- The descriptor probe reproduced the missing input on the current checkout.
- The focused SDK task write and protobuf round-trip tests passed. The focused
  backend exact command, project repository validation, parent inheritance,
  and task writer tests passed.
- The specification catalog and lint, PR documentation coverage preflight,
  and SDK release check passed. GitHub confirmed the issue assignment to
  `carlosflorencio`.
- The SDK protobuf round-trip cases, Host wire and exact-command cases, and
  Office task creation tests passed. The historical exact request digest stayed
  unchanged when `project_id` was omitted.
- The affected backend packages built. Proto generation, SDK consumer smoke,
  SDK release check, public documentation validation, specification catalog
  validation, specification lint, changed-Go lint, and `git diff --check` passed.
- PostgreSQL-specific coverage was not run because
  `KANDEV_TEST_POSTGRES_DSN` is unset.
- Review follow-up covered project deletion between admission and repository
  source reads, exact-create refusal receipts and replay, and PostgreSQL-aware
  project test setup. Focused regressions passed; the PostgreSQL branch remains
  unexecuted because `KANDEV_TEST_POSTGRES_DSN` is unset.

## Risks

- Exact commands store a digest from the old SDK shape. Serializing a new
  field on omitted input would turn a valid retry into a conflict.
- Older Kandev releases ignore unknown protobuf fields. Plugin authors need
  the host minimum from the SDK release that includes project selection.
