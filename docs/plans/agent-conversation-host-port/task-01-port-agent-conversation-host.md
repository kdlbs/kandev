---
id: "01-port-agent-conversation-host"
title: "Port the AgentConversation Host contract"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGINS-001.3
  - AC-PLUGINS-PLUGINS-001.7
  - AC-PLUGINS-PLUGINS-001.11
system_design:
  - ../../specs/plugins/system-design/plugins-01.md
---

# Task 01: Port the AgentConversation Host Contract

## Summary

Port the maintained AgentConversation Host contract into canonical Kandev. The
contract lets a plugin that declares `agent_conversation: true` ensure, dispatch
to, and delete hidden workspace conversations through the existing gRPC Host and
the Go-native SDK manager.

## In scope

- Add the additive `EnsureAgentConversation`, `DispatchAgentConversation`, and
  `DeleteAgentConversation` protobuf RPCs and generated stubs.
- Expose typed descriptors, dispatch outcomes, and the optional
  `pluginsdk.AgentConversations(host)` manager.
- Gate every RPC on `capabilities.agent_conversation`, returning typed
  `PermissionDenied` when it is absent.
- Create, repair, dispatch, and delete plugin-owned hidden ephemeral task/session
  records with workspace isolation and idempotent occurrence keys.
- Keep managed conversations out of Quick Chat restoration and expiry; remove
  them during plugin lifecycle cleanup.
- Publish the capability and manager contract in manifest and authoring guides.

## Out of scope

- Workspace-agent-principal binding from the maintained fork's optional
  `workspace_agent_principals` capability.
- A repository metadata predicate, general metadata index, or unrelated schema
  migration.
- A user-facing SPA surface or a Coordinator plugin package change.

## Acceptance

- A plugin with `agent_conversation: true` can ensure a stable hidden
  conversation, dispatch a prompt exactly once per occurrence key, and delete
  only its own matching conversation.
- A plugin without that capability receives gRPC `PermissionDenied`; no direct
  database, shell, REST, MCP, or prompt-derived authority is introduced.
- Managed records are isolated by workspace and plugin ownership, excluded from
  Quick Chat and its sweeper, and removed on disable or uninstall.
- The public manifest and authoring documentation describe the capability,
  manager, outcomes, and compatibility behavior.

## Verification

```bash
cd apps/backend && go test -tags fts5 -count=1 ./internal/task/service/ ./internal/task/repository/sqlite/ ./internal/plugins/... ./pkg/pluginsdk/ ./internal/backendapp/
cd apps/backend && go build ./... && go vet ./...
cd apps/backend && golangci-lint run ./internal/task/service/... ./internal/task/repository/sqlite/... ./internal/plugins/... ./pkg/pluginsdk/... ./internal/backendapp/...
node --test .github/scripts/pr-docs.test.cjs
git diff --check
```

## Results

- Ported the maintained contract from `612f46f7a4b206c7cd81863efc4202c123901542`
  onto canonical-main APIs, including generated proto, SDK, Host, task service,
  lifecycle wiring, tests, and public references.
- Added cancellation-safe compensation for dispatch claims and recorded messages,
  SQLite cross-connection claim coverage, Quick Chat exclusion coverage, and
  stale ephemeral metadata compatibility coverage.
- Focused and touched-package tests, build, vet, lint, documentation checks, and
  diff validation passed at the delivery head before PR review follow-up.

