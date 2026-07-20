---
status: shipped
created: 2026-07-20
owner: kandev
---

# Local Workspace Repositories

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
- A saved repository must continue resolving to its recorded canonical location. Git metadata
  outside that location is accepted only for a verifiable linked worktree with reciprocal metadata.
- Provider-backed repositories may be saved without a local path and are unaffected until Kandev
  materializes a local clone.
- Path behavior is platform-native, including Windows drive-letter paths and UNC paths.

Decision: [ADR-2026-07-20-explicit-local-repository-trust](../../decisions/2026-07-20-explicit-local-repository-trust.md).

## Data Model

The existing `repositories` record is the durable grant:

| Field | Contract |
| --- | --- |
| `id` | Stable repository identity used by later Git operations. |
| `workspace_id` | Workspace that owns the repository grant. |
| `source_type` | `local` for explicitly selected on-machine repositories; `provider` may remain pathless. |
| `local_path` | Canonical absolute path for a saved local repository. Empty is permitted for pathless provider repositories. |

No parent-directory grant or install-wide discovery-root record is created.

## API Surface

- `GET /api/v1/workspaces/:id/repositories/discover`
  continues scanning only configured discovery roots. A caller-provided `root` must remain within
  those roots.
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

This feature follows Kandev's current trusted-local-user model: explicitly selecting or saving a
repository is a user grant to access that exact path. Automatic discovery remains constrained by
deployment configuration. A future multi-user authorization model must gate repository grants
directly rather than treating discovery roots as an authorization mechanism.

## Failure Modes

| Condition | Observable behavior |
| --- | --- |
| Path is missing | Validation reports that the path does not exist; create/update returns 4xx. |
| Path is not a directory | Validation reports that it is not a directory; create/update returns 4xx. |
| Directory is not a Git repository | Validation reports that it is not a Git repository; create/update returns 4xx. |
| Canonicalization or access fails | The operation fails without persisting or mutating repository state. |
| `.git` metadata points at an unrelated repository or unverifiable external metadata | Validation fails and the path is not persisted. |
| A saved path later resolves to a different canonical location | Identity-bound reads and mutations fail closed. |
| Saved repository later disappears | Read and Git operations surface the filesystem error; the stored grant remains until edited or deleted. |
| Automatic scan requests an unconfigured root | Discovery rejects the request and does not scan it. |
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

- A settings page for managing install-wide discovery roots.
- Automatically trusting an entire drive, home-directory parent, or repository parent directory.
- Changing provider clone placement or container host-path mounting.
- Introducing multi-user authentication or repository-grant roles.
- Making packaged runtime configuration files writable from the UI.

## Implementation Plan

See [Explicit Local Repository Trust plan](../../plans/explicit-local-repository-trust/plan.md).
