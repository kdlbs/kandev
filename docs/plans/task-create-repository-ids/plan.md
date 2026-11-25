---
created: 2026-11-25
status: implemented
requirements:
  - REQ-TASKS-HTTP-REPOSITORY-SELECTION-001
system_design:
  - ../../specs/tasks/system-design/http-task-repository-selection.md
legacy_specs: []
---

# Fix plan for REST task repository IDs

[GitHub issue 5503](https://github.com/kdlbs/kandev/issues/5503)

## Root cause and reproduction

`httpCreateTaskRequest` declares only `repositories`. The JSON binder discards
the unknown `repository_ids` field. The handler passes a nil selection to the
task service, so a successful create can lose the caller's selection.

Send this body directly to `POST /api/v1/tasks` with a workflow and two
registered repositories in the same workspace. These IDs match the existing
`registeredReplacementFixture` in the Go handler tests.

```json
{
  "workspace_id": "replacement-ws",
  "workflow_id": "replacement-wf",
  "workflow_step_id": "replacement-step",
  "title": "Repository selection",
  "repository_ids": ["next-b", "next-a"]
}
```

A temporary Go test used the registered route and SQLite at `c671d8080883`.
The ID list returned HTTP 200 and stored zero task repository rows. The
equivalent `repositories` request returned HTTP 200 and stored both IDs in order.
A request with `repositories: []` and a nonempty ID list also returned HTTP 200.

`buildCreateTaskBody` in `apps/web/e2e/helpers/api-client.ts` rewrites ID lists
to `repositories`. The regression test must send raw JSON to the route.

## Scope

- REST task creation with ID-only repository selections.
- Conflict responses, existing repository validation, API documentation, and existing `external_id` retries before request-specific validation.

### Out of scope

- WebSocket, MCP, plugin, and task update payloads.
- Task repairs, data migrations, and browser changes.

## Technical approach

Resolve an existing `external_id` in `httpCreateTask` before request-specific validation, including repository selection validation.
On an identity miss, expand the ID list before `convertCreateTaskRepositories`.
Keep presence through `convertTaskRepositories` so project source selection
can distinguish an empty array from omission. Return the existing create
rejection body for two arrays. The same normalized inputs drive the rest of
task creation and its existing repository validation.

- [Requirements](../../specs/tasks/requirements/http-task-repository-selection.md)
- [System design](../../specs/tasks/system-design/http-task-repository-selection.md)
- [Compatibility decision](../../decisions/2026-11-25-rest-task-repository-ids.md)

## Tests

The Go tests in `apps/backend/internal/task/handlers/task_http_repository_ids_test.go`
exercise HTTP, the task service, and SQLite.

| Criterion | Regression test |
| --- | --- |
| `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.1` | `TestHTTPCreateTaskRepositoryIDs` verifies stored repository IDs, order, and parity with canonical ID-only input. |
| `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.2` | `TestHTTPCreateTaskRepositoryIDsPresence` verifies project source selection for omission, null, and an explicit empty list. |
| `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.3`, `AC-TASKS-HTTP-REPOSITORY-SELECTION-001.4` | `TestHTTPCreateTaskRepositoryIDsRejection` verifies the conflict response, invalid payloads, and rejection of a mixed-workspace list before task allocation. |
| Existing identity retry contract | `TestHTTPCreateTask_FoundOutcomeSkipsRepositoryValidationInRetry` verifies that an existing identity is returned before validating an empty repository ID. See the post-review handler command in the work order. |

## Work order

- [x] [Task 01: Normalize repository IDs in REST task creation](task-01-normalize-http-repository-ids.md) (`done`)

## Verification results

The temporary direct HTTP test passed its canonical control and failed on the
missing attachments and conflict status. The Go overlay used temporary files.

Task 01 is complete. Its focused HTTP and service tests, changed-file lint,
backend build, public documentation checks, specification checks, documentation
coverage preflight, and `git diff --check` passed. See the work order for exact
commands, lifecycle evidence, and isolated reproduction steps.

## Compatibility risks

- Clients that currently send both arrays must send one list.
- An empty ID array suppresses automatic project sources for a root task.
  Clients that want automatic selection can omit the field or send `null`.
