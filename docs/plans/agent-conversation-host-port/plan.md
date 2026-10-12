---
created: 2026-09-14
status: completed
requirements:
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
legacy_specs: []
---

# Implementation Plan: AgentConversation Host Contract Port

## Overview

Port the maintained `agent_conversation` Host contract to canonical Kandev so a
plugin can manage one hidden conversation per plugin, workspace, and conversation
key through the existing typed gRPC Host channel. The source contract is
`yattdev/kandev` commit `612f46f7a4b206c7cd81863efc4202c123901542`; the target
branch is canonical `main` as it stood at merge base `753e5549e`.

## Scope

The port adds `EnsureAgentConversation`, `DispatchAgentConversation`, and
`DeleteAgentConversation` to `kandev.plugin.v1`, Go-native SDK access through
`pluginsdk.AgentConversations(host)`, and capability-gated Host wiring. `Ensure`
creates or repairs a hidden, workflowless ephemeral task/session; `Dispatch`
records durable occurrence keys so retries do not double-send; `Delete` removes
only conversations owned by the calling plugin. Installation, disable, and
uninstall paths clean up plugin-owned conversations. Quick Chat and its expiry
sweeper exclude those managed sessions.

The port preserves typed `PermissionDenied` for an undeclared
`agent_conversation` capability, replay-safe dispatch statuses, workspace
isolation, pagination, cancellation-safe cleanup, and the published manifest and
authoring guidance. It does not add workspace-agent-principal binding, a general
metadata query API, a user-facing UI, or a schema index unrelated to the scoped
contract.

## Compatibility and rollback

The RPCs and capability are additive. Plugins that do not declare
`capabilities.agent_conversation: true` retain their existing behavior; plugins
that do declare it receive a typed denial when an older Host does not implement
the contract. Rolling back the Host leaves existing managed ephemeral records
inert; normal plugin uninstall or the lifecycle cleanup path removes them.

## Verification

- Regenerate the proto with the repository-pinned toolchain.
- Run focused task-service, plugin, SDK, SQLite repository, and backend-app
  tests; run build, vet, and touched-package lint.
- Cover capability denial, idempotent Ensure/Dispatch/Delete, occurrence claims,
  cleanup under cancellation, cross-workspace isolation, lifecycle cleanup, and
  managed-session Quick Chat and sweeper exclusion.
- Validate the public manifest and authoring references.

## Work orders

- [x] [Task 01: Port the AgentConversation Host contract](task-01-port-agent-conversation-host.md)
