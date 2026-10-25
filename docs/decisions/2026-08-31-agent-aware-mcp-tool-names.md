# ADR-2026-08-31-agent-aware-mcp-tool-names: Preserve Canonical MCP Tool Names Through Agent Namespacing

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, agentctl, protocol

## Context

Kandev registers built-in MCP tools with a durable `_kandev` suffix and injects
the server as `kandev`. Most supported clients present the registered name
unchanged. Auggie instead presents every tool as
`<registered-name>_<server-name>`, so the model sees
`ask_user_question_kandev_kandev` while the Kandev system prompt instructs it to
call `ask_user_question_kandev`.

Removing the registered suffix would break clients that do not add a server
namespace. Changing every prompt to the doubled form makes the Kandev
contract depend on one external client. The external Auggie implementation is
outside this repository.

## Decision

1. Keep canonical built-in registrations and prompt references in the
   `*_kandev` form.
2. Keep the injected MCP server name `kandev`.
3. Declare client-side server namespacing as an explicit agent runtime
   capability. Auggie opts in. The default is off.
4. For an opted-in per-instance Kandev MCP endpoint, expose built-in canonical
   tools without one trailing `_kandev` on `tools/list`. Reverse that mapping on
   `tools/call` before registry lookup, validation, logging, and handler
   dispatch.
5. Resolve aliases against the live canonical registry. An exact registered
   name wins, and unknown names retain the normal tool-not-found behavior.
6. Keep system prompts agent-neutral because every model continues to see the
   canonical name after the client applies its presentation behavior.

## Consequences

- Auggie models see and call the same canonical names documented by Kandev,
  without a doubled suffix.
- Cursor ACP, Codex ACP, Claude ACP, and other non-namespacing clients keep their
  existing tool lists and calls.
- The MCP server adds a small presentation and reverse-routing layer, while its
  canonical registry and handlers remain unchanged.
- Every executor must preserve the runtime capability through agentctl instance
  creation. Tests must pin both the producer and the MCP-server consumer.
- Plugin-contributed names that do not end in `_kandev` remain governed by
  their separate naming contract.

## Alternatives Considered

### Remove `_kandev` from every registered tool

Rejected because non-namespacing clients rely on the suffix for
disambiguation. Existing prompts, tests, and external integrations also use the
canonical names.

### Teach prompts to use doubled names for Auggie

Rejected because handlers remain canonical. Each tool reference then needs an
agent-specific prompt variant. The presentation detail of the external client
also becomes the public Kandev contract.

### Rename the injected server

Rejected because any non-empty server name still changes the name that Auggie
presents. The change also affects server identity, deduplication, attachment
evidence, and user configuration.

### Branch on `AgentType == "auggie"` inside agentctl

Rejected because tool presentation is a client capability that another agent
can share. The agent definition is the durable source. Transport and MCP
packages consume the declared behavior without naming a provider.

### Change the Auggie CLI

Rejected because it is an external dependency and Kandev must remain compatible
with released clients.
