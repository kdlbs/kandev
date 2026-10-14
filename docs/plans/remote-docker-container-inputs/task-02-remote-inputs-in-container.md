---
id: "02-remote-inputs-in-container"
title: "Remote container inputs move into the container"
status: done
wave: 2
depends_on:
  - "01-archive-delivery-seam"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-002
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-002.1
  - AC-EXECUTORS-REMOTE-DOCKER-002.2
  - AC-EXECUTORS-REMOTE-DOCKER-002.3
  - AC-EXECUTORS-REMOTE-DOCKER-002.4
  - AC-EXECUTORS-REMOTE-DOCKER-002.5
  - AC-EXECUTORS-REMOTE-DOCKER-002.6
  - AC-EXECUTORS-REMOTE-DOCKER-002.7
  - AC-EXECUTORS-REMOTE-DOCKER-002.12
system_design:
  - ../../specs/executors/system-design/remote-docker-container-inputs.md
---

# Task 02: Remote Container Inputs Move Into the Container

## Summary

Deliver the `agentctl` helper, the E2E mock-agent helper, the per-instance agent
session directory, and the agent's seeded credentials and configuration bundles
into the remote container through the Engine API, and stop writing any of them
to the remote host.

## In scope

- `remoteContainerHostFiles` delivers instead of mounting: `AgentctlBinary` and
  `MockAgentBinary` resolve the backend-side bytes for the probed platform and
  contribute tar entries for `/usr/local/bin/agentctl` and
  `/usr/local/bin/mock-agent` at mode `0755`; `SessionDir` returns an empty
  source with the agent's declared target, so `expandMounts` adds no mount.
- `seedRemoteAgentSessionDir` keeps calling `UploadCredentialFiles` and
  `UploadPortableConfigBundles` unchanged, with `tarFileUploader` in place of
  `sshFileUploader`, writing into the in-container session-directory target.
- `RemoteDockerExecutor` wires `ContainerManager.seedCreatedContainer` to one
  delivery that carries helpers and seeded files, and wires the reconnect
  delegate's `beforeContainerStart` to a helper-only redelivery.
- The unsupported-platform check keeps its position ahead of any delivery.
- `sshHostFileStore` and its `EnsureAgentctl` / `EnsureSessionDir` /
  `EnsureFile` methods, the `remoteHostFileStore` interface, and the remote
  Docker path's use of `sftpUploadBytes` and `expandRemoteHome` are removed.
  `remoteKandevHomeDir` survives for task 03 only.

## Out of scope

- The SSH executor's `ensureAgentctlOnHost`, `sftpUploadBytes`,
  `expandRemoteHome`, and `sshFileUploader`, which stay for that runtime.
- Teardown of the pre-existing remote session directory. That is task 03.
- Any local Docker behavior.

## Acceptance

- A remote launch produces no bind mount whose source is a remote host path, and
  performs no SFTP session and no filesystem-mutating SSH exec. The test fails
  the run if either is attempted.
- The seeded archive places credential files at the agent's declared
  session-directory target with mode `0600`, and the helpers at their
  `/usr/local/bin` paths at mode `0755`, with every intermediate directory
  present.
- A helper delivery failure fails the launch and leaves no container; a
  credential failure emits a launch warning and the launch proceeds. A resume of
  a stopped container re-delivers the helper and does not re-seed credentials.

## Verification

Start from a failing test asserting that a remote launch opens no SFTP session
and produces no remote-path mount source. Confirm it fails first. Then, from
`apps/backend`:

```bash
go test ./internal/agent/runtime/lifecycle/... -run 'RemoteDocker|HostFiles|Seed|Mount|Agentctl' -race -count=1
go test ./internal/agent/runtime/lifecycle/... -race -count=1
golangci-lint run ./internal/agent/runtime/lifecycle/... --timeout=5m
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/container_host_files.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_host_files_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_docker_host_files.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_docker_seed.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_docker_seed_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker_test.go`

## Dependencies

Task 01's `CopyToContainer`, `tarFileUploader`, `seedCreatedContainer`, and
`beforeContainerStart`.

## Risks

- The ordering changes: seeding moves from before the launch to inside it. A
  seeding failure now has a created container to clean up, which is task 01's
  contract and must be exercised from this side too.
- `ContainerHostFiles.SessionDir` keeps its `(source, target, error)` shape, and
  an empty source with a non-empty target now means "delivered, not mounted".
  That is a quiet meaning change; assert it directly rather than relying on
  `expandMounts` happening to skip.
- Removing `sshHostFileStore` removes the only remaining consumer of some SSH
  helpers from this file set. Do not delete helpers the SSH executor still uses.
- Credential files are read from the backend host and held in memory until the
  single `CopyToContainer` call. Do not log their contents or paths beyond what
  the existing seeder already logs.

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-002`.
- `container_host_files.go` `remoteContainerHostFiles`, `remoteHostFileStore`.
- `remote_docker_host_files.go`, `remote_docker_seed.go`.
- `executor_remote_docker.go` `seedRemoteSessionDir`, `reconnectToContainer`.

## Results

- `remoteContainerInputs` (`remote_docker_inputs.go`) owns every container
  input. `DeliverLaunchInputs` carries the helpers, the session directory, and
  the seeded credentials in one archive; `DeliverHelpers` carries only the
  helpers, for a resume. Both extract at `/`.
- `seedAgentSessionArchive` replaces `seedRemoteAgentSessionDir` and still calls
  `UploadCredentialFiles` and `UploadPortableConfigBundles` unchanged, so what an
  agent receives does not depend on which daemon runs it.
- `remoteContainerHostFiles` now supplies no mount source at all, and exists
  only to reject an unsupported platform before a container is created.
  `TestRemoteContainerMountsNothing` asserts the remote mount set is empty.
- `sshHostFileStore`, the `remoteHostFileStore` interface, and
  `remote_docker_seed.go` are deleted. The remote Docker path opens no SFTP
  session and issues no filesystem-mutating SSH exec; the SSH executor keeps its
  own `sftpUploadBytes`, `expandRemoteHome`, and `ensureAgentctlOnHost`.

### Two deviations from the work order as written

- `SessionDir` returns an empty source **and** an empty target, not an empty
  source with a non-empty target. The work order flagged the latter as a quiet
  meaning change; returning both empty removes the ambiguity, and the deliverer
  reads the target from the command builder itself.
- `remoteKandevHomeDir` is not kept. Leaving an unused constant to reserve it
  for task 03 fails the `unused` linter and is dead code either way; task 03
  introduces it at its use site.

### One thing the work order did not anticipate

`ContainerConfig` gained an `OnProgress` field. Seeding moved from before the
launch to inside it, so the portable-config warnings no longer have the request
at hand. The container manager is shared by every launch on a profile, so
carrying the callback on the manager would race concurrent launches; carrying it
on the per-launch config does not.

### Verified with

- `go test ./internal/agent/runtime/lifecycle/... -race -count=1` — ok (54s, goleak clean)
- `golangci-lint run ./internal/agent/runtime/lifecycle/... --timeout=5m` — 0 issues
- `go build ./...` — ok
