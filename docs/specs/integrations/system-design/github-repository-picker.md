---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001
---

# GitHub Repository Picker System Design

## Purpose and boundaries

The integration system owns GitHub watch filters and the workspace GitHub
repository scope. Both store GitHub `owner/name` identities, so they select
from provider repositories, not from workspace repositories.

Kandev has two repository pickers with different domains:

- **Workspace mode:** `WorkspaceRepoChips` selects registered and
  filesystem-discovered repositories, paired with branches, for task launch
  (task creation, automation "Repository access"). Unchanged here.
- **Remote mode:** the remote repository picker in
  `task-create-dialog-remote-repo-chip.tsx` selects provider repositories from
  `useRemoteRepositories` and accepts a pasted URL.

This design moves GitHub selections onto remote mode, restricted to the GitHub
provider. It does not create a third picker. The task system keeps owning the
task-creation remote chip, its branch pill, and its row state.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.1` | [Shared picker panel](#shared-picker-panel); [Provider restriction](#provider-restriction) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.2` | [Shared picker panel](#shared-picker-panel) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.3` | [Provider restriction](#provider-restriction); [Watch filter](#watch-filter) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.4` | [Typed entry](#typed-entry) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.5` | [GitHub multi-select](#github-multi-select); [Watch filter](#watch-filter) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.6` | [Repository scope setting](#repository-scope-setting) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.7` | [Failure and recovery](#failure-and-recovery) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.8` | [GitHub multi-select](#github-multi-select) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.9` | [Data and contracts](#data-and-contracts) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.10` | [GitHub organization multi-select](#github-organization-multi-select); [Watch filter](#watch-filter); [Repository scope setting](#repository-scope-setting) |
| `AC-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001.11` | [List refresh](#list-refresh) |

## Components and responsibilities

### Shared picker panel

`apps/web/components/remote-repository-picker.tsx` exports
`RemoteRepositoryPickerPanel`, extracted from the task-creation chip's
`RemoteRepoPopoverContent`, `PickerList`, `RepoOption`, provider tabs, and
connect banner. Props:

- `accessible: UseRemoteRepositoriesResult` — the loaded, searchable list.
- `onPick(repo: RemoteRepository)` — a listed repository was chosen.
- `onSubmitText(value: string): RemoteRepositoryTextSubmitResult` — Enter on
  typed text. Returns `accepted`, `ignored` (Enter falls through), or
  `rejected` with a localized message shown in the list. Task creation passes
  its existing URL-paste handler.
- `isTextSubmittable(value: string): boolean` and `stagedHint` — the hint shown
  while the typed value can be submitted.
- `isRepoAllowed?(repo): boolean` — hides repositories outside a caller's
  scope.
- `isRepoSelected(repo): boolean` — drives the existing "Already added"
  marker.
- `placeholder` — localized search placeholder.

Keyboard contract: plain Enter in the search input always stays inside the
picker (`preventDefault`), even when `onSubmitText` ignores the text, so it
never activates a surrounding dialog's default action; ArrowDown/ArrowUp move
focus between the input and the option buttons (`movePickerFocus` in
`picker-controls.tsx`). Submit errors stay visible while the provider list is
unavailable; task creation opts out with `hideSubmitErrorWhenUnavailable`.
Loading rows use `PickerLoadingStatus`, a translated `role="status"` region
with the spinner hidden from assistive technology.

The panel drives `accessible.search` from its input, so filtering stays the
hook's existing case-insensitive substring match over `fullName`, host, and
URL. Task creation's `RemoteRepoPill` renders this panel without visible
behavior change.

### Provider restriction

`useRemoteRepositories(workspaceId, options?)` accepts
`options.providers?: readonly RemoteRepositoryProvider[]`. When it is set, only
those built-in providers are eligible and plugin repository sources load only
when their provider id is listed. GitHub selections pass `["github"]`, so they
issue no GitLab, Azure DevOps, or plugin requests. Omitting the option keeps
today's behavior for every existing caller.

### GitHub multi-select

`apps/web/components/github/github-repository-multi-select.tsx` exports
`GitHubRepositoryMultiSelect`:

- Props: `workspaceId`, `selected: RepoFilter[]`, `onChange(RepoFilter[])`,
  `scope?: GitHubWorkspaceSettings | null`, `disabled?`, `testIdPrefix`.
- Renders the selected repositories as removable entries and an
  `AddRepositoryButton`-styled "Add repository" trigger that opens a popover
  with `RemoteRepositoryPickerPanel`, using
  `useRemoteRepositories(workspaceId, { providers: ["github"] })`.
- Picking a listed repository appends `{ owner, name }` unless an entry with
  the same case-insensitive owner and name exists, then closes the popover.
- Popover content stays inside the viewport (`max-w-[calc(100vw-2rem)]`, the
  remote chip's height bound) and uses the existing touch-sized option, add,
  and remove sizes.

### GitHub organization multi-select

`apps/web/components/github/github-organization-multi-select.tsx` exports
`GitHubOrganizationMultiSelect`:

- Props: `workspaceId`, `selected: string[]` (logins), `onChange(string[])`,
  `isOrgAllowed?(login): boolean`, `disabled?`.
- Loads `GET /api/v1/github/orgs` through `useGitHubOrgs` in
  `apps/web/hooks/domains/github/use-github-orgs.ts`. The backend already
  prepends the authenticated login, so the account is listed first.
- Same anatomy as the repository multi-select: removable chips (shared
  `SelectionChip`), an "Add organization" trigger, and a popover panel with the
  shared search row (`PickerSearchRow` in `apps/web/components/picker-controls.tsx`),
  substring filtering, the "Already added" marker, and touch-sized rows.
- Enter on a well-formed GitHub login (`parseGitHubLogin` in
  `apps/web/lib/github/repository-input.ts`) adds it even when unlisted;
  `isOrgAllowed` rejects it with a localized message.
- Deduplicates case-insensitively.

### List refresh

`PickerSearchRow` renders a refresh icon button beside the search input when
the caller passes `onRefresh`, spinning while the list loads. The repository
panel passes `accessible.refresh`; the organization panel passes
`useGitHubOrgs().refresh`. The task-creation remote chip inherits the control
through the shared panel.

`useRemoteRepositories().refresh` already bumped a refresh version. The
built-in GitHub source now also sends `refresh=true` to
`GET /api/v1/github/repos` for the first successful load after each refresh.
The backend's `ListAccessibleReposForWorkspace(..., refresh)` then skips the
60-second accessible-repository cache read for both the personal and the
automation list, and stores the fresh result. The organizations endpoint is
not cached, so its refresh is a plain refetch.

### Typed entry

`useRemoteRepositories` exposes the workspace's `githubHost` so the
multi-select parses GitHub Enterprise repository URLs.
`parseGitHubRepositoryInput(value, githubHost?)` in
`apps/web/lib/github/repository-input.ts` returns `{ owner, name }` for
`owner/name` (single slash, both segments matching GitHub's allowed
characters) or for a GitHub repository URL via `parseGitHubRepoUrl`, else
`null`. The multi-select uses it for `isTextSubmittable` and `onSubmitText`;
an invalid value or one that `repoAllowedByScope` rejects returns a localized
error instead of being added.

### Watch filter

`RepoFilterSelector` keeps its props, header, "All repositories" switch, and
scope fetch. Its organization badges are replaced by
`GitHubOrganizationMultiSelect`, fed the `name === ""` entries and filtered by
`orgAllowedByScope`; it is hidden in `repos` scope mode as the badges were.
Organization changes are merged back as `{ owner, name: "" }` entries. Scope predicates (`repoAllowedByScope`,
`orgAllowedByScope`, `sameRepoFilter`) move to `apps/web/lib/github/repo-scope.ts`
so the multi-select shares them. `RepoSearchCombobox`, `useRepoSearch`, and the
`searchOrgRepos` call are removed; `GitHubRepositoryMultiSelect` with the
fetched scope replaces the combobox and the `SelectedFilters` badges. The
multi-select receives only repository entries while the organization picker is
shown; when it is hidden, it receives every entry and renders organization
entries as `owner/*` so saved ones stay removable. Every
consumer must pass `workspaceId`: without it neither the scope nor the
repository list can load. `github-pr-config.tsx` previously omitted it, which
is why the automation "PR: opened" picker never returned results even for a
typed `owner/` prefix; `trigger-card.tsx` now passes it as it already did for
`github-pr-merged-config.tsx`. The review and issue watch dialogs already pass
it.

### Repository scope setting

`GitHubRepoScopeSection` stores the repository draft as `RepoFilter[]` instead
of a comma-separated string. In `repos` mode it renders
`GitHubRepositoryMultiSelect` without `scope`. The baseline, dirty check,
discard, save payload (`repo_scope_repos`), and stale-response guards compare
canonical `owner/name` lists. The invalid-entry message, its
`useCommaSeparatedOwnerRepoValues` locale key, and `invalidReason` disappear because the control can no longer hold a malformed
value. The organizations draft is likewise a login list rendered by
`GitHubOrganizationMultiSelect` without a scope filter, disabled unless the
mode is `orgs`; the save payload (`repo_scope_orgs`) is unchanged.

## Data and contracts

No persistence change. The only API change is the optional `refresh=true`
query parameter on `GET /api/v1/github/repos` (see [List refresh](#list-refresh)). Watch filters and trigger configs keep
`RepoFilter[]`; repository scope keeps `repo_scope_repos: RepoFilter[]`. The
frontend `searchOrgRepos` client stays exported but has no caller; the backend
`/api/v1/github/repos/search` endpoint stays in place. The locale keys used only
by the removed combobox (`searching`, `noReposFoundFor`,
`typeOwnerRepoToSearch`, `private` in the `github` namespace) are removed.

## Control flow

```text
Add repository -> popover opens -> useRemoteRepositories(ws, github)
  -> GET /api/v1/github/repos?workspace_id=..&limit=100 (cached 60s server-side)
  -> list renders, filtered client-side per keystroke and by scope
  -> pick or Enter(owner/name) -> onChange([...selected, {owner, name}])
```

## Failure and recovery

The panel reuses the task-creation states: loading, "No repositories found",
load error, and the connect-provider banner when GitHub is unavailable. Typed
`owner/name` entry remains available in each state. The hook's existing
`refresh`, now exposed as the refresh control, keeps recovery explicit.

## Persistence

None. Drafts live in existing component state until the owning form saves.

## Security

No new data crosses a boundary. Scope filtering is a client-side authoring
aid; backend scope enforcement is unchanged.

## Observability

None added.
