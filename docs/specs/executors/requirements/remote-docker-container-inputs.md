---
status: active
system: executors
created: 2026-09-19
owners:
  - kandev
---

# Remote Docker Container Inputs Requirements

## Overview

The remote Docker executor keeps the backend's filesystem out of its containers,
but it does so by writing to the **remote host's** filesystem instead of using
the Docker Engine API it already speaks for everything else.

Four things land under `~/.kandev` on the remote account today:

1. `~/.kandev/bin/agentctl` plus its `agentctl.sha256` cache marker, uploaded
   over SFTP and bind-mounted at `/usr/local/bin/agentctl`;
2. `~/.kandev/bin/mock-agent`, the same mechanism, in E2E builds only;
3. `~/.kandev/agent-sessions/<instance-id>/`, created with `mkdir -p` over an
   SSH exec channel and bind-mounted at the agent's session-directory target;
4. inside that directory, the agent's login and credential files and any
   selected portable configuration bundles, uploaded over SFTP.

Two consequences follow, and this capability owns both.

**The host must accept writes.** A deliberately minimal or immutable host, where
the Docker daemon is intended to be the only management surface, has no
appropriate location for a persistent `~/.kandev` tree. The current design also
requires the remote account to have a writable home directory and the remote SSH
server to expose an SFTP subsystem. Neither is guaranteed on an appliance-style
host, and neither is needed by any other part of this runtime: the daemon
connection is an SSH exec channel running `docker system dial-stdio`, and the
agentctl endpoints are `direct-tcpip` forwards.

**The credentials are never removed.** The local Docker executor removes
`<kandev-home>/agent-sessions/<instance-id>/` on a destructive stop. The remote
Docker executor has no equivalent: `StopInstance` releases the session and stops
the container, and nothing removes the remote directory. Every task therefore
leaves a directory holding that agent's credential files on the remote host,
accumulating without bound, including after task archive and delete, which is
exactly when a user expects those credentials to be gone.

## Terminology

- **Container input:** a file or directory a task container needs that is not
  content of its image. For this runtime they are the `agentctl` helper, the
  E2E mock-agent helper, the per-instance agent session directory, and the
  credential and portable-configuration files seeded into that directory.
- **Remote host filesystem:** any path on the machine running the remote Docker
  daemon that is reached other than through the Docker Engine API. Paths inside
  Docker-managed storage (a container's own filesystem, a Docker volume) are
  not remote host filesystem for this purpose, even though the daemon
  ultimately stores them on that machine's disks.
- **Terminal stop:** a stop whose reason is **Reset Environment**, archive,
  delete, or a cascade of one of those. These are the reasons for which the
  local Docker executor removes the container and its session directory.
- **Resumable stop:** every other stop, including an ordinary user stop, a
  failed-agent stop, and backend shutdown. These preserve the container.

## Requirements

### REQ-EXECUTORS-REMOTE-DOCKER-002: Deliver remote Docker container inputs through the Docker Engine API

**Intent:** A remote Docker task shall obtain every container input through the
Docker daemon, so the runtime works on a host that accepts no filesystem writes
and exposes no SFTP subsystem, and so the container's removal removes the
agent's credentials with it.

**User story:** As an operator whose remote host is a minimal immutable OS where
Docker is the only management surface, I want Kandev to run tasks there without
creating a persistent tree in my remote home directory and without leaving agent
credentials at rest on that machine, so that the host stays as I configured it.

#### Acceptance criteria

- **AC-EXECUTORS-REMOTE-DOCKER-002.1:** Launching, resuming, or stopping a
  `remote_docker` task shall not create, write, modify, or delete any path on
  the remote host filesystem, except for the legacy removal required by
  `AC-EXECUTORS-REMOTE-DOCKER-002.8`.
- **AC-EXECUTORS-REMOTE-DOCKER-002.2:** The runtime shall require of the remote
  SSH service only an exec channel, for `docker system dial-stdio` and the
  platform probe, and `direct-tcpip` forwarding, for the agentctl endpoints. A
  remote account whose home directory is read-only, and a remote SSH server that
  exposes no SFTP subsystem, shall both support a successful launch, resume, and
  terminal stop.
- **AC-EXECUTORS-REMOTE-DOCKER-002.3:** The `agentctl` helper matching the
  probed remote platform shall be present and executable at the container's
  expected helper path before the container's entrypoint runs. No bind mount
  shall supply it.
- **AC-EXECUTORS-REMOTE-DOCKER-002.4:** The per-instance agent session directory
  shall exist only inside the container, at the agent's declared
  session-directory target, with no bind mount and no path on the remote host
  filesystem corresponding to it. An agent that declares no session-directory
  target shall get no such directory, matching the local Docker path.
- **AC-EXECUTORS-REMOTE-DOCKER-002.5:** The agent's credential files and
  selected portable configuration bundles shall be delivered into that
  in-container directory with the same relative layout and the same restrictive
  file mode the local Docker path uses, and shall be readable by the account the
  container's agent runs as.
- **AC-EXECUTORS-REMOTE-DOCKER-002.6:** Container inputs shall be delivered
  after the container is created and before it is started, on a fresh launch and
  on a resume that restarts a preserved container. A resume shall re-deliver the
  `agentctl` helper, so a container preserved across a backend upgrade does not
  resume on a stale helper. A resume shall not re-seed credentials into a
  container that already holds a seeded session directory.
- **AC-EXECUTORS-REMOTE-DOCKER-002.7:** A failure to deliver `agentctl` shall
  fail the launch, naming that cause, and shall leave no container behind. A
  failure to seed credentials or configuration bundles shall be reported as a
  launch warning and shall not abort the launch, matching the local Docker path.
- **AC-EXECUTORS-REMOTE-DOCKER-002.8:** On a terminal stop the system shall
  remove the remote host directory
  `<remote-kandev-home>/agent-sessions/<instance-id>/` when it exists, so a
  container provisioned before this capability does not leave its credentials
  behind. Failure to remove it shall be reported and shall not fail the stop.
- **AC-EXECUTORS-REMOTE-DOCKER-002.9:** After a terminal stop completes, no
  credential or configuration file seeded for that instance shall remain
  anywhere on the remote machine, neither on the remote host filesystem nor in
  Docker-managed storage.
- **AC-EXECUTORS-REMOTE-DOCKER-002.10:** A resumable stop shall preserve the
  container together with its delivered inputs, and the following resume shall
  reach the same seeded session directory.
- **AC-EXECUTORS-REMOTE-DOCKER-002.11:** Local Docker launches shall be
  unchanged: the mount set, the session-directory location under the backend's
  Kandev home, and the existing destructive-stop cleanup of that directory all
  keep their current behavior.
- **AC-EXECUTORS-REMOTE-DOCKER-002.12:** No executor-profile field, install
  setting, or runtime flag shall select between remote storage models. Container
  inputs are delivered one way.

## System design

See [Remote Docker container inputs system design](../system-design/remote-docker-container-inputs.md).

## Relationship to the Remote Docker Executor requirements

This capability tightens
[`AC-EXECUTORS-REMOTE-DOCKER-001.8`](remote-docker-executor.md), which permits a
container input to be materialized "on the remote host or inside the container".
That remains true of the backend's filesystem, which is still never a mount
source. The remote host is no longer an acceptable destination.
