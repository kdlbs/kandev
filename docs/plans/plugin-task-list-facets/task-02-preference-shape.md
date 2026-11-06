---
id: "02-preference-shape"
title: "Accept facet selections in the stored preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-TASKLIST-FACETS-003
  - REQ-PLUGINS-TASKLIST-FACETS-002
acceptance_criteria:
  - AC-PLUGINS-TASKLIST-FACETS-003.2
  - AC-PLUGINS-TASKLIST-FACETS-003.4
  - AC-PLUGINS-TASKLIST-FACETS-002.7
system_design:
  - ../../specs/plugins/system-design/task-list-facets.md
---

# Task 02: Accept Facet Selections In The Stored Preference

## Summary

Teach the user-settings sort and group preference to accept
`facet:<pluginId>:<facetId>` by shape, so a facet selection round-trips through
the store, the service, and the boot-state route, while an unrecognized value
still normalizes to the built-in default.

## In scope

- Add the facet prefix and a per-segment shape check to
  `apps/backend/internal/user/models/tasks_list_preferences.go`, exposed as a
  named predicate `IsTaskListFacetValue(value string) bool` so callers can test
  the facet shape directly. The plugin-id
  segment follows the manifest identity grammar
  (`^[a-z0-9][a-z0-9._-]*$` in `internal/plugins/manifest/validate.go`, so dots
  and underscores are legal and a colon is not); the facet-id segment follows the
  host registry's `^[a-z0-9]+(?:-[a-z0-9]+)*$` rule.
- Add an exported query-side resolver, `TasksListApiSort(value)`, that returns
  `TasksListSortDefault` when the value is a facet (after trimming) and otherwise
  defers to `NormalizeTasksListSort`. It must test the facet predicate, never
  `IsValidTasksListSort`: this same task widens `IsValid*` to accept facets, so a
  resolver built on it would forward a facet into the order-by input, which is
  the defect this work order exists to prevent.
- Trim before matching in both `IsTaskListFacetValue` and `TasksListApiSort`. The
  shipped helpers trim (`IsValidTasksListSort` compares against
  `strings.TrimSpace(value)`, `NormalizeTasksListSort` trims first), so an
  untrimmed predicate would treat `" facet:plugin:tags "` as a non-facet, let
  `NormalizeTasksListSort` trim and accept it, and pass the facet identifier into
  `ListTasksByWorkspaceWithArchiveMode` and `taskListOrderBy`. Cover the padded
  facet value in the models test, the handler test, and the boot test.
- Route the task-list HTTP handler's sort gate through the new resolver:
  `internal/task/handlers/task_http_handlers.go` currently calls
  `usermodels.NormalizeTasksListSort(c.Query("sort"))`, and `NormalizeTasksListSort`
  delegates to `IsValidTasksListSort`, so widening that predicate makes
  `GET /api/v1/workspaces/:id/tasks?sort=facet:<pluginId>:<facetId>` pass a facet
  identifier into `ListTasksByWorkspaceWithArchiveMode` and on to
  `taskListOrderBy`. Change that one call to `usermodels.TasksListApiSort(...)`.
  Leave the other `Normalize*` call sites alone: the two user-store writes and
  the boot payload's `tasksListSort` field must keep accepting a facet value.
- Split the boot handler's single resolved sort into its two uses in
  `internal/backendapp/boot_state_routes.go`: the first-page task query
  (`tasksForWorkspace` → `taskSvc.ListTasksByWorkspace`) takes
  `TasksListApiSort(tasksListSort)`, while the returned `tasksListSort` field
  keeps the requested value so the client can bind the facet control. Resolving
  only one half is a defect either way: resolving both strips the facet on every
  first load, resolving neither sends the facet to the order-by input.
- Widen `IsValidTasksListSort` and `IsValidTasksListGroup` to accept a built-in
  value or a facet value. Keep `NormalizeTasksListSort` and
  `NormalizeTasksListGroup` returning the built-in default for anything else,
  including a malformed facet form such as `facet:`, `facet:Plugin:tags`,
  `facet:plugin:tag id`, or `facet:plugin:`.
- Update the rejection message in
  `internal/user/service/service.go#applyTasksListPreferences` to name both
  accepted forms instead of listing only the built-in enums.
- Add a models test for both enums covering the accepted built-in values,
  accepted facet values (including a dotted or underscored plugin id such as
  `com.example.tags` and `my_plugin`, and a whitespace-padded value), rejected
  shapes, and normalization. These are the same examples the client parser test
  in task 03 must use.
- Add a service test proving a facet value is accepted and persisted, and an
  unknown value is rejected with an error whose text names both accepted forms
  (a built-in value and `facet:<pluginId>:<facetId>`). Inspect the returned
  error, not only that the call failed: the acceptance requires the message, and
  nothing else in the package asserts its wording.
- Extend `internal/backendapp/boot_state_routes_test.go` for a facet query value
  winning over the stored default, a stored facet surviving boot, and the split
  itself: with a facet sort requested, the first-page task query receives a
  built-in sort while the payload's `tasksListSort` field keeps the facet. The
  split needs an observable seam: `bootStateBuilder` calls the concrete
  `*taskservice.Service`, and the order-by default body is byte-identical to the
  `updated_desc` case, so the payload's row order cannot distinguish "query
  resolved" from "query not resolved". Build the boot test's service over a
  recording `taskrepo.TaskRepository` decorator that captures the sort passed to
  `ListTasksByWorkspaceWithArchiveMode` and delegates to the harness repository,
  and assert the captured value. Asserting only the payload field or the row
  order satisfies the wording without proving the regression the split exists to
  prevent.
- Add a models test proving `TasksListApiSort` maps a facet value and an unknown
  value to the built-in default and passes a built-in value through unchanged.
- Extend `internal/task/repository/sqlite/task_order_test.go` to prove a facet
  sort string yields the default ordering, and add a handler test proving
  `GET /api/v1/workspaces/:id/tasks?sort=facet:<pluginId>:<facetId>` reaches the
  service with a built-in sort rather than the facet identifier.

## Out of scope

- Enumerating installed facets on the backend. The backend validates shape only.
- Any frontend change.
- New columns, routes, or migrations.

## Acceptance

- `IsValidTasksListSort` and `IsValidTasksListGroup` accept every built-in value
  and a well-formed `facet:<pluginId>:<facetId>` whose plugin-id segment may
  contain dots or underscores, and reject an empty value, a single-segment
  facet, an empty segment, an uppercase facet id, and an unknown bare value;
  `Normalize*` preserves an accepted facet value, including one padded with
  whitespace, and returns the built-in default for anything else.
- A user-settings write with a facet sort or group succeeds and reads back
  unchanged for both enums, and a write with an unknown value is rejected with a
  message naming both accepted forms.
- For a facet sort, the booted first page queries the task list with a built-in
  sort while the boot payload's `tasksListSort` field keeps the facet, a
  `sort=facet:<pluginId>:<facetId>` request to the task-list endpoint reaches the
  service as a built-in sort (including a whitespace-padded value such as
  `" facet:plugin:tags "`), the boot payload keeps a stored facet and prefers a
  valid query facet over the stored default, and a facet sort string passed to
  `taskListOrderBy` produces the default ordering.

## Verification

```bash
cd apps/backend && go test ./internal/user/... ./internal/backendapp/... ./internal/task/repository/sqlite/... ./internal/task/handlers/...
make -C apps/backend lint
```

The handler package is not optional: it is the only place the rerouted `sort`
gate can be proven, and the package currently has no `sort` assertion at all.

## Files likely touched

- `apps/backend/internal/user/models/tasks_list_preferences.go`
- `apps/backend/internal/user/models/tasks_list_preferences_test.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_routes_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/repository/sqlite/task_order_test.go`

## Dependencies

None.

## Risks

- Widening `IsValid*` reaches further than its two direct callers, because
  `NormalizeTasksListSort` delegates to `IsValidTasksListSort`. The `Normalize*`
  sites are the user store (encode at
  `internal/user/store/sqlite.go`, decode at the same file's payload path), the
  task-list HTTP handler, the repository's own normalize, the boot payload's
  `tasksListSort` field, and the boot route resolver. Two of them must keep
  accepting a facet value (the store's preference write, and the boot payload
  field the client binds) and the two query sites must not (the HTTP list
  handler, covered above, and the boot handler's task query). That is why the
  handler call changes: leaving it on `NormalizeTasksListSort` keeps
  `?sort=facet:…` alive as a sanctioned path into the order-by input, which
  `AC-PLUGINS-TASKLIST-FACETS-002.7` forbids. The order-by default branch
  absorbs it only by accident.
- `TasksListApiSort` must branch on `IsTaskListFacetValue`, not on
  `IsValidTasksListSort`. Because this task widens `IsValid*` to accept facets,
  a resolver written as "not valid, so default" silently passes the facet
  through to `ListTasksByWorkspace`, which is exactly the defect the boot split
  closes. Cover it with a test that a facet value resolves to the default.
- The shape rule and the frontend rule are two implementations of one contract,
  and they use different segment rules on purpose. The plugin-id segment must
  accept every id the manifest accepts (`^[a-z0-9][a-z0-9._-]*$`); a narrower
  rule rejects a legally named plugin's preference write, whose failure the
  client swallows. Use the same accepted and rejected examples in both test
  suites.
- The service rejection message must not imply an exhaustive enumerated set that
  no longer exists.
- The client parser must trim to match the existing server-side trim
  (`AC-PLUGINS-TASKLIST-FACETS-003.4`, and
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3`, which forbids keeping the
  asymmetry). The server side already trims; keep it that way and cover the
  padded example here so task 03 can mirror it.

## Parallelism

`parallel-safe` with task 01.

## Inputs

- `REQ-PLUGINS-TASKLIST-FACETS-002`, `REQ-PLUGINS-TASKLIST-FACETS-003`
- `docs/specs/plugins/system-design/task-list-facets.md`
- `docs/decisions/2026-09-22-plugin-facet-selection-persistence.md`

## Results

Ported onto the current `main` base (`02ff057835`) and re-verified here. The
reference branch recorded the RED observations for each behavior (facet forms
rejected by the model, the service error listing only built-ins, the HTTP `sort`
gate and the boot first-page query forwarding a facet); this port carries the
same tests and re-ran them after the port.

- `gofmt -l` over the changed Go files: clean.
- `go test ./internal/user/... ./internal/task/repository/sqlite/...
  ./internal/task/handlers/...` (apps/backend): all packages passed, including
  `internal/user/models` (`TestTaskListFacetPreferences`,
  `TestTasksListApiSortNeverForwardsFacet`), `internal/user/service`,
  `internal/task/repository/sqlite` (`TestTaskListOrderBy_FacetSortUsesDefaultOrdering`)
  and `internal/task/handlers` (`TestHTTPListTasksByWorkspaceNeverForwardsFacetSort`).
- `go test ./internal/backendapp/...`: passed, including
  `TestTasksPageBootDataResolvesFacetQueryWithoutDiscardingSelection`, which pins
  the boot split: the payload keeps the facet while the first-page query receives
  only a built-in sort.
- `make -C apps/backend build`: passed (rebuild before the Playwright runs).
