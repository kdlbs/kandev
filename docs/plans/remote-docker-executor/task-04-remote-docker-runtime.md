---
id: "04-remote-docker-runtime"
title: "Remote Docker runtime"
status: pending
wave: 2
depends_on:
  - "01-ssh-dialed-docker-client"
  - "02-container-host-file-provider"
  - "03-container-endpoint-resolver"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.7
  - AC-EXECUTORS-REMOTE-DOCKER-001.12
  - AC-EXECUTORS-REMOTE-DOCKER-001.13
  - AC-EXECUTORS-REMOTE-DOCKER-001.15
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 04: Remote Docker Runtime

## Summary

Replace the `remote_docker` stub with a runtime that composes the SSH-dialed
client, the remote host-file provider, and the forwarding endpoint resolver into
a working container lifecycle.

## In scope

- `CreateInstance`, `StopInstance`, and `RecoverInstances` implemented against a
  remote daemon, following the Docker executor's structure.
- Session state recording container ID, SSH target, and published ports.
- Resume: re-dial SSH, re-verify fingerprint, rebuild forwards, reconnect to the
  preserved container by ID.
- SSH keepalive so a dead transport surfaces as a session failure.
- Source-rule rejection of local Git repositories and arbitrary folders.
- Removal of the "not yet implemented" errors and the stub's no-op health check.

## Out of scope

- Profile UI, create flow, and the HTTP test endpoint.
- E2E fixtures and public documentation.
- Sharing a pooled connection with an `ssh` executor profile on the same host.

## Acceptance

- A task launches into a container on a remote daemon, clones at `/workspace`,
  and reaches `agentctl` through forwards, with no backend-host path mounted.
- Stop preserves the container; resume reconnects to the same container ID after
  rebuilding the transport.
- A dropped SSH connection marks the session failed rather than leaving it
  apparently healthy.

## Verification

Start with a failing test that `CreateInstance` returns a running instance
rather than `remote_docker runtime is not yet implemented`. Confirm it fails
before the production change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/agent/runtime/lifecycle/... -run 'RemoteDocker' -race
rtk go test ./internal/orchestrator/executor/... -run 'RemoteDocker|Resume' -race
rtk make test
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_resume.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/task/service/service_resources.go`

## Dependencies

Tasks 01, 02, and 03.

## Risks

- `manager_launch.go` currently short-circuits `remote_docker` to skip
  preparation, and `skill/delivery.go` and `default_scripts.go` branch on the
  string. Every one of those sites assumes the stub and must be revisited.
- Reconnect-by-container-ID must verify the container belongs to this executor
  before adopting it; a recycled ID on a shared daemon is a real hazard.
- Orphan reaping already lists `RuntimeRemoteDocker`; confirm it does not delete
  live remote containers once they actually exist.

## Parallelism

`sequential`

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-001`.
- `executor_docker.go` for lifecycle structure.
- `executor_ssh.go` `ResumeRemoteInstance` for transport rebuild.

## Results

_Not started._
