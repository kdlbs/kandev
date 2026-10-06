---
status: current
system: ui
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
---

# File Editor Mutation Ownership System Design

## Purpose and boundaries

The editor action layer publishes completed workspace mutations into the live
Dockview editor. This UI-owned contract complements
[task navigation responsiveness](task-navigation-responsiveness.md) and the
local reply-owner pattern in [Files reply freshness](file-browser-reply-freshness.md).
The existing WebSocket filesystem APIs remain authoritative. Rejecting local
publication does not undo a successful remote mutation.

Tablet `TaskCenterPanel` uses a separate local file-tab action consumer in
`components/task/task-center-panel-restoration.ts`. It applies the same owner
rule with a local visit token and tab incarnation. New/restored tabs receive a
transient token; typing and preview changes preserve it. Explicit persisted
descriptor and request projections exclude the token. Save/delete feedback,
LSP publication and functional tab updates require the live owner; per-action
pending markers cannot clear a replacement save. A missing tab cannot authorize
closure. No common coordinator or responsive layout change is introduced.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-UI-FILE-EDITOR-MUTATION-001.1`, `.2` | Completion ownership |
| `AC-UI-FILE-EDITOR-MUTATION-001.3`, `.4` | Save and delete publication |
| `AC-UI-FILE-EDITOR-MUTATION-001.5` | Saving indication and repository identity |
| `AC-UI-FILE-EDITOR-MUTATION-001.6` | Remote update and responsive presentation |

## Components and current lifecycle

- `hooks/use-file-editors.ts` observes `tasks.activeSessionId`, updates
  `activeSessionIdRef`, clears/restores global `openFiles`, persists tab
  descriptors and removes buffer state on actual panel removal. Its multiple
  consumers include `FileEditorPanel`, `usePanelActions` and LSP file opening.
- `lib/state/dockview-store.ts` defines `FileEditorState`.
  `lib/state/dockview-file-state.ts` supplies its set/update/remove/clear actions.
  Both initial opens/restores and `FileEditorPanel.useFileLoader` use
  `setFileState`; typing and workspace reconciliation use `updateFileState`.
- `hooks/use-file-save-delete.ts` owns save/delete/reload actions and current
  panel publication. `FileEditorPanel` consumes its existing public action
  signatures and `savingFiles.has(fileKey)`.
- `lspClientManager.saveDocument` accepts session, path, repository, persisted
  text and live text. It already preserves a newer live buffer during didSave.

## Completion ownership

Capture a small local owner snapshot at action start: originating session ID,
the hook's current session-visit token, repo-scoped file key, buffer incarnation
when present, and Dockview API identity. No coordinator or server-state cache
is introduced.

`useFileEditors` creates a new visit token on session transition and retires it
on effect cleanup/unmount, including a transition to no active session. Keep
the existing active-session ref and token coherent before restore/publication
effects. A return to A after B cannot revive A's old token. Forward this local
ownership through the immediate action parameters.

Assign an internal incarnation token when `setFileState` installs a buffer.
`updateFileState` preserves it, so ordinary typing, preview promotion and remote
indications remain in the same lifetime. Remove/clear followed by set creates
another token even for identical content/hash. Do not compare whole buffer
objects: immutable edits replace those objects. This token is transient and
never enters persisted tab descriptors or transport payloads.

Before every completion sink, require the live visit/session, captured buffer
incarnation when applicable, and panel host to match. A missing/replaced buffer
is unwritable. For delete, capture whether a panel exists at dispatch; resolve
the current pinned/preview panel only within that same editor lifetime. A
panel still loading without buffer state can be identified by its captured
panel object. An originally absent panel gives no authority to close a later
one. Promotion within the same open buffer is not an editor replacement.

Use the same publication predicate for success, rejected-response feedback and
exceptions. Pending-marker cleanup separately compares its captured operation
identity: it may release its own retired bookkeeping, never a replacement's
marker. Recheck inside any deferred functional state updater. Retired
completions settle their promises without publishing or retrying.

## Save and delete publication

Build the save diff/request from the captured dirty buffer and use its session
and stored repository. After acceptance, reread only the still-owned buffer.
Publish the saved snapshot/hash as baseline; derive dirty state from the latest
buffer versus that saved snapshot. Preserve the existing remote-indication
clearing and overwritten-save feedback for owned success. Pass the captured
disk text and the owned live text to LSP. Only a clean result clears panel dirty
state/title through the existing panel helper. Suppress LSP entirely for a
retired owner, rather than combining one session's disk boundary with another
session's live text.

Keep delete's remote-first ordering, file-repository routing and existing
failure feedback. An owned successful completion closes its matching pinned
or preview panel; the normal remove-panel subscription drops buffer state.
Do not change that subscription or close a replacement found only by key.

## Saving indication and repository identity

Keep `buildRepoScopedItemId` as the buffer/panel key and existing stored `repo`
as the request's repository. Ownership tokens supplement that identity; they
do not change request routing or introduce session IDs into persisted panel IDs.

Local pending-save markers carry their visit, buffer incarnation, panel host and
operation identity (the marker object). `useFileEditors` continues to expose a
Set of saving file keys, derived only from markers matching the selected session,
live buffer and panel host. Reset markers on each visit transition. Retire old visit
markers on navigation; a reopened buffer cannot inherit a previous spinner.
Finally removes only its own marker, so A's completion cannot clear B's pending
save at the same key. This is cleanup ownership, not a new save-ordering policy.

## Remote update and responsive presentation

`applyRemoteUpdate` retains current remote content/hash handling. Its optional
`calculateHash` await uses the same captured owner before applying state/panel
changes. Already available hashes retain the synchronous publication path.
General workspace refresh concurrency remains outside this repair.

Desktop keeps its pinned/preview editor and controls. Phone keeps the focused
Files/document flow in `mobile/session-mobile-layout.tsx`; `usePanelActions`
already routes phone document opening separately from desktop Dockview opening.
This correction changes shared action/state publication only, with no new phone
surface or mutation entry point. Meaningful hook/store/panel integration meets
the state/data exception in `/mobile-parity`; browser/build/E2E work would add
no evidence about responsive geometry here.

## Persistence and related decisions

No API, schema, setting, dependency, metric or persistence changes are needed.
UI retains transient reply ownership; workspace and task authority remain
unchanged. The requirement/design preserve sufficient rationale for this local
guard extension, so `/record` does not require a separate ADR. Session-only
equality cannot reject A-to-B-to-A; path/hash equality cannot detect identical
replacements; whole-object equality incorrectly rejects typing. Stable local
lifetime tokens cover those cases without a generic coordinator.

## Validation

Use deferred transport replies against the real hook and Dockview store with
panel API doubles recording publication/removal and firing actual registered
removal handlers. Include session navigation, return, replacement, unmount,
same-session typing, clean success, active/retired failures, repo independence,
replacement saving cleanup and current remote reload. Inspect real state,
panel effects, persistence and LSP arguments rather than isolated predicates.
