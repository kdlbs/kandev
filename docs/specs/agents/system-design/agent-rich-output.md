---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
---

# Agent rich-output system design

## Purpose and boundaries

This design defines how Agents preserve native rich-output tool identity and
arguments across ACP dialects so a completed `show_rich_output_kandev` call
replays as one standalone presentation. It covers Cursor and Grok provider
envelopes, provider-neutral persistence, and historic title compatibility.
It also defines the local lifetime of workspace file previews in the shared
rich-output renderer.

The
[UI MCP tool-results design](../../ui/system-design/kandev-mcp-tool-results.md)
owns shared transcript result selection and renderer dispatch. This design does
not change the version 1 rich-output schema, chart rendering, or settings.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-AGENT-RICH-OUTPUT-001` | [ACP identity normalization](#acp-identity-normalization), [Historic replay](#historic-replay), [Failure behavior](#failure-behavior), [File-preview lifetime](#file-preview-lifetime) |

## ACP identity normalization

Cursor and Grok may transport MCP calls in an ACP `other` frame whose
`rawInput` contains `providerIdentifier`, `toolName`, and `args`. Their ACP
dialects recognize only a complete envelope: non-empty provider, non-empty
tool name, and object-valued arguments.

Recognized envelopes persist as the existing generic MCP payload:

- name: `<provider>/<tool>`
- input: the unwrapped argument object
- output: the completed MCP result when present

Provider identity stays in the stored name so a foreign tool whose name
resembles a Kandev tool cannot select a native Kandev renderer. Ordinary
`other` frames and incomplete envelopes remain generic activity.

Codex continues to use its existing `_meta.is_mcp_tool_call` plus
`{server, tool, arguments}` recognition. Claude and bare
`*_kandev` titles keep their existing paths.

## Historic replay

Messages already persisted with title `kandev: <tool>_kandev`,
`generic.name: "other"`, and arguments under `raw_input.args` are not migrated.
The shared frontend stem and argument helpers accept that title prefix only
when the remainder is a valid `*_kandev` tool name, and unwrap `args` after
Codex `arguments`. Replay uses stored metadata; the agent does not need to
call the tool again.

## Failure behavior

- Incomplete Cursor-style envelopes stay generic activity.
- Foreign providers keep their `<provider>/<tool>` identity and never select
  Kandev-native renderers.
- Unregistered Kandev tools and unrelated titles such as `kandev: Edit` or
  `mcp__github__...` keep the generic path.
- Malformed rich-output arguments keep the existing unavailable presentation
  fallback after identity matching succeeds.

## File-preview lifetime

`RichOutputRenderer` validates arguments with `parseRichOutput` and retains its
existing outer block identity by type and position. In its `RichOutputBlockView`
file branch, the actual `FilePreviewBlock` instance has a key derived from the
ordered tuple of `sessionId`, `block.repo`, and `block.path`. Serialize the tuple
without delimiter ambiguity; presentation metadata is not target identity.
This boundary implements AC-AGENTS-AGENT-RICH-OUTPUT-001.9 through .12.

Replacing any tuple member unmounts the previous file instance and creates an
idle, collapsed instance. `FilePreviewBlock` owns disclosure and delegates
content state to `useWorkspaceFilePreview`. Only its existing expansion handler
calls `load`; neither mounting nor replacing a descriptor reads a file.
The hook's existing generation cleanup invalidates pending settlements on
unmount. Old success or failure cannot populate the new instance. No transport
cancellation or shared cache is required.

An unchanged tuple retains the instance through reparsing, new argument objects,
title/caption updates, and unrelated rerenders. Successful previews remain
cached locally; failed previews retain the existing explicit retry flow.
Different outer block positions own separate instances even for identical
tuples. Chart and metric identities, parsed-payload memoization, workspace
authorization, file-open callbacks, and persistence keep their current paths.

The shared file card keeps its existing responsive header and controls; the
correction changes state lifetime only. Its current `min-[420px]` composition,
preview scroll region, localized labels, and desktop/mobile file-viewer routing
remain the rendering boundary. The narrow state/data exception in
[`mobile-parity` line 118](../../../../.agents/skills/mobile-parity/SKILL.md#mobile-e2e-expectations)
is satisfied by rendered component tests at the full consumer boundary.

Regression coverage renders the real `RichOutputRenderer`, parser, file card,
and hook with only WebSocket client acquisition and file-content transport
mocked. It replaces descriptors at the same block position, checks disclosure
and read arguments, settles deferred old promises, and exercises same-target
and separate-card controls. The original renderer memoization test remains
separate because it mocks parsing for its own contract.

Implementation record: [File-preview target reset](../../../plans/rich-file-preview-targets/plan.md).
