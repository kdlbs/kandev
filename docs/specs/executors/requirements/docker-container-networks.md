---
status: active
system: executors
created: 2026-09-19
owners:
  - kandev
---

# Docker Container Network Selection Requirements

## Overview

A task container launched by the Local Docker or Remote Docker executor is
always attached to the daemon's default bridge. An operator cannot place it on
a user-defined network, so a task agent cannot resolve sibling services by DNS
name, cannot be confined to an `internal` network that denies egress, and
cannot be given an L2 presence on the daemon host's physical LAN.

The install-wide `docker.defaultNetwork` configuration key exists, is
documented as a compatibility field that is "not wired into current executor
networking", and is read by nothing. It is the only setting whose name suggests
this capability, and it does not provide it.

This capability makes container network placement an explicit, observable part
of an executor profile, and makes `docker.defaultNetwork` a real install-wide
default for the Local Docker executor.

Kandev reaches a task container's `agentctl` through a published host port. The
Local Docker executor reads the published port from the container's port
bindings; the Remote Docker executor reads the same bindings and forwards that
remote port back to the backend's loopback. A Docker `macvlan` or `ipvlan`
network ignores published ports, so a container whose only attachment is such a
network has no reachable `agentctl` endpoint. The L2 case therefore requires a
container attached to more than one network: a port-publishing network that
carries `agentctl`, plus one or more additional attachments that provide the
LAN presence.

## Terms

- **Primary network.** The single network a container is created on. It
  provides the container's `NetworkMode` and is the attachment Docker publishes
  container ports on.
- **Additional network.** A further network the container is attached to beyond
  the primary one. Additional attachments do not publish ports.
- **Port-publishing network.** A network whose Docker driver honours published
  container ports. `bridge` and `overlay` do; `macvlan`, `ipvlan`, `none`, and
  `host` do not, or do not in a way Kandev's endpoint resolution can read.
- **Gateway priority.** The Docker endpoint property that selects which of a
  container's attachments provides its default route. The attachment with the
  highest value wins.

## Requirements

### REQ-EXECUTORS-DOCKER-NETWORKS-001: Container network selection

**Intent:** An operator shall be able to choose which Docker network a task
container is created on, per executor profile and as an install-wide default
for the local daemon, and the chosen network shall take effect without breaking
the backend's ability to reach `agentctl`.

As an operator, I want to place task containers on a network I control, so that
an agent can reach the sibling services, or be denied the egress, that my
deployment requires.

#### Acceptance criteria

- **AC-EXECUTORS-DOCKER-NETWORKS-001.1:** A `local_docker` or `remote_docker`
  executor profile shall accept a primary network name. When the profile names
  one, every task container launched on that profile shall be created on that
  network.
- **AC-EXECUTORS-DOCKER-NETWORKS-001.2:** When a `local_docker` profile names no
  primary network, the install-wide `docker.defaultNetwork` value shall be used.
  When that value is also empty, the container shall be created on the daemon's
  default network, which is the behavior shipped before this capability.
- **AC-EXECUTORS-DOCKER-NETWORKS-001.3:** The default value of
  `docker.defaultNetwork` shall be empty. An installation that has never
  configured a network shall observe unchanged container networking after
  upgrading.
- **AC-EXECUTORS-DOCKER-NETWORKS-001.4:** The install-wide
  `docker.defaultNetwork` value shall not select, override, or contribute to a
  `remote_docker` profile's container network. A `remote_docker` profile that
  names no primary network shall use the remote daemon's default network.
- **AC-EXECUTORS-DOCKER-NETWORKS-001.5:** A profile's network selection shall be
  authoritative over any network value supplied in a task's launch metadata,
  including when the profile's value is empty.
- **AC-EXECUTORS-DOCKER-NETWORKS-001.6:** A primary network shall be rejected
  when it is not a port-publishing network, when it names a network mode rather
  than a network (`host`, `none`, `bridge` as a mode, or a `container:` form),
  or when it does not exist on the target daemon. The rejection shall name the
  profile field, the network name, and the reason, and shall be reported before
  or in place of a launch, never as a container that starts and cannot be
  reached.
- **AC-EXECUTORS-DOCKER-NETWORKS-001.7:** A container created on a named primary
  network shall still publish its `agentctl` port, and the backend shall resolve
  and reach that endpoint through the same published-port path it uses today,
  for both the local and the remote daemon.

### REQ-EXECUTORS-DOCKER-NETWORKS-002: Additional network attachments

**Intent:** An operator shall be able to attach a task container to further
networks beyond its primary one, with explicit control over which attachment
provides the container's default route, so that an L2 attachment can coexist
with a port-publishing network.

As an operator running hardware-in-the-loop or security-testing work, I want the
task container to hold an address on my physical LAN while Kandev still reaches
its agent, so that the agent can talk to a physical device as a first-class host
on that segment.

#### Acceptance criteria

- **AC-EXECUTORS-DOCKER-NETWORKS-002.1:** A `local_docker` or `remote_docker`
  executor profile shall accept an ordered list of additional networks. Each
  entry shall carry a network name and an optional gateway priority.
- **AC-EXECUTORS-DOCKER-NETWORKS-002.2:** Every configured additional network
  shall be attached to the container before the container is started, so that
  the agent process observes every interface for the whole of its life.
- **AC-EXECUTORS-DOCKER-NETWORKS-002.3:** An additional network shall be
  attachable regardless of its driver, including `macvlan` and `ipvlan`. The
  port-publishing restriction applies only to the primary network.
- **AC-EXECUTORS-DOCKER-NETWORKS-002.4:** When a gateway priority is configured
  for the primary network or for an additional network, it shall be applied to
  that attachment's endpoint, so that the operator selects which attachment
  provides the container's default route. When no priority is configured
  anywhere, Docker's own selection shall be left unchanged.
- **AC-EXECUTORS-DOCKER-NETWORKS-002.5:** A duplicate network name across the
  primary and additional entries shall be rejected with a message naming the
  duplicate.
- **AC-EXECUTORS-DOCKER-NETWORKS-002.6:** When an additional attachment fails,
  the launch shall fail with an error naming the network and the daemon
  failure, and the partially attached container shall be removed rather than
  left running with an incomplete network configuration.
- **AC-EXECUTORS-DOCKER-NETWORKS-002.7:** When no additional networks are
  configured, no network attachment call shall be made, and container creation
  shall be byte-identical to the single-network case.

### REQ-EXECUTORS-DOCKER-NETWORKS-003: Network configuration surface

**Intent:** An operator shall be able to read and change a profile's container
network placement from the executor profile editor, and shall be able to see
which network a running task container is on.

#### Acceptance criteria

- **AC-EXECUTORS-DOCKER-NETWORKS-003.1:** The executor profile editor shall
  present a network section for `local_docker` and `remote_docker` profiles
  only, containing the primary network name and the additional network list
  with a gateway priority per entry.
- **AC-EXECUTORS-DOCKER-NETWORKS-003.2:** For a `local_docker` profile whose
  primary network is empty, the editor shall state which install-wide default
  applies, including that the default is the daemon's own default network when
  `docker.defaultNetwork` is unset.
- **AC-EXECUTORS-DOCKER-NETWORKS-003.3:** For a `remote_docker` profile whose
  primary network is empty, the editor shall state that the remote daemon's
  default network applies and that the install-wide value does not.
- **AC-EXECUTORS-DOCKER-NETWORKS-003.4:** Saving a profile with an empty primary
  network and an empty additional list shall persist no network configuration,
  and shall leave an existing profile's launch behavior unchanged.
- **AC-EXECUTORS-DOCKER-NETWORKS-003.5:** All network section copy shall be
  localized in every shipped locale, with no string compared by equality and no
  translation call at module scope.

## Exclusions

- Creating, removing, or otherwise managing Docker networks. Kandev attaches
  containers to networks an operator has already created on the daemon.
- Per-endpoint IP address, MAC address, alias, driver option, or subnet
  assignment. Only the network name and the gateway priority are configurable.
- The Kubernetes, SSH, Sprites, and Standalone executors. Their network models
  are owned elsewhere and are unchanged.
- `docker.tlsVerify` and `docker.volumeBasePath`, which remain documented
  compatibility fields.
- Egress filtering as a product feature. An `internal` network is one thing an
  operator can select, not a Kandev-enforced policy; the Sprites network policy
  rules remain the separate, unrelated mechanism.
- Container-to-container discovery guarantees. Kandev does not promise a DNS
  name for a task container on the selected network.
