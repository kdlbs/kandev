# ADR-2026-09-17-restricted-assistant-broker: Restrict the assistant to a native broker

**Status:** accepted
**Date:** 2026-09-17
**Area:** backend, protocol, security

## Context

An MCP inventory cannot constrain a provider's own shell or filesystem tools.
The installed Codex ACP 1.10.0 implementation maps its `read-only` mode to a
workspace-write sandbox with on-request approval. A mode label or advisory
plugin annotation therefore cannot establish inspection authority.

## Decision

A private assistant selects an owner-controlled mode, defaulting to `inspect`.
Mode changes increment the binding version. Runs capture the binding, intent,
selected profile/executor and configuration fingerprint. Every broker call
rechecks these identities under the recorded owner's native workspace access;
task effects recheck again immediately before dispatch. Discovery is advisory.

The initial supported central assistant uses managed Claude ACP 0.75.1, whose
package pins Claude Agent SDK 0.3.257, on a local, repository-free executor.
Reject custom launcher commands, CLI flags, profile environment overrides,
unsupported versions, passthrough, automatic fallback and executor setup/cleanup
scripts. Other provider/executor paths report unsupported before process launch.
Ordinary coordinator and worker profiles retain their existing contracts.

Use a typed `assistant-broker-v1` MCP surface. It receives only the managed
`agentctl kandev assistant-mcp` stdio server, with named native operations.
The private runtime JWT uses capabilities `assistant_broker` and audience
`kandev:assistant-broker`; it never inherits a coordinator's broader access.
The general native MCP surface is empty, including plugins with read-only hints.
No profile or ambient MCP server is attached to this provider session.

Claude session creation and loading both set `tools: []`, `settingSources: []`,
`plugins: []`, `strictMcpConfig: true`, disabled hooks and file checkpointing, and
only the broker's permission allowlist. The CLI receives `--permission-mode
dontAsk` and `--disable-slash-commands`. ACP host file/terminal methods and
permission escalation fail closed. Provider mode/config changes cannot loosen
this policy. Managed command validation and protocol version detection also
apply on restart. Existing unrestricted runtimes cannot be promoted in place.
Native resume and steering require the current run and retained session policy.

Inspection may read authorized records and write conversation/objective/audit
receipts. It cannot mutate delivery tasks, repositories, external services,
credential descriptors, memory, or plugin settings. Design/execute permits
native task management under native workflow/context/approval gates; the central
assistant itself still has no shell. An internal reply targets only its own
conversation. Workers use separately selected native execution profiles.
Operations progress through prepared, dispatched and acknowledged/failed states.
Ambiguous delivery and interrupted receipts become unknown and cannot be retried
as a fresh effect under the same durable identity. Unknown outcomes also block
a new operation ID at the same native endpoint under that intent; a transport
retry cannot bypass uncertainty by minting another ID.

The qualified options are documented by the official
[Claude Agent SDK TypeScript reference](https://code.claude.com/docs/en/agent-sdk/typescript).
The exact bridge option forwarding and version are checked against the pinned
[Claude ACP source](https://github.com/agentclientprotocol/claude-agent-acp/tree/v0.75.1).
The pinned SDK binary's help verifies the extra CLI switches. A provider
upgrade requires renewed negative tests; it does not inherit this qualification.

## Consequences

The assistant has one deliberately narrow provider path. A profile can remain
usable for ordinary work while being incompatible with the assistant. Owners
see the incompatibility; Kandev never silently selects a different account.
The broker may gain new named operations only with effect classification and
native authorization tests. External MCP and plugin effects remain unknown.
Cross-workspace grants later extend this broker's explicit resource intersection,
without widening workspace-coordinator tokens.

This is an enforced tool boundary, not an operating-system container guarantee
against a malicious installed provider executable. Provider credential storage,
SDK caches and conversation persistence remain native runtime bookkeeping.
Provider availability and answer quality are qualified separately in task 11.

## Provider launch qualification correction, 2026-09-18

Task 11's real subprocess trial showed that ACP 0.75.1 unconditionally enables
the SDK's `allowDangerouslySkipPermissions` option for a non-root process. This
overrides the caller's SDK option and is incompatible with the CLI's
`--restricted` switch, even when the selected permission mode is `default`.
The earlier option-forwarding check did not establish successful launch.

The broker boundary therefore uses the independently enforced empty built-in
tool set and strict managed MCP attachment, with deny-by-default `dontAsk` mode.
The mode is supplied through `extraArgs` because ACP also overwrites the SDK's
top-level `permissionMode`. There is no shell, file, web, external MCP, plugin,
skill or hook tool to authorize outside the broker. Every broker operation still
requires current native authority, and ACP host operations and mode/config
changes remain denied. This is not permission-bypass mode; the upstream option
only makes that mode available, and Kandev refuses attempts to select it.

Regression tests cover the options on new/load, sole managed attachment on
new/load/reset, and denied host operations/escalation. The real trial must also
show a successful scoped tool read, a denied broker write, absent host writes
and unchanged ordinary task/repository/workflow snapshots. The
[acceptance receipt](../review/orchestration/assistant-evidence.md) distinguishes
those executed checks from static adapter assertions.

## Alternatives Considered

- Prompt-only inspection: cannot prevent a shell or tool from mutating state.
- Native MCP filtering alone: leaves provider-native tool paths available.
- Codex ACP mode selection: the pinned implementation does not enforce the label.
- A bespoke second agent runtime: would duplicate lifecycle, recovery and ownership.
- Broad plugin read-only hints: metadata does not prove host or remote effects.
