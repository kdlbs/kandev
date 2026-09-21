---
id: "01-primary-network"
title: "Resolve and validate the primary container network"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-DOCKER-NETWORKS-001
acceptance_criteria:
  - AC-EXECUTORS-DOCKER-NETWORKS-001.1
  - AC-EXECUTORS-DOCKER-NETWORKS-001.2
  - AC-EXECUTORS-DOCKER-NETWORKS-001.3
  - AC-EXECUTORS-DOCKER-NETWORKS-001.4
  - AC-EXECUTORS-DOCKER-NETWORKS-001.5
  - AC-EXECUTORS-DOCKER-NETWORKS-001.6
  - AC-EXECUTORS-DOCKER-NETWORKS-001.7
system_design:
  - ../../specs/executors/system-design/docker-container-networks.md
---

# Task 01: Resolve and Validate the Primary Container Network

## Summary

A task container is created on the network its executor profile names. A
`local_docker` profile that names none falls back to `docker.defaultNetwork`; a
`remote_docker` profile falls back to the remote daemon's default. The
install-wide default becomes empty, so an install that configures nothing
behaves exactly as it does today. A primary network that cannot publish ports,
or that does not exist, fails the launch with a message naming the profile
field and the reason, before any container is created.

## In scope

- Add the `docker_network` and `docker_network_gw_priority` profile config keys
  and project them into launch metadata as **authoritative** keys in
  `profileConfigAuthoritativeKeys`, so a task's own metadata cannot override the
  profile's placement, including when the profile value is empty.
- Replace `ContainerManager.networkName` with a resolved network plan carrying
  the primary network name and its optional gateway priority. Resolve the plan
  on the launch path in `buildDockerContainerConfig`, with the install-wide
  fallback applied for `local_docker` and deliberately omitted for
  `remote_docker`.
- Change `docker.defaultNetwork`'s default from `kandev-network` to empty in
  `config.go` and `catalog.go`, and update its row and sample YAML comment in
  `docs/public/configuration.md` and `docs/configuration.md` to describe real
  behavior.
- Add `Client.InspectNetwork` over `moby/moby/client`'s `NetworkInspect`, and
  validate the primary network before `ContainerCreate`: reject `host`, `none`,
  `default`, and `container:` forms on the string alone; reject a missing
  network; reject the `macvlan`, `ipvlan`, and `null` drivers with an error
  pointing at the additional-networks list.
- Add the optional primary `NetworkEndpoint` to `docker.ContainerConfig` and
  build a one-entry `network.NetworkingConfig` from it in `CreateContainer`,
  returning `nil` when absent so the no-priority path keeps today's arguments.
- Narrow `Client.GetContainerIP` to the primary network's endpoint when a
  primary network is named, so the local resolver's fallback cannot return a
  map-ordered address from a future secondary attachment.

## Out of scope

- Additional network attachments and `NetworkConnect` (task 02).
- Any profile editor or locale change (task 03).
- End-to-end coverage (task 04).
- Creating, removing, or inspecting networks for any purpose other than
  primary-network validation.

## Acceptance

- A `local_docker` profile with `docker_network: lab-bridge` creates its task
  container on `lab-bridge`; with no profile value it uses
  `docker.defaultNetwork`; with both empty it sets no `NetworkMode` and the
  daemon default applies. A `remote_docker` profile with no profile value sets
  no `NetworkMode` regardless of `docker.defaultNetwork`.
- A task that supplies `docker_network` in its own launch metadata does not
  change the container's network when a profile applies.
- A primary network naming a network mode, a missing network, or a
  non-port-publishing driver fails the launch before create, with an error
  naming the profile field, the network, and the reason.

## Verification

```sh
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/... ./internal/common/config/... ./internal/orchestrator/executor/...
cd apps/backend && gofmt -l ./internal/agent ./internal/common/config ./internal/orchestrator/executor
make -C apps/backend lint
```

New tests must cover: the three-step local resolution order; the omitted
install-wide step for `remote_docker`; authoritative precedence over task
metadata including the empty-profile-value case; each rejection in the
validation table; and that a container created with no configured priority
passes a `nil` `NetworkingConfig`.

## Likely files

- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/docker_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_remote_docker.go`
- `apps/backend/internal/agent/docker/client.go`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/catalog.go`
- `apps/backend/internal/orchestrator/executor/executor_state.go`
- `docs/public/configuration.md`, `docs/configuration.md`

## Dependencies and risks

`NewContainerManager`'s signature changes, so every caller and test helper that
passes the current `networkName` string is touched. The local executor caches
one `ContainerManager` in `ensureClient` while the profile is per-launch, so the
per-launch part of the plan must travel on the launch path and not be baked into
the cached manager; baking it in would give every task the first task's network.

Use `/docs-maintainer` for the public configuration change: the
`docker.defaultNetwork` row currently promises the opposite of the new behavior.
