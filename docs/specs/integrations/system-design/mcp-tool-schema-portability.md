---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001
created: 2026-09-16
owners:
  - kandev
---

# MCP tool schema portability system design

## Purpose and boundaries

The integration system owns the MCP tool contract that Kandev advertises to a
connected agent. This design keeps every advertised tool schema within a subset
that a downstream model provider's function-calling validator accepts, and moves
constraints the subset cannot express into the tool handlers. It does not change
tool availability by mode, transport, authorization, or backend action payloads.

The constraint originates below MCP. The `initialize` handshake negotiates the
transport protocol version and capability sets between the MCP client and server;
it carries no signal about the JSON Schema dialect the client's model provider
accepts. Kandev therefore targets a portable lowest-common-denominator subset
rather than branching schemas on the negotiated protocol version.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001` (.1, .2) | [Portability rule and enforcement seam](#portability-rule-and-enforcement-seam) |
| `REQ-INTEGRATIONS-MCP-TOOL-SCHEMA-PORTABILITY-001` (.3, .4) | [Change-request schema rewrite](#change-request-schema-rewrite) |

Paths in this design are relative to `apps/backend/`.

## Components and responsibilities

- `internal/mcp/toolschema/schema.go`: `Compile` is the shared schema compiler
  used for plugin install-time validation and plugin invocation. It owns the
  root-combinator rejection so plugins and built-ins share one rule.
- `internal/mcp/server/tool_argument_validation.go`: `compileToolArgumentSchema`
  and `rebuildToolArgumentValidators` compile one validator per registered
  built-in tool. A rejected schema records a compile error, and
  `validateToolArguments` already fails a call whose validator carries an error.
- `internal/mcp/server/server.go`: `validatePluginToolSnapshot` rejects a plugin
  snapshot whose tool schema fails `Compile`, before the snapshot is advertised.
- `internal/mcp/handlers/task_change_request.go` and the change-request handlers
  in `internal/mcp/server`: own the operation, target, and prompt-exclusivity
  checks that move out of the two rewritten schemas.

## Portability rule and enforcement seam

The portable subset forbids `oneOf`, `allOf`, and `anyOf` at the root of a tool
schema object. Nested use below a property (for example under `blocks.items`, as
`show_rich_output_kandev` already does) is unaffected, because the failure is
specific to the top level of a function definition.

Enforcement is centralized so it covers both tool families and cannot be bypassed
per registration site:

- `toolschema.Compile` rejects a document whose root declares any of the three
  keywords, in addition to its existing empty-document and object-root checks.
  This makes `validatePluginToolSnapshot` reject a violating plugin tool before
  it is advertised, and it fails closed for plugin invocation compilation.
- `compileToolArgumentSchema` applies the same root-combinator rejection before
  it compiles a built-in tool's validator, so `rebuildToolArgumentValidators`
  records a compile error for a violating built-in and
  `validateToolArguments` returns `registered schema is invalid` for a call.

`TestAllRegisteredToolSchemasCompile` already iterates every registered tool in
each mode and asserts a compiling validator. With the rule in the compiler, that
test fails when any built-in schema reintroduces a root combinator, so the
regression is caught before release.

## Change-request schema rewrite

`taskChangeRequestToolSchema` and `taskChangeRequestAutomationToolSchema` keep
their portable parts: declared properties, `additionalProperties: false`, enum
values, string `minLength`, integer `minimum`, array `minItems`/`uniqueItems`,
object `minProperties`, and the always-required root properties. They drop the
root `oneOf` (operation branches) and the root `allOf` (association-scope prompt
exclusion) and the target-object `oneOf` (association-vs-task exclusivity).

The dropped constraints move to the handlers, which already validate arguments
and already own the backend-agreement contract:

- `manage_task_change_request_kandev`: reject `old_provider`,
  `old_repository_id`, or `old_number` on `link`/`unlink`; require all three on
  `replace`.
- `update_task_change_request_automation_kandev`: for an association target
  require `provider`, `repository_id`, and `number` and reject `providers`; for a
  task target require a nonempty unique `providers` array and reject association
  identity; reject `auto_fix_prompt_override` on an association target.

Each rejection returns a tool error before any backend side effect and names the
violated constraint without echoing argument values.

## Failure and recovery

A tool schema that violates the rule fails closed. A plugin snapshot is rejected
and the previous snapshot stays advertised. A built-in violation surfaces as a
compile error at registration and a failing registration test, and a runtime call
returns a tool error rather than forwarding an invalid definition. A handler-side
constraint violation returns a tool error and performs no mutation.

## Security

No change to trust boundaries. Handler validation continues to run under the
resolved principal, and cross-field checks that move into handlers run before any
authorization-sensitive backend work, preserving fail-before-mutation behavior.

## Observability

A failed built-in schema compilation logs the tool name and error through the
existing `rebuildToolArgumentValidators` error log. No new metric family is
required.

## Related decisions

- [Portable MCP tool schemas](../../../decisions/2026-09-16-portable-mcp-tool-schemas.md).
- [Provider-neutral change request MCP](../../../decisions/2026-09-14-provider-neutral-change-request-mcp.md).
- [Validate MCP tool arguments](../../../decisions/2026-08-01-validate-mcp-tool-arguments.md).
