---
id: "04-host-materialization"
title: "Host materialization"
status: done
wave: 3
depends_on: ["02-attachment-service"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 04: Host Materialization

## Acceptance

- Worktree repository additions and mixed Local/Worktree folder sources materialize under a safe
  Kandev-owned task root using live, canonical, platform-native directory links.
- An idle Local execution can re-root/rebind without losing task conversation state; Worktree keeps
  synchronous sibling materialization. Active work is never interrupted.
- Agentctl adopts the new workdir, refreshes file scope and Git trackers, and materialization rolls
  back safely when linking, restart, rescan, or event publication fails.

## Verification

```bash
cd apps/backend
rtk go test ./internal/backendapp/... ./internal/agent/runtime/lifecycle/... ./internal/agentctl/server/api/... ./internal/agentctl/server/process/...
```

## Files likely touched

- `apps/backend/internal/backendapp/branch_materializer.go`
- `apps/backend/internal/backendapp/workspace_source_materializer.go` (new)
- `apps/backend/internal/backendapp/*workspace_source_materializer*_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_rescan.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`
- `apps/backend/internal/agentctl/server/api/workspace_rescan.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- platform-specific link helper/test files under `apps/backend/internal/worktree/`

## Dependencies

Task 02.

## Inputs

- Spec: host live-reference semantics and idle gate.
- ADR-2026-07-19-workspace-symlink-entries.
- Existing `branch_materializer.go` root promotion and agentctl rescan path.

## Output contract

Summary, files changed, tests run, blockers, risks, divergence, and task/plan status updates.
