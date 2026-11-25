---
id: "01-normalize-http-repository-ids"
title: "Normalize repository IDs in REST task creation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-HTTP-REPOSITORY-SELECTION-001
acceptance_criteria:
  - AC-TASKS-HTTP-REPOSITORY-SELECTION-001.1
  - AC-TASKS-HTTP-REPOSITORY-SELECTION-001.2
  - AC-TASKS-HTTP-REPOSITORY-SELECTION-001.3
  - AC-TASKS-HTTP-REPOSITORY-SELECTION-001.4
system_design:
  - ../../specs/tasks/system-design/http-task-repository-selection.md
---

# Task 01: Normalize repository IDs in REST task creation

## Summary

Decode the ID-only array in `httpCreateTaskRequest` and normalize it in
`httpCreateTask`. Use the existing repository converter, service, and
response paths.

## In scope

- Add `RepositoryIDs []string` with the `repository_ids` JSON name. Compare
  slice presence before calling `convertCreateTaskRepositories`.
- When both slices are present, respond with HTTP 400 and
  `rejectedCreateMessage("repositories and repository_ids cannot both be provided")`.
- When the ID slice supplies the selection, create an equally sized
  `[]httpTaskRepositoryInput` and put each ID in `RepositoryID`. Assign that
  slice to `body.Repositories`.
- Add the direct HTTP regression tests and document the REST fields near
  **Create idempotency with external_id** in `docs/public/automation-and-mcp.md`.
  Document per-repository options, the alias, and the empty, null, and conflict rules.

## Out of scope

- Global JSON decoder changes and frontend API client changes.
- Alternate repository lookup paths, storage changes, and task updates.

## Acceptance

- The ID array creates the same ordered attachments as canonical ID-only
  input. The service supplies the existing branch behavior.
- The route preserves presence. In a root project request, an explicit
  empty ID list skips project sources. Omission and `null` use project defaults
  when the other field is absent.
- Two arrays return HTTP 400 with `creation_rejected: true`. This includes
  empty arrays. A malformed ID value or workspace mismatch fails before task
  allocation through the existing validation paths.

## Test procedure

1. Add `TestHTTPCreateTaskRepositoryIDs` and
   `TestHTTPCreateTaskRepositoryIDsRejection` with `registeredReplacementFixture`.
   Send literal JSON through its route and read back rows from SQLite. Use a
   canonical request as the success control. Check both empty-array conflict
   forms, a non-array value, and an empty ID. For a list with one valid and one
   foreign repository, compare the rejection with canonical input.
2. For rejected requests, check the task count and `task.created` events.
   Use the success control to exercise those effects. Check the rejection
   marker and field names for the conflict response.
3. Add `TestHTTPCreateTaskRepositoryIDsPresence` with
   `newCreationAttachmentHandler` and the existing project source reader seam.
   Register the HTTP route. Seed a project source backed by a temporary Git
   repository. Check omission, null, and an empty ID array. Include requests
   with one null field and one array to exercise both directions.
4. Run the Go tests before the production change. Check that the ID-list
   case fails on missing attachments and the two-array case fails on status.
   Apply the smallest handler change. Rerun the checks after the final edit.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/handlers -run '^(TestHTTPCreateTaskRepositoryIDs(Presence|Rejection)?|TestHTTPCreateTaskRecordsFinalLastUsedSelections|TestHTTPCreateTask_ExternalIDGoldenPath)$')
(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/service -run '^TestCreateTaskProjectRepositorySourceSelectionPrecedence$')
bash scripts/lint-go-changed apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/task_http_repository_ids_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the documentation coverage preflight before handoff. During design it
includes the planned handler path. During implementation it checks the
complete changed path set.

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const git = (...args) => execFileSync('git', args, { encoding: 'utf8' });
const base = git('merge-base', 'HEAD', 'origin/main').trim();
const changedFiles = [...new Set([
  ...git('diff', '--name-only', '-z', base).split('\0').filter(Boolean),
  ...git('ls-files', '--others', '--exclude-standard', '-z').split('\0').filter(Boolean),
  'apps/backend/internal/task/handlers/task_http_handlers.go',
])];
const docs = changedFiles.filter(p => p.startsWith('docs/') && p.endsWith('.md') && fs.existsSync(p));
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
JS
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_repository_ids_test.go` (new)
- `docs/specs/tasks/README.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/coverage.json`

## Dependencies

Existing task HTTP request conversion and task service repository admission.

## Risks

- A length check misses conflicts with empty arrays.
- A nil result for an empty ID array triggers automatic project selection.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/http-task-repository-selection.md)
- [System design](../../specs/tasks/system-design/http-task-repository-selection.md)
- [Registered task repository test fixture](../../../apps/backend/internal/task/handlers/task_repository_replacement_test.go)
- [Project repository selection tests](../../../apps/backend/internal/task/service/service_project_repositories_test.go)

## Results

Implemented the REST alias at the handler boundary. The handler rejects two
present arrays before task creation, maps ID entries to canonical repository
inputs in order, and preserves an explicit empty list.

### Verification results

- RED: the focused tests failed before the handler change. The ID-only request
  stored no repositories, conflicts returned HTTP 200, and an empty ID list
  selected the project source.
- `(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/handlers -run '^(TestHTTPCreateTaskRepositoryIDs(Presence|Rejection)?|TestHTTPCreateTaskRecordsFinalLastUsedSelections|TestHTTPCreateTask_ExternalIDGoldenPath)$')`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/service -run '^TestCreateTaskProjectRepositorySourceSelectionPrecedence$')`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/task/handlers/task_http_handlers.go apps/backend/internal/task/handlers/task_http_repository_ids_test.go`: passed, 0 issues.
- `make -C apps/backend build-kandev`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 63 tests.
- `node scripts/validate-public-docs.mjs`: passed, 56 published pages.
- `python3 scripts/list-docs.py validate`: passed, 465 decisions and 2,116 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: passed with status `covered`.
- `git diff --check`: passed.

### Lifecycle evidence

| Scenario | Acceptance and observable invariant | Regression evidence | Result |
| --- | --- | --- | --- |
| Creation and task events | AC 001.1, 001.3, 001.4: valid selections attach in order; rejected requests allocate no task and publish no `task.created`. | `TestHTTPCreateTaskRepositoryIDs` and `TestHTTPCreateTaskRepositoryIDsRejection`, in the handler command above. The success control proves task and event effects are reachable. | Passed. |
| Project defaults | AC 001.2: omission and `null` use project sources; an empty ID list suppresses them. | `TestHTTPCreateTaskRepositoryIDsPresence`, in the handler command above. | Passed. |
| Duplicate create retry | An existing `external_id` returns its task before repository conversion or other request-specific validation. | `TestHTTPCreateTask_FoundOutcomeSkipsRepositoryValidationInRetry`, in the handler command above. | Passed. |
| Entry paths | The changed contract is REST-only. The registered `/api/v1/tasks` route exercises normalization, service validation, and SQLite persistence. | All three new handler tests above. | Passed. |

Pause/resume and restart recovery do not apply because validation is synchronous
and rejected requests allocate no task or pending work. Ownership changes,
cancellation, and replacement do not apply because no task or session exists
before request validation. Concurrent delivery and async merging do not apply
because this change adds no background work or new shared state. WebSocket, MCP,
and plugin creation remain outside the stated scope.

### Isolated reproduction

Run `scripts/dev-isolated --web` and use the printed web URL. In that disposable
instance, create a workspace, workflow, and two registered repositories. Send a
raw `POST /api/v1/tasks` request with their IDs in `repository_ids`, then make
sure that the response lists both repositories in the submitted order. Use the
printed teardown command when finished.

### PR review follow-up

The initial regression test failed with HTTP 400 when a found `external_id`
retry supplied `repository_ids: [""]`. The handler now checks for an existing
identity before repository conversion; a miss continues through the existing
validation path.

- `(cd apps/backend && go test -trimpath -tags fts5 -count=1 ./internal/task/handlers -run '^(TestHTTPCreateTask_(FoundOutcomeSkipsRepositoryValidationInRetry|FoundSettledSkipsPostCreateWork|FoundUnsettledReportsIncomplete|InvalidExternalIDReturns400|ExternalIDGoldenPath)|TestHTTPCreateTaskRepositoryIDs(Presence|Rejection)?)$')`: passed.
