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
| `AC-UI-FILE-EDITOR-MUTATION-001.7` | Workspace refresh admission and ordering |
| `AC-UI-FILE-EDITOR-MUTATION-001.8` | Refresh consumer lifetime |
| `AC-UI-FILE-EDITOR-MUTATION-001.9` | Live refresh reconciliation; Refresh verification and surfaces |

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
The mutation delivery package does not change general workspace refresh
concurrency. The bounded open-buffer extension below has its own delivery record.

Desktop keeps its pinned/preview editor and controls. Phone keeps the focused
Files/document flow in `mobile/session-mobile-layout.tsx`; `usePanelActions`
already routes phone document opening separately from desktop Dockview opening.
This correction changes shared action/state publication only, with no new phone
surface or mutation entry point. Meaningful hook/store/panel integration meets
the state/data exception in `/mobile-parity`; browser/build/E2E work would add
no evidence about responsive geometry here.

## Workspace refresh admission and ordering

`hooks/file-editors-sync.ts:syncOpenFileFromWorkspace` owns reconciliation of
already-open Dockview buffers. Its two production callers are
`useOpenFileWorkspaceSync` (Git-signature changes, called by `useFileEditors`)
and `FileEditorPanel.useResyncOnTabActivate` (initial active tab and subsequent
activation). Ordering must span both callers and independently mounted readers
because they publish into the same Dockview buffer.

At admission, require a live caller, current session visit, existing buffer with
its stable `instanceId`, matching path/repository, and captured Dockview host.
Reject a missing/replaced buffer before fetching; a refresh never installs one.
Capture these identities before the first await. Do not compare whole buffer
objects or content/hash identity: typing replaces objects while preserving the
incarnation, and identical reopen must still retire old replies.

Keep a small module-local pending-publication map keyed by the existing
repo-scoped `fileKey`. Its current entry is a unique request token carrying the
captured incarnation, not a file-content cache. Admission of another eligible
read to that same buffer replaces the token synchronously, even across caller
instances. Different keys remain independent. Admission by a retired caller
must not supersede a current read. No global counter, coordinator, store action,
deduplication or request cancellation is required.

Validate caller, incarnation, host and token before transport, after
`requestFileContent`, and after `calculateHash`. Token equality is required at
publication even if the newer request already settled: cleaning up its entry
must never make an older token current again. In `finally`, remove the entry
only if it is still this request's entry. Thus the map retains at most one
publication entry per key with outstanding work, without a history of closed
buffers. A retired request may release only its own bookkeeping.

Newest admission owns the result even when it fails. Leave the current editor
unchanged; suppress the older success rather than resurrecting it as a fallback.
The next real Git-signature or tab-activation trigger may refresh normally.
There is no automatic retry or change to the existing first-observation skip.
This policy follows the existing Files per-path publication-token pattern and
prevents failure from silently reauthorizing known-obsolete work.

## Refresh consumer lifetime

Forward `useFileEditors`' existing `activeEditorVisitRef` into
`useOpenFileWorkspaceSync`. Capture its committed token and session ref when
starting each refresh; a local caller guard checks both. The current
`useLayoutEffect` already retires that visit on session change, null, unmount
and StrictMode cleanup. Do not create tokens or retire owners during render.
A committed A-to-B-to-A transition creates a new A visit; it cannot revive an
earlier read. Ordinary same-session renders and typing do not retire the visit.

For tab activation, create a local committed effect lifetime in
`useResyncOnTabActivate`, tied to its existing subscription identity (panel,
session, file key/path/repository and availability). Layout cleanup retires it
before an old passive subscription or deferred reply can publish. Capture this
token in the registered callback and pass a caller guard to the shared helper;
do not read a replacement token and adopt it on behalf of an old callback.
Retain the initial-is-active sync, true-only activation callback and disposable
cleanup. Capture/check the subscribed portal API identity as well, so replacing
an API cannot authorize its old callback; no new manager-level subscription or
layout restoration redesign is introduced.

The helper's immediate arguments carry this local `isCurrent` guard; both
production callers must supply it. The helper itself owns request/buffer/host
checks, avoiding a caller-specific ordering map. A retired subscription cannot
admit a new read or displace a live caller's token. A live consumer can still
refresh after another consumer unmounts. Transport, normalization and hashing
retain their current contracts.

## Live refresh reconciliation

After asynchronous preparation, obtain the still-owned buffer again immediately
before deciding which update to publish. There is no await between this live
read and the state update. All dirty/content comparisons use this final object,
never an object captured before hashing. Apply the existing matching-dirty,
nonmatching-dirty and clean branches with the genuine remote hash and metadata.
Typing during hashing therefore follows the dirty branch and retains its text.

For matching dirty text, clear dirty/remote state and call the real
`updatePanelAfterSave` only while the owner remains current; recheck before that
panel sink because synchronous store subscribers may retire an owner during
buffer publication. For different dirty text, preserve the buffer/baseline and
publish `remoteContent`/`remoteOriginalHash` with the existing reload affordance.
For clean text, publish current content/baseline/hash/binary/resolved path and
clear obsolete remote state. Keep existing unchanged-content no-ops and symlink
metadata reconciliation. Failures stay noncritical and publish no editor change.

## Refresh verification and surfaces

Desktop and compact desktop (768-1023px with a fine pointer) both select
`DockviewDesktopLayout` through `TaskLayout` and `usesDesktopWorkbench`.
`dockview-shared.tsx` and `dockview-panel-content.tsx` render `FileEditorPanel`.
All `useFileEditors` consumers inherit its Git-status synchronization, including
panel actions, LSP openers, review/walkthrough controls and chat file opening.
The full caller inventory and exact tests live in the refresh work order.

Coarse-pointer tablet selects `SessionTabletLayout` and `TaskCenterPanel`; its
restoration reads `requestFileContent` independently into local tabs. Phone
selects `SessionMobileLayout` and its keyed `MobileFileViewerPanel`, using
`fetchAndOpenFile` and selected-file state independently. `usePanelActions`
can mount `useFileEditors` outside desktop, but its Dockview open-file actions
are gated on `usesDesktopWorkbench`; this is not evidence that tablet/phone
viewer content goes through the background helper.

This correction changes state publication only: no rendered layout, copy,
touch, scrolling, navigation or viewport-dependent interaction changes. The
`/mobile-parity` state/data exception permits targeted real helper/hook/panel
tests plus this explicit surface inventory. No new browser, build, E2E, ASCII
composition or phone/tablet read implementation is required or claimed.

Use independently authored deferred `client.request` tests with real
`syncOpenFileFromWorkspace`, Dockview actions, `requestFileContent` normalization
and `calculateHash`. Use real `StateProvider`/store actions and required providers
at the `useFileEditors` boundary; drive actual Git status via the session's
environment mapping. Mount real `FileEditorPanel` and use real portal-manager
registration with a minimal panel-API event double for activation and disposal.
Retain current initial-active/activation/dirty-title/reload positive controls.
No mocked ownership predicate, store, request helper, hash implementation,
consumer hook or affected panel can provide the ownership regression evidence.

To deliberately hold hashing, instrument only `crypto.subtle.digest`: defer its
completion, call the saved genuine digest, and forward the real bytes. Restore
the descriptor and settle/join all work in teardown. This boundary instrumentation
proves typing during real async hash preparation without replacing `calculateHash`
or using sleeps. Cover both completion orders, newer failure/older success,
same-key identical reopen/replacement, independent files/repos/readers, typing,
current clean/empty/dirty outcomes, metadata, failures, committed visits, unmount
and applicable StrictMode replay. Never replay/import the protected ROOT proof.

The [workspace refresh delivery plan](../../../plans/editor-workspace-refresh-ownership/plan.md)
extends this pair's missing criteria while the completed mutation package keeps
its original scope and recorded results. It introduces no framework or new
authority boundary; the requirement, design and tests preserve the local policy
and rationale, so no additional ADR is needed under `/record`.

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
