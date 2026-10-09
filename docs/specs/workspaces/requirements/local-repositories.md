---
status: active
system: workspaces
created: 2026-07-20
updated: 2026-10-06
owners:
  - kandev
---
# Local Workspace Repositories Requirements

## Overview

Users connect repositories on the Kandev host. Server discovery uses configured
roots or Home; Desktop requires selected roots to avoid unexpected macOS access.

## Requirements

### REQ-WORKSPACES-LOCAL-REPOSITORIES-001: Local Workspace Repositories

**Intent:** Connect accessible repositories, including Windows paths outside Home,
without widening scans or editing packaged configuration.

#### Acceptance criteria

- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.1:** A user can add a local Git repository by entering or selecting an absolute path that the Kandev process can access.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.2:** Manual selection is valid independently of `repositoryDiscovery.roots`; those roots govern only automatic discovery scans.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.3:** Kandev validates and canonicalizes a non-empty local repository path before saving it. A saved repository records the exact canonical path the user selected.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.3a:** An initialized Git submodule with reciprocal canonical `core.worktree` metadata can be registered as its selected local repository path.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.3b:** A regular-file `.git` pointer without reciprocal ownership proof, including a missing, empty, mismatched, or alternate-source `core.worktree`, is rejected and not persisted.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.4:** Trusting one repository does not trust its parent directory, filesystem volume, or sibling repositories.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.5:** Saved repositories remain usable for branch listing, current status, refresh, task creation, and fresh-branch workflows after restart.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.6:** A saved repository without an `origin` remote supports Merge and Rebase when the selected base branch exists locally.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.7:** A repository with an `origin` remote refreshes and uses `origin/<base>` for Merge and Rebase.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.8:** A missing local base branch causes a clear error before Merge or Rebase changes repository history.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.9:** In workspace settings, manual validation shall belong to the current dialog visit, workspace, and trimmed input. Editing to another path, selecting a discovered repository, closing, changing workspace, or leaving the page shall retire that validation. Returning to the same path or workspace, or reopening the dialog, shall not restore retired success, error, or busy state. The input shall remain editable during validation.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.10:** If validations overlap for unchanged input, only the newest started attempt shall publish success, invalid-path feedback, rejection feedback, or busy state. Retired work shall still settle for its initiating caller and shall not clear a current pending attempt. A current unchanged-input result shall retain normal success or failure feedback; whitespace-only spelling changes shall preserve its validity.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-001.11:** Use Repository shall create an unsaved card only from a current discovered selection or the current successful manual validation. Manual confirmation shall use the returned canonical path even when its spelling differs from the input. Empty input, no workspace, pending or failed validation, and an action retained from a retired context shall not admit a manual draft. Confirmation alone shall not persist a repository.

### REQ-WORKSPACES-LOCAL-REPOSITORIES-002: Runtime-aware repository discovery

**Intent:** Repository discovery must preserve server convenience without causing
unexpected filesystem access from the desktop application.

**User story:** As a desktop user, I want to choose where Kandev searches, so
that repository discovery does not request access while I am idle.

#### Acceptance criteria

- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.1:** A server launch shall use
  `repositoryDiscovery.roots` and shall use the server user's home when those
  roots are empty.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.2:** A desktop launch without a saved
  or operator-configured root shall not scan the user's home or open a protected
  directory.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.3:** A desktop repository-selection
  surface shall show saved repositories before it offers automatic discovery.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.4:** A desktop user shall start root
  selection from a visible action that opens the native folder picker.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.5:** The native folder picker shall
  start at an existing local workspace folder under Home when one is available.
  Otherwise, it shall use the operating system's default location without
  forcing Home as the initial directory. The user can still navigate to and
  select Home or a narrower root. The desktop process shall remain responsive
  to operating-system events while the modal dialog is open. Cancellation
  shall leave discovery roots unchanged.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.6:** Kandev shall scan only roots that
  the desktop user selected or an operator configured.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.7:** Selecting a root shall save its
  canonical path and shall start one immediate discovery scan.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.8:** A saved desktop root shall remain
  available after application restart until the user removes it or access fails.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.9:** When access to a saved root fails,
  Kandev shall stop automatic refresh for that root and offer a Reconnect action.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.10:** The desktop SPA shall receive only
  a narrow native folder-selection command. It shall not receive general native
  filesystem authority.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.11:** Browser and phone clients shall
  retain server discovery and the server folder browser when they use a
  server-launched backend.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.12:** The Tauri WebView shall use the
  native folder picker. It shall not call the HTTP directory-listing API.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.13:** An ordinary browser that connects
  to a desktop-launched backend shall use desktop discovery policy. It can use
  the HTTP folder browser for explicit selection.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.14:** A macOS scan with the user's home
  as its root shall skip direct `Desktop`, `Documents`, and `Downloads`
  children. A scan shall include one of these folders when that folder is a root.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.15:** On upgrade, a desktop launch shall
  retain all operator-configured roots. An existing implicit-home installation
  shall show a confirmation action without an automatic home scan. Clicking
  **Continue Home Discovery** shall save the backend user's canonical Home
  and start one scan without opening a folder picker. The backend shall add
  Home only while confirmation is pending. A retry may return an existing
  Home root without another scan. A stale action after another root was
  selected shall not add Home.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.16:** Desktop discovery roots shall have
  install-wide scope. A workspace-scoped discovery response shall apply the same
  effective roots for each workspace.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-002.17:** A macOS privacy dialog that Kandev
  can describe shall state why repository or task-folder access is necessary.

### REQ-WORKSPACES-LOCAL-REPOSITORIES-003: Bounded discovery refresh

**Intent:** Repository choices need recent data without a background scan that
can display a permission dialog after the user leaves the application.

#### Acceptance criteria

- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.1:** Discovery results shall include the
  scan time, root state, and whether a refresh is in progress.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.2:** Opening a repository-selection
  surface shall show cached results immediately when they exist.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.3:** A visible repository-selection
  surface shall start no more than one refresh when the cache is at least 30
  minutes old.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.4:** Kandev shall not run a repository
  discovery timer while all repository-selection surfaces are closed.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.5:** A user action shall allow an
  immediate refresh without waiting for the 30-minute freshness period.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.6:** Concurrent requests for the same
  roots shall share one scan.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.7:** A failed refresh shall preserve the
  last successful result. Failed-root details shall remain in structured backend
  diagnostics. Repository selectors shall retain their existing manual Refresh
  action, and saved desktop roots shall retain their explicit recovery actions.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.8:** An empty or filtered discovery
  result shall show a visible manual Refresh action.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.9:** When a descendant is inaccessible,
  discovery shall continue through accessible siblings and retain repositories
  already found. An accessible root shall not require reconnection solely
  because a descendant is inaccessible.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.10:** When at least one root succeeds
  and another fails, discovery shall return fresh results from every successful
  root. It shall retain previous results only for failed roots. Successful
  empty scans shall remove obsolete results from those roots.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.11:** When roots fail, browser and phone
  repository selectors shall keep available repositories selectable and retain
  their normal manual Refresh action. They shall not render failed-root paths or
  a failed-root warning. Failed-root details shall remain in structured backend
  diagnostics. Saved desktop roots shall retain their Reconnect and Remove
  actions.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.12:** A missing clone directory shall
  not prevent results from other roots from appearing on initial or later scans.
  Discovery shall not create the directory to recover from this condition.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.13:** When cached reads and refreshes
  overlap for one workspace, only the latest started distinct request shall replace
  shared choices, root/freshness metadata, or errors, including empty results.
  Its failure shall preserve accepted choices. Older successes or failures
  shall not reverse that outcome or trigger automatic refresh. Joining pending
  unchanged-root work shall retain its order. Browser and phone consumers shall
  share accepted state. Busy indicators shall clear after pending work finishes
  unless accepted metadata reports a scan.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.14:** After successful Add, Home
  confirmation, Reconnect, or Remove, the initiating workspace in that tab shall
  synchronize through a new read. Every consumer sharing its discovery state
  shall observe the accepted changed-root result, including empty collections.
  Pre-mutation reads shall not publish data, metadata, errors, busy state, or
  automatic follow-up afterward. Failed mutations shall not invalidate reads.
  Unchanged-root reads shall still share work. Immediate synchronization of
  other workspaces or tabs is outside this guarantee.

### REQ-WORKSPACES-LOCAL-REPOSITORIES-004: Filesystem access diagnostics

**Intent:** Users and support need to identify the operation, path, and trigger
behind a macOS access failure.

#### Acceptance criteria

- **AC-WORKSPACES-LOCAL-REPOSITORIES-004.1:** Discovery, directory listing,
  repository validation, Git polling, and file monitoring shall identify their
  operation and trigger in structured logs.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-004.2:** A denied filesystem operation shall
  write a warning with the canonical path, operation, runtime mode, trigger,
  task or session identity when present, and operating-system error.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-004.3:** Repeated identical poll failures
  shall use bounded warning output and shall report the suppressed count.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-004.4:** After one final file and Git scan,
  an idle session without a focused client or in-flight operation shall not keep
  polling active.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-004.5:** A denied watched path shall stop
  automatic polling until a visible user action retries access.

## Migrated source detail

## What

- A saved repository must continue resolving to its recorded canonical location. Git metadata
  outside that location is accepted only for a verifiable linked worktree or initialized submodule
  with reciprocal canonical `core.worktree` metadata.
- Provider-backed repositories may be saved without a local path and are unaffected until Kandev
  materializes a local clone.
- Path behavior is platform-native, including Windows drive-letter paths and UNC paths.

Decision: [ADR-2026-07-20-explicit-local-repository-trust](../../../decisions/2026-07-20-explicit-local-repository-trust.md).

## Data Model

The existing `repositories` record is the durable grant:

| Field | Contract |
| --- | --- |
| `id` | Stable repository identity used by later Git operations. |
| `workspace_id` | Workspace that owns the repository grant. |
| `source_type` | `local` for explicitly selected on-machine repositories; `provider` may remain pathless. |
| `local_path` | Canonical absolute path for a saved local repository. Empty is permitted for pathless provider repositories. |

Saved repositories remain exact-path grants. Desktop discovery roots are a
separate install-wide discovery preference and do not save every repository
found below a root.

## API Surface

- `GET /api/v1/workspaces/:id/repositories/discover`
  continues scanning only configured discovery roots. A caller-provided `root` must remain within
  those roots.
- Desktop discovery-root endpoints list, add, reconnect, and remove roots only
  for a desktop-owned launch. The Tauri client gets a path from the native
  picker. An ordinary browser can get a path from the HTTP folder browser.
- `GET /api/v1/workspaces/:id/repositories/validate?path=...`
  validates an explicitly selected path without applying discovery-root containment. It returns the
  existing path, existence, Git, default-branch, and message fields. The legacy `allowed` field is
  retained for compatibility but no longer represents discovery-root containment.
- `POST /api/v1/workspaces/:id/repositories` and `PATCH /api/v1/repositories/:id`
  validate and canonicalize non-empty local paths server-side. Invalid paths return a 4xx response
  and are not persisted.
- Read-only pre-registration branch and local-status requests may use an explicit raw path.
- Fetch and destructive fresh-branch operations resolve a persisted repository ID before touching
  the filesystem.
- Workspace-qualified repository requests reject IDs owned by another workspace before provider or
  filesystem access.

## Permissions

This feature follows Kandev's current trusted-local-user model. Selecting a
desktop discovery root grants scanning below that root. Saving one repository
grants later repository operations only for that exact canonical path. Server
discovery remains constrained by deployment configuration.

## Failure Modes

| Condition | Observable behavior |
| --- | --- |
| Path is missing | Validation reports that the path does not exist; create/update returns 4xx. |
| Path is not a directory | Validation reports that it is not a directory; create/update returns 4xx. |
| Directory is not a Git repository | Validation reports that it is not a Git repository; create/update returns 4xx. |
| Canonicalization or access fails | The operation fails without persisting or mutating repository state. |
| `.git` metadata points at an unrelated repository or unverifiable external metadata | Validation fails and the path is not persisted. |
| Submodule metadata enables Git includes or `config.worktree` overrides | Validation fails and the path is not persisted. |
| A saved path later resolves to a different canonical location | Identity-bound reads and mutations fail closed. |
| A pre-canonical saved path contains symbolic-link components | The user re-saves it once to persist its canonical location. |
| Saved repository later disappears | Read and Git operations surface the filesystem error; the stored grant remains until edited or deleted. |
| No `origin` remote and the selected local base branch is missing | Merge and Rebase report that the local base branch does not exist. The operation does not change repository history. |
| An `origin` fetch fails | Merge and Rebase report the fetch error. They do not use a local branch as a fallback. |
| Automatic scan requests an unconfigured root | Discovery rejects the request and does not scan it. |
| Desktop has no effective root | Discovery is not called; the UI offers explicit folder selection. |
| Desktop has configured discovery roots | Discovery retains those roots and does not require new consent. |
| Existing desktop install used the implicit home fallback | Discovery does not scan Home. The UI offers a Continue Home Discovery action. |
| Home is a macOS discovery root | Discovery skips direct Desktop, Documents, and Downloads children. |
| Desktop, Documents, or Downloads is an explicit root | Discovery scans the selected root and macOS can request access. |
| A saved desktop root becomes inaccessible | The cached result remains visible, automatic refresh stops, and the UI offers Reconnect. |
| macOS forgets access after an unsigned update | The first failed operation logs the target and the UI offers Reconnect without retrying in the background. |
| Destructive request supplies only an untrusted raw path | The operation fails closed and does not run Git. |

## Persistence Guarantees

The canonical `repositories.local_path` survives backend and launcher restarts through the existing
repository store. No in-memory root mutation or packaged `config.yaml` edit is required. Deleting
the repository record removes that exact durable grant from the workspace.

## Examples

- Saving `D:\Projects\app` with Home discovery records its canonical native path
  without scanning `D:\Projects` unless separately configured. Casing and trailing
  separators follow Windows filesystem semantics.
- Merge/Rebase without `origin`, when local `main` is missing, reports
  `base branch "main" does not exist locally` without changing history.

## Out of Scope

- Editing operator-configured server discovery roots from the UI.
- Automatically trusting an entire drive or the parent of the user's home.
- Full Disk Access automation or any claim that an unsigned build can retain a
  stable macOS privacy identity across updates.
- Changing provider clone placement or container host-path mounting.
- Introducing multi-user authentication or repository-grant roles.
- Making packaged runtime configuration files writable from the UI.
- Making Pull, Push, or change-request creation work without a configured remote.

## Implementation Plans

- [Manual Repository Validation Ownership](../../../plans/manual-repository-validation-ownership/plan.md)
- [Repository Discovery Root Mutations](../../../plans/repository-discovery-root-mutations/plan.md)
- [Repository Discovery Ordering](../../../plans/repository-discovery-ordering/plan.md)
- [Repository Discovery Failure Recovery](../../../plans/repository-discovery-failure-recovery/plan.md)

- [Explicit Local Repository Trust](../../../plans/explicit-local-repository-trust/plan.md)
- [Local-only Merge and Rebase](../../../plans/local-only-merge-rebase/plan.md)
- [Desktop Repository Discovery Consent](../../../plans/desktop-repository-discovery-consent/plan.md)
