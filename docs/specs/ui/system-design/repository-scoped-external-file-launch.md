---
status: current
system: ui
requirements:
  - REQ-UI-EXTERNAL-FILE-TARGET-001
---

# Repository-scoped external file launch design

## Boundary and mapping

This is the reusable UI handoff for external file actions, independent of the embedded editor's downstream path serialization. The [requirements](../requirements/repository-scoped-external-file-launch.md) and [work package](../../../plans/repository-scoped-external-file-launch/plan.md) cover this bounded correction; workspace path authority and the backend editor endpoint remain unchanged.

| Criteria | Design section |
| --- | --- |
| 001.1, 001.2 | Callers and identity; Resolution |
| 001.3, 001.4 | Resolution; Failure and compatibility |
| 001.5 | Responsive behavior |

All suffixes refer to `AC-UI-EXTERNAL-FILE-TARGET-001`.

## Current path and evidence

`ReviewDiffHeader` supplies `file.repository_id` and `file.repository_name` to `FileDiffToolbar`. Desktop renders `FileActionsDropdown`; phone renders `FileActionsMenuItems` inside its existing overflow. Both currently discard that identity. The shared hook invokes `useOpenSessionInEditor` with `filePath` only. Its real `useRequest` calls `openSessionInEditor`, which already transports `worktree_id`. Backend `resolveSessionPath` selects `session.Worktrees[0]` when the ID is omitted; an explicit ID is checked against a session worktree ID or association ID. It otherwise falls back to the session repository checkout. Thus the qualified secondary `src/shared.ts` reaches the HTTP-domain call without an ID and selects the primary worktree, while the primary control passes. Qualification was a rendered HappyDOM boundary check, not a Chromium or native-editor launch.

`FileEditorPanel` supplies `activeSession.repository_id`, the session primary, and the file's `repo` separately. `MonacoCodeEditor` passes that scope as `repositoryName` to its desktop toolbar and overflow. `CodeMirrorCodeEditor` passes both to its toolbar, but only its shared overflow contains `FileActionsMenuItems`. The file path is relative to `repo` (`dockview-store.ts`), not a tree-rooted path. Do not treat the editor panel's primary repository ID as a file-specific identity when a named scope is present. Static/preview viewers and `FileTreeEditorProvider` do not consume this shared file action; no new action is introduced there.

## Resolution

Pass file-specific `repositoryId` and scope name through both Review branches. For the editor callers, pass the named scope through Monaco desktop and both overflows; only use the editor panel's session-primary ID when the file has no named scope. Resolve in a domain hook under `hooks/domains/session/`, using `useSessionWorktrees` and the hydrated session from established selectors. Its association mapping sorts API `session.worktrees` by position, selects `worktree_id || id`, fills missing fields from live worktree data, unions live IDs, and uses the legacy single ID when present. IDs are session-local evidence; do not substitute repository IDs for worktree IDs.

The result is either an explicit `{worktreeId, filePath}`, a permitted default `{filePath}`, or unavailable. Empty names/IDs count as absent. For a file-specific Review ID, require exactly one matching associated worktree and use its ID, even if a scope name is present but `workspace_path` or other placement evidence is missing. The ID is authoritative; missing, unmatched, or ambiguous redundant name evidence does not invalidate it. If independently available placement evidence uniquely maps the scope to a different session worktree, that is a concrete contradiction: disable the external action. A name alone that does not match any worktree does not prove a contradiction.

For a named scope without a file-specific ID, require exactly one known worktree whose observed path maps to that scope relative to the session's actual `workspace_path`. Reuse `workspaceRelativeFilePath(worktree.path, session.workspace_path)` for this comparison and compare its result exactly with the scope. This helper already handles separators, Windows root comparison and component boundaries. `isWorkspaceTreePath` may validate the scope using the same existing tree-identity contract; do not add a separator/case parser or infer from display names, provider names, arbitrary aliases, or a worktree basename. Do not use `getSessionWorkspacePath`'s legacy `worktree_path` fallback as a task workspace root for named placement: the primary worktree path cannot establish where a secondary scope lives. Pass the unchanged scope-relative file path. Name-only nested scopes and attached folders are unavailable unless they exactly match an associated worktree placement; no tree root-name inference or path registry is introduced.

For an unscoped action, a single known associated worktree may be sent explicitly; with more than one known worktree, disable the external action. With no associated worktree, retain the no-ID backend default for an existing session with legacy `worktree_id`/`worktree_path` or repository checkout identity. A file-specific ID without an association may use that default only when there are no worktrees and it equals the session's `repository_id`; a named scope never receives this fallback. Missing session, missing or ambiguous placement when the name is the only file identity, duplicate ID associations, missing usable worktree ID, or an independently established ID/name contradiction disables the external action. Do not infer a default from an absent session. Exact name-only matching limits availability when placement is incomplete; it avoids selecting a different repository.

### Legacy single-worktree follow-up

A legacy unscoped file may carry the session's primary repository ID even when the one live worktree has no repository ID. Preserve the old no-ID backend default only when **all** of these facts hold: the file scope name is absent; the file repository ID equals the nonempty `session.repository_id`; `session.worktrees` is absent or empty; the known worktree list has exactly one entry with a nonempty usable ID; that worktree has no repository ID; and its ID equals the nonempty `session.worktree_id`. Return `{}` rather than an explicit worktree ID. The backend's omitted-ID branch selects its first associated worktree or repository checkout; an explicit ID is rejected when the server has no matching association. This preserves the existing default, not a promise that the backend uses the legacy worktree path.

A different worktree ID, any known repository ID mismatch, multiple known worktrees, a named scope, an unknown file repository ID, or an API association prevents this special fallback. Existing unique matching associations and other reviewed resolution rules remain unchanged. Do not infer named repository identity or change worktree inventory, recovery, transport, or backend behavior.

Apply the resolved target and disabled state to external editor items (`kind !== "internal_vscode"`) only. Keep embedded items' current dispatch, availability and semantics. Keep folder and copy actions untouched. Use existing disabled menu behavior and labels; no new text or tooltip. Re-evaluate from current session state when rendered and ensure selecting a disabled item cannot call the opener. The endpoint remains authoritative for membership and file path errors; no HTTP schema, backend, caching or lifetime changes.

## Responsive behavior

Review desktop uses its existing open-with dropdown; phone uses its existing file overflow menu. Monaco desktop has a dropdown and an overflow; CodeMirror uses the existing overflow. This changes target data and external-item availability, not menu composition, layout, copy, touch geometry, navigation or scroll ownership. The mobile-parity pure state/data exception applies, with an explicit rendered phone Review overflow test that activates the real editor item and asserts the HTTP-domain target. It is HappyDOM evidence only; no mobile Chromium or native launch is claimed.

## Verification boundary

Before production changes, independently author a real rendered `FileDiffToolbar` regression with two explicit session-worktree associations, distinct repository and worktree IDs and paths, the same `src/shared.ts` in both repositories, a legacy primary `worktree_path`, and no `workspace_path`. Use real `FileActionsDropdown`/menu, `useOpenSessionInEditor`, `useRequest`, `StateProvider` and store; stub only the HTTP-domain boundary and unrelated display dependencies. Click the actual VS Code menu item and assert `file_path` plus secondary `worktree_id`. Run the primary control in the same fixture and prove that an omitted ID selects the known first worktree under the existing backend default (accept either omitted or the explicit primary ID after the fix), while the secondary assertion fails for the intended missing ID; record actual collection, exit and failure before implementing. The control checks the boundary payload against the documented backend default; it does not execute the backend. Do not copy qualification fixture material.

After the fix test both Review surfaces and real editor toolbar consumers at that boundary, plus resolver cases: an explicit file-specific ID without a name, ID with an unresolvable redundant name, independently proven ID/name contradiction, name-only placement, duplicate or missing placement, unknown names, missing session, multiple unscoped associations, single worktree, legacy primary and repository checkout controls. The legacy single-worktree follow-up additionally requires a real CodeMirror overflow regression and negative controls described in its work order. Include external editor vs embedded/folder/copy controls and same-path collisions. Existing reviewed suites and new browser-locales suites must be selected by their actual project mapping; verify nonzero collection. These checks establish routing to the HTTP boundary, not backend process launch or native rendering.
