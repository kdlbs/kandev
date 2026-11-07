# Task 01: Bound newly created Docker containers

## Outcome and scope

Apply finite log retention and temporary scratch capacity to the common normal
and interactive Docker creation path. Preserve explicit `/tmp` task mounts.
Daemon-wide configuration, existing-container recreation, persistent storage
quotas and task-evidence expiry are excluded.

## Inputs

- [Plan](plan.md).
- [Requirements](../../specs/executors/requirements/docker-storage-bounds.md):
  REQ-EXECUTORS-STORAGE-001, AC-EXECUTORS-STORAGE-001.1 through .3.
- [System design](../../specs/executors/system-design/docker-storage-bounds.md).

## Implementation acceptance

1. New HostConfig values carry the documented log and scratch bounds.
2. Explicit mounts remain intact and existing reconnect paths are unchanged.
3. A network-isolated real container validates its log settings, scratch
   capacity and a small write, then its own resources are removed.

## Files and dependencies

`internal/agent/docker/client.go`, `storage_bounds.go`, focused unit and
integration tests. The canary requires an existing local Linux image with shell
and filesystem tools. No image download or frontend dependency is needed by it.

## Verification and results

From `apps/backend`:

```
GOMAXPROCS=2 go test -p 2 -race ./internal/agent/docker -count=1
KANDEV_DOCKER_STORAGE_CANARY_IMAGE=postgres:16 GOMAXPROCS=2 go test -p 2 -race ./internal/agent/docker -count=1
```

Both passed. The default-budget regression failed before the implementation.
The real canary proved a 2 GiB tmpfs, a 4 MiB scratch write, retained log
rotation options, and removal of its own container. The upstream port also passed package race tests and the same isolated daemon
canary against current upstream main.
