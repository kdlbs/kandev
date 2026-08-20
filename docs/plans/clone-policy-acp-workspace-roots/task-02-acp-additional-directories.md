---
id: "02-acp-additional-directories"
title: "ACP additional directories"
status: pending
wave: 2
depends_on: ["01-clone-policy-attestation"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 02: ACP Additional Directories

## Acceptance

- Lifecycle, agentctl client/API, instance configuration, and ACP adapter transmit only canonical executor-side source roots to new sessions.
- The adapter sends `additionalDirectories` only after the initialized agent advertises the ACP capability, removes `cwd` and duplicates while preserving order, and never substitutes a parent/root.
- Local/Worktree and clone executor multi-repository sessions have protocol and real launch/session tests; unsupported providers retain narrow CWD behavior or receive a precise explicit failure when required scope cannot be represented.

## Verification

```bash
cd apps/backend
go test ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/acp
go test -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/workspace_materialization.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/adapter/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/shared/config.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- associated client, API, adapter, and lifecycle tests

## Dependencies

Task 01.

## Parallelism

Sequential: source-root shape is shared with clone policy installation.

## Inputs

Attach Workspace Sources spec, ADR-2026-07-22, ADR-2026-08-20, and ACP SDK `SessionCapabilities.AdditionalDirectories` / `NewSessionRequest.AdditionalDirectories`.

## Output contract

Summary, files changed, protocol and launch/session test receipts, unsupported-provider behavior, no-widening proof, blockers, risks, and task/plan status update.

## Results

Pending.
