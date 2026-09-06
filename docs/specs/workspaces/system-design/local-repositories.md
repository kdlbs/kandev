---
status: draft
system: workspaces
created: 2026-08-27
owners:
  - kandev
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-001
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-003
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-004
---

# Local Repository Discovery System Design

## Overview

The workspace system will separate server discovery from desktop discovery.
Server launches retain automatic home discovery. Desktop launches require a
user-selected root before discovery can read the home directory.

This design extends the existing local-repository contract. It does not turn a
discovered repository into a saved repository grant.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-WORKSPACES-LOCAL-REPOSITORIES-001 | API Surface, Permissions, Failure Modes, Persistence Guarantees |
| REQ-WORKSPACES-LOCAL-REPOSITORIES-002 | Runtime policy, Desktop folder selection, Home scan exclusions, Persistence and state, Upgrade behavior |
| REQ-WORKSPACES-LOCAL-REPOSITORIES-003 | Discovery flow, User interface, Persistence and state |
| REQ-WORKSPACES-LOCAL-REPOSITORIES-004 | Workspace polling, Diagnostics, Failure handling |

## Goals

- Preserve automatic home discovery for server and web deployments.
- Prevent automatic home scans from an unconfigured desktop application.
- Use a native folder picker for desktop folder selection.
- Return cached repository choices without a filesystem scan.
- Refresh stale data only while a repository-selection surface is visible.
- Explain denied filesystem access through structured diagnostics.
- Stop idle workspace polling after the active operation finishes.

## Non-goals

- Programmatically grant Full Disk Access.
- Guarantee macOS permission persistence for unsigned application updates.
- Give the SPA a generic Tauri filesystem plugin.
- Change provider repository discovery or clone placement.
- Move existing worktrees or scratch workspaces.

## Runtime policy

The Tauri shell sets a dedicated internal desktop marker when it starts the Go
runtime. This process marker selects discovery policy for all connected clients.

The boot payload exposes a desktop picker capability. This client capability
selects the available folder-selection interface. It does not change backend
discovery policy.

| Backend and client | Effective roots | Folder selection |
| --- | --- | --- |
| Server backend and any browser | Configured roots, otherwise server user Home | HTTP folder browser |
| Desktop backend and Tauri WebView | Configured roots plus selected desktop roots | Native folder picker |
| Desktop backend and ordinary browser | Configured roots plus selected desktop roots | HTTP folder browser |

A desktop backend with no effective root does not scan Home. An ordinary
browser can select a root explicitly, but it does not restore the home fallback.

The desktop marker is internal wiring. It does not become a supported operator
configuration key. If the marker is absent, the backend uses server policy.

## Desktop folder selection

The desktop bridge adds one origin-checked command. The command opens a native
directory panel at the current user's home and returns one selected canonical
directory or a cancellation result.

The command does not list directories, read files, or accept a caller-provided
path. The Tauri WebView sends the returned path to the desktop root API. The
backend validates the directory before it saves the root.

The same adapter supplies explicit folder selection for repository discovery
and repo-less task folders. The Tauri WebView never calls
`GET /api/v1/fs/list-dir`.

The HTTP directory-listing endpoint remains available in desktop mode. An
ordinary browser can use it because the browser selects paths on the backend
host. Opening that browser is an explicit user action.

Apple documents that a standard open panel grants access to a selected folder
and its descendants for a sandboxed application. Some files can remain
inaccessible for other policy reasons. The implementation must treat selection
as user intent, not as proof that every descendant is readable.

The macOS bundle supplies usage descriptions for Desktop, Documents, and
Downloads access. These descriptions improve dialog text. They do not reduce
dialog frequency or create a stable code identity.

## Home scan exclusions

On macOS, a scan whose canonical root equals the current user Home skips these
direct children:

- `Desktop`
- `Documents`
- `Downloads`.

The walker applies this rule only to direct children of Home. If one protected
folder is an effective root, the walker scans that folder normally.

This rule makes Home discovery useful without three default privacy dialogs. A
user with repositories in a protected folder can select that folder explicitly.

## Persistence and state

The backend stores desktop discovery roots in SQLite as install-wide records.
Each record contains:

- canonical path
- display path relative to home when possible
- state: `connected` or `reconnect_required`
- last successful scan time
- last failure class and time

Operator-configured roots remain in startup configuration. The effective root
set combines configured roots with desktop records. Workspace identifiers do
not change this root set.

The backend seeds the effective set from configured roots on each launch. It
does not copy these roots into SQLite.

The workspace-scoped discovery endpoint returns repositories for one workspace.
It does not give workspace scope to the desktop root records.

The repository-discovery cache is keyed by the normalized root set and maximum
depth. It stores the last successful repositories, scan time, and root state.
One single-flight scan serves concurrent workspace requests for the same key.

## Upgrade behavior

On the first desktop launch after this change, configured roots enter the
effective root set immediately. This path requires no new user selection.

If an existing installation depended on implicit Home, the backend records a
`home_confirmation_required` migration state. It does not create a Home root.

The SQLite migration distinguishes an existing database from a new database.
For an existing database, it records this state only when configured and saved
roots are both empty. A new database starts in the normal unconfigured state.

The UI then shows Continue Home Discovery. If the user selects this action, the
native picker opens at Home. Saved repositories remain available during this
migration.

## Discovery flow

1. A repository-selection surface requests the current discovery snapshot.
2. The backend returns cached repositories and freshness metadata without a scan.
3. The frontend renders saved and cached repositories immediately.
4. If the surface is active and the snapshot is 30 minutes old, it requests a refresh.
5. The backend shares an existing scan or starts one scan for the root set.
6. Success replaces the cache and broadcasts the new snapshot.
7. Failure preserves the cache and marks only the failed root for recovery.

The shared discovery coordinator owns one activation count in each browser tab.
An open consumer acquires one activation lease. It releases the lease when the
surface closes or unmounts.

The coordinator starts a stale refresh only when both conditions are true:

- the activation count is more than zero
- `document.visibilityState` is `visible`.

The coordinator does not cancel an active scan when the document becomes
hidden. It starts no new scan until both conditions are true again.

No interval runs when the activation count is zero. A manual Refresh action
bypasses the freshness test but still shares an active scan.

## User interface

Create Task, Add Workspace Sources, Automations, Office project setup, and
Workspace Repositories consume one discovery-state hook.

Desktop with no effective root shows saved repositories and one action named
Choose folders to discover repositories. The action explains that the user can
select Home or a narrower folder.

If migration needs Home confirmation, the surface also shows Continue Home
Discovery. Cancellation leaves the current choices unchanged.

An inaccessible saved root shows Reconnect and Remove actions. Reconnect opens
the native picker again. It does not retry the denied path in the background.

An empty result and a filtered result show a visible Refresh action. This action
lets a user find a repository that arrived before the 30-minute limit.

Phone web with a server backend keeps the server-host repository picker. A
browser on a desktop backend uses desktop policy and the HTTP folder browser.

Temporary repository choices use the current phone-native picker or drawer
composition. No native Tauri control appears in a browser or mobile viewport.

## Workspace polling

Repository discovery and workspace monitoring remain separate functions. Normal
worktrees use `~/.kandev/tasks`, which macOS does not protect with folder TCC.

Polling changes reduce CPU use for normal worktrees. They reduce privacy dialogs
only for a direct-local task whose repository is in a protected folder.

Monitoring keeps fast mode for a focused client. It keeps slow mode for an
unfocused agent or terminal operation.

The target modes are:

| State | File monitor | Git status |
| --- | --- | --- |
| Focused client | 2 seconds | 3 seconds |
| Unfocused operation in flight | 30 seconds | 30 seconds |
| Turn completes without a focused client | One final scan | One final scan |
| No focused client and no operation in flight | Paused | Paused |
| No mode push during the 60-second startup grace | One final scan, then paused | One final scan, then paused |

The lifecycle manager uses the activity lease as the operation signal. The
`releaseActivity(executionActivityKey(...))` seam already identifies turn
completion.

When the last runtime activity ends, the manager requests one final workspace
refresh. Then it removes runtime interest and pushes `paused` unless UI interest
requires `fast` or `slow`.

The aggregator must send a `paused` transition to agentctl. It must not suppress
that transition when it removes the final contribution.

If no mode push arrives during the 60-second startup grace, agentctl performs
one final refresh and enters `paused`. It does not stay in `slow` forever.

An access-denied result pauses the affected tracker. A visible task or explicit
retry action can reactivate it. This rule prevents a denied path from producing
repeated macOS access attempts.

## Diagnostics

Every filesystem operation carries an operation context with these fields:

- operation name
- canonical target path
- trigger, such as `user_select`, `manual_refresh`, `stale_refresh`, or `poll`
- runtime mode
- workspace, task, and session identifiers when available
- poll mode when the operation came from a tracker.

Expected scan starts and skips use `info`. Access denial uses `warn`. Identical
poll warnings use a bounded logger that records the suppressed count. The first
warning must contain enough data to reproduce the operation.

Suggested event names are:

- `repository.discovery.picker_opened`
- `repository.discovery.scan_started`
- `repository.discovery.scan_skipped`
- `filesystem.access_denied`
- `workspace.poll_paused_after_denial`.

## Failure handling

- Picker cancellation changes no grant and starts no scan.
- An invalid selected path is rejected without persistence.
- A partial scan returns the last successful cache and identifies failed roots.
- A denied root moves to `reconnect_required` and receives no automatic retry.
- An unsigned update can change macOS code identity. The UI offers Reconnect and
  does not claim that one consent will survive every update.
- An unavailable desktop bridge shows a typed error and does not fall back to
  the server folder browser inside the Tauri WebView.
- An ordinary browser on the desktop backend can use the HTTP folder browser.
- An old implicit-home installation receives a confirmation state. It does not
  receive an automatic Home root.

## Security and privacy

The desktop bridge follows the owned-loopback-origin check used by external
links and native notifications. It returns only a path selected by the user.
No command accepts an arbitrary path or exposes directory contents.

The Tauri permission surface names only the picker command. The implementation
adds `tauri-plugin-dialog` to the `desktop-runtime` feature. It updates
`capabilities/default.json`, generated command permissions, `tauri.conf.json`,
and the bundle usage descriptions. The picker module is exported through
`src/lib.rs` because the binary target has `test = false`.

The HTTP directory-listing API remains part of the trusted local-user model. The
Tauri WebView cannot use it as a fallback.

Diagnostic bundles can contain local paths. Public recovery documentation must
state this before a user exports a bundle.

## Explicit local repository validation

The workspace system canonicalizes an explicit repository path before it is
saved or used. Automatic discovery roots do not constrain explicit selection.

Standalone `.git` directories remain valid when they do not redirect their
common directory. A regular-file `.git` pointer is accepted through one of two
independent reciprocal validators: linked worktrees require `gitdir`,
`commondir`, and placement under `<common>/worktrees`; initialized submodules
require a non-empty `[core] worktree` value in module metadata. Relative values
resolve from the canonical metadata directory and must canonically equal the
selected repository. A `commondir` file excludes the submodule validator. Git
include sections and `extensions.worktreeConfig` are rejected for submodule
metadata because the validator does not evaluate alternate configuration
sources.

When neither validator succeeds, their errors are joined to retain diagnostics.

The explicit repository validation contract is recorded in [Explicit submodule
repository trust](../../../decisions/2026-08-28-explicit-submodule-repository-trust.md).

## Verification strategy

- Go tests cover runtime policy, canonical roots, cache freshness, single-flight
  scans, reconnect state, partial failures, and structured log fields.
- Rust tests cover origin checks, cancellation, directory-only selection, and
  the absence of generic filesystem commands.
- Frontend tests cover all repository-selection consumers through one shared hook.
- Web E2E uses a stubbed native-picker adapter for selection, cancellation,
  cache, denial, reconnect, removal, and migration states.
- Browser E2E covers server Home discovery at desktop and phone widths. It also
  covers an ordinary browser on a desktop backend.
- Rust tests cover the native command and origin rule through the library target.
- Manual macOS QA covers `NSOpenPanel` and real privacy dialogs.
- Lifecycle tests cover final refresh, paused delivery, startup fallback, focus,
  and new operation activity.

## Decisions

- [Use Explicit Roots for Desktop Repository Discovery](../../../decisions/2026-08-27-explicit-desktop-repository-discovery-roots.md)
