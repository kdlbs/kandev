---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-001
created: 2026-09-16
owners:
  - kandev
---

# Remote Docker Executor System Design

## Purpose and boundaries

Implement the `remote_docker` runtime by composing two runtimes that already
exist: the Docker executor's container model and the SSH executor's transport.
This design owns the daemon connection, the three host-coupled assumptions that
make the current Docker path local-only, and the profile surface.

It does not change `local_docker` behavior, the container bootstrap contract,
the agentctl protocol, or the executor enum.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-…-001.1`, `AC-…-001.2`, `AC-…-001.3` | [Daemon connection](#daemon-connection) |
| `AC-…-001.4`, `AC-…-001.5`, `AC-…-001.16` | [Profile surface and connection test](#profile-surface-and-connection-test) |
| `AC-…-001.6` | [Image build](#image-build) |
| `AC-…-001.7`, `AC-…-001.8`, `AC-…-001.9` | [Removing host coupling](#removing-host-coupling) |
| `AC-…-001.10`, `AC-…-001.11` | [Endpoint reachability](#endpoint-reachability) |
| `AC-…-001.12`, `AC-…-001.13` | [Lifecycle, resume, and liveness](#lifecycle-resume-and-liveness) |
| `AC-…-001.14` | [Failure taxonomy](#failure-taxonomy) |
| `AC-…-001.15` | [Source rules](#source-rules) |

## Why composition, not a new runtime

`ContainerManager` and `docker.Client` are written against the Docker Engine
API, not against a local socket. Every container operation the Docker executor
performs (create, start, inspect, exec, remove, build, port lookup) works
unchanged against a remote daemon once the transport is redirected.

Three things do not survive the move, and they are the whole of this design:

1. bind mounts whose sources are backend-host paths;
2. the published `agentctl` ports, which land on the remote host's loopback;
3. `agentctl` binary delivery, currently a host bind mount.

## Daemon connection

`moby/client` accepts an arbitrary dialer. The Docker CLI's `ssh://` support
spawns the system `ssh` binary through `connhelper`; Kandev must not, because
the SSH executor already owns native `golang.org/x/crypto/ssh` dialing with
fingerprint pinning, `IdentityAgent` expansion, and `ProxyJump`.

The remote client is therefore constructed as:

- `client.WithHost("http://docker.example.invalid")` — a synthetic host so
  `ParseHostURL` and `sockets.ConfigureTransport` take the default branch;
- `client.WithDialContext(dialer)` applied **after** `WithHost`, because the
  default branch of `ConfigureTransport` installs a TCP dialer that must be
  overridden;
- `client.WithAPIVersionNegotiation()`, as the local client already does.

`dialer` opens an SSH session on the pooled connection for the profile's target,
runs `docker system dial-stdio`, and adapts the session's stdin/stdout pipes to
`net.Conn`. Close semantics follow the SSH executor's existing half-close
handling so response bodies drain rather than aborting.

`SSHTarget` resolution, `DialSSH`, and fingerprint verification are reused
verbatim from `executor_ssh_connection.go`. One SSH connection is pooled per
`(host, user, identity-source)` and shared by every session on that profile,
matching the SSH executor.

Daemon address validation rejects any scheme: the profile stores an SSH target,
not a Docker host URL. This satisfies the `tcp://` exclusion by construction
rather than by a denylist.

## Profile surface and connection test

A `remote_docker` profile is a Docker profile plus an SSH target. The create
flow is the SSH executor's test-then-trust flow with two extra probe steps:

| Step | Probe | Failure cause |
| --- | --- | --- |
| Connect | `DialSSH` with a recording host-key callback | SSH unreachable |
| Trust | fingerprint shown, checkbox required | not applicable |
| Platform | `SSHProbeRemote` | unsupported architecture |
| Daemon | `docker system dial-stdio` then `Ping` | socket access denied, daemon down |
| Version | API version from `Ping` | version negotiation failure |

The socket-access failure is distinguished from a dead daemon by the exit status
and stderr of the `dial-stdio` command, which fails with a permission error when
the SSH user is outside the `docker` group. That distinction is required by
`AC-…-001.14` and is the most likely first-run failure.

The trusted fingerprint is written to the executor `Config` as
`host_fingerprint`, reusing the SSH executor's storage key and mismatch
handling.

The editor carries a persistent notice that this profile grants effective root
on the remote host.

## Image build

The existing profile build action sends a Dockerfile-only context to the
configured daemon. Because the remote client is a `docker.Client` like any
other, the build targets the remote daemon with no change to the build request,
and the built image stays on that daemon. No registry is involved.

The existing warning that Dockerfile instructions execute with the daemon's
authority becomes a statement about the remote host.

## Removing host coupling

### Mounts

`ContainerManager.expandMounts` and `buildContainerConfig` currently add:

| Mount | Local source | Remote replacement |
| --- | --- | --- |
| `agentctl` binary | host resolver path | uploaded to the remote, mounted from there |
| per-instance session dir | `<kandev-home>/agent-sessions/<id>` | created on the remote, seeded over SFTP |
| `LocalClonePath` | host clone path | unsupported; rejected by source rules |
| `MainRepoGitDir` | host worktree `.git` | not reachable; `remote_docker` requires clone URLs |
| mock-agent binary | host build path | e2e only; uploaded with the same mechanism |

Workspace content needs no mount: the Docker executor already sets
`WorkspacePath: ""` and clones inside the container.

The seam is a `ContainerHostFiles` provider on `ContainerManager` with a local
implementation (today's behavior, byte-identical) and a remote implementation
that uploads through the SSH executor's `sftpUploadBytes` and
`ensureAgentctlOnHost`, both of which already exist and are sha256-cached.

### agentctl delivery

`ensureAgentctlOnHost` caches the platform-matched helper at
`~/.kandev/bin/agentctl` on the remote and returns its path. That path becomes
the mount source. Platform selection uses `SSHRemotePlatform` from the
connection test, which fixes the current Docker limitation of always selecting
the `linux/amd64` helper.

## Endpoint reachability

The container publishes `AgentCtlPort` and the instance range 41001-41100 on
the daemon host's `127.0.0.1` with ephemeral host ports.
`resolveContainerEndpoint` reads the published port through
`GetContainerHostPort` and hands it to `agentctl.NewClient`.

For a remote daemon those endpoints are on the remote loopback. The design
introduces an `EndpointResolver` on `ContainerManager`:

- local: returns `(host, port)` unchanged;
- remote: calls `StartPortForward(sshClient, publishedPort, logger)` and returns
  `("127.0.0.1", fwd.LocalPort())`.

Forwards are created on demand, because an instance port is only known after
the control server allocates it, and are owned by the session so they close
with it. `StartPortForward` and `SSHPortForwarder` are reused unchanged from
the SSH executor.

This keeps every `agentctl` consumer, including the WebSocket event stream,
pointed at backend loopback and satisfies `AC-…-001.11` without touching the
agentctl client.

## Lifecycle, resume, and liveness

Stop, resume, and reconnect follow the Docker executor: the container is
preserved on stop and `tryReconnect` re-attaches by container ID. Resume
additionally re-dials SSH, re-verifies the fingerprint, and rebuilds the
forwards before reconnecting, mirroring `SSHExecutor.ResumeRemoteInstance`.

Session state records the container ID, the SSH target, and the published
ports, so a backend restart can rebuild forwards without recreating the
container.

Transport liveness reuses the SSH executor's keepalive so a dead connection
surfaces as a failure rather than a hung session, satisfying `AC-…-001.13`.

## Failure taxonomy

Each cause in `AC-…-001.14` maps to a distinct typed error surfaced both by the
connection test and by launch preparation. They are not collapsed into a
generic "docker unavailable", which is the current Docker executor behavior and
is not adequate when the daemon is a network hop away.

## Source rules

`remote_docker` already reports `RequiresCloneURL() == true`. Source validation
rejects local Git repositories and arbitrary folders for this executor type,
matching the documented remote-executor rules.

## Test strategy

Unit coverage: dialer construction and option order, daemon-address validation,
failure classification, mount composition for both host-file providers, endpoint
resolution for both resolvers, and platform selection.

E2E coverage belongs to the `containers` Playwright project, which already
builds `kandev-sshd:e2e` for SSH and already exercises the Docker executor. A
remote Docker scenario runs a Docker daemon reachable over that sshd container,
which is the one piece of fixture work this design adds.

## Open questions

- Whether the pooled SSH connection should be shared with an `ssh` executor
  profile that happens to target the same host, or kept separate per executor
  type. Separate is assumed here; sharing is an optimization, not a contract.
- Whether remote image builds should stream build output to the profile editor
  as the local build does. Assumed yes, since the client is unchanged, but the
  transfer size over SSH is unmeasured.
