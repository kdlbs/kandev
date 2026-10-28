---
status: current
system: ui
requirements:
  - REQ-UI-TOOL-FILE-OPEN-001
---

# Tool file reference opening design

## Boundary and mapping

UI owns eligibility and activation for the shared tool-card file reference;
workspace and session ownership remain unchanged. The [requirement](../requirements/tool-file-reference-opening.md)
is distinct from [file-tree path scope](file-tree-path-scope.md), which owns
normalization and tree identity after an action. Markdown links and external
embedded-editor dispatch have separate contracts.

| Criteria | Design sections |
| --- | --- |
| 001.1, 001.3 | Semantics and eligibility |
| 001.2 | Runtime flow |
| 001.4 | Responsive behavior and verification |

Suffixes refer to `AC-UI-TOOL-FILE-OPEN-001`.

## Semantics and eligibility

`streams.ReadFilePayload.FilePath` carries a read target; `CodeSearchPayload`
separates `Path` (search scope) from `Output.Files` (listing entries). ACP's
explicit `raw_input.type=directory` read is normalized as search with the
scope in `Path`; `NormalizeToolResult` parses a newline listing into `Files`.
Those strings carry no per-entry type or verified existence. In particular a
listing can contain unmarked directories or stale/invalid entries. A basename
with a dot is no more reliable as a file test than one without it.

`FilePathButton` has only two production consumers: `ToolReadMessage` and
`ToolSearchMessage` result entries. For these file-reference contexts retain
its blank, `.` and trailing `/` exclusions and its missing-opener inactive
branch, but remove the basename-dot heuristic. Inspect whitespace for eligibility
only; do not alter the path forwarded by this component. The separate search
scope remains plain text. No stat, directory inference, existence guarantee,
new protocol type, or frontend security boundary is introduced. An unmarked
directory may produce the existing file-open failure; do not claim it was
identified as a file. Existing failure behavior is unchanged.

## Runtime flow

`message-renderer.tsx` routes `tool_read` and `tool_search` to their consumers
with the current context. Read uses `splitReadFiles` and `ReadFileLink`'s real
`useOpenFileAtLine`: selector and offset/limit behavior remain as implemented;
that hook normalizes a contained absolute workspace alias and keys pending
cursor state with the session before opening or scrolling. Search results pass
their recorded paths directly to the existing supplied opener. Do not introduce a
new search line, alter session/workspace selection or bypass the existing
file service. The path displayed may be relative; the original reference is
passed to the existing action, subject to its existing normalization.

The read selector has an intentional ambiguity for comma-joined extensionless
names without selectors. This change does not alter that grammar or assert that
such a bundle splits into separate files. It covers individual references and
existing supported selectors. Existing path-scope and filesystem restrictions
continue to apply; this is not a promise of successful reads for arbitrary paths.

## Responsive behavior and verification

The shared row stays inline in the existing chat scroller. On desktop the
existing Dockview file editor is the destination; on phones the dedicated
`MobileFileViewerPanel` is the destination, following the shipped chat-read
example in `mobile-file-viewer.spec.ts` and the task layout's separate phone
composition. No new surface, navigation, copy, scroll owner, breakpoint or sizing
rule is planned. Existing inline controls are retained; demonstrate a real tap
without relying on hover. Compare rendered phone behavior against the plan's
preview and ensure the reference is reachable without clipping.

An independently authored component regression mounts the actual read and
search consumers and real `FilePathButton` and read hook, dispatches clicks,
and asserts the original path at the button boundary, normalized opener path,
positive line/session cursor state, and one invocation per activation. Include
extensionless read and search, dotted and hidden controls, `.` and trailing `/`,
blank, missing opener, and inert search scope. Expand search through its UI.
The prior independent reproduction is evidence of the defect, not a test to copy
or rerun. A failing assertion on the intended open behavior before the fix is
required; collection, setup, or timeout failure is not causal RED.

Desktop and mobile Playwright tests seed actual files and normalized read and
search messages, activate both and assert the correct file content in the real
editor/viewer. Read a sufficiently long extensionless file at a positive line. The phone
viewer must show that line; desktop verifies the exact file tab and distinct
content, while the component test proves the line/session cursor request.
Exercise search expansion before activation. Use separate projects and actual
task/session workspace. These checks establish rendered user flow, not provider
normalization correctness or universal file support.

The initial desktop run opened `Dockerfile` but showed lines 1-34 instead of
the requested line 300. A matched `control.txt` read followed the same path
and showed the same result. The dot gate does not affect that dotted control;
no editor or cursor production code changed. This is an existing desktop cursor
visibility limitation in this fixture, with deeper cause unknown. It is deferred
from this bounded fix and is not claimed fixed. Phone visibly reached line 300.
No new telemetry, persistence or public API changes are needed.
