---
status: current
system: tasks
requirements:
  - REQ-TASKS-OPEN-FOLDER-001
---

# Open task folder system design

## Purpose and boundaries

Consolidate the session folder action into the editor dropdown using the existing editors service folder operation.
This extends task workspace access; it introduces no persistence or new OS integration.
See [requirements](../requirements/open-task-folder.md) and
[workspace actions](attach-workspace-sources.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-OPEN-FOLDER-001 | Components, Selection and transport, Presentation and failures |

## Components

`apps/web/components/task/task-top-bar.tsx` keeps `EditorsMenu` in
`TopbarToolsGroup`, inside the existing unarchived-task branch. The standalone
folder button is removed.
`apps/web/components/task/editors-menu.tsx` delegates the dropdown shell to
`EditorActionsDropdown` in `editor-actions-dropdown.tsx`, which renders a separated
Open folder row after the editor entries. Reuse `useTaskFolderAction` and `TaskFolderPicker`; retain
`useOpenSessionFolder` for requests, per-session duplicate suppression, loading,
and localized errors. Folder opening never changes the default editor.

The dropdown trigger requires a session, not a configured editor. With zero
editors, retain the disabled no-editors row and primary editor button while
allowing access to the folder row. Disable the folder row using the shared
folder action's capability/loading state; it must not disable editor entries.
The dropdown trigger uses the localized `task:editorActions` name, explicitly
announcing both editor and folder actions.

Close the dropdown before opening the folder picker. Follow
`WorkspaceActionsMenu` in `file-browser-toolbar.tsx`: queue selection in a ref,
activate from `onCloseAutoFocus`, and pass the persistent dropdown trigger to
`action.open`. Cancel restores that trigger, never an unmounted menu item.
Mount the picker outside dropdown content. Reuse `buildWorktreeOptions` and
`useSessionWorktrees` for labels and selected-session target resolution.

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
where needed. The dropdown trigger and folder row have accessible names and keyboard activation,
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
failed, or false discovery disables the folder row in the editor dropdown,
Files menu, and `file-actions-dropdown.tsx`, plus `useOpenSessionFolder`. Editor
entries retain their existing `editors` and `useOpenSessionInEditor` behavior.
No desktop dialog or mobile bottom sheet opens while unavailable. This checks
executable installation, like IDE discovery; launch-time failures still show errors.

## Implementation plan

[Editor dropdown folder action](../../../plans/editor-dropdown-folder/plan.md) records the implemented relocation. The original plan preserves historical validation evidence.
