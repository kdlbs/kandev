---
status: current
system: ui
requirements:
  - REQ-UI-KANDEV-MCP-TOOL-RESULTS-001
---

# MCP tool results

## Purpose and boundaries

This design defines how ACP adapters preserve Kandev MCP tool identity and
arguments, and how the web transcript selects those calls and their results for
native rendering. It does not change tool execution or storage.

The ACP adapter removes recognized provider wrappers and keeps a
provider-neutral tool name, arguments, and standard MCP `CallToolResult`. The UI
then identifies the renderer and selects one usable payload from that result.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-KANDEV-MCP-TOOL-RESULTS-001` | [Result selection](#result-selection), [Responsive behavior](#responsive-behavior) |

The Agents-owned
[native rich-output contract](../../agents/requirements/agent-rich-output.md)
and its
[ACP identity design](../../agents/system-design/agent-rich-output.md)
consume this shared identity, argument, result-selection, and replay path.

## Components and responsibilities

- `normalizeCodexMCPToolResult` removes the Codex `{error, result}` wrapper.
  It keeps the complete standard MCP result.
- Provider dialects recognize their explicit MCP envelopes and persist the
  provider plus actual tool name instead of the broad ACP category.
- Persisted tool-message metadata stores that result at
  `metadata.normalized.generic.output`.
- Persisted generic input stores the unwrapped tool arguments.
- `kandevToolStemOf` identifies registered Kandev renderers from normalized
  identity or a compatible historic title.
- `KandevToolMessage` sends the stored output to `extractMcpResult`.
- `extractMcpResult` selects and parses the payload for all native Kandev tool
  renderers.
- Each renderer reads its fields and shows the native transcript row.

## Identity and argument normalization

Provider dialects normalize only envelopes with explicit MCP identity:

- Codex uses `_meta.is_mcp_tool_call` plus
  `{server, tool, arguments}`.
- Cursor and Grok use `{providerIdentifier, toolName, args}`.

Both become a generic MCP payload named `<provider>/<tool>` whose input is the
unwrapped argument object. The provider identity is retained so a foreign tool
whose name resembles a Kandev tool cannot select a native Kandev renderer.

Historic messages are not migrated. For replay, the frontend also accepts the
observed Cursor title `kandev: <tool>_kandev` and unwraps
`input.raw_input.args`. The prefix establishes Kandev identity only when the
remainder is a valid suffixed tool name. Bare categories, foreign namespaces,
and unrelated tools remain generic activity.

## Result selection

`extractMcpResult` uses this precedence:

1. Use `structuredContent` when its value is not null.
2. Use `structured_content` when its value is not null.
3. Parse text blocks from `content`.
4. Parse a string in `output`.
5. Unwrap a recognized `result` wrapper.
6. Keep a plain object unchanged.

An explicit null structured value means that no structured result exists. It
must not suppress a valid text fallback.

## Control flow

1. The agent emits a Kandev MCP tool call.
2. The ACP dialect stores provider-neutral identity and arguments in normalized
   message metadata.
3. The agent completes the call and the adapter stores the standard result.
4. The transcript selects the registered renderer from normalized or compatible
   historic identity.
5. The parser applies the result-selection precedence.
6. The registered renderer shows the parsed values.

Live updates and replay use the same stored message shape and parser.

## Failure and recovery

Malformed JSON text remains plain text. An unrecognized or foreign tool keeps
the generic activity path. A recognized result with no usable payload keeps the
existing renderer fallback and does not break adjacent transcript rows.

The repair must preserve non-null structured results and historic wrapper
support. These paths protect rich-output results and older persisted messages.

## Persistence

The repair adds no table or migration. Existing message metadata remains the
only persisted source. Corrected adapters normalize new messages at ingestion;
the frontend compatibility path makes already-persisted Cursor envelopes
replayable without rewriting history.

## Responsive behavior

Desktop and mobile chat use the same parser and renderer registry. This repair
changes data selection only. It does not change composition or interaction.

Existing desktop and mobile MCP transcript tests remain the nearest surface
examples. Focused parser and renderer tests reproduce the provider envelope.

## Related decisions

- [Keep Agent Rich Output Host Native](../../../decisions/2026-08-14-kandev-native-agent-rich-output.md)
