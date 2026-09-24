---
status: current
system: tasks
requirements:
  - REQ-TASKS-OPEN-FOLDER-001
---

# Open task folder system design

## Purpose and boundaries

Add a discoverable session shortcut using the existing editors service folder operation.
This extends task workspace access; it introduces no persistence or new OS integration.
See [requirements](../requirements/open-task-folder.md) and
[workspace actions](attach-workspace-sources.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-OPEN-FOLDER-001 | Components, Selection and transport, Presentation and failures |

## Components

`apps/web/components/task/task-top-bar-tools.tsx` renders `EditorsMenu` in
the Task tools disclosure. An independent folder control sits immediately after
`EditorsMenu`, inside the unarchived-task branch. It is not tied to enabled editors.
Use `open-task-folder-button.tsx` and shared `useTaskFolderAction` for selection
and opener focus. The existing `useOpenSessionFolder` owns native-launch requests,
immediate duplicate-request suppression, loading state, and localized errors. Reuse `buildWorktreeOptions` and `useSessionWorktrees`
for repository/branch labels rather than duplicating target resolution.

The phone Files workspace-actions menu already offers Open workspace folder.
Reuse that entry and its responsive menu treatment; route multiple-worktree choices
through `TaskFolderPicker`, using `Dialog` on desktop and `MobilePickerSheet` for
the short phone or coarse-pointer choice. The Files menu closes before opening
the picker and passes its persistent trigger for focus restoration.
Inspect `file-browser.tsx`, `file-browser-data.ts`, and the existing
`mobile-add-workspace-sources.spec.ts` as the current entry-point precedent.

## Selection and transport

Extend `openSessionFolder` in `lib/api/domains/session-api.ts` compatibly: retain
its existing second `ApiRequestOptions` argument and add an optional third payload
`{ worktree_id?: string }`. Extend the hook's `open` method to accept an optional
worktree ID. Existing no-argument callers retain their behavior.

The existing POST `/api/v1/task-sessions/:id/open-folder` already accepts
`OpenFolderRequest.WorktreeID`. `Service.OpenFolder` resolves it through
`resolveSessionPath`: a non-empty worktree ID must belong to the session or fail
with `ErrWorkspaceNotFound`. Only an empty ID permits the first-worktree and then
repository-local-path fallbacks. The browser sends identifiers, never a raw path
or shell command. Multiple choices must be explicit at the new shortcut and phone
entry point. Clear stale picker selection on session changes; closing a picker
must not issue a request.

Keep the existing backend host boundary. `OpenFolder` dispatches `open`,
`xdg-open`, or `explorer` with the resolved path as an argument. The backend also reports opener availability and rejects unavailable commands. Successful HTTP completion means the launch
request was accepted; it does not prove a native window became visible.
Remote/headless host limitations remain those of the existing action.

## Presentation and failures

Reuse translated folder copy when possible, adding a localized host-location hint
where needed. The control has an accessible name, keyboard activation, tooltip,
loading indicator, and disabled no-session/loading/unavailable states. Pending
folder launches are shared per session across all controls and clear only when
the initiating request settles; another session remains independent. Catch rejected requests
and use the existing localized toast pattern in `useOpenSessionInEditor` so callers
do not leave unhandled promise rejections. Return null after a surfaced error.

On phones, a visible Files menu opens a short temporary choice surface. The shared
mobile picker supplies one scrolling body, safe-area spacing, dismissal and focus
return. Its worktree rows have at least 44px hit areas. Keep desktop 28px control
geometry; do not inherit unconditional touch sizing into `TopbarToolsGroup`.
No layout/editor preferences or task runtime state change.

## Verification boundary

Frontend unit tests prove payload selection, stale-session handling, missing-session
behavior, failure recovery, and editor independence. Playwright proves the actual
button/menu flows and request payload on desktop and phone. Stub the native-opening
HTTP response in UI tests rather than launching file managers on CI. Existing Go
editor tests guard session/worktree resolution. A macOS smoke check proves Finder
opens the selected folder; record it separately if no macOS host is available.

## Host availability

The existing GET `/api/v1/editors` response includes `folder_opening_available`.
The editors service uses `exec.LookPath` for the platform command (`open`,
`xdg-open`, or `explorer`); unsupported platforms report false. The same command
mapping is used for opening, and the POST operation rejects unavailable openers.
The editors boot payload includes `folderOpeningAvailable` alongside its loaded
items. The shared editors store retains the capability; loaded editor items with
an unknown capability still trigger discovery. Each effect reads the live store
before claiming discovery so simultaneous consumers share one request/retry chain. Transient failure gets one delayed
retry while controls remain disabled; repeated failure settles to false. Unknown,
failed, or false discovery disables folder-opening controls in the task toolbar,
Files menu, and `file-actions-dropdown.tsx`, plus `useOpenSessionFolder`. Editor
entries retain their existing `editors` and `useOpenSessionInEditor` behavior.
No desktop dialog or mobile bottom sheet opens while unavailable. This checks
executable installation, like IDE discovery; launch-time failures still show errors.
