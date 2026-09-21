---
created: 2026-09-19
status: planned
requirements:
  - REQ-EXECUTORS-DOCKER-NETWORKS-001
  - REQ-EXECUTORS-DOCKER-NETWORKS-002
  - REQ-EXECUTORS-DOCKER-NETWORKS-003
system_design:
  - ../../specs/executors/system-design/docker-container-networks.md
legacy_specs: []
---

# Implementation Plan: Docker Container Network Selection

## Overview

Make the Docker network a task container is created on a resolved property of
the executor profile, add additional network attachments with gateway priority
for the L2 case, and surface both in the executor profile editor.

The primary network wiring already exists end to end
(`ContainerManager.networkName` to `HostConfig.NetworkMode`); both construction
sites pass `""`. The work is resolving a value, validating it, adding the
multi-attachment step, and building the editor surface.

## Scope

Task 01 resolves the primary network from the profile and, for `local_docker`
only, from `docker.defaultNetwork`, changes that key's default to empty,
validates the network against the target daemon before create, and updates the
configuration documentation. Task 02 adds additional attachments and gateway
priority on top. Task 03 adds the profile editor surface and the five locale
catalogs. Task 04 adds container-backed end-to-end coverage.

Out of scope for the whole package: creating or removing Docker networks,
per-endpoint IP/MAC/alias/driver options, non-Docker executors, and
`docker.tlsVerify` / `docker.volumeBasePath`.

## Dependency order

| Wave | Work order | Depends on |
| --- | --- | --- |
| 1 | [Task 01: Resolve and validate the primary container network](task-01-primary-network.md) | none |
| 2 | [Task 02: Additional network attachments and gateway priority](task-02-additional-networks.md) | 01 |
| 3 | [Task 03: Executor profile network editor](task-03-profile-editor.md) | 01, 02 |
| 4 | [Task 04: Container end-to-end coverage](task-04-e2e.md) | 03 |

Waves 1 and 2 are sequential because task 02 extends the same resolution
structure and the same `docker.ContainerConfig` that task 01 introduces.

## Compatibility and rollback

`docker.defaultNetwork` changes from `kandev-network` to an empty default in the
same change that makes it effective. Nothing in the tree creates a network named
`kandev-network`, so honouring the shipped default verbatim would fail every
container create; an empty default keeps current behavior exactly and makes the
setting inert until an operator names a network. The public configuration
documentation currently describes the key as a compatibility field that is not
wired in, so no documented behavior is withdrawn.

No schema change. Both new profile keys live in the existing
`executor_profile.config` string map. A rolled-back backend leaves them inert.

## Risks

- **A named primary network that cannot publish ports strands the launch.**
  Mitigated by rejecting `macvlan`, `ipvlan`, `null`, and network-mode values
  before create, with an error naming the profile field (task 01).
- **A macvlan attachment captures the default route** and breaks the return path
  for the inbound `agentctl` connection. Mitigated by making gateway priority
  settable on both the primary and the additional attachments (task 02).
- **`Client.GetContainerIP` iterates `NetworkSettings.Networks` in map order**
  and, with more than one attachment, can return an unroutable address as the
  local resolver's fallback. Narrowed to the primary endpoint in task 01, before
  task 02 makes multiple attachments reachable.
- **Task metadata could override profile network placement**, escaping an
  `internal` confinement or granting LAN presence. Mitigated by making both keys
  authoritative in `profileConfigAuthoritativeKeys` (task 01).
- **Remote daemon divergence.** The install-wide value deliberately does not
  reach a `remote_docker` profile; a test asserts the omission rather than
  leaving it implicit (task 01).

## Verification strategy

Go unit tests own resolution order, authoritative-metadata precedence, the
validation table, endpoint construction, and attachment failure cleanup, with
fake daemon clients as the existing lifecycle tests do. Web unit tests own the
serializer and the form state hook. The Playwright `containers` project owns the
end-to-end evidence that a task container lands on a named network and remains
reachable.

## ASCII UI previews

Task 03 is the only work order that changes rendered UI. Both views below are
sections of `Settings > Executors > <profile>`; the page scrolls, each card is
fixed height for its content.

### UI-01: Docker network card, local_docker profile, desktop

```text
+--------------------------------------------------------------------------+
| Container networks                                                       |
| Choose the Docker networks this profile's task containers attach to.     |
|                                                                          |
| Primary network                                                          |
| +------------------------------------------+  Gateway priority           |
| | lab-bridge                               |  +---------+                |
| +------------------------------------------+  |       0 |                |
| Leave empty to use the install default (docker.defaultNetwork).          |
| This install has no default set, so the daemon's own default applies.    |
| The primary network carries the published agentctl port, so it must be   |
| a network that publishes ports. macvlan and ipvlan belong below.         |
|                                                                          |
| Additional networks                                                      |
| +------------------------------------------+  +---------+  +---+        |
| | lan-macvlan                              |  |     -10 |  | x |        |
| +------------------------------------------+  +---------+  +---+        |
| +------------------------------------------+  +---------+  +---+        |
| | metrics-internal                         |  |         |  | x |        |
| +------------------------------------------+  +---------+  +---+        |
| [ + Add network ]                                                        |
| The highest gateway priority provides the container's default route.     |
+--------------------------------------------------------------------------+
```

Structural requirements: the primary network is a single field distinct from
the list; each additional row is name plus optional priority plus remove; the
port-publishing constraint and the default-route rule are stated in the card,
not only in an error. The empty-state helper line is the `local_docker` variant
of `AC-…-003.2`. Field widths and the placeholder values are illustrative.

### UI-02: Empty-state helper, remote_docker profile

```text
| Primary network                                                          |
| +------------------------------------------+  Gateway priority           |
| |                                          |  +---------+                |
| +------------------------------------------+  |         |                |
| Leave empty to use the remote daemon's default network. The install      |
| setting docker.defaultNetwork does not apply to a remote daemon.         |
```

Only the helper text differs from UI-01; the controls are identical. This is
`AC-…-003.3`.

### UI-03: Docker network card, phone

```text
+--------------------------------+
| Container networks             |
|                                |
| Primary network                |
| +----------------------------+ |
| | lab-bridge                 | |
| +----------------------------+ |
| Gateway priority               |
| +----------------------------+ |
| | 0                          | |
| +----------------------------+ |
| Leave empty to use the         |
| install default. This install  |
| has no default set.            |
|                                |
| Additional networks            |
| +----------------------------+ |
| | lan-macvlan            [x] | |
| | priority                   | |
| | +------------------------+ | |
| | | -10                    | | |
| | +------------------------+ | |
| +----------------------------+ |
| [ + Add network ]              |
+--------------------------------+
```

Structural requirement: on phone each additional network is a stacked card with
its own labelled priority field and an in-card remove control, not a horizontal
row compressed to fit. Follow `/mobile-parity`; every desktop capability is
present.

## Work orders

- [x] [Task 01: Resolve and validate the primary container network](task-01-primary-network.md)
- [x] [Task 02: Additional network attachments and gateway priority](task-02-additional-networks.md)
- [ ] [Task 03: Executor profile network editor](task-03-profile-editor.md)
- [ ] [Task 04: Container end-to-end coverage](task-04-e2e.md)
