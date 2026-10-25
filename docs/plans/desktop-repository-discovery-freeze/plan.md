---
created: 2026-09-25
status: completed
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-002
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Fix plan: Desktop repository discovery picker

## Problem and evidence

An upgraded desktop installation can show Continue Home Discovery. The
current `RepositoryDiscoveryRootControls` uses `FolderPicker` for that
button, so the click opens a native panel instead of confirming Home.
The native `pick_directory` command calls `set_directory(Home)` and
`blocking_pick_folder()`. A slow panel therefore keeps its Tauri command
waiting. The reported macOS Finder beachball is consistent with this path,
but the specific AppKit XPC or directory-indexing cause is not verified in
this Linux workspace.

The suggested request body `{ "path": "~" }` is not a safe fix.
`canonicalDiscoveryRoot` calls `filepath.Abs` and `EvalSymlinks` on the
literal string; it does not expand tilde to the backend user's Home.

At planning time, focused tests passed for two frontend files (8 tests), the
Go desktop migration, and three Rust folder-picker cases. They did not cover
direct Home confirmation or a slow native panel.

## Result

- Continue Home Discovery sends one path-free confirmation request. The
  backend resolves its own Home, saves it only for a pending migration,
  and starts one scan. No native or HTTP folder picker opens for this action.
- Choose folders and Reconnect keep the existing folder-selection flow.
  The native picker starts in the first existing local workspace folder from
  a short list. If none exists, it uses the system default location.
- The Tauri command returns its selected, cancelled, or failed outcome
  asynchronously. The desktop event loop remains responsive while the modal
  panel is open.
  A slow operating-system panel can still be slow; macOS validation must
  check its actual behavior.

## Root cause and repair boundary

The Home banner and native picker behavior belong to
[Workspaces discovery](../../specs/workspaces/requirements/local-repositories.md).
The [system design](../../specs/workspaces/system-design/local-repositories.md)
defines the backend confirmation contract and Tauri picker boundary.
`AC-WORKSPACES-LOCAL-REPOSITORIES-002.5` and `.15` are amended. Ordinary
server discovery, protected-child scan exclusions, and exact repository
grants stay as they are.

## Technical approach

Add `POST /api/v1/repositories/discovery/roots/confirm-home` with no client
path. The service accepts it only for a desktop runtime with pending Home
confirmation. It resolves the backend user's Home and uses the existing
root-add path. A retry returns the existing Home root without a second scan;
a stale request after a different root was chosen cannot add Home.

Replace only the Home banner's `FolderPicker` with a translated `Button`.
Add a dedicated frontend action and hook method. Keep Choose folders and
Reconnect wired to `FolderPicker`. The button shows a busy state during the
request and reloads the discovery snapshot after success.

Change `pick_directory` to an async Tauri command. Use the dialog plugin's
callback-based `pick_folder` and await a one-shot result. Keep the current
owned-origin check and `Selected`/`Cancelled`/`Failed` result shape. Resolve
the first existing non-symlink directory in this order: `Projects`,
`Developer`, `src`, `Code`, `workspace`, `Development`, `repos`. If none
exists, do not set a starting directory.

## ASCII UI preview

`UI-01: Upgraded desktop, before and after`

```text
BEFORE                                  AFTER
+----------------------------------+    +----------------------------------+
| Home was searched before.         |    | Home was searched before.         |
| [ Continue Home Discovery ]        |    | [ Continue Home Discovery ]        |
|   opens a folder picker            |    |   saves Home and starts scan       |
| [ Choose folders... ]              |    | [ Choose folders... ]              |
+----------------------------------+    +----------------------------------+
```

`UI-02: Narrow browser on a desktop backend`

```text
+----------------------------+
| Home was searched before.   |
|                            |
| [ Continue Home Discovery ] |
| [ Choose folders...       ] |
+----------------------------+
```

The two actions remain distinct and reachable in the existing scroll
region. On a coarse pointer, each action has at least a 44px hit target.
The direct action uses the existing translated label in all six locales.

## Regression tests

| Acceptance | Failing test to add first |
| --- | --- |
| `AC-WORKSPACES-LOCAL-REPOSITORIES-002.15` | Go service and handler tests for pending, retry, stale, server-runtime rejection, canonical Home, and first scan; frontend component/action test that Home never invokes `pick_directory` |
| `AC-WORKSPACES-LOCAL-REPOSITORIES-002.5` | Rust preferred-directory and callback/outcome tests; rendered picker test for responsive pending/cancel state |
| `AC-WORKSPACES-LOCAL-REPOSITORIES-002.4`, `.12`, `.13` | Desktop and narrow-browser E2E: direct Home action posts without picker; Choose folders still invokes picker or HTTP browser as appropriate |

## Work orders

- [x] [Task 01: Confirm Home without a picker](task-01-direct-home-confirmation.md)
- [x] [Task 02: Keep native folder selection responsive](task-02-async-folder-picker.md)

Task 01 establishes the backend contract before wiring the direct UI action.
Task 02 can follow without changing that API.

## Verification

Run work-order checks from `apps/`, `apps/backend/`, or the desktop crate
as specified in each task. Add a real macOS smoke check for the native
panel, because Linux tests cannot prove AppKit behavior.

## Risks and exclusions

- A direct UI confirmation is not an operating-system filesystem grant.
  The backend still reports inaccessible paths and preserves root recovery.
- The callback-based command protects IPC responsiveness. It cannot
  guarantee that macOS Finder or an extension responds quickly.
- Do not send `"~"` to the existing root-add API or grant a generic native
  filesystem capability to the WebView.
- Do not change server-mode implicit Home discovery or the macOS Home scan
  exclusions for `Desktop`, `Documents`, and `Downloads`.

## Verification results

The focused Go service and handler tests passed. Discovery-control frontend
tests passed (8), as did the desktop repository-discovery E2E (4), mobile
repository-discovery E2E (4), and public-doc validation (47 pages). The desktop
E2E confirmed an empty-body Home request without invoking a native or HTTP
picker. Choose folders still uses the native picker on desktop and the HTTP
picker on mobile. Rust folder-picker tests passed (7), and the full desktop
Rust library suite passed (81). Web typecheck, lint, i18n checks, and the Linux
desktop build/smoke passed. Finder behavior and panel responsiveness still
require a macOS smoke test.
