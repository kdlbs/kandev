---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-REMOTE-DOCKER-002
created: 2026-09-19
owners:
  - kandev
---

# Remote Docker Container Inputs System Design

## Purpose and boundaries

Replace the remote Docker runtime's SSH/SFTP materialization of container inputs
with delivery through the Docker Engine API, and close the missing teardown of
the per-instance session directory.

This design owns `remoteContainerHostFiles`, `sshHostFileStore`,
`seedRemoteAgentSessionDir`, the container create/start sequence in
`ContainerManager`, and `RemoteDockerExecutor.StopInstance`.

It does not change the local Docker mount set, the SSH executor, the agentctl
protocol, the daemon transport, the endpoint resolver, the profile surface, or
the executor enum.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-…-002.1`, `AC-…-002.2`, `AC-…-002.12` | [Delivery model](#delivery-model) |
| `AC-…-002.3` | [agentctl and the E2E mock agent](#agentctl-and-the-e2e-mock-agent) |
| `AC-…-002.4`, `AC-…-002.5` | [Session directory and credentials](#session-directory-and-credentials) |
| `AC-…-002.6` | [Where delivery happens](#where-delivery-happens) |
| `AC-…-002.7` | [Failure handling](#failure-handling) |
| `AC-…-002.8`, `AC-…-002.9`, `AC-…-002.10` | [Teardown](#teardown) |
| `AC-…-002.11` | [Local Docker is untouched](#local-docker-is-untouched) |

## Delivery model

The Engine API's `PutArchive` (`CopyToContainer` in the Go client) extracts a
tar stream into a container's filesystem. It is available over the same
SSH-dialed `docker.Client` the runtime already uses, so every container input
becomes an archive extracted into the container that consumes it.

### Why not named volumes

The issue that motivates this capability proposes a named volume per instance
for the session directory. This design does not use one.

A volume would exist to give the session directory a lifetime independent of the
container. It has no such lifetime: the directory is created for one instance,
consumed by that instance's container, and is meaningless once the container is
gone. A per-instance volume would therefore be an extra Docker object created at
launch, mounted for the container's whole life, and removed at teardown, with a
new failure mode of its own (a volume orphaned when teardown is interrupted)
in exchange for nothing the container's own filesystem does not already give.
`PutArchive` into the container needs no volume, no mount, and no removal step.

A volume is also not usable as a write target without a container: a named
volume's contents can only be populated through a container that mounts it, so
seeding one means either a helper container or the same `PutArchive` call into
the task container. The volume adds a step rather than removing one.

### Why not an executor-profile storage setting

The issue offers a `Storage: host directory | Docker volumes` profile setting as
a fallback if changing the default is undesirable. This design does not add one.

Host-directory storage has no advantage to preserve. It is not faster, it is not
more durable, and nothing about it is visible to a user except the leftover
files this capability exists to stop creating. A setting would keep two delivery
paths, two teardown paths, and two sets of remote-host prerequisites alive
permanently, so that one of them could remain the worse option. `AC-…-002.12`
states the single model as a requirement rather than a default.

### What the remote host is still used for

After this change the remote SSH connection carries exactly three kinds of
traffic: the platform probe (`uname`), the daemon transport
(`docker system dial-stdio`), and `direct-tcpip` forwards to published agentctl
ports. `sftpUploadBytes`, `expandRemoteHome`, and the `mkdir -p` exec disappear
from this runtime's launch path. The SSH executor keeps all of them; they are
its own contract, not shared code this design removes.

## Where delivery happens

`ContainerManager.createAndStartContainer` already separates creation from
start:

```text
buildContainerConfig -> CreateContainer -> StartContainer -> GetContainerIP -> resolveContainerEndpoint
```

A created-but-not-started container has a filesystem the daemon will extract
into, so the seam is exactly the gap between those two calls. The design adds
one optional hook to `ContainerManager`:

```go
// seedCreatedContainer delivers container inputs into a created container
// before it is started. Nil for a daemon that shares the backend's
// filesystem, where the inputs are bind-mounted instead.
seedCreatedContainer func(ctx context.Context, containerID string, config ContainerConfig) error
```

When the hook returns an error, the launch removes the container it just
created and fails, reusing `removeContainerBestEffort`. Nothing is started, so
no agent observes a partially delivered container.

The resume path needs the same treatment for `agentctl` alone.
`DockerExecutor.ensureContainerRunning` starts a preserved container that is
stopped; a container preserved across a backend upgrade would otherwise resume
on the helper that was current when it was created. A second optional hook,
`beforeContainerStart func(ctx, containerID) error`, is set on the delegate the
remote runtime constructs for reconnect and delivers only the helper binaries.
Credentials are not re-seeded: the container still holds the directory seeded at
launch, which is also what the bind-mounted design did.

## agentctl and the E2E mock agent

`AgentctlResolver` already selects a platform-matched helper from the backend's
build tree, and `SSHProbeRemote` already reports the remote platform. Both stay.
What changes is the destination: instead of `sftpUploadBytes` to
`~/.kandev/bin/agentctl` followed by a bind mount, the bytes go into a tar entry
for `/usr/local/bin/agentctl` with mode `0755` and are extracted into the
created container.

`/usr/local/bin/agentctl` stays the in-container path. It is a constant with
consumers well outside this runtime — the container startup script, the
`kandev.agentctl.install` script-engine placeholder, and the GitHub credential
helper path handed to the agent — so a per-runtime path would be a contract
change for user-authored prepare scripts. Delivering to the same path keeps
every one of those consumers correct by construction.

The unsupported-platform check keeps its current position: it runs before any
delivery, so an unsupported remote fails with its own cause rather than
producing a container whose helper cannot execute.

The E2E mock-agent binary follows the identical path to
`/usr/local/bin/mock-agent`, and still resolves to nothing in production.

### The content-hash cache is dropped

`ensureAgentctlOnHost` compares a remote `agentctl.sha256` marker against the
local binary and skips the upload when they match, so a second task on the same
host transfers nothing. Both the marker and the binary live on the remote host
filesystem, so that cache cannot survive this change.

The cost is one helper-sized transfer per container start, over the SSH-tunneled
Engine API. This is the Sprites executor's existing behavior — it writes
`agentctl` into every environment it provisions rather than caching it — so it
is a known-acceptable shape rather than a new one, and it is bounded by
container starts, not by prompts or turns.

Restoring a cache would mean a content-addressed named volume mounted at a
directory, and therefore a second in-container helper path, which is the
contract change the previous section rejects. It is recorded as an open question
rather than designed here.

## Session directory and credentials

`remoteContainerHostFiles.SessionDir` currently creates the directory on the
remote host and returns it as a bind-mount source. It now returns no source at
all: the directory is created by the seeded archive, in the container's own
filesystem, at the target `CommandBuilder.GetSessionDirTarget(ag)` reports. An
agent with no declared target still gets nothing, as today.

`ContainerHostFiles.SessionDir` keeps its `(source, target, error)` shape. The
remote implementation returning an empty source with a non-empty target is the
signal that the directory is delivered rather than mounted, and `expandMounts`
adds a mount only when both are non-empty, which it already requires.

Seeding reuses `UploadCredentialFiles` and `UploadPortableConfigBundles`
unchanged. Both write through the `FileUploader` interface, whose whole surface
is `WriteFile(ctx, path, data, mode)`, so the remote Docker path swaps
`sshFileUploader` for a `tarFileUploader` that accumulates entries in memory and
is flushed as one `CopyToContainer` call. Credential mode stays
`credentialFileMode` (`0600`); the tar carries it, and the daemon's extraction
preserves it.

Ownership: entries are written with uid/gid `0`. The Docker executor's
containers run as root, which is what makes the current bind-mounted directory
(owned by the remote SSH user) readable at all. A future image that drops
privileges would need the archive's uid/gid to follow the image's configured
`User`; that is a change to this seam, not to its callers, and is noted as a
risk rather than built speculatively.

The archive is extracted at `/`, with an explicit directory entry for each path
component of the session-directory target. `PutArchive` requires its destination
to exist, and `/` always does, so intermediate directories are created by the
archive itself rather than by a prior exec.

## Failure handling

The split is the one the local Docker path already draws, and
`AC-…-002.7` fixes it:

| Failure | Result |
| --- | --- |
| helper resolution or unsupported platform | launch fails before the container is created |
| helper delivery | launch fails, container removed |
| credential seeding | warning on the launch step, launch continues |
| portable config bundle | existing `PortableConfigWarning` path, launch continues |

A credential failure is tolerated because some agents authenticate from the
environment or their in-container setup script; a missing helper leaves nothing
for agentctl to be, so it is fatal.

## Teardown

A container's removal removes its filesystem, so the session directory and every
credential in it go with the container that `stopDockerContainer` already
removes on a terminal stop. That satisfies `AC-…-002.9` for anything provisioned
by this design, with no new cleanup step, and it closes the asymmetry with
`DockerExecutor.StopInstance` that the issue reports.

One explicit removal remains. A container provisioned before this change
bind-mounts `<remote-kandev-home>/agent-sessions/<instance-id>/`, and that
directory survives the container. `RemoteDockerExecutor.StopInstance` therefore
performs one best-effort `rm -rf` of exactly that path over the live SSH
connection on a terminal stop, before the session is released. It is guarded so
that:

- it runs only for the stop reasons `shouldRunExecutorCleanup` accepts, the same
  predicate the local Docker executor uses;
- the path is composed from the runtime's own constant and the instance ID, and
  shell-quoted, never taken from stored metadata;
- a failure is logged and does not fail the stop, because stop runs inside
  archive and delete.

This is not a migration step to remove later. It is the correct teardown for any
container whose session directory is a host bind mount, and such containers can
be resumed indefinitely.

## Local Docker is untouched

`localContainerHostFiles` and `cm.seedCreatedContainer == nil` are the local
path. It keeps the backend-home session directory, the two bind mounts, and the
`CleanupAgentSessionDir` call on destructive stop. The existing mount-set
characterization test is the guard.

## Test strategy

Unit coverage:

- the remote host-file provider returns no mount source for the session
  directory and no remote path for either helper;
- the tar uploader produces the expected entry set, modes, and directory
  components for a representative agent's credential and bundle layout;
- the create/seed/start ordering, including that a seeding error removes the
  container and starts nothing;
- the reconnect hook re-delivers the helper before starting a stopped container
  and does not re-seed credentials;
- terminal-stop removal of the legacy remote directory fires for exactly the
  reasons `shouldRunExecutorCleanup` accepts, and a resumable stop does not
  remove it.

Integration coverage: the existing `remote_docker_integration_test.go` runs
against a real daemon and is where `PutArchive` extraction into a created
container is proven, including that the helper is executable before start.

E2E coverage: the `containers` Playwright project's remote Docker scenario
gains an assertion that no `~/.kandev` tree is created on the sshd fixture host
across a full launch, stop, resume, and delete cycle.

## Open questions

- Whether to restore a content-addressed `agentctl` cache as a named volume
  once a second in-container helper path is acceptable. The transfer cost is a
  helper-sized payload per container start over the user's link.
- Whether `PutArchive` should receive a gzip-compressed tar. The daemon
  decompresses transparently, which would cut the helper transfer substantially,
  but the Go client sets no content encoding, so it relies on daemon sniffing
  and is left out of the first implementation.
- Whether images that run the agent as a non-root user should set the archive's
  uid/gid from the image's configured `User`. No shipped image does today.
