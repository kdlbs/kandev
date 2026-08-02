# ADR-2026-08-01-validate-mcp-tool-arguments: Validate MCP Tool Arguments at the Shared Server Boundary

**Status:** accepted
**Date:** 2026-08-01
**Area:** backend, protocol

## Context

Kandev registers JSON Schema for every MCP tool, but `mcp-go` v0.43.2 dispatches `tools/call` directly to the handler without validating arguments against that schema. Handlers commonly read known keys with permissive getters, so unknown keys and mistyped optional values can disappear while the tool reports success. Per-handler checks would leave current tools inconsistent and allow every newly registered tool to repeat the same failure mode.

## Decision

All Kandev MCP tool calls are validated in `apps/backend/internal/mcp/server` before their handlers or backend actions run.

- The currently registered tool schema is the source of truth for required fields, value types, arrays, objects, enums, and other declared constraints.
- Validation treats the root argument object as closed even when the advertised schema omits `additionalProperties: false`; unknown top-level arguments fail without increasing every tool schema's context-token cost.
- Nested objects retain their declared schema behavior. Open maps such as executor configuration and MCP server configuration remain open unless their own schema closes them.
- Schemas are compiled once for the active tool set and rebuilt whenever `SetMode` replaces that set. A schema that cannot compile fails closed for calls to that tool and is covered by a registration test.
- Explicit compatibility normalization may run before validation for an established caller mismatch. `create_task_kandev` advertises `prompt` for agent instructions and accepts the former `description` name as an unadvertised alias; supplying both is an error. The handler maps `prompt` to the backend's existing `description` field. Such exceptions require focused tests and do not weaken unknown-argument rejection for other keys.
- Argument failures return MCP tool error results and do not invoke the tool handler or dispatch a backend action.

## Consequences

Invalid calls fail visibly and consistently across task, configuration, external, and Office MCP modes. Future tools inherit the validation boundary automatically when they use the standard registration and `wrapHandler` path. Kandev adds a JSON Schema validation dependency and must keep every registered schema compilable. Closing only the root preserves dynamic nested configuration maps while preventing accidental top-level parameter loss. Using `prompt` for agent instructions aligns create-task with the other agent-starting MCP tools without advertising both names or increasing the tool schema's parameter count; `description` remains appropriate for descriptive metadata in other contracts.

## Alternatives Considered

- **Validate only `create_task_kandev`.** Rejected because the same permissive dispatch behavior applies to every handler and every future tool.
- **Add checks independently inside each handler.** Rejected because duplicated allowlists and type checks drift from registered schemas and are easy to omit on new tools.
- **Only advertise `additionalProperties: false`.** Rejected because the current MCP server does not enforce advertised schemas, and repeating metadata across all tools increases agent context without guaranteeing correctness.
- **Close every nested object recursively.** Rejected because several tool inputs intentionally carry arbitrary configuration maps.
- **Upgrade `mcp-go` as the repair.** Rejected because the current library path does not provide the required dispatch validation, and a broad protocol dependency upgrade is unnecessary for this invariant.
