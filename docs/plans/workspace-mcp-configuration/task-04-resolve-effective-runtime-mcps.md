---
id: "04-resolve-effective-runtime-mcps"
title: "Resolve effective runtime MCPs"
status: done
wave: 3
depends_on:
  - "03-migrate-scoped-mcp-selections"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.3
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 04: Resolve Effective Runtime MCPs

## Summary

Make one typed resolver the source of truth for MCP configuration at every
agent attachment boundary. Compose the additive scope union, resolve secrets
ephemerally, apply transport policy, and enrich session-owned attachment
evidence with revisions and origins.

## In scope

- Add `ResolutionContext`, effective server, origin, and ephemeral delivery
  models to `internal/agent/mcpconfig`.
- Union all task repositories, the workspace-contextual effective profile, the
  task, and the session by stable definition ID.
- Accumulate origins, reject name collisions, sort by runtime name, resolve
  secret bindings, then run executor/provider policy.
- Preserve Kandev's reserved internal MCP injection path.
- Use the resolver for new, restart, resume/load, context reset, and workspace
  rebind flows.
- Feed the composed result into existing ACP and passthrough strategies.
- Deliver remote definitions without local installation. Materialize exact npm
  packages through the managed Node runtime in an executor-owned cache.
- Resolve existing executable commands inside the executor and report a typed
  error when a command is absent. Replace the silent warn-log drop in
  `instance.Manager.buildMcpServerConfigs` with a filtered-attachment decision.
- Map remote, managed-package, and existing-executable modes onto each
  passthrough strategy in `mcpconfig/passthrough.go`. A remote definition must
  reach Codex as `url`/`http_headers`, not as a stdio command.
- Extend attachment evidence with definition ID, revision, and bounded origins.
- Retain legacy profile fallback for an unimported profile-workspace pair.

## Out of scope

- Session selection mutation and live idle reconnect.
- Frontend origin presentation.
- Registry or catalog changes.

## Acceptance

- Multi-repository and multi-scope inputs produce one stable, deterministic
  effective set with every origin and no duplicate server delivery.
- Resolved secret values exist only in the delivery object and never appear in
  stored evidence, stream payloads, logs, or errors. Executor-side exposure
  through passthrough config files and Codex `-c` arguments is expected and is
  asserted as the documented boundary, not treated as a leak to fix here.
- Package materialization changes no repository file or lockfile and reuses
  only a cache entry with the same complete package and platform key.
- Every agent attachment path uses the same resolver before transport policy,
  including legacy fallback and passthrough providers.

## Verification

```bash
cd apps/backend && go test ./internal/agent/mcpconfig -run 'Test.*(Resolve|Union|Origin|Secret|LegacyFallback|Materializ|Remote)'
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test.*MCP.*(Start|Restart|Reset|Resume|Rebind|Passthrough|Attachment)'
cd apps/backend && go test ./internal/agentctl/types/streams
```

Write the resolver matrix first. The RED cases must include duplicate origins,
two task repositories, a global profile in two workspaces, a disabled
definition, and a failed secret binding.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/resolve.go`
- `apps/backend/internal/agent/mcpconfig/resolve_test.go`
- `apps/backend/internal/agent/mcpconfig/executor_policy.go`
- `apps/backend/internal/agent/mcpconfig/passthrough.go`
- `apps/backend/internal/agent/mcpconfig/passthrough_test.go`
- `apps/backend/internal/agentctl/server/instance/` command-resolution path
- `apps/backend/internal/agent/mcpconfig/materialize.go`
- `apps/backend/internal/agent/mcpconfig/materialize_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_profile.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rebind.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_mcp_attachment_test.go`

## Dependencies

- Task 03 supplies typed associations and per-pair legacy import status.

## Risks

- Current reset and rebind paths resolve MCPs at different points. Do not leave
  any path reading raw profile JSON directly.
- The internal Kandev MCP endpoint must remain reserved and must not be
  overwritten by a workspace definition.
- Filtering after composition must retain a typed reason and origins even when
  the provider cannot receive a server.

## Parallelism

`sequential`

## Inputs

- Requirement sections 003, 004, and 007.
- System-design sections `Effective-set resolution`, `Runtime delivery`,
  `Security`, and `Observability`.
- ADR-0014, ADR-0020, ADR-2026-07-30, and ADR-2026-09-01.

## Results

- Added deterministic additive resolution across profile, repository, task, and task-session scopes with stable-ID deduplication and origin reporting.
- Wired current definition revisions, secret binding resolution, executor materialization, transport filtering, and typed filtered attachment evidence into start, resume, restart, reset, and rebind paths.
- Preserved legacy profile configuration as a per-workspace fallback until import completes and kept resolved secrets out of persisted evidence.
- Verification passed through resolver, lifecycle, agentctl, passthrough, handler, and full backend tests.
