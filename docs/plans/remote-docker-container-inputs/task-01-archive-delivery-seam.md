---
id: "01-archive-delivery-seam"
title: "Archive delivery and the container seeding seam"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-002
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-002.6
  - AC-EXECUTORS-REMOTE-DOCKER-002.7
  - AC-EXECUTORS-REMOTE-DOCKER-002.11
system_design:
  - ../../specs/executors/system-design/remote-docker-container-inputs.md
---

# Task 01: Archive Delivery and the Container Seeding Seam

## Summary

Give the runtime a way to put files inside a container through the Docker Engine
API, and a place in the launch sequence to do it: after the container is
created, before it is started.

## In scope

- `Client.CopyToContainer(ctx, containerID, dstPath string, tarStream io.Reader)`
  on `internal/agent/docker`, wrapping the SDK's `CopyToContainer`.
- A `tarFileUploader` implementing `FileUploader` (`WriteFile(ctx, path, data,
  mode)`) that accumulates entries in memory, emits a directory entry for every
  path component, and produces one tar stream.
- `ContainerManager.seedCreatedContainer func(ctx, containerID string, config
  ContainerConfig) error`, invoked by `createAndStartContainer` between
  `CreateContainer` and `StartContainer`. A non-nil error removes the created
  container via `removeContainerBestEffort` and fails the launch.
- `DockerExecutor.beforeContainerStart func(ctx, containerID string) error`,
  invoked by `ensureContainerRunning` immediately before it starts a stopped
  container.
- Both hooks are nil by default, so the local Docker path is byte-identical.

## Out of scope

- Any remote Docker caller of these seams. Task 02 wires them.
- Named volumes and the Docker volume API.
- Removing anything from `sshHostFileStore`.

## Acceptance

- Against a real daemon, a tar written by `tarFileUploader` and extracted into a
  created-but-not-started container yields the expected files, modes, and
  intermediate directories, and an executable delivered to `/usr/local/bin` is
  executable when the container starts.
- A `seedCreatedContainer` error leaves no container: the launch returns the
  error and the container ID it created is gone.
- The local Docker mount set and launch sequence are unchanged, asserted by the
  existing characterization test.

## Verification

Write the failing tests first — the tar entry set and modes, the seed-error
teardown, and the local mount-set equivalence — and confirm they fail before the
production change. Then, from `apps/backend`:

```bash
go test ./internal/agent/docker/... -race -count=1
go test ./internal/agent/runtime/lifecycle/... -run 'Mount|HostFiles|SeedCreated|CreateAndStart|Reconnect' -race -count=1
golangci-lint run ./internal/agent/docker/... ./internal/agent/runtime/lifecycle/... --timeout=5m
```

The daemon-backed case runs with the existing integration build tag used by
`remote_docker_integration_test.go`; state the exact invocation in Results.

## Files likely touched

- `apps/backend/internal/agent/docker/client.go`
- `apps/backend/internal/agent/docker/client_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/credential_uploader.go` (the
  `FileUploader` interface is already the right shape; the new implementation
  belongs in a new file)

## Dependencies

None. The Docker SDK already ships `CopyToContainer`, and `archive/tar` is
already imported by `internal/agent/docker`.

## Risks

- `PutArchive` into a created-but-not-started container is the load-bearing
  assumption of the whole package. Prove it here, before task 02 depends on it.
  If it does not hold, the fallback is a short-lived helper container and the
  change stays inside this work order.
- `createAndStartContainer` is on the local Docker launch path. The nil-hook
  default plus the mount-set characterization test are what keep it safe.
- Accumulating the tar in memory is fine for credentials but holds the helper
  binary too; keep the stream single-use and released after the call.

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-002`, acceptance criteria `.6`, `.7`, `.11`.
- `container.go` `createAndStartContainer`, `removeContainerBestEffort`.
- `executor_docker.go` `ensureContainerRunning`, `shouldStartExistingDockerContainer`.
- `credential_uploader.go` `FileUploader`, `credentialFileMode`.

## Results

- `Client.CopyToContainer(ctx, containerID, dstPath string, archive []byte)` in
  `internal/agent/docker/container_archive.go`. It takes bytes rather than the
  `io.Reader` this work order sketched: the archive is accumulated in memory
  anyway, and bytes make the stream trivially single-use. Failures route through
  `ExplainRemoteFailure`, so a remote transport error keeps its own cause.
- `tarFileUploader` in `internal/agent/runtime/lifecycle/container_archive.go`
  satisfies `FileUploader` unchanged, so both existing seeders work against a
  container with no knowledge of the destination. It emits one directory entry
  per path component at mode `0700`, de-duplicates them, writes every entry with
  uid/gid 0, and exposes `IsEmpty` so a launch that seeds nothing sends nothing.
- `createSeedAndStart` owns create -> seed -> start and removes the container on
  either failure. `ContainerManager.seedCreatedContainer` and
  `DockerExecutor.beforeContainerStart` are the two hooks, both nil by default,
  so the local Docker path makes exactly the daemon calls it made before.
- The narrow `containerStarter` interface exists so the sequence is testable
  without a daemon. `*docker.Client` satisfies it as-is.

### The load-bearing assumption holds

`TestArchiveDeliveryIntoACreatedContainer` proves it against a real daemon:
a container in state `created` receives the archive, and after `StartContainer`
its entrypoint runs the delivered helper, reads the delivered credential file,
and reports mode `600`. No fallback helper container is needed, so task 02 can
proceed as designed.

One correction the probe forced: a tar written with the backend's own uid left
the files owned by an account the container does not have. Entries are now
written with uid/gid 0, covered by
`TestTarUploaderWritesRootOwnedEntries`.

Also confirmed: `PutArchive` requires its destination to exist, so extraction
happens at `/` with the directory entries carried in the archive. Copying
straight to a path the image lacks fails.

### Verified with

- `go test ./internal/agent/docker/... -race -count=1` — ok
- `go test ./internal/agent/runtime/lifecycle/... -race -count=1` — ok (54s, goleak clean)
- `KANDEV_TEST_DOCKER=1 go test ./internal/agent/runtime/lifecycle/ -run TestArchiveDeliveryIntoACreatedContainer -count=1` — PASS against Docker 29.7.2
- `golangci-lint run ./internal/agent/docker/... ./internal/agent/runtime/lifecycle/... --timeout=5m` — 0 issues
