---
status: current
system: ui
requirements:
  - REQ-UI-KANDEV-MCP-TOOL-RESULTS-001
---

# Kandev MCP Tool Results System Design

## Purpose and boundaries

This design defines how the web transcript selects a Kandev MCP result for
native rendering. It does not change tool execution, transport, or storage.

The ACP adapter removes provider wrappers and keeps the standard MCP
`CallToolResult`. The UI then selects one usable payload from that result.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-KANDEV-MCP-TOOL-RESULTS-001` | [Result selection](#result-selection), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- `normalizeCodexMCPToolResult` removes the Codex `{error, result}` wrapper.
  It keeps the complete standard MCP result.
- Persisted tool-message metadata stores that result at
  `metadata.normalized.generic.output`.
- `KandevToolMessage` sends the stored output to `extractMcpResult`.
- `extractMcpResult` selects and parses the payload for all native Kandev tool
  renderers.
- Each renderer reads its fields and shows the native transcript row.

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

1. The agent completes a Kandev MCP tool call.
2. The ACP adapter stores the standard result in normalized message metadata.
3. The transcript passes the result to the shared parser.
4. The parser applies the result-selection precedence.
5. The registered renderer shows the parsed values.

Live updates and replay use the same stored message shape and parser.

## Failure and recovery

Malformed JSON text remains plain text. A result with no usable payload keeps
the existing renderer fallback and does not break adjacent transcript rows.

The repair must preserve non-null structured results and historic wrapper
support. These paths protect rich-output results and older persisted messages.

## Persistence

The repair adds no table or migration. Existing message metadata remains the
only persisted source.

## Responsive behavior

Desktop and mobile chat use the same parser and renderer registry. This repair
changes data selection only. It does not change composition or interaction.

Existing desktop and mobile MCP transcript tests remain the nearest surface
examples. Focused parser and renderer tests reproduce the provider envelope.

## Related decisions

- [Keep Agent Rich Output Host Native](../../../decisions/2026-08-14-kandev-native-agent-rich-output.md)
