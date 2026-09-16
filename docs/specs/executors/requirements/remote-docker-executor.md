---
status: active
system: executors
created: 2026-09-16
owners:
  - kandev
---

# Remote Docker Executor Requirements

## Overview

Kandev can give a task a container boundary, and it can give a task a remote
host, but not both unless the remote host runs Kubernetes.

- `local_docker` provisions a container, but only on the daemon reachable from
  the machine running the backend. The daemon address comes from install-wide
  `docker.host`; the per-executor `docker_host` field is not consumed by the
  runtime.
- `ssh` reaches an owned host, but the agent runs there as an ordinary process
  in a task directory. There is no image, no per-task filesystem reset, and
  every task shares the host's installed toolchain.
- `k8s` provisions remote containers and is independent of where the control
  plane runs, so a single-node cluster already satisfies the functional need.
  It requires a storage provisioner, namespace, RBAC, admission review, and a
  worker image built and pushed to a registry by digest.

The gap this capability closes is the one-machine case: a user with a single
remote Linux box who wants a container boundary, is not running Kubernetes, and
wants to keep the Docker executor's in-profile image build loop.

`remote_docker` exists as a registered executor type whose `CreateInstance` and
`StopInstance` return `remote_docker runtime is not yet implemented`. The
capability below replaces that stub. It does not introduce a new executor enum
value.

## Requirements

### REQ-EXECUTORS-REMOTE-DOCKER-001: Remote Docker Executor

**Intent:** A user shall be able to run a task inside a container on a Docker
daemon hosted on another machine reached over SSH, with the task experience,
image build loop, and lifecycle semantics of the Local Docker executor.

#### Acceptance criteria

- **AC-EXECUTORS-REMOTE-DOCKER-001.1:** A `remote_docker` executor profile shall
  store its daemon connection on the profile. The install-wide `docker.host`
  value shall not select, override, or contribute to a `remote_docker` profile's
  daemon, and a `local_docker` profile's behavior shall be unchanged.
- **AC-EXECUTORS-REMOTE-DOCKER-001.2:** The daemon transport shall be SSH,
  addressed as a host or an OpenSSH client-configuration alias, and shall reuse
  the SSH executor's target resolution, authentication, and host-key pinning.
  When a `host_alias` is configured, `HostName`, `Port`, `User`,
  `IdentityAgent`, `IdentityFile`, and one `ProxyJump` shall be inherited, and
  explicit form values shall win.
- **AC-EXECUTORS-REMOTE-DOCKER-001.3:** A `tcp://` daemon transport is out of
  scope. The system shall not accept a `tcp://`, `unix://`, or `npipe://` value
  as a `remote_docker` profile daemon address.
- **AC-EXECUTORS-REMOTE-DOCKER-001.4:** Profile creation shall be gated on a
  connection test that reports, as distinct per-step results: SSH reachability,
  the observed host-key fingerprint, Docker daemon reachability through that
  connection, the daemon API version, and the remote architecture. Save shall
  be enabled only after the user explicitly trusts the reported fingerprint.
- **AC-EXECUTORS-REMOTE-DOCKER-001.5:** After save, the trusted fingerprint
  shall be stored on the executor. A fingerprint mismatch on any later
  connection shall be a hard error with no silent re-pin.
- **AC-EXECUTORS-REMOTE-DOCKER-001.6:** The profile's image tag and Dockerfile
  shall build on the remote daemon through the existing profile build action,
  so the basic case requires no external registry.
- **AC-EXECUTORS-REMOTE-DOCKER-001.7:** A task launched on a `remote_docker`
  profile shall run its agent in a container on the remote daemon, with its
  repositories cloned inside that container at `/workspace`, matching the Local
  Docker clone-inside-container model.
- **AC-EXECUTORS-REMOTE-DOCKER-001.8:** No backend-host filesystem path shall be
  bind-mounted into a remote container. The `agentctl` helper, the per-instance
  agent session directory, and any seeded agent configuration shall be
  materialized on the remote host or inside the container.
- **AC-EXECUTORS-REMOTE-DOCKER-001.9:** The `agentctl` helper delivered to the
  remote shall match the remote architecture reported by the connection test.
  An unsupported or mismatched remote platform shall fail the launch with that
  cause named, rather than starting a container that cannot run the helper.
- **AC-EXECUTORS-REMOTE-DOCKER-001.10:** The backend shall reach the container's
  `agentctl` control and instance endpoints through SSH port forwards to the
  ports published on the remote daemon host's loopback. No backend component
  shall dial a container IP or a remote loopback address directly.
- **AC-EXECUTORS-REMOTE-DOCKER-001.11:** Chat, terminals, the file tree, diffs,
  and Git operations shall behave as they do for a Local Docker session.
- **AC-EXECUTORS-REMOTE-DOCKER-001.12:** Stop shall preserve the remote
  container, and a later resume shall reconnect to it, re-establishing the SSH
  connection and forwards. **Reset Environment**, archive, and delete shall
  remove the container on the remote daemon.
- **AC-EXECUTORS-REMOTE-DOCKER-001.13:** When the SSH connection to the daemon
  drops while a session is live, the session shall surface a transport failure
  rather than appearing healthy, consistent with SSH transport liveness.
- **AC-EXECUTORS-REMOTE-DOCKER-001.14:** Launch and connection-test failures
  shall be reported with distinct causes for: SSH unreachable, host-key
  fingerprint mismatch, the SSH user lacking Docker socket access, the daemon
  unreachable through an established SSH connection, an unsupported remote
  architecture, and `agentctl` delivery failure.
- **AC-EXECUTORS-REMOTE-DOCKER-001.15:** Local Git repository sources and
  arbitrary host folders shall be rejected for `remote_docker` tasks, matching
  the existing Local Docker and remote-executor source rules.
- **AC-EXECUTORS-REMOTE-DOCKER-001.17:** Creating, editing, testing, and
  building a `remote_docker` executor shall require an administrator. A saved
  profile grants effective root on the remote host, so it is not an ordinary
  member operation.
- **AC-EXECUTORS-REMOTE-DOCKER-001.16:** The profile editor shall state that a
  remote Docker profile grants effective root on the remote host, because the
  SSH user must reach the Docker socket and Dockerfile instructions execute with
  the daemon's authority.

## System design

See [Remote Docker Executor system design](../system-design/remote-docker-executor.md).
