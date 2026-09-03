---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001
---

# Watcher Remote Base Branches System Design

## Purpose and boundaries

The integration system owns the repository and base-branch values stored on a
watcher. The workspace system owns branch discovery and worktree materialization.
This design preserves branch identity between those systems without adding a
new watcher setting or persistence field.

The runtime behavior of local and remote worktree bases remains defined by the
[Worktree Base Refresh System Design](../../workspaces/system-design/worktree-base-refresh.md).

## Requirement mapping

| Acceptance criterion | Design section |
| --- | --- |
| `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.1` | [Branch projection](#branch-projection) |
| `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.2` | [Save and launch flow](#save-and-launch-flow) |
| `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.3` | [Compatibility](#compatibility) |
| `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.4` | [Shared picker interaction](#shared-picker-interaction) |
| `AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.5` | [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- `internal/task/service.ListBranchesWithCurrent` returns branch records with
  `name`, `type`, and `remote` identity.
- `useBranches` loads those records into the workspace branch cache.
- The New Task branch option helpers sort branch records, preserve qualified
  remote values, expose search keywords, and render local or remote badges.
- `BranchSelector` supplies the full-width searchable combobox and refresh
  action already reused by repository branch-policy settings.
- `WatcherRepositoryFields` projects supported branch records into the shared
  watcher selector used by Jira, Linear, Sentry, and GitLab watcher dialogs.
- Each integration service validates and persists its existing `baseBranch`
  field, and its watcher source copies that value into the generated task's
  repository binding.
- `internal/worktree.Manager` resolves the stored base ref during launch under
  the repository's existing worktree-refresh policy.

## Branch projection

`WatcherRepositoryFields` reuses `sortBranches` and `branchToOption` from the
New Task branch picker. Local branches use their short name. Remote branch
records from the supported `origin` remote use `origin/<name>`. Remote records
with another named remote are omitted because the watcher launch contract only
validates and refreshes `origin/<branch>` refs. Provider-backed records without
a remote name keep their short value. Exact projected refs are deduplicated
after that shared mapping, so repeated records do not create duplicate
combobox items while `main` and `origin/main` remain distinct.

A provider-backed branch record without a remote name keeps its short name.
Provider-backed repositories do not combine that record with a local branch
list, so this fallback does not erase a known remote identity.

## Shared picker interaction

Only the base-branch control changes presentation. The repository control
keeps its existing select behavior. The base-branch field uses the shared
`BranchSelector` and New Task branch options so it inherits:

- path-aware search through `scoreBranch`;
- the `local` badge for local refs and the `origin` badge for qualified remote
  refs;
- preferred ordering for `main`, `master`, and `develop`; and
- the branch refresh action backed by `useBranches.refresh`.

The repository-default sentinel remains the first option and continues to map
to an empty persisted value. A stored value that is temporarily absent from
the fetched branch list remains visible as a fallback option, so opening the
dialog cannot visually replace a saved choice. Refresh updates the available
options without changing the selected watcher value.

The reusable boundary is the branch option model and the full-width
`BranchSelector` presentation. The compact New Task `Pill` trigger remains
task-specific because watcher dialogs need a labeled, full-width form field.

## Save and launch flow

1. The user selects a repository in a watcher dialog.
2. `useBranches` supplies local and remote branch records.
3. The shared searchable watcher field displays `origin/main` with an `origin`
   badge and passes that exact string through `onBaseBranchChange`.
4. The integration's existing create or update request persists the qualified
   ref without a schema change.
5. The watcher source copies the stored ref into `task_repositories.base_branch`.
6. Worktree launch applies the repository's `pull_before_worktree` policy. An
   enabled policy refreshes the selected remote branch; the workspace system
   owns refresh failure and fallback behavior.

The existing `securityutil.IsValidBaseBranchRef` contract accepts
`origin/<branch>` and continues to reject unsafe refs. The watcher selector
does not offer other named remotes because the worktree refresh and launch
contract is origin-specific.

## Compatibility

The repository-default sentinel still maps to an empty request value so the
integration service resolves the repository default. Local selections keep
their short names. Existing watchers with a stored local or qualified ref load
without conversion.

No backend, API, or database migration is required.

## Responsive behavior

The watcher dialog remains the single scroll owner. Desktop keeps the current
two-column repository and base-branch row. Phone keeps the current stacked row.
The searchable combobox popover is retained on phone because it is a temporary,
one-dimensional choice with built-in search and viewport-bounded width and
height. The trigger and option rows use touch-sized hit areas on coarse
pointers, and the option list owns its internal overflow. The component shares
values, filtering, selection, and refresh behavior across viewports; no
mobile-only branch state is introduced.

The nearest mobile precedent is the existing full-width branch-policy picker,
which already uses `BranchSelector` in a form surface. The implementation adds
scoped touch sizing where needed instead of introducing a global popover rule.
It does not change navigation, dialog geometry, or primary actions.

## Failure and recovery

A branch-list failure keeps the existing loading or default placeholder and
does not alter the saved watcher. A remote-refresh failure is handled by the
workspace worktree-refresh contract and is not converted into a local branch
selection by the watcher UI.

## Test strategy

- Focused component coverage proves local and supported origin records render as
  distinct exact refs with badges; unsupported named remotes stay hidden; search
  filters the choices; refresh invokes the branch hook; and duplicate exact
  refs collapse without regressing disabled, default, and loading states.
- Desktop and mobile Playwright flows open the Jira watcher dialog, select and
  save `origin/main`, reload the settings page, and verify the qualified value
  remains selected. The mobile flow also verifies touch-sized branch rows,
  viewport containment, internal list scrolling, and no document overflow.
