---
id: "05-ssh-redial"
title: "SSH redial"
status: pending
wave: 3
depends_on: ["04-reconnect-coordinator", "01-offline-budget"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.6
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.3
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity.md
---

# Task 05: SSH redial

## Summary

`SSHExecutor` implements `RedialRemoteInstance`. It replaces the lost session
entry with a fresh transport, forward, and watchdog to the same remote
agentctl, so a VPN reset recovers by itself. This task also carries the
end-to-end proof on a real SSH executor.

## In scope

- **`RedialRemoteInstance`** in a new `executor_ssh_redial.go`:
  - take the lost `sshSessionState` under `r.mu`;
  - dial with the recorded target and pinned fingerprint;
  - probe the recorded `pid` with `kill -0`;
  - open a new `StartPortForward`;
  - check health with the recorded auth token;
  - install a new state with a new watchdog under the same InstanceID;
  - return a `RemoteInstanceRefresh` with `ProcessRestarted=false`.
- **Error mapping:** a dial failure maps to `ErrRedialUnreachable`, and a dead
  pid or failed health check to `ErrRedialTargetGone`. A host-key mismatch is
  a hard error.
- **Orphan reap:** before returning `ErrRedialTargetGone`, read the session's
  `agent.pgid`.
  - If that process group is alive with the recorded start time, send
    `SIGTERM` to the group, wait 10 s, then send `SIGKILL`.
  - Report `reaped`, `already_gone`, or `reap_failed`.
  - See the design section "Orphaned agent after agentctl loss".
- **Guards:** the `CreateInstance` and `ResumeRemoteInstance` lost-entry
  guards (`executor_ssh.go`) must see either the lost or the new entry, never
  a missing one.
- **E2E:** a containers test that cuts the SSH network mid-turn, restores it,
  and asserts auto-reconnect, replay once, and the notice.

## Out of scope

- Keepalive tuning (`ssh-transport-liveness`).
- The reachability poller.

## Acceptance

1. After a watchdog teardown, redial to a live agentctl returns a refresh.
   The session continues without a new agentctl or harness process.
2. A dead pid yields `ErrRedialTargetGone`. A host-key mismatch never
   re-pins. With agentctl killed and the agent left running, redial stops the
   agent's process group before returning. A reused PID with a different
   start time is never signalled.
3. The containers E2E passes: network cut, then restore, then automatic
   reconnect with no user action.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agent/runtime/lifecycle/... -run 'TestSSHRedial|TestSSHTransport|TestSSHOrphanReap')
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/ssh/detached-reconnect.spec.ts)
make -C apps/backend lint
```

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_redial.go`
  and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh.go`,
  `executor_ssh_keepalive.go`
- New `apps/web/e2e/tests/ssh/detached-reconnect.spec.ts`, plus containers
  fixtures if needed

## Dependencies

- Task 04: the contract and the coordinator.

## Risks

- **Two forwards at once.** A new forward's local port must not collide with
  the old one's teardown. The old forward is already closed by
  `transportTeardown`.
- **Cutting the network in the containers fixture.** Use
  `docker network disconnect`, not by stopping the container, so that
  agentctl survives.

## Parallelism

`parallel-safe` with tasks 06 and 08. The files are disjoint.

## Inputs

- System design section: Redial contract, SSH.
- `docs/specs/executors/system-design/ssh-transport-liveness.md`.

## Results

Pending.
