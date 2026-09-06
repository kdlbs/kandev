# E2E and Frontend Test Resource Safety

Keep local browser runs bounded. The managed and raw E2E entry points enforce
one Playwright worker per shard. The guard recognizes `--workers`,
`--workers=N`, `-j N`, `-j=N`, and compact `-jN` forms, so a worker override
cannot bypass the limit by changing CLI spelling. The managed runner allows at
most three local shards, and lowers that limit when available memory is below
12 GiB or 6 GiB. Its available-memory value is the minimum of host
`MemAvailable` and the remaining cgroup v1 or v2 memory limit when the process
runs inside a container. This prevents host memory reporting from authorizing
more shards than the container can hold.

Start with the default single shard; use two or three only when the host has
capacity for separate Go backends, SPA processes, Chromium instances, and mock
agents. If a command is rejected, reduce its shard or worker count. Do not
silence the guard during ordinary verification.

Do not run a local full Vitest suite with `VITEST_MAX_WORKERS=100%`, another
all-worker value, or several overlapping full-suite processes. Local Vitest
configuration clamps unsafe worker overrides to its 20-percent budget. Set
`KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM=1` or
`KANDEV_ALLOW_UNSAFE_TEST_PARALLELISM=1` only for a deliberate, monitored
resource experiment, and record the resource limit and result.
