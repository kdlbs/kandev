---
status: current
system: ui
created: 2026-11-17
requirements:
  - REQ-UI-MERMAID-RENDERING-002
owners:
  - kandev
---

# Mermaid Markdown boundaries

## Purpose and boundaries

UI owns the reusable presentation of Mermaid diagrams embedded in Markdown.
[Requirement 002](../requirements/mermaid-rendering.md#req-ui-mermaid-rendering-002-markdown-diagram-boundaries) defines the recovered diagram and prose.
[Platform segmentation](../../platform/requirements/durable-agent-text-segmentation.md) continues to own ID-less chunk concatenation.
The [fix package](../../../plans/cursor-mermaid-fence-recovery/plan.md) records incident evidence and delivery checks.

The correction stays in three existing frontend files.
It adds a small source-selection helper, extends the current render path, and uses the shared remark pipeline to preserve document reference definitions for recovered prose.
No fence-position metadata, recovery component, cache, setting, dependency, or backend change is needed.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-MERMAID-RENDERING-002` | [Source selection](#source-selection), [Markdown composition](#markdown-composition), [Existing render lifecycle](#existing-render-lifecycle), [Persistence and failure behavior](#persistence-and-failure-behavior) |

## Existing path

`MemoizedMarkdown` normalizes message text and passes it to `ReactMarkdown`.
`MarkdownCode` routes diagram code to `MermaidBlock`.
`useMermaidRender` already owns the lazy Mermaid loader, theme configuration, 300-millisecond debounce, cancellation flag, and error handling.

The glued line is not a valid Markdown closing fence.
Markdown consequently includes both that line and following prose in the code node.
The [CommonMark fence rules](https://spec.commonmark.org/0.31.2/#fenced-code-blocks) explain that parse.

## Source selection

Add a small helper in `components/shared/mermaid-utils.ts`.
It receives the code text and the configured Mermaid syntax validator.
It returns the selected diagram source and optional following Markdown.

1. Find lines with zero to three spaces, three or more backticks, and directly attached non-whitespace text.
2. Without a candidate, return the original diagram source.
3. Parse the complete source with `mermaid.parse` and `{ suppressErrors: true }`.
4. If the complete source is valid, return it unchanged.
5. Otherwise, parse candidate prefixes in source order.
6. Select the first valid prefix as the diagram and preserve the text after that candidate's backticks as following Markdown.
7. If no prefix is valid or validation unexpectedly rejects, return the original source.

Apply `sanitizeMermaidCode` before each syntax probe, using the same configuration as rendering.
Probe the full source once and candidate prefixes only until one validates.
Do not reorder candidates or interpret prose by language, punctuation, or timing.
Preserve prefix and suffix characters apart from the interpreted backtick delimiter and its leading indentation.
The extractor handles LF, CRLF, and lone CR.

The complete-source check protects valid diagrams containing apparent fence text.
When a glued closing run makes the full source invalid, prefix validation skips apparent candidates inside unfinished labels and can reach the true closing run.

Recovery works on a parsed code node. Its original outer fence length, closing state, and parent container are not needed.
An invalid Mermaid block can recover the same malformed source whether its outer Markdown fence is closed or unclosed.
Valid diagrams and literal wrappers remain protected by syntax validation and explicit language routing.

## Markdown composition

Add a small `MarkdownPre` override to `markdown-components.tsx`.
For an explicit `language-mermaid` code child, render `MermaidBlock` directly at block level.
Pass the owning task ID and a callback that renders following text through the existing Markdown plugins and component overrides.
The shared remark pipeline attaches the original source for Markdown reference definitions to explicit Mermaid code nodes.
The callback prepends those definitions while parsing recovered prose, so reference links keep the owning document's scope.
Other languages and inline code keep their existing routes. Untagged diagram detection remains unchanged and does not opt into recovery.

Extend `MermaidBlock` with that optional callback.
Publish SVG and recovered text together after source selection and rendering succeed.
Render following Markdown as a sibling after the diagram, outside `pre` and the diagram container.
Use the selected diagram source for the diagram's code view.

The callback inherits `MarkdownTaskContext` and `MarkdownFileLinkContext`.
Do not import the parent renderer into `MermaidBlock` and create a module cycle.
Keep the current plugins, HTML policy, and URL/file-link restrictions.

The existing diagram viewport and chat transcript remain the desktop and phone composition.
The transcript owns vertical scrolling. Diagram controls, scaling, navigation, and touch behavior stay as shipped.

## Existing render lifecycle

Perform source selection inside the existing debounced `useMermaidRender` operation.
Reuse its lazy loader and cancellation handling.
Do not create a second timer, recovery hook, or independently owned async state.

Retain the current SVG behavior while a newer render is pending.
Associate recovered prose with its source revision and task so an earlier suffix is never shown for a newer input.
The existing cancelled-operation check guards publication after replacement or unmount.
Suppress syntax-probe errors. Only the normal render path can report a final error.

## Persistence and failure behavior

Stored messages, transport chunks, message IDs, and delivery cursors remain unchanged.
Reload applies the same rendering rule to the same stored message.

When recovery cannot prove the split, send the original source through the existing error path.
Preserve current diagram diagnostics and task-scoped toast behavior.
No new logging, telemetry, raw HTML support, or backend mutation accompanies recovery.

## Verification

Use the real installed parser to prove the glued case, valid literal labels, and invalid-prefix fallback.
Use focused component tests for sibling placement, current link/task context, quiet recovery, and stale-result rejection.
Extend the existing desktop and phone Mermaid-fit specs with the recovery and reload case.
Their ordinary diagram cases continue to protect viewport behavior.

## Implementation status

The bounded source-selection helper is integrated in the existing debounced render lifecycle.
Explicit Mermaid blocks render verified following prose through the existing Markdown components.
The static-review regressions for file-preview routing and complete backtick-run matching pass in the focused suite.
PR review follow-up also preserves document reference definitions and shows the latest failed Mermaid input in the code view while retaining the previous SVG.
Additional PR review follow-up checks later candidates after invalid prefixes and initializes Mermaid after validation immediately before rendering.
The earlier desktop and phone checks passed before those code-only corrections and were not rerun.
Stored message content and provider segmentation remain unchanged.
