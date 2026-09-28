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

The
[UI MCP tool-results design](../../ui/system-design/kandev-mcp-tool-results.md)
owns shared transcript result selection and renderer dispatch. This design does
not change the version 1 rich-output schema, chart rendering, or settings.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-AGENT-RICH-OUTPUT-001` | [ACP identity normalization](#acp-identity-normalization), [Historic replay](#historic-replay), [Failure behavior](#failure-behavior) |

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
