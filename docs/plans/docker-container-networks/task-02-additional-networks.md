---
id: "02-additional-networks"
title: "Additional network attachments and gateway priority"
status: done
wave: 2
depends_on: ["01-primary-network"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-DOCKER-NETWORKS-002
acceptance_criteria:
  - AC-EXECUTORS-DOCKER-NETWORKS-002.1
  - AC-EXECUTORS-DOCKER-NETWORKS-002.2
  - AC-EXECUTORS-DOCKER-NETWORKS-002.3
  - AC-EXECUTORS-DOCKER-NETWORKS-002.4
  - AC-EXECUTORS-DOCKER-NETWORKS-002.5
  - AC-EXECUTORS-DOCKER-NETWORKS-002.6
  - AC-EXECUTORS-DOCKER-NETWORKS-002.7
system_design:
  - ../../specs/executors/system-design/docker-container-networks.md
---

# Task 02: Additional Network Attachments and Gateway Priority

## Summary

A task container can be attached to networks beyond its primary one, each with
an optional gateway priority, so an operator can give the container an L2
presence on a `macvlan` or `ipvlan` network while the primary bridge keeps
carrying the published `agentctl` port and, when the operator says so, the
default route.

## In scope

- Add the `docker_additional_networks` profile config key, a JSON array of
  `{"name": string, "gw_priority": int?}` objects, projected into launch
  metadata as an authoritative key alongside the task 01 keys, parsed on the
  launch path into the resolved network plan.
- Add `Client.ConnectNetwork` over `moby/moby/client`'s `NetworkConnect`, and
  attach every configured additional network inside `createSeedAndStart`, after
  `CreateContainer` and before the container-input seed hook and
  `StartContainer`, applying `network.EndpointSettings.GwPriority` when the
  entry configures one.
- Reject a name duplicated between the primary network and the additional list,
  and a duplicate within the list, naming the duplicate.
- On attachment failure, remove the created container through
  `removeContainerAfterFailure`, the path `createSeedAndStart` already uses for
  a failed seed, and return an error naming the network and wrapping the
  daemon's message. Log a cleanup failure without masking the attachment error.
- Log the resolved primary network and the additional attachment count at info
  level on the container-manager logger; log a validation rejection and an
  attachment failure at error level with the network name.

## Out of scope

- The primary network's own resolution, validation, and endpoint construction
  (task 01, which this builds on).
- Per-endpoint IP, MAC, alias, or driver options. Only name and gateway
  priority are configurable.
- Any profile editor or locale change (task 03).
- Validating an additional network's driver. Any driver is permitted there,
  which is the point of the capability.

## Acceptance

- A profile configuring a bridge primary network and a `macvlan` additional
  network launches a container attached to both, started only after both
  attachments succeed, whose `agentctl` endpoint the backend still resolves
  through its published port on both the local and the remote daemon.
- With a gateway priority configured on the primary endpoint above the macvlan
  entry's, the primary attachment provides the container's default route.
- A failing additional attachment fails the launch with the network named, and
  leaves no container behind.
- With no additional networks configured, no `NetworkConnect` call is made and
  the create arguments are identical to task 01's single-network case.

## Verification

```sh
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/agent/docker/...
cd apps/backend && gofmt -l ./internal/agent
make -C apps/backend lint
```

New tests must cover: parsing a well-formed and a malformed
`docker_additional_networks` value; attachment ordering relative to
`ContainerStart`; `GwPriority` reaching the endpoint settings for both the
primary and an additional entry; duplicate rejection; container removal on
attachment failure, including the case where removal itself fails; and the
zero-additional-network no-call assertion.

## Likely files

- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/docker_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/docker/client.go`
- `apps/backend/internal/orchestrator/executor/executor_state.go`

## Dependencies and risks

Depends on task 01's resolved network plan and `docker.ContainerConfig`
endpoint field. Both new client methods go through the same `*client.Client`, so
the remote executor gets them over its SSH-dialed transport with no extra work;
a test should still exercise the remote path so that stays true.

`GwPriority` requires a daemon new enough to honour it. An older daemon ignores
the field rather than failing, so a configuration that depends on it degrades to
Docker's own default-route selection. Report the daemon's API version in the
attachment log line so a support case can tell the two apart.
