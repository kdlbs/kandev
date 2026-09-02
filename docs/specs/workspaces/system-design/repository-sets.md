---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-001
  - REQ-WORKSPACES-REPOSITORY-SETS-002
  - REQ-WORKSPACES-REPOSITORY-SETS-003
---

# Repository Sets System Design

## Purpose and boundaries

The workspace system owns repository-set persistence, authorization, transport,
and settings behavior. The task form consumes a set as a reusable draft input.

A set stores an optional base for each member. Applying a set copies this value
into the task draft. The task service remains the authority for task-repository
snapshots and launch behavior.

The saved base does not change `repositories.default_branch`. It also does not
create a live link between a set and a task.

## Requirement mapping

| Requirement                          | Design sections                                                                                                                                                                |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `REQ-WORKSPACES-REPOSITORY-SETS-001` | [Persistence](#persistence), [Service and transport](#service-and-transport), [Events and boot state](#events-and-boot-state)                                                  |
| `REQ-WORKSPACES-REPOSITORY-SETS-002` | [Settings experience](#settings-experience), [Shared branch selector](#shared-branch-selector), [Branch loading](#branch-loading), [Responsive behavior](#responsive-behavior) |
| `REQ-WORKSPACES-REPOSITORY-SETS-003` | [Task form application](#task-form-application), [Local executor semantics](#local-executor-semantics), [Failure and recovery](#failure-and-recovery)                          |

## Data model

`repository_sets` keeps its current fields and workspace ownership.

`repository_set_items` adds `base_branch TEXT NOT NULL DEFAULT ''`. An empty
value means that the set does not override normal task-form defaulting.

The `RepositorySetItem` model and DTO add `BaseBranch` and `base_branch`.
Responses always include the ordered member list. A member can omit
`base_branch` when it has no override.

The set remains a draft template. It does not store a branch-policy ID,
checkout branch, generated task branch, or pull-request target.

## Persistence

The SQLite and PostgreSQL schema migration adds the new column before any query
uses it. The migration is replay-safe. The base schema includes the column for
new databases.

Repository-set create and update operations write metadata and all member
fields in one transaction. Membership replacement preserves the requested
order and assigns contiguous positions.

Existing rows receive an empty base. Backup and restore include the new column
through the existing database process.

## Service and transport

The service accepts ordered member inputs with `repository_id` and optional
`base_branch`. It validates these facts before the transaction:

- Each repository is live and belongs to the set workspace.
- Each repository occurs once.
- Each non-empty base passes `securityutil.IsValidBaseBranchRef`.

The service does not query Git during a write. Branch existence can change
after any check, and Git access can require credentials that are not available
to a settings request.

Create and update payloads add an ordered `repositories` field:

```json
{
  "name": "Central",
  "description": "Core services",
  "repositories": [
    { "repository_id": "repo-document", "base_branch": "develop" },
    { "repository_id": "repo-classifier", "base_branch": "" }
  ]
}
```

The existing `repository_ids` field remains a compatibility input. Each ID
becomes a member with an empty base. A request that contains both member fields
is invalid because their order or values can conflict.

HTTP and WebSocket handlers use the same request type and service methods. The
same error categories remain stable. An unsafe base produces a validation
error and no write.

## Events and boot state

Repository-set DTOs include `base_branch` in list, get, create, and update
responses. The `repository_set.created` and `repository_set.updated` events use
the same member shape.

The boot-state projection uses the DTO without a second mapping. The workspace
store and WebSocket handlers preserve the complete member objects.

## Settings experience

The approved editor uses one inline row for each selected repository. The
desktop modal contains these elements:

- Name and description fields.
- A search field for the selected-member list.
- Ordered member rows with a repository label and base selector.
- An Add repository action.
- A Reset bases action.
- Cancel and Save actions.

The base selector starts with `Task default`. When available, supporting text
also shows `repositories.default_branch`. Choosing this option stores an empty
base and preserves current task-form defaulting.

The list shows only selected members. The Add repository action opens a
searchable list of remaining workspace repositories. Existing move controls
preserve explicit member order.

Set summary cards keep their current member chips. A member with a saved base
shows the base as secondary text or a compact suffix. The card does not fetch
branch lists.

## Shared branch selector

The set editor reuses the `Pill` popover/list primitive from
`task-create-dialog-pill.tsx`, the same primitive rendered by New Task's
`RepoChipBranchPill`. It does not create a settings-only branch selector or a
second lookalike popover.

The editor builds branch options with `sortBranches`, `branchOptionValue`, and
`branchToOption` from `task-create-dialog-branch-options.tsx`. This shared
option model supplies these behaviors:

- Search uses the same branch scoring and keywords as New Task.
- Remote branches keep qualified names such as `origin/main`.
- Local branches show the `local` badge.
- Remote branches show the remote name, such as `origin` or `followup`.
- Provider branches without a remote name show the `remote` badge.
- The shared popover keeps search and refresh in one row, groups branches under
  `Branches`, and preserves the New Task keyboard, pointer, and portal behavior.

`Task default` is a synthetic first option for base pickers. It does not change
the shared mapping, search terms, ordering, badges, or selected-branch display.

The shared `Pill` accepts an optional open-state callback for settings' lazy
branch loading. New Task keeps its current behavior when the callback is
absent.

## Branch loading

The trigger can show these values without a branch request:

- A saved base from the set member.
- The repository default from the workspace repository DTO.
- The loading or unavailable state.

The editor enables `useBranches` only while a row selector is open. This rule
prevents an immediate request for every member in a large set.

After the list loads, the selector compares the saved base with all available
branch option values from `branchOptionValue`. A missing value remains in the
selector as a disabled fallback option with an unavailable marker.

The frontend can save an unchanged unavailable value. A user cannot select a
new value that is absent from the current list. The backend still validates the
Git-ref syntax for all values.

## Responsive behavior

Desktop uses the existing dialog pattern. The dialog has a bounded height and
one internal scroll region for member rows.

Phone layouts use the full-height drawer pattern from repository branch-policy
settings. The drawer uses `100dvh`, one scrolling body, and a fixed footer. The
footer includes safe-area padding.

Each member row stacks its repository label above a full-width base selector on
phones. Add, remove, move, reset, save, and cancel actions have touch targets of
at least 44 CSS pixels.

The phone editor uses the same searchable `Pill` popover as New Task. The
popover stays inside the viewport and owns the temporary option-list scroll. It
keeps the origin badges, refresh action, focus return, and keyboard behavior.

The domain draft, validation, mutations, and branch option mapping are shared
between desktop and phone layouts. Only the surface composition changes.

## Task form application

`RepositorySetItem` supplies the saved base to `applyRepositorySet`. A new
`TaskRepoRow` keeps the copied base separate from checkout state.

Applying a set follows this order:

1. Resolve the member against the current workspace repository list.
2. Skip the member when the repository is missing or already in the form.
3. Add a row with the saved base and no branch-policy selection.
4. Let normal row defaulting resolve an empty saved base.

The action never changes an existing row. This rule preserves manual edits and
keeps repeated or overlapping set application idempotent.

The task branch selector shows the effective base. If a copied saved base is
unavailable, the row shows the value and blocks submission. The user must select
an available branch or `Task default`. Selecting `Task default` clears the
copied base so normal task-form branch autoselection runs again.

`Save as set` sends ordered member objects. It captures each workspace
repository and the effective base in the draft. A `Task default` choice sends
an empty base. The visible summary states that base choices are included.

## Local executor semantics

The task draft keeps these values separate:

- The effective base from the set member or repository default.
- The checkout branch that represents the current local working branch.

For worktree-based execution, the effective base remains the source for
`base_branch`. For local execution, submission sends the effective base as
`base_branch` and the local working branch as `checkout_branch` when they
differ.

The task row shows both values when local execution makes them different. A
copied saved base never changes the local checkout branch as a side effect.

## Failure and recovery

| Condition                                    | Behavior                                                          |
| -------------------------------------------- | ----------------------------------------------------------------- |
| A member repository is missing               | Applying skips the member and reports the count.                  |
| A saved base is unsafe                       | The service rejects the set write with no partial change.         |
| A saved base disappears                      | Settings and task creation keep the value visible as unavailable. |
| A branch request fails                       | The open selector shows retry behavior and keeps the draft value. |
| An old client sends `repository_ids`         | The service stores members with no saved bases.                   |
| An open task draft already contains a member | Applying keeps the existing row unchanged.                        |

The frontend does not substitute a repository default for an unavailable saved
base. This rule prevents tasks from starting from an unintended branch.

## Permissions and security

Repository-set routes keep their current workspace authorization. Item routes
resolve the owning workspace before read or write access.

The service validates all repository IDs against the set workspace. It does not
reveal whether a cross-workspace ID exists. Safe Git-ref validation prevents a
saved base from becoming a command argument with unsafe syntax.

## Localization and accessibility

All new labels, hints, unavailable states, errors, and summaries use i18n keys.
The five supported catalogs contain the same keys and placeholders.

The editor uses semantic controls and visible labels. Selector triggers expose
accessible names. Focus returns to the trigger after a selector closes.

## Verification strategy

- Repository tests cover fresh schema, replay, existing-row migration, ordered
  base persistence, atomic replacement, and SQLite/PostgreSQL behavior.
- Service and handler tests cover validation, compatibility payloads, DTOs,
  events, and authorization.
- Frontend unit tests cover API mapping, store events, apply rules, unavailable
  bases, save-as-set input, and local base-versus-checkout submission.
- Component tests cover search, add, remove, reorder, reset, lazy branch loads,
  and responsive surface selection.
- Desktop and mobile Playwright tests create a set, save member bases, apply the
  set, and create a task with the expected repository bases.
