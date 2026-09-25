# ADR-2026-09-16-portable-mcp-tool-schemas: Keep MCP tool schemas within a portable subset

**Status:** accepted
**Date:** 2026-09-16
**Area:** backend, protocol, integrations

## Context

Kandev advertises built-in and plugin MCP tools to the connected agent. The
agent forwards each tool's input schema to its own model provider as a function
definition. Provider function-calling validators are stricter than JSON Schema
Draft 7, and they do not share one dialect.

PR #3671 added `manage_task_change_request_kandev` with a top-level `oneOf` and
`update_task_change_request_automation_kandev` with a top-level `allOf`. Both
compile and validate correctly under Draft 7, so Kandev's own registration and
argument-validation passed. Auggie forwards the tool list to the Augment API on
every `session/prompt`, and that API rejects the whole request:

```
Invalid tool definition: Tool schema does not support oneOf, allOf, or anyOf at the top level
```

The rejection returns `400 Bad Request` and no turn runs. Every Auggie session
in a GitHub- or GitLab-backed task hit this, because the `task-pr-links` group
registers these tools whenever a task has a GitHub or GitLab provider.

The MCP `initialize` handshake negotiates the transport protocol version and
capability sets (tools, resources, prompts) between the MCP client and server.
It does not negotiate the JSON Schema dialect of a tool's input schema, and it
carries no signal about which schema keywords the client's downstream model
provider accepts. The restriction lives one layer below MCP, at the model
provider, which the MCP server cannot observe from the connected client.

## Decision

MCP tool input and output schemas exposed by Kandev stay within a portable JSON
Schema subset that every mainstream model provider accepts. The root object of a
tool schema declares no `oneOf`, `allOf`, or `anyOf` keyword.

Kandev enforces this at the shared schema compile seam so a violating schema
fails closed at registration for both built-in tools
(`compileToolArgumentSchema` / `rebuildToolArgumentValidators`) and plugin tools
(`toolschema.Compile` via `validatePluginToolSnapshot`). A registration test
across every mode fails when a built-in schema violates the rule.

Cross-field and mutually-exclusive argument constraints that the portable subset
cannot express at the root are enforced inside the tool handler before any
backend side effect, so the rejection behavior an agent observes is preserved.

## Consequences

- The two change-request tools drop their top-level `oneOf`/`allOf` and move the
  same operation-, target-, and prompt-exclusivity checks into their handlers.
- New tools and plugins cannot reintroduce the failure: a top-level combinator
  is rejected before the tool is advertised, and the registration test catches a
  built-in regression in CI.
- Handlers own more argument validation. Handler and schema validation must stay
  in agreement, including for direct backend callers.
- The rule is a lowest-common-denominator constraint. A provider that does allow
  top-level combinators gains no expressiveness through Kandev tool schemas.

## Alternatives Considered

- **Serve a different tool schema per MCP protocol version.** The MCP protocol
  version is negotiated with the client transport, not the downstream model
  provider, and does not correlate with the provider's schema restrictions.
  Branching schemas on it would target the wrong layer and still fail for a new
  provider on the same protocol version.
- **Rewrite schemas per detected client or provider.** The MCP server does not
  reliably learn the client's model provider, the set of restrictions differs
  per provider and changes over time, and maintaining per-provider schema
  variants multiplies the surface that must stay in agreement with handlers.
- **Keep the top-level combinators and document the risk.** This leaves the
  contract able to break any provider that tightens function-schema validation,
  with no registration-time signal, which is what shipped in PR #3671.
- **Strip combinators at the ACP/agentctl boundary before forwarding.** Silent
  rewriting would change advertised validation without the handler enforcing the
  dropped constraint, so an invalid argument combination could reach a handler
  that still assumed the schema rejected it.

## Related decisions

- [Provider-neutral change request MCP](2026-09-14-provider-neutral-change-request-mcp.md).
- [Validate MCP tool arguments](2026-08-01-validate-mcp-tool-arguments.md).
