---
id: "06-remote-docker-redial"
title: "Remote Docker redial"
status: pending
wave: 3
depends_on: ["04-reconnect-coordinator", "01-offline-budget"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.6
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 06: Remote Docker redial

## Summary

`RemoteDockerExecutor` implements `RedialRemoteInstance`. It keeps the dial
target after a transport loss, then rebuilds the SSH client, the Docker
client, and the forward to the same container.

## In scope

- **Keep the target on transport loss:** when the watchdog's `takeSession`
  runs (`executor_remote_docker.go`), keep the instance's entry in `targets`.
- **`RedialRemoteInstance`:**
  - rebuild through the `reconnect` hook and `reconnectToContainer`;
  - check agentctl health;
  - return a `RemoteInstanceRefresh`.
- **Error mapping:** a missing container maps to `ErrRedialTargetGone`, and a
  dial failure to `ErrRedialUnreachable`.
- **Cleanup:** an explicit stop removes the target, so a stopped instance is
  never redialed.
- **Orphan reap:** when agentctl is gone but the container still exists, run
  the `agent.pgid` reap inside the container through `docker exec` before
  returning `ErrRedialTargetGone`.

## Out of scope

- Local Docker.
- Container image or network changes.

## Acceptance

1. After a transport loss, redial reattaches to the running container's
   agentctl without restarting it.
2. A removed container yields `ErrRedialTargetGone`. A stopped instance is
   not redialed. An orphaned agent in a surviving container is stopped before
   the error returns.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestRemoteDockerRedial|TestRemoteDockerTransport|TestRemoteDockerOrphanReap')
make -C apps/backend lint
```

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_redial.go`
  and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`,
  `remote_docker_dialer.go`

## Dependencies

- Task 04.

## Risks

- **Stale targets.** Keeping targets could leak memory for instances that
  never return. Remove the target on terminal outcome and on stop.

## Parallelism

`parallel-safe` with tasks 05 and 08.

## Inputs

- System design section: Redial contract, remote Docker.
- `docs/specs/executors/system-design/remote-docker-executor.md`.

## Results

Pending.
