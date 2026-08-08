---
id: "01-ssh-workspace-lifecycle"
title: "SSH workspace lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ssh-executor/spec.md"
---

# Task 01: SSH Workspace Lifecycle

## Acceptance

- An ordinary provider-backed SSH task resolves and runs the default or stored prepare script on the
  target before `agentctl`, then starts the controller with the verified primary checkout as its
  workspace.
- Preparation failure, timeout, cancellation, missing checkout, or conflicting `origin` fails closed
  without a controller/forwarder leak; a matching existing checkout preserves local work.
- Terminal archive/delete runs the stored cleanup script before controller teardown; ordinary Stop
  and backend restart skip cleanup, and cleanup failure remains best-effort.
- Existing remote-contribution identity, exact-SHA, upstream, credential-redaction, and resume
  guarantees remain intact.

## Verification

```bash
cd apps/backend
rtk go test ./internal/agent/runtime/lifecycle -run 'Test.*SSH.*(Prepare|Workspace|Cleanup|Contribution|Origin|Checkout)'
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/default_scripts.go`
- `apps/backend/internal/agent/runtime/lifecycle/default_scripts_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_remote_contribution.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_scripts.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_scripts_test.go` (new)

## Dependencies

None.

## Parallelism

Sequential. Preparation and cleanup share SSH lifecycle ordering, metadata, command execution, and
security tests.

## Inputs

- SSH spec sections **Workspace lifecycle on the remote**, **Telemetry & errors**, and the new
  preparation/cleanup scenarios.
- Sprites patterns in `executor_sprites_operations.go`, `executor_sprites.go`, and
  `executor_sprites_lifecycle.go`.
- Existing SSH contribution materialization and credential-stdin handling.

## Risks

- Never put environment secrets into remote argv, persisted metadata, progress output, or returned
  errors.
- Keep prepare/cleanup environment access distinct from the existing narrow agent-subprocess
  allowlist; cache cleanup values only in live SSH session state and reconstruct them on resume.
- Do not reset a reused task branch or delete local/untracked work.
- Do not allow a custom prepare script to start a duplicate controller through agentctl placeholders.
- Preserve SSH client/forwarder cleanup on every failure edge.

## Output contract

Report root-cause path replaced, files changed, RED/GREEN evidence, exact targeted test result,
credential and cleanup boundaries, blockers/risks, and synchronized task/plan status.

## Results

- Implemented the SSH default prepare script and pre-agentctl lifecycle ordering.
- Added workspace/repository/script-engine resolution, fail-closed checkout verification, bounded/redacted prepare output, and terminal cleanup execution.
- Added explicit archive/delete tree stop reasons so handoff teardown invokes SSH cleanup before the later durable cascade stop.
- Targeted SSH lifecycle tests: 23 passed; full lifecycle package: 1,141 passed; race-focused SSH run: 47 passed.
