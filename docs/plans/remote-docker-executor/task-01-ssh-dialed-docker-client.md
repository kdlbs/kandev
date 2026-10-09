---
id: "01-ssh-dialed-docker-client"
title: "SSH-dialed Docker client"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
acceptance_criteria:
  - AC-EXECUTORS-REMOTE-DOCKER-001.1
  - AC-EXECUTORS-REMOTE-DOCKER-001.2
  - AC-EXECUTORS-REMOTE-DOCKER-001.3
  - AC-EXECUTORS-REMOTE-DOCKER-001.14
system_design:
  - ../../specs/executors/system-design/remote-docker-executor.md
---

# Task 01: SSH-Dialed Docker Client

## Summary

Construct a `docker.Client` whose transport dials `docker system dial-stdio`
over a pooled Kandev SSH connection, and classify its connection failures.

## In scope

- A constructor taking a resolved `SSHTarget` and returning a `*docker.Client`.
- Option order: `WithHost("http://docker.example.invalid")`, then
  `WithDialContext`, then `WithAPIVersionNegotiation`.
- A `net.Conn` adapter over an SSH session's stdin/stdout with half-close.
- Daemon-address validation that rejects any URL scheme, so a profile stores an
  SSH target rather than a Docker host URL.
- Typed errors for: SSH unreachable, fingerprint mismatch, socket access
  denied, daemon unreachable, unsupported remote platform.

## Out of scope

- Container operations, mounts, and port forwarding.
- Profile persistence and UI.
- Connection pooling policy beyond reusing the SSH executor's pool.

## Acceptance

- A client built against a fake SSH server that serves the Engine API over
  `dial-stdio` completes a `Ping` and reports the negotiated API version.
- Reversing the option order fails a test, pinning the
  `sockets.ConfigureTransport` override hazard.
- A `dial-stdio` exit with a permission error classifies as socket-access-denied
  and not as daemon-unreachable.
- Any daemon address containing `://` is rejected before a dial is attempted.

## Verification

Start with a failing test that a remote client's `Ping` succeeds over a stub SSH
transport. Confirm it fails before the production change. Then run:

```bash
# From apps/backend:
rtk go test ./internal/agent/docker/... -run 'RemoteClient|DialStdio|Classify' -race
rtk go test ./internal/agent/runtime/lifecycle/... -run 'RemoteDockerClient' -race
```

## Files likely touched

- `apps/backend/internal/agent/docker/remote_client.go`
- `apps/backend/internal/agent/docker/remote_client_test.go`
- `apps/backend/internal/agent/docker/remote_errors.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go`

## Dependencies

None.

## Risks

- `WithHost` installs a TCP dialer through `sockets.ConfigureTransport`'s
  default branch; applying `WithDialContext` first is silently wrong.
- The SSH session's stdout must be half-closed rather than closed so HTTP
  response bodies drain; the SSH executor's forwarder has the same constraint
  and is the reference.
- `docker system dial-stdio` is a CLI subcommand; the remote must have the
  Docker CLI, not only the daemon. Probe it explicitly and name that cause.

## Parallelism

`parallel-safe`

## Inputs

- `REQ-EXECUTORS-REMOTE-DOCKER-001`.
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go`.
- `moby/client` `client_options.go` and `go-connections/sockets`.

## Results

- Added `docker.NewRemoteClient`, whose Engine API requests travel through a
  supplied dialer instead of a local socket or a TCP address.
- Pinned the option-order hazard with a test asserting the supplied dialer is
  never consulted under the reversed order. Asserting a failed ping instead
  would also pass under a slow resolver, for the wrong reason. Verified by
  mutation: flipping the shipped order fails two tests.
- Dropped `client.WithAPIVersionNegotiation()`; it is deprecated and a no-op in
  `moby/client v0.5.0`, where negotiation is on by default.
- Added `ValidateRemoteDaemonAddress`, rejecting every URL scheme so the
  `tcp://` exclusion holds by construction rather than by denylist.
- Added `ClassifyRemoteDialError`, separating socket-permission-denied, a
  missing Docker CLI, and an unreachable daemon while preserving remote stderr.
- Added `newSSHDockerDialer`, running `docker system dial-stdio` on a pooled
  Kandev SSH connection and adapting it to `net.Conn` with `CloseWrite`.
- Connection lifetime is deliberately not bound to the dial context:
  `http.Transport` pools connections across requests, so tearing one down when
  its dialing request ends would close a connection later requests still use.
- Extended the fake SSH server with a streaming exec handler. The canned
  handler reads stdin to EOF before replying, which no real failing command
  does and which deadlocks a client awaiting a response.
- Verified with:
  - `go test ./internal/agent/docker/ -count=1 -race`
  - `go test ./internal/agent/runtime/lifecycle/ -count=1` (50s, goleak clean)
  - `golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/...` (0 issues)
