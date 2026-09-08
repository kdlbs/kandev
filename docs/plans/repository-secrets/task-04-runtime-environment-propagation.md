---
id: "04-runtime-environment-propagation"
title: "Propagate approved runtime environments"
status: done
wave: 4
depends_on: ["03-task-environment-resolver"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 04: Propagate Approved Runtime Environments

## Acceptance

- Repository setup, agent processes, child shells, and newly opened terminals share the provisioned
  snapshot on Local/Worktree, standalone, Docker, Remote Docker, and Sprites paths.
- The effective decrypted map remains memory-only and is defensively copied and cleared on removal.
- SSH forwards explicit repository-approved keys plus its existing credential allowlist, while an
  unrelated request/host/profile key remains absent remotely.
- Remote agent and remote terminal instances receive the same approved snapshot.
- Open terminals and warm resumes keep their old snapshot; fresh recreation resolves current data.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/gateway/websocket/terminal_handler.go`
- Runtime, terminal, Docker/Sprites/standalone, SSH, and execution-store tests

## Inputs

- Task 03's validated flattened map and origin/approval metadata.
- Existing `AgentExecution.RuntimeEnvironment` and SSH credential allowlist contracts.
- Spec `Runtime lifecycle` and ADR `Provisioning snapshot and runtime propagation`.

## Dependencies

Task 03.

## TDD sequence

1. Add failing lifecycle tests for setup/process/shell/terminal parity and snapshot lifecycle.
2. Add failing SSH tests for approved arbitrary repository keys and negative unrelated-key cases.
3. Thread explicit approval metadata through request/instance configuration and implement transport
   filtering.
4. Run the wider lifecycle suite to detect regressions in profile env, Git credentials, and resume.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/gateway/websocket/... ./internal/orchestrator/executor/...
make -C apps/backend lint
```

## Risks

- Never change SSH to forward all of `req.Env`.
- Avoid logging serialized env maps in request, error, or test failure output.
- Legacy terminal fallback must not re-resolve current repository bindings and violate the snapshot
  boundary.

## Output contract

Report propagation by runtime, SSH approval representation, terminal behavior, memory clearing,
negative security tests, files changed, commands run, and residual risks.

## Result

Propagated the validated in-memory snapshot through setup, agent processes, child shells, terminals,
Docker/Remote Docker/Sprites plumbing, and SSH. SSH now receives only explicitly approved repository
keys plus the managed credential allowlist; unrelated request/host/profile values are excluded.
Snapshot-copy/clear, lifecycle, terminal, and negative SSH tests passed, as did the Docker and SSH
container E2E scenarios.
