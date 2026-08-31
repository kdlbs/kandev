# ADR-2026-08-31: Generic plugin Host boundary

**Status:** proposed
**Date:** 2026-08-31
**Area:** backend, frontend, protocol, security, workflow

## Context

The Coordinator product is moving into a separately released plugin. Its policy,
scheduling, durable state, reconciliation, prompts, reports, tools, and product UI
belong to that plugin. The plugin still needs a supported way to observe and change
Kandev state.

The current Host contract provides plugin state, secrets, events, and a growing set
of data calls. Its manifest declarations are a package-authored ceiling. They do not
provide a workspace approval record, a revocation history, or a generic audit
identity.

Earlier migration work showed the boundary risk. Host PR [#2793](https://github.com/kdlbs/kandev/pull/2793)
combined generic conversation and relation work with Coordinator principals,
grants, storage, and UI. The Coordinator plugin also used dispatch bookkeeping
instead of a durable intent and outbox record. These changes are migration evidence,
not contracts to extend.

This decision defines the boundary that later Host work must follow. It does not
reserve every RPC needed by one plugin, and it does not implement the boundary.

## Decision

### Ownership and sanctioned call chain

Coordinator identity, policy, scheduling, durable plugin state, reconciliation,
prompts, reports, namespaced agent tools, and product UI remain plugin-owned.

Kandev owns generic Host contracts, shared domain commands and queries, capability
authorization, audit and event infrastructure, and generic UI and configuration
primitives.

The production call chain is:

```text
plugin-managed agent
  -> plugin-owned agent tool
  -> plugin backend
  -> capability-gated Host contract
  -> shared Kandev domain command or query
  -> repository and event infrastructure
```

An ordinary task agent can reach the same domain command or query through the global
MCP adapter. The Host and MCP adapters can use different request DTOs. Neither
adapter owns workflow, WIP, authorization, ownership, idempotency, lifecycle,
relation, transition, or audit rules.

A plugin must not use ambient Kandev MCP, private REST, direct database access, or
shell commands as a Host substitute. Prompt or comment text never grants authority.
Kandev core must not gain a Coordinator table, field, profile, role, principal,
grant, setting, tool, or audit vocabulary.

### Generic Host contract

Host methods describe platform concepts such as tasks, sessions, messages,
interactions, relations, transitions, and execution. They do not describe a product
workflow such as coordination, scheduling policy, review policy, or reports.

Each capability family gets a focused design when a real plugin use case proves the
need. A capability-family design defines its DTOs, visibility, domain owner,
preconditions, result vocabulary, audit data, and event behavior.

The following families are boundaries for future designs. They are not a frozen
list of RPC names:

| Capability family | Generic responsibility |
| --- | --- |
| State and configuration | Store plugin-owned state and read plugin-owned operator configuration. |
| Read projections | Read workspace, workflow, task, session, message, and interaction projections. |
| Communications | Read sanitized communication data and issue or resolve typed task directives. |
| Task commands | Create, update, move, label, and relate tasks through domain services. |
| Execution lifecycle | Establish and recover task-run and session generations. |
| Transition commands | Read and cancel pending transitions when the domain contract supports it. |
| Provider evidence | Read provider-neutral change evidence after a separate design proves stable identities. |

Provider actions such as merge, deploy, force-push, draft-to-ready, rerun, and
reviewer notification are not part of this decision. They need provider-neutral
target identity, exact current-state checks, idempotency, rate-limit behavior, audit,
and authoritative readback before Kandev reserves a Host writer.

The Host maps internal models to versioned DTOs. It routes reads and writes through
the owning domain service. A plugin never receives internal structs or repository
access.

### Authorization and installation identity

The Host creates an opaque `installation_id` when it installs a plugin. The plugin
connection supplies that identity. Plugin requests cannot supply or replace it.

The manifest remains the package-authored maximum capability set. For a capability
that needs workspace approval, the effective authority is:

```text
manifest declaration
  intersected with current installation/workspace approval
  intersected with immutable Human-reserved policy
```

Kandev performs this intersection on the Host side. A missing, revoked, or stale
approval fails closed. A capability change fences outstanding directives or
execution tokens when that capability family uses them.

The Host can expose a connection-bound capability context so a plugin can discover
approved workspaces and supported capability families. The context is a read-only
description. It is not an authority token. The Host remains the authority for each
request.

Do not require every request to echo an approval revision. A request carries a
revision only when that operation needs an explicit approval-generation precondition.
The Host reads current approval for ordinary calls.

### Legacy v1 compatibility

Existing v1 state, secret, event, configuration, read, and writer methods keep their
wire shape during migration. This protects already-installed plugins.

Compatibility must not create a new approval bypass. The Host records a compatibility
grant for each existing installation that still uses a v1 writer. A new installation
cannot use a v1 writer as a substitute for a workspace-approved capability-family
contract. The Host rejects that call unless the installation has an explicit
compatibility grant.

The grant is server-side. The plugin cannot request a grant through a Host call, and
the grant cannot widen the manifest or immutable Human policy. Migration work must
define how existing grants expire or convert to the new capability-family grants.

`GetConfig(GetConfigRequest {})` remains the plugin-global v1 configuration call. A
future scoped configuration contract uses a new versioned method and cannot change
the meaning of this call.

Do not add a `*Exact` twin for every v1 method. Use a versioned extension service or
capability-family interface when a new contract needs stronger preconditions. Keep
the old method only for compatibility until its migration path is complete.

### Domain safety and result behavior

Every mutating capability family defines the preconditions that matter to its own
domain command. The domain service revalidates those preconditions atomically with
the side effect and returns a typed conflict without a partial mutation.

For example, a command that can affect a pending task transition checks the current
transition predicate in the same critical section as the command. A read followed by
a later write is not a concurrency guarantee.

Mutating Host calls use an idempotency identity when a retry can repeat a side effect.
They return authoritative readback and an audit identity. The result vocabulary
includes applied, already applied, no change, conflict, denied, not found, invalid,
unsupported, rate limited, and unavailable. A family can add a result only when it
defines durable progress and readback for that result.

Agent-originated writes can carry server-minted execution provenance. The provenance
identifies the managed conversation, session, and execution generation. It never
creates or widens plugin authority. The Host validates it in the same critical
section as the domain write.

Events are at-least-once hints. They include an immutable resource identity and a
resource version when the source has one. A plugin repairs a dropped, delayed, or
duplicated event with an authoritative Host read. Event text never grants authority.

### Plugin agent tools

Plugin agent tools stay in the installed manifest under the existing `agent_tools`
surface. Kandev exposes them through the existing MCP server and invokes them through
the supervised plugin process. A plugin does not need a second MCP server.

The backend owns the active catalog, namespace, applicability, and invocation
authorization. Tool discovery is not authorization. Kandev revalidates plugin state,
the declaration, the task or session surface, and the input schema for every call.

Plugin tools do not add a Host capability by themselves. The plugin uses Host
capabilities for its data, state, secrets, and domain commands.

### Versioning and follow-up decisions

The existing `kandev.plugin.v1` wire contract evolves additively. Removing a field,
changing its meaning, or reusing an enum value requires a new protocol version.
Unknown result values map to `UNSUPPORTED` and never fall through to success.

Follow-up decisions are required for:

1. installation and workspace approval storage, revocation, upgrade, rollback, and
   reinstall behavior;
2. the first concrete read and write capability family used by a released plugin;
3. execution-generation fencing when a concrete lifecycle use case proves its need;
4. provider evidence or cleanup projections that remain generic after review.

Those decisions belong in focused ADRs and implementation plans. They must not reserve
Coordinator-specific tables, tools, roles, or workflow policy in Kandev core.

## Consequences

- The Coordinator remains independently releasable.
- Kandev gains reusable Host boundaries without exposing a Coordinator API.
- Agent-facing tool growth stays inside the plugin namespace and the existing MCP
  catalog.
- Existing plugins keep a migration path, while new plugins cannot use legacy writers
  to avoid workspace approval.
- Capability-family designs require more deliberate work, but they prevent premature
  wire commitments and duplicate authorization logic.
- Kandev and plugin code must implement authoritative readback, audit, and domain
  preconditions for each mutating family.

## Alternatives considered

### Continue the monolithic Host migration

Rejected because it couples Coordinator identity, grants, scheduling, and UI to
Kandev core and freezes product-specific concepts in the Host contract.

### Let the plugin call global MCP or private REST

Rejected because it creates ambient authority, couples the plugin to network topology,
and bypasses installation identity and domain-service ownership.

### Treat the manifest as Human approval

Rejected because a package authors its manifest and an upgrade can widen it without a
workspace approval record or revocation history.

### Freeze every future RPC in one ADR

Rejected because an unimplemented inventory cannot prove the correct DTO, ownership,
or concurrency contract. Focused capability-family designs provide that evidence.
