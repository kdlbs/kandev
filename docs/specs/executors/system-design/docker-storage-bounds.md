---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-STORAGE-001
---

# Docker executor storage bounds design

The Docker client's shared `buildHostConfig` is used by normal and interactive
container creation. Set `LogConfig` to `json-file` with `max-size=100m` and
`max-file=3`. This overrides the daemon's default logging driver and rotation
limits for these containers. Set `/tmp` to a 2 GiB tmpfs unless the caller
explicitly mounts storage at that target. Compare mount targets with POSIX
path normalization, without changing the caller's mount values.
Preserve the caller's mounts, resource limits,
networking and security options.

This implements [REQ-EXECUTORS-STORAGE-001](../requirements/docker-storage-bounds.md)
at the create boundary. Reconnect paths keep their existing container. No
Docker daemon restart or global logging configuration change is needed.

Tmpfs consumes host memory/swap and counts against the container's memory limit.
It hides image content under `/tmp` and loses its contents when the container
stops, including before a restart of that same container.
Persistent outputs and required evidence belong in designated task
storage. The policy does not impose a writable-layer or persistent-volume quota.
See the [delivery plan](../../../plans/docker-executor-storage-bounds/plan.md)
and [public reference](../../../public/executors.md#docker-log-and-temporary-storage-limits).
