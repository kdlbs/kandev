---
id: "01-profile-and-launch"
title: Docker profile and launch settings
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-EXECUTORS-STORAGE-001
acceptance_criteria:
  - AC-EXECUTORS-STORAGE-001.3
  - AC-EXECUTORS-STORAGE-001.4
  - AC-EXECUTORS-STORAGE-001.5
  - AC-EXECUTORS-STORAGE-001.6
  - AC-EXECUTORS-STORAGE-001.7
system_design:
  - ../../specs/executors/system-design/docker-storage-bounds.md
---

# Task 01 Docker profile and launch settings

## Summary

Use profile limits only when a new Docker container is created. Keep Docker
in charge of selecting and validating the logging driver.

## In scope

- Project the three profile keys through the existing Docker network metadata
  pattern. Test that blank or missing profile values clear task-supplied values.
- In `buildDockerContainerConfig`, convert a supplied `/tmp` size to bytes
  with `RAMInBytes` and pass nonempty log strings through to Docker. Carry the
  settings through `ContainerManager.buildContainerConfigWithContext`.
- Replace the forced defaults in `applyContainerStorageBounds`. Keep mount
  precedence and propagate incompatible log options through the existing error
  path. Use Docker's rules for the inherited driver and unspecified options.

## Out of scope

The profile editor and public reference belong to Task 02. Retained-container
migration and daemon-wide policies are outside the work package.

## Acceptance

1. Profile values reach both Docker runtimes. A nonblank malformed or
   nonpositive `/tmp` size fails fresh launch with the setting named in the
   error.
2. Both create paths use zero-value storage settings by default and honor
   independent options. An explicit mount at a normalized `/tmp` takes
   precedence. Other HostConfig values remain intact.
3. The request inherits daemon logging. Docker errors reach the caller with
   no fallback create. Retained containers keep their IDs and settings.

## Verification

Start with `TestNewContainerInheritsDockerStorageDefaults`. Run it before
changing production code and confirm it fails on both forced overrides.
Expand the request test with default, partial, full, and explicit-mount cases.
Use a fake Docker error to check that an unsupported option surfaces.

From `apps/backend`:

```bash
go test ./internal/agent/docker -run '^TestNewContainerInheritsDockerStorageDefaults$' -count=1
go test ./internal/agent/docker ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -run 'DockerStorage|StorageBounds|ExplicitTemporaryMountIsNotShadowed|Docker.*Reconnect|RemoteDocker.*Recover|CreateContainerReturnsUnsupportedLogOptionErrorWithoutRetry' -count=1
KANDEV_DOCKER_STORAGE_CANARY_IMAGE=postgres:16-alpine go test ./internal/agent/docker -run '^TestContainerTmpfsOnLocalDaemon$' -count=1 -v
KANDEV_DOCKER_STORAGE_CANARY_IMAGE=postgres:16-alpine go test ./internal/agent/docker -run '^TestContainerLogRotationOnCompatibleLocalDaemon$' -count=1 -v
```

Use an existing local image and the local test daemon for both canaries. The
tmpfs test does not set log options, so it runs with any daemon logging driver.
The log rotation test skips when the daemon rejects those options. Record the
driver, Docker version, and any skipped case.

## Files likely touched

- `apps/backend/internal/agent/docker/{client.go,storage_bounds.go,storage_bounds_test.go,storage_bounds_create_test.go,storage_bounds_integration_test.go}`
- `apps/backend/internal/agent/runtime/lifecycle/{docker_launch.go,container.go,executor_backend.go,container_storage_bounds_test.go}`
- `apps/backend/internal/orchestrator/executor/{executor_state.go,executor_state_docker_storage_test.go}`
- `apps/backend/go.mod` to list the existing `github.com/docker/go-units` version as a direct dependency

## Dependencies

None.

## Risks

The effective Docker logging driver can be different on the two daemons.
The daemon owns option validation at create or start. Preserve the original
error through each launch path.

## Parallelism

Sequential.

## Inputs

[Requirements](../../specs/executors/requirements/docker-storage-bounds.md) · [System design](../../specs/executors/system-design/docker-storage-bounds.md) · [Mock Docker request test](../../../apps/backend/internal/agent/docker/storage_bounds_create_test.go) · [Profile metadata pattern](../../../apps/backend/internal/orchestrator/executor/executor_state_networks_test.go)

## Results

Complete. The focused Docker, lifecycle, and executor tests passed, including
the unsupported-log-option no-retry case under `-race`. The separate
`TestContainerTmpfsOnLocalDaemon` and
`TestContainerLogRotationOnCompatibleLocalDaemon` canaries passed on Docker
Engine `26.1.5+dfsg1` with the `json-file` driver.
`go build -trimpath -o /tmp/kandev-backend-check ./cmd/kandev` passed.
Specification gates `python3 scripts/list-docs.py validate` and
`python3 scripts/lint-spec-files.py --all` passed. The diff gate
`git diff --check` passed.
