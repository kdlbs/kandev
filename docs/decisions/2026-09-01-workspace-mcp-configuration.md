# ADR-2026-09-01-workspace-mcp-configuration: Compose Workspace MCP Definitions Additively

**Status:** accepted
**Date:** 2026-09-01
**Area:** backend, frontend, protocol, security

## Context

Kandev currently stores raw MCP server JSON on an agent profile. This makes a
server hard to reuse, repeats credentials and transport details, and cannot
express repository or task needs. It also makes the profile appear to own a
server whose credentials and access policy belong to a workspace.

Users need a workspace catalog that supports curated setup, public MCP Registry
discovery, and custom definitions. Repositories, agent profiles, tasks, and
individual task agent sessions must be able to select from that catalog.

These scopes need a composition rule. Precedence or subtraction requires
scope-specific overrides, conflict resolution, and a way to explain why a
server disappeared. The requested product behavior is additive: each scope
contributes capabilities.

Runtime changes also need a protocol boundary. ACP v1 accepts an MCP server list
on `session/new`, `session/load`, and `session/resume`. It does not define a
standard method that replaces the MCP list inside an active turn. Provider
capability advertisement can permit reconnection, but it cannot prove that a
provider will accept a changed list.

Global agent profiles create another ownership issue. A global profile can
serve many workspaces. A workspace definition and its secrets cannot become
install-wide with that profile.

## Decision

MCP definitions are workspace-owned reusable resources. Repository, profile,
task, and task-session selections reference stable definition identifiers.

The runtime effective set is the additive union of:

1. selections from every repository attached to the task.
2. the effective agent profile's selection in the task workspace.
3. task selections.
4. task-session selections.

The resolver collapses duplicate definition identifiers and retains every
origin for diagnostics. Different definitions cannot share one runtime name in
the same workspace. Provider and executor transport policy runs after union.
Kandev's internal MCP endpoint remains separately injected under a reserved
name.

Agent-profile selection is workspace contextual. A workspace-scoped profile
uses its workspace directly. A global profile has an independent selection for
each workspace. This keeps definition, credential, and authorization ownership
inside the workspace.

Marketplace installation copies reviewed discovery metadata into a workspace
definition. This action does not download, connect to, or start the server.
Definitions pin their source identity and do not follow registry changes
automatically. Registry metadata is untrusted publisher input.

A Registry entry can provide packages, remote endpoints, or both. A remote
choice needs no local installation. A managed package is materialized lazily
inside each task executor when an effective MCP set first uses it. The
materializer uses an exact version and a Kandev-owned cache. It never changes
repository files or lockfiles.

The first release materializes exact npm packages through Kandev's managed Node
runtime. It also supports compatible remote HTTP and SSE entries. Other package
types remain visible but unavailable until Kandev has a typed materializer.

A custom definition selects one explicit mode: remote endpoint, supported
managed package, or existing executable. Saving a custom definition does not
execute it. Kandev never installs an existing-executable definition.

Every attachment attempt resolves the current definitions and selections. It
records definition revisions and origins in session-owned MCP evidence. Secret
values exist only in the ephemeral delivery value.

An active turn never receives an in-place MCP list mutation. For an idle task
agent session, Kandev saves a desired selection revision and attempts to
reconnect the same provider session with the full effective list. It prefers
advertised ACP `session/resume` and falls back to advertised `session/load`.
Unsupported adapters defer the desired revision to the next start. A failed
reconnect leaves the prior applied revision authoritative and exposes retry or
next-start recovery.

## Consequences

Users can configure one server and reuse it without copying JSON. Each scope
can add capabilities without editing another scope. The same resolver serves
start, restart, resume, reset, and live idle-session changes.

The effective configuration remains explainable because every server has a
stable identity and origin list. Session-owned attachment evidence can
distinguish configured, delivered, and provider-observed states.

Workspace-contextual selections add an extra choice when editing a global
profile. This is intentional. A global selection can leak workspace credentials
or require a second global definition store.

Idle-session changes are best effort across providers. A provider can advertise
resume or load and still reject a changed MCP list. The UI must show desired and
applied state instead of promising immediate mutation.

The `session/load` fallback is not free. Kandev's adapter implementation of that
method also clears pending wakeups, cancels the armed wakeup timer and async
turn completions, resets dialect-specific correlation state, and replays the
conversation under suppression. An idle session with a scheduled wakeup loses it.
The fallback therefore discloses that cost, and `session/resume` is preferred for
that reason as well as for cost. Neither capability may be read from the static
`agents.RuntimeConfig.SupportsSessionResume` registry flag, which nearly every
ACP agent sets and which describes Kandev's own resume behavior rather than the
ACP method.

Binding a workspace secret to an MCP definition exposes it inside the task
executor. Delivery writes it into an agent configuration file or, for Codex,
into process arguments. Kandev's confidentiality guarantee covers its own
storage, APIs, streams, logs, and evidence, not the executor. Setup must say so.

Registry aggregation requires a persisted cache and degraded mode because the
preview service provides no uptime or durability guarantee. Installed
definitions remain independent from that cache.

Workspace installation and executor materialization are separate states. A
definition can exist before any executor downloads its package. Each executor
can have its own materialization result and cache.

Existing raw profile configurations require an idempotent workspace-aware
migration and a temporary fallback. The legacy table is not dropped in the
first delivery.

## Alternatives considered

- **Keep raw JSON on each profile.** Rejected because it prevents reuse,
  repeats secrets, and cannot represent repository, task, or session needs.
- **Use last-writer or most-specific-scope precedence.** Rejected because it
  silently removes capabilities and requires override semantics that are
  outside the first release.
- **Union servers by runtime name.** Rejected because names are mutable. Two
  definitions can hide different executable or credential configurations.
- **Put MCP definitions on global profiles.** Rejected because workspace
  secrets and authorization cannot safely become install-wide profile data.
- **Read the public registry on every search.** Rejected because it couples the
  settings UI to a preview service without uptime guarantees. It also ignores
  the aggregator guidance.
- **Automatically update installed definitions from registry metadata.**
  Rejected because publisher changes can alter executable behavior without
  workspace review.
- **Download every package when it is added to the workspace.** Rejected because
  installation depends on executor platform and runtime capability. Eager
  download also executes work before any task needs the server.
- **Translate every Registry package type into an arbitrary shell command.**
  Rejected because this hides missing runtime support. It also weakens package,
  version, cache, and integrity controls.
- **Restart the agent whenever MCP selections change.** Rejected because it can
  disrupt a conversation and hides provider capability. Unsupported paths must
  be explicit and user controlled.
- **Treat capability advertisement as successful reconfiguration.** Rejected
  because real agents can advertise load or resume while rejecting a specific
  reconnect request.
- **Change the MCP list during an active turn.** Rejected because ACP v1 has no
  standard in-place update method. A mid-turn capability change is
  nondeterministic.
