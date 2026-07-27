---
id: "05-remote-materialization"
title: "Remote materialization"
status: done
wave: 4
depends_on: ["03-protocol-surfaces", "04-host-materialization"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 05: Remote Materialization

## Acceptance

- Agentctl can atomically clone/checkout a validated repository into a collision-checked workspace
  subdirectory with cancellation and cleanup defenses.
- Docker, SSH, and Sprites use their live agentctl clients and existing credential resolution to
  materialize and rescan repository sources; secrets are absent from logs and metadata.
- Container and remote folder requests are rejected before persistence or filesystem access, and the
  same executor capability is available to the frontend picker.
- New/reset environments rebuild all durable sources, and adapter tests cover failure rollback plus
  the future remote-Docker capability boundary.

## Verification

```bash
cd apps/backend
rtk go test ./internal/agentctl/... ./internal/agent/runtime/lifecycle/... ./internal/backendapp/...
```

## Files likely touched

- `apps/backend/internal/agentctl/client_workspace_sources.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_sources.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_sources_test.go` (new)
- `apps/backend/internal/agentctl/server/api/server.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- executor/materialization tests beside those files

## Dependencies

Tasks 03 and 04.

## Inputs

- Spec: executor source capabilities, credential boundary, persistence, failure modes.
- Existing `workspace/copy-files`, remote copy-files, Docker prepare, SSH upload, and Sprites file
  uploader patterns.

## Output contract

Summary, files changed, tests run, blockers, risks, divergence, and task/plan status updates.
