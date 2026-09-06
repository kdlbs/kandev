---
status: active
system: workspaces
created: 2026-07-20
updated: 2026-08-30
owners:
  - kandev
---
# Local Workspace Repositories Requirements

## Overview

Users need to connect repositories already present on the machine running Kandev.
Server launches can discover repositories from operator-configured roots or the
server user's home. Desktop launches need explicit user-selected discovery roots
so macOS does not receive unexpected protected-folder access.

## Requirements

### REQ-WORKSPACES-LOCAL-REPOSITORIES-001: Local Workspace Repositories

**Intent:** Users need to connect repositories already present on the machine running Kandev, including native Windows repositories outside the user's home directory. Explicitly adding one repository should work without widening automatic filesystem scans or editing packaged runtime configuration.

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
  start at the user's home and shall allow selection of home or a narrower root.
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
  shall show a confirmation action without an automatic home scan.
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
  last successful result and shall expose the failed root and recovery action.
- **AC-WORKSPACES-LOCAL-REPOSITORIES-003.8:** An empty or filtered discovery
  result shall show a visible manual Refresh action.

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

## Why

Users need to connect repositories already present on the machine running Kandev, including native
Windows repositories outside the user's home directory. Explicitly adding one repository should
work without widening automatic filesystem scans or editing packaged runtime configuration.

## What

- A user can add a local Git repository by entering or selecting an absolute path that the Kandev
  process can access.
- Manual selection is valid independently of `repositoryDiscovery.roots`; those roots govern only
  automatic discovery scans.
- Kandev validates and canonicalizes a non-empty local repository path before saving it. A saved
  repository records the exact canonical path the user selected.
- Trusting one repository does not trust its parent directory, filesystem volume, or sibling
  repositories.
- Saved repositories remain usable for branch listing, current status, refresh, task creation, and
  fresh-branch workflows after restart.
- A saved repository without an `origin` remote supports Merge and Rebase when the selected base
  branch exists locally.
- A repository with an `origin` remote refreshes and uses `origin/<base>` for Merge and Rebase.
- A missing local base branch causes a clear error before Merge or Rebase changes repository
  history.
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

## Scenarios

- **GIVEN** automatic discovery is rooted at the user's home directory, **WHEN** a Windows user
  manually validates and saves `D:\Projects\app`, **THEN** Kandev accepts the repository and persists
  its canonical native path.
- **GIVEN** a manually saved repository outside every discovery root, **WHEN** the user lists or
  refreshes its branches after a restart, **THEN** Kandev resolves the saved repository ID and the
  operation succeeds.
- **GIVEN** a task worktree has no `origin` remote and has a local `main` branch, **WHEN** the user
  merges or rebases from `main`, **THEN** Kandev uses the local branch and the operation succeeds.
- **GIVEN** a repository has an `origin` remote, **WHEN** the user merges or rebases from `main`,
  **THEN** Kandev fetches and uses `origin/main`.
- **GIVEN** a task worktree has no `origin` remote and the selected `main` base branch is missing
  locally, **WHEN** the user starts Merge or Rebase, **THEN** Kandev reports
  `base branch "main" does not exist locally` and leaves repository history unchanged.
- **GIVEN** a manually saved repository outside every discovery root, **WHEN** the user confirms a
  fresh-branch operation for it, **THEN** Kandev resolves the saved repository ID before changing
  the working tree.
- **GIVEN** `D:\Projects\app` was explicitly saved, **WHEN** automatic discovery runs, **THEN** it does
  not scan `D:\Projects` unless that directory is separately configured as a discovery root.
- **GIVEN** a missing directory, ordinary directory, or inaccessible path, **WHEN** a user tries to
  save it as a local repository, **THEN** the backend rejects the request even if the frontend did
  not validate first.
- **GIVEN** path spelling differs only by Windows drive-letter casing or trailing separators,
  **WHEN** Kandev canonicalizes and compares the path, **THEN** it treats the spellings according to
  Windows filesystem semantics.

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

- [Explicit Local Repository Trust](../../../plans/explicit-local-repository-trust/plan.md)
- [Local-only Merge and Rebase](../../../plans/local-only-merge-rebase/plan.md)
- [Desktop Repository Discovery Consent](../../../plans/desktop-repository-discovery-consent/plan.md)
