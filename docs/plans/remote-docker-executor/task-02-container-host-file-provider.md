---
id: "02-container-host-file-provider"
title: "Container host-file provider"
status: done
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

- Added the `ContainerHostFiles` seam covering the three container inputs whose
  source is a filesystem path rather than image content: the `agentctl` helper,
  the per-instance agent session directory, and the E2E mock-agent binary.
- Locked today's local mount set with a characterization test written before
  the refactor. It immediately caught a wrong assumption of mine: agents
  without a full `SessionDirTemplate`+`SessionDirTarget` pair add no session
  mount at all, so the test now uses an agent that has one.
- Added `remoteContainerHostFiles`, resolving every source on the remote host
  and asserting no backend-host path reaches a remote container. The test fails
  the run if the backend-side agentctl resolver is called at all.
- Agentctl selection now follows the probed `SSHRemotePlatform`, fixing the
  current unconditional `linux/amd64` choice. That is invisible with a local
  amd64 daemon and fatal against an arm64 remote.
- `SessionDir` reports an error instead of returning an empty path when the
  directory should exist but could not be produced. The first version swallowed
  it, which would have started an agent without its seeded credentials and no
  stated cause; `expandMounts` and `buildContainerConfig` thread the error.
- `MockAgentBinary` is always empty for a remote daemon, since it resolves from
  the backend's own build tree.
- Verified with:
  - `go test ./internal/agent/runtime/lifecycle/ -count=1` (50s, goleak clean)
  - `golangci-lint run ./internal/agent/runtime/lifecycle/...` (0 issues)
