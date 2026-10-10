---
status: current
system: ui
requirements:
  - REQ-UI-EMBEDDED-EDITOR-TARGET-001
---

# Embedded editor file targets design

## Boundary and mapping

UI owns lossless interpretation of an editor target between the existing Go
editor resolver and embedded-editor dispatch. This design uses, without changing,
workspace path authority, session admission, and the
[executor availability contract](../requirements/embedded-vscode-executor-availability.md).
The [requirement](../requirements/embedded-editor-file-targets.md) defines the
observable handoff; the implementation record is the
[file-target plan](../../../plans/embedded-editor-file-targets/plan.md).

| Criteria | Design sections |
| --- | --- |
| 001.1, 001.2, 001.3 | Private sentinel, Resolution and consumer |
| 001.4, 001.5 | Resolution and consumer, Failures and compatibility |
| 001.6 | Responsive behavior |

All criterion suffixes refer to `AC-UI-EMBEDDED-EDITOR-TARGET-001`.

## Audited runtime path

`internal/editors/handlers.RegisterRoutes` accepts POST
`/api/v1/task-sessions/:id/open-editor`. `Controller.OpenSessionEditor` maps the
existing DTO to `Service.OpenEditor`, then returns the existing `{url}` response.
`OpenEditor` resolves the session, settings/editor, executor availability,
worktree and file path before `dispatchEditorKind` invokes
`buildInternalVscodeURL`. No-file embedded actions take the existing early return.

`apps/web/lib/api/domains/session-api.ts:openSessionInEditor` uses `fetchJson`.
`useOpenSessionInEditor` uses real `useRequest` and toast context; its private
`parseInternalVscodeURL` supplies `openInternalVscode`, which reveals the panel
and invokes `openFileInVscode` unless `isDirectory` is set. That API uses the real
`WebSocketClient.request("vscode.openFile", {session_id, path, line, col})`.

`FileTreeEditorProvider` in `file-tree-editor-menu.tsx` resolves the node's
worktree, passes `filePath`, `worktreeId` and `isDirectory`; file actions in
`file-actions-dropdown.tsx` pass `filePath`; `EditorsMenu` passes no file and
may pass a selected worktree. These callers require no production changes.

## Private sentinel

Keep `internal://vscode` for a workspace-level action. For a file, make `goto`
contain only the resolved path. Encode the query with Go `net/url.Values.Encode`;
carry positive `line` and, only with a positive line, positive `column` as
separate query parameters. Missing coordinates mean zero. These are private
sentinel parameters, not added HTTP DTO or WebSocket fields.

For example, `src/name:part.ts` at `(17,5)` is represented by query values
`goto=src/name:part.ts`, `line=17`, `column=5` before encoding. Go's serialized
form is `internal://vscode?column=5&goto=src%2Fname%3Apart.ts&line=17`.
`notes:2026` without coordinates is `internal://vscode?goto=notes%3A2026`.

The consumer uses `URLSearchParams` on the query and reads the three values.
`goto` is never split on colons. Query decoding happens once through that API;
do not additionally call `decodeURIComponent`. Go's `+` space encoding and
literal `+`/`%` escaping thereby round-trip, including `%3A`, `%25`, Unicode,
spaces, `&`, `?`, `#`, and `=` in supported names. Preserve the current positive
coordinate policy; no general validation framework is introduced.

## Resolution and consumer

Retain `resolveSessionPath`, `findSessionWorktreePath`, and `resolveFilePath`.
An explicit worktree or association ID must belong to the session; omitted
selection retains first-worktree/repository fallback. File paths retain existing
cleaning and rejection behavior. `buildInternalVscodeURL` derives `filepath.Rel`
from the resolved absolute target and selected worktree, as it does today.
The browser consumes that returned path, never raw `options.filePath`.

Preserve bare-sentinel and empty-target behavior. Continue to reveal the panel
using the existing store action, and suppress file dispatch for `isDirectory`.
Coordinate-only no-file input still produces a bare sentinel. Keep the
fire-and-forget open-file call and its existing failure policy.

## Downstream admission and residuals

`VscodeHandlers.wsVscodeOpenFile` rejects empty session/path, obtains or ensures
the execution and its agentctl client, then forwards the three fields.
`internal/agent/runtime/agentctl.Client.VscodeOpenFile` POSTs them to
`/api/v1/vscode/open-file`. The agentctl handler forwards to
`process.Manager.VscodeOpenFile`, which admits startup, auto-starts if needed,
waits for running state and calls `VscodeManager.OpenFile`.

`OpenFile` resolves a relative path against its `workDir`, verifies running
state/binary/Remote CLI, waits for the IPC socket, and constructs
`--goto <absolute-path>[:line[:col]]`. CLI interpretation of numeric-colon names
can remain ambiguous downstream. This work proves the handoff tuple only; it
does not establish the native CLI or rendered selection, nor fix runtime-root
mapping for a selected secondary repository. Do not launch native editors to
extend the accepted evidence by implication.

## Failures and compatibility

Keep handler status mapping and existing HTTP error toast/null result. A failed
open-editor request must not dispatch `vscode.openFile`. WebSocket open-file
errors remain logged and swallowed by the existing API; they do not become a
new hook error/toast policy. Missing session remains a no-request/null action.
External schemes retain `window.open`; empty external responses retain their
existing result without an embedded dispatch.

Producer and consumer ship together. The sentinel is ephemeral and is neither
persisted nor a documented external navigation URL. Do not guess the old
colon-delimited grammar or add dual-writer migration: old numeric-colon targets
are undecidable. A stale web bundle paired with a new backend requires refresh;
mixed-version sentinel compatibility is not guaranteed. No public API,
persistence, transport, authorization, or availability rule changes.

This is a local private serialization correction. The requirement, design and
shared behavioral fixture preserve the necessary rationale; no ADR is needed.

## Responsive behavior

The change is pure target data interpretation in the existing shared hook.
No markup, copy, navigation, touch geometry, focus, scroll owner or breakpoint
changes. Existing file-tree/file-action entry points remain intact, including
the existing absence of the desktop editor topbar on phones. The mobile-parity
pure state/data exception is satisfied by the shared hook integration tests;
no preview or new mobile Playwright scenario is required.

## Regression boundary

Use one shared JSON oracle at
`apps/backend/internal/editors/service/testdata/embedded-editor-file-targets.json`.
It records inputs, expected canonical tuple and private URL. Go tests execute
the real `Service.OpenEditor` and compare its output to the oracle; frontend
tests import that same JSON only as deferred HTTP wire responses and assert
actual real WebSocket request frames. Tests do not copy the producer or parser.

Include colon with `(17,5)`, numeric-colon with no coordinates, ordinary file,
line-only, ignored column without line, spaces, Unicode, percent literals,
literal plus and query punctuation. Use a caller path such as
`src/sub/../name:part.ts` whose returned canonical path differs, selected/default
worktree, invalid selection, repository fallback, bare/root/directory action,
missing session, HTTP failure and external editor controls.

Go service tests use narrow repository/settings fixtures only. Frontend tests
mount the real hook, `useRequest` and `ToastProvider`, real API/`fetchJson`, and
real `WebSocketClient`; stub only `fetch` and WebSocket wire. Assert POST body,
exact `(session_id,path,line,col)`, response/null result and request state.
Restore globals/client/timers and actually drain pending work. Dockview may
remain null: this proves dispatch, not panel rendering. Existing mocked hook
tests must update their response fixtures to the new producer grammar, and
remain part of the focused run.
