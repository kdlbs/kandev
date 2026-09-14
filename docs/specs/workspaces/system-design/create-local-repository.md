---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-CREATE-LOCAL-REPOSITORY-001
---

# Local repository creation system design

## Ownership and scope

Workspaces owns repository initialization and registration. Task creation consumes the
returned repository and selects it in the originating row. This design extends picker
availability and preserves the existing initialization API and filesystem guarantees.

## Requirement mapping

| Acceptance criteria | Design boundary |
| --- | --- |
| AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.1 | Picker availability |
| AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.7 | Row selection and cache update |
| AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.8 | Executor handling |
| AC-WORKSPACES-CREATE-LOCAL-REPOSITORY-001.2 through .6 | Existing initialization contract |

## Picker availability

`WorkspaceRepoChips` passes the optional creation callback to each `RepoChip`, bound
to its stable row key. Remove the `rows.length === 1` gates from both creation
and refresh callbacks. Forward the shared refresh callback to every row.
`Pill` already renders the action outside filtered options; retain that placement.
Repository inventory, selected values, and search matches do not gate the action.
Keep caller opt-in and the create-mode/unlocked checks in
`localRepositoryCreationEnabled`. Keep both toolbar actions outside search results.
Refresh retains its existing callback, loading spinner, and in-flight disablement;
it does not change row selections. Preserve optional callbacks for other consumers.

`useRepositories` leaves loading completion to the response/store update and request
finalizer. Cached data is not evidence that an active refresh has finished. Remove
the effect that clears loading solely because the workspace is already loaded. Each
automatic or manual request owns one loading marker; cancellation releases that
marker immediately, while concurrent requests keep the workspace loading until the
last request settles.

## Row selection and cache update

`RepoChips` records the originating key in `creatingForRowKey` and opens
`CreateLocalRepositorySurface`. On success, `applyCreatedLocalRepository` selects
the returned ID and `main`, clears local-path selection, and upserts the workspace
cache. Preserve every sibling row and unrelated draft field. Apply the normal
repository-change clearing of stale branch-policy metadata to the target row.
If the originating row disappears before completion, retain the created repository
in the workspace cache but do not replace another row or change its executor.
The creation surface captures the row callback at submission. A completion for an
older request still updates that stable row/cache handler, but it does not dismiss a
newer creation surface opened for another row.

## Executor handling

Retain the existing single-row direct-local selection policy. For multiple rows,
preserve the selected executor/profile and omit executor setters and executor
last-used writes from repository creation. Do not require
`findDirectLocalExecutorProfile` to return a profile for this path.

Thread an explicit multi-row creation context through the existing creation props
and surface. Its readiness and notice describe repository initialization without
promising a switch to Local. Reuse translated generic initialization copy when
accurate; add localized copy only if necessary. Do not disguise task creation as
the existing `workspace` context. Use current form state when applying a response.

Task submission retains the established executor compatibility validation, including
`getMultiRepoExecutorDisabledReason`. Creation does not broaden executor capabilities.

## Desktop and phone composition

The shared search popover remains a short, touch-usable selector. Its creation
button opens the existing desktop Dialog or phone Drawer. The nearest shipped
example is `CreateLocalRepositorySurface`; `MobilePickerSheet` supplies the drawer
precedent. Keep search and the action above the internally scrolling option list.

The phone form retains its name/path hierarchy, one directory-list scroll owner,
dynamic viewport height, safe-area footer, and return focus to the originating
selector. Creation remains a temporary step inside task composition. Keep actions
reachable without hover and at least 44px on touch surfaces. Reuse the shared
control-sizing helper for the affected action's 28px fine-pointer desktop size.
No global picker redesign is part of this change.

## Existing initialization contract

`initializeLocalRepository` calls the workspace-scoped `initialize-local` endpoint.
The backend creates the validated target and an empty initial commit on `main`,
then registers the repository. Conflicts do not modify an existing target.
Form errors retain inputs; dismissal before submission creates nothing. Successful
repositories persist independently of task cancellation. No API, schema, trust,
or backend initialization change is required.

The [explicit local repository trust decision](../../../decisions/2026-07-20-explicit-local-repository-trust.md)
continues to govern exact-path registration and access.

## Validation and delivery

Use component coverage for inventory/search/row-count combinations and callback
identity. Handler and form tests cover executor preservation and absence of direct
profiles. Desktop and phone E2E create into a second row, preserve the first row,
and submit with Worktree. Both viewports also refresh from the second row and
verify updated repository options without changing selections. Cover visible,
disabled refresh during loading. Preserve single-row and conflict coverage.

See the [implementation plan](../../../plans/repository-creation-availability/plan.md).
