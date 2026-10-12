---
id: "03-container-endpoint-resolver"
title: "Container endpoint resolver"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.10
  - AC-EXECUTORS-REMOTE-DOCKER-001.11
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 03: Container Endpoint Resolver

## Summary

Make the container manager's published-port resolution pluggable so a remote
daemon's loopback endpoints are reached through SSH forwards.

## In scope

- An `EndpointResolver` seam replacing direct `resolveContainerEndpoint` calls
  for the control port and each instance port.
- A local resolver returning today's `(host, port)` unchanged.
- A remote resolver that calls `StartPortForward` for a published port and
  returns backend loopback plus the forward's local port.
- Session-scoped ownership of forwards, closed with the session.

## Out of scope

- Creating the SSH connection, owned by task 01.
- Resume-time forward reconstruction, owned by task 04.
- Changes to `agentctl.Client` or the event WebSocket.

## Acceptance

- The local resolver produces endpoints identical to current behavior.
- The remote resolver creates one forward per distinct published port on
  demand, reuses it for repeat lookups, and closes it with the session.
- No caller retains a container IP or a remote loopback address.

## Verification

Start with a failing test that a remote-resolved endpoint is backend loopback
and not the published remote port. Confirm it fails before the production
change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/agent/runtime/lifecycle/... -run 'Endpoint|PortForward|ContainerHostPort' -race
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_endpoints.go`
- `apps/backend/internal/agent/runtime/lifecycle/container_endpoints_test.go`

## Dependencies

None. Reuses `StartPortForward` and `SSHPortForwarder` as they are.

## Risks

- Instance ports are allocated by the control server at instance-create time,
  so forwards cannot be established eagerly; a lazy path that misses a port
  produces a hang rather than an error. Fail fast with a named cause.
- Leaked forwards accumulate file descriptors across sessions; ownership and
  close must be covered by a test, not left to review.

## Parallelism

`parallel-safe`

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-001`.
- `container.go` `resolveContainerEndpoint`, `dockerAgentctlPortBindings`.
- `SSHPortForwarder` and `StartPortForward`.

## Results

- Added the `containerEndpointResolver` seam with a local implementation that
  preserves today's behavior, including the container-IP fallback when the
  published port cannot be read.
- Added `remoteEndpointResolver`, which forwards each distinct published port
  back to backend loopback once and reuses it for repeat lookups.
- The remote resolver deliberately has no container-IP fallback. A remote
  container's IP is on the remote daemon's network, so that fallback would
  return an address that hangs on first use instead of naming the cause.
- Propagated resolution errors out of `resolveContainerEndpoint` and both call
  sites. The first wiring absorbed the error back into the container-IP
  fallback, which silently defeated the fail-loudly property the remote
  resolver exists to provide.
- Forwards are session-scoped and closed together; a leaked forward outlives
  its container and holds a listener for the backend's lifetime.
- Concurrent resolution of the same port keeps the winning forward and closes
  the loser rather than leaking it.
- Verified with:
  - `go test ./internal/agent/runtime/lifecycle/ -count=1 -race` (endpoint tests)
  - `go test ./internal/agent/runtime/lifecycle/ -count=1` (50s, goleak clean)
  - `golangci-lint run ./internal/agent/runtime/lifecycle/...` (0 issues)
