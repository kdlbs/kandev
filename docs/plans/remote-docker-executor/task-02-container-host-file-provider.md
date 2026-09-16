---
id: "02-container-host-file-provider"
title: "Container host-file provider"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.8
  - AC-EXECUTORS-REMOTE-DOCKER-001.9
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 02: Container Host-File Provider

## Summary

Put the container's host-side file dependencies behind a provider so their
sources can be materialized on a remote host instead of the backend's
filesystem, without changing local behavior.

## In scope

- A `ContainerHostFiles` interface on `ContainerManager` covering the
  `agentctl` binary, the per-instance agent session directory, and the e2e
  mock-agent binary.
- A local implementation that reproduces today's mount sources byte-for-byte.
- A remote implementation that uploads through `ensureAgentctlOnHost` and
  `sftpUploadBytes` and returns remote paths.
- Platform-correct `agentctl` selection from a probed `SSHRemotePlatform`,
  replacing the unconditional `linux/amd64` choice for the remote path.

## Out of scope

- `LocalClonePath` and `MainRepoGitDir`, which remote profiles reject.
- Workspace content, which is cloned inside the container.
- The runtime that selects between providers.

## Acceptance

- Local Docker launches produce an identical mount set before and after the
  refactor, asserted against the existing expectations.
- The remote provider returns remote paths and performs no backend-host path
  lookup.
- A remote `arm64` platform selects the `arm64` helper; an unsupported platform
  returns a named error before any container is created.

## Verification

Start with a failing test that the remote provider yields no backend-host mount
source. Confirm it fails before the production change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/agent/runtime/lifecycle/... -run 'Mount|HostFiles|SessionDir|Agentctl' -race
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_host_files.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_host_files_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`

## Dependencies

None. Consumes the SSH upload helpers, which already exist.

## Risks

- This refactor touches the local Docker path. Mount-set equivalence tests must
  come first, or a regression here breaks a shipped executor.
- Seeded agent configuration lands under the remote user's home and can affect
  other processes on a shared account; the SSH executor documents the same
  hazard.

## Parallelism

`parallel-safe`

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-001`.
- `container.go` `expandMounts` and `buildContainerConfig`.
- `executor_ssh_operations.go` `ensureAgentctlOnHost`, `sftpUploadBytes`.

## Results

_Not started._
