---
id: "01-create-bounds"
title: "Bound newly created Docker containers"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-STORAGE-001
acceptance_criteria:
  - AC-EXECUTORS-STORAGE-001.1
  - AC-EXECUTORS-STORAGE-001.2
  - AC-EXECUTORS-STORAGE-001.3
system_design:
  - ../../specs/executors/system-design/docker-storage-bounds.md
---

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

## Contributor verification

The contributor reported the results below for original head
`627d39c35bf4874d40719821e42ef1d2cf81000c`. They do not verify the review changes.

From `apps/backend`:

```
GOMAXPROCS=2 go test -p 2 -race ./internal/agent/docker -count=1
KANDEV_DOCKER_STORAGE_CANARY_IMAGE=postgres:16 GOMAXPROCS=2 go test -p 2 -race ./internal/agent/docker -count=1
```

Both passed. The default-budget regression failed before the implementation.
The real canary proved a 2 GiB tmpfs, a 4 MiB scratch write, retained log
rotation options, and removal of its own container. The upstream port also passed package race tests and the same isolated daemon
canary against current upstream main.

## Review changes and pending verification

- Equivalent `/tmp` mount targets suppress the default tmpfs. Other mount
  targets keep the default. The caller's mount values remain unchanged.
- Request-level tests cover normal and interactive creation, default scratch,
  explicit mounts, rotating logs, and preservation of the memory limit.
- The real-daemon canary uses `WaitContainer` on the client that creates its
  container. It needs no Docker CLI and cannot select a different CLI context.
- The public reference and design describe the driver override, memory
  accounting, hidden image files, and scratch loss at container stop.

The canary requires an existing local Linux image with `/bin/sh`, `stat`, `df`,
`tail`, `awk`, and `dd`. It creates a network-isolated container, inspects its
log budget, checks the scratch capacity, and writes a small scratch probe.
Cleanup removes only that container and its anonymous volumes. The canary
does not download an image or recreate an existing executor.

Builds, tests, linters, and validation hooks were not run during the review,
as the user requested. The commands above remain pending for these changes.
