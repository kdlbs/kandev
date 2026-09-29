---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-DOCKER-NETWORKS-001
  - REQ-EXECUTORS-DOCKER-NETWORKS-002
  - REQ-EXECUTORS-DOCKER-NETWORKS-003
created: 2026-09-19
owners:
  - kandev
---

# Docker Container Network Selection System Design

## Purpose and boundaries

Make the network a task container is created on, and the further networks it is
attached to, a resolved property of the executor profile rather than a constant
empty string.

This design owns the resolution order for the network name, the profile
configuration keys, the primary-network validation rule, the additional-network
attachment step and its failure handling, and the profile editor surface.

It does not change the container bootstrap contract, the `agentctl` protocol,
endpoint resolution, image build, or any non-Docker executor.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-…-001.1`, `AC-…-001.2`, `AC-…-001.5` | [Resolution order](#resolution-order) |
| `AC-…-001.3` | [Retiring the install-wide key](#retiring-the-install-wide-key) |
| `AC-…-001.6`, `AC-…-002.5` | [Primary network validation](#primary-network-validation) |
| `AC-…-001.7` | [Published ports are unchanged](#published-ports-are-unchanged) |
| `AC-…-002.1`, `AC-…-002.2`, `AC-…-002.3`, `AC-…-002.7` | [Additional attachments](#additional-attachments) |
| `AC-…-002.4` | [Gateway priority](#gateway-priority) |
| `AC-…-002.6` | [Attachment failure](#attachment-failure) |
| `AC-…-003.1` to `AC-…-003.4` | [Profile editor surface](#profile-editor-surface) |

## Current state

`ContainerManager` already carries a `networkName` field into
`docker.ContainerConfig.NetworkMode`, and `docker.Client` already maps that onto
`container.HostConfig.NetworkMode`. The only reason no container is ever placed
on a named network is that both construction sites pass a literal `""`:

- `DockerExecutor.ensureClient` in `executor_docker.go`.
- `RemoteDockerExecutor.connect` in `executor_remote_docker.go`.

`config.DockerConfig.DefaultNetwork`, its `SetDefault("docker.defaultNetwork",
"kandev-network")`, and its `catalog.go` entry exist and are read by nothing.
They are removed rather than wired; see
[Retiring the install-wide key](#retiring-the-install-wide-key).

The wiring this design needs therefore already exists for the primary network;
what is missing is a resolved value, a validation rule, and the multi-attachment
step.

## Retiring the install-wide key

`docker.defaultNetwork` is removed: the struct field, the `SetDefault` call,
the catalog entry and its audit row, and every public documentation mention.

Wiring it instead was the issue's minimal reading, and it was tried. It does not
survive contact with two daemons. A network name is scoped to the daemon that
owns it, so one install-wide value cannot describe both a `local_docker`
profile's daemon and a `remote_docker` profile's. Honouring it therefore needs a
three-step precedence rule, a local-versus-remote asymmetry rule, a default-value
change (the shipped `kandev-network` exists on no daemon, so passing it through
would fail every container create), and a new admin-only read endpoint so the
profile editor can tell an operator which value is in force. That is a large
surface for a setting whose per-profile equivalent is clearer in every case.

The key had no reader before this change, so nothing depended on it, and
`decodeConfig` calls `v.Unmarshal` without `ErrorUnused`: an installation whose
`config.yaml` still carries the key starts normally and ignores it.

## Resolution order

There is no order. The executor profile's `docker_network` value is the only
source; empty leaves `NetworkMode` unset, which is the daemon's own default and
the behavior that shipped before this capability.

`ContainerManager.networkName` is removed rather than replaced. A local executor
builds one manager and caches it in `ensureClient`, while the profile that names
the network is per-launch, so a network held on the manager would give every
task the first task's network. `buildDockerContainerConfig` resolves the
`containerNetwork` from launch metadata and it travels on the launch's own
`ContainerConfig`.

### Profile configuration keys

Two `executor_profile.config` keys, following the existing
`allow_user_namespaces` and `image_tag` pattern:

- `docker_network`: the primary network name.
- `docker_additional_networks`: a JSON array of
  `{"name": "<network>", "gw_priority": <int>}` objects. `gw_priority` is
  omitted when unset.

A third key, `docker_network_gw_priority`, carries the primary attachment's
gateway priority. It is separate rather than folded into `docker_network` so the
common case stays a plain network name.

Both are projected into launch metadata by
`applyProfileConfigToMetadata` in `internal/orchestrator/executor/executor_state.go`
as **authoritative** keys, added to `profileConfigAuthoritativeKeys` alongside
`MetadataKeyAllowUserNamespaces`. Network placement is a containment boundary: a
task that could supply `docker_network` in its own metadata could move itself off
an `internal` network the profile confined it to, or onto a LAN segment the
profile never granted. The profile value wins unconditionally, including when it
is empty, and `clearAuthoritativeMetadataKeys` blanks it when no profile applies.

That guard covers launch metadata, not who may write the profile. The
agent-facing `create_executor_profile` and `update_executor_profile` MCP tools
take an arbitrary config map, so all three keys also join
`operatorOnlyConfigKeys` in `internal/mcp/handlers`: an agent that could set
them on a profile it creates would be choosing its own containment. The
"operator already exposes `prepare_script`, which is strictly more powerful"
reasoning that keeps other keys off that list does not apply here, because a
prepare script runs inside the container and cannot reach a network the
container was never attached to.

The keys are added to `metadataPassthroughKeys` in `executor_backend.go` so they
survive metadata round-trips, and read on the launch path with the existing
`getMetadataString` helper.

## Primary network validation

The primary network must be one the backend can still reach `agentctl` through.
Validation runs at launch, in the executor, against the daemon that will host the
container, before `ContainerCreate`:

1. Reject a value that names a network *mode* rather than a network: `host`,
   `none`, `default`, and any `container:` prefix. These are rejected on the
   string alone, with no daemon call. `bridge` is not in that set: it names the
   daemon's real default bridge network, which publishes ports.
2. `NetworkInspect` the name. A missing network is reported as a launch failure
   naming the profile field, the network, and the daemon.
3. Reject a driver that does not honour published ports: `macvlan`, `ipvlan`,
   and `null`. The error states that the primary network must publish ports and
   points the operator at the additional-networks list for an L2 attachment.
4. Reject a name that also appears in the additional list.

Validating against the daemon rather than at profile save time is deliberate. A
`remote_docker` profile's daemon is reachable only over an SSH connection the
launch path already establishes, and a network that exists at save time can be
removed before a launch. The editor validates name syntax only.

The failure is a normal launch failure on the existing error path, so the user
sees it in the Executor Settings popover with its cause, in the same place a
failed image pull or a failed prepare script appears. There is no partially
launched container to clean up, because validation precedes creation.

## Published ports are unchanged

`dockerAgentctlPortBindings()` and the `containerEndpointResolver` pair are not
touched. Once the primary network is restricted to a port-publishing driver,
`NetworkSettings.Ports` is populated exactly as it is today, so
`Client.GetContainerHostPort` (local) and `dockerPublishedPorts` +
`remoteEndpointResolver` (remote) keep working unmodified.

`Client.GetContainerIP`, the local resolver's fallback when the published port
cannot be read, iterates `NetworkSettings.Networks` and returns the first
endpoint with a valid address. With more than one attachment that iteration is
map-ordered and therefore non-deterministic, so it could return a macvlan
address the backend cannot route to. `Client.GetContainerIPOn` takes the
preferred network and falls back to any attachment; `GetContainerIP` calls it
with an empty preference, so callers that know no network keep today's
behavior.

## Additional attachments

Additional networks are attached with `NetworkConnect` inside
`createSeedAndStart`, immediately after `CreateContainer` and before the
container-input seed hook and `StartContainer`. Attaching before start means the
agent process sees every interface for the whole of its life.

Attachment precedes the seed rather than following it because the seed extracts
a tar carrying the agent's credentials into the container. A network failure
after that point would have already placed those credentials in a container the
launch is about to delete.

`docker.Client` gains two methods over `moby/moby/client`, which the vendored
`client v0.5.0` already provides:

- `InspectNetwork(ctx, name)`, returning the driver, for validation.
- `ConnectNetwork(ctx, networkID, containerID, endpoint)`, wrapping
  `NetworkConnect`.

Both go through the same `*client.Client`, so the remote executor gets them over
its SSH-dialed transport with no additional work.

When the additional list is empty, neither method is called and the create path
is unchanged, which keeps the single-network case free of new daemon round
trips.

## Gateway priority

`network.EndpointSettings.GwPriority` in `moby/moby/api v1.55.0` is the field
that selects the default-route attachment; the highest value wins.

For an **additional** network, the priority is set on the `EndpointSettings`
passed to `NetworkConnect`.

For the **primary** network, there is no connect call to carry it, so the
container is created with a `network.NetworkingConfig` whose `EndpointsConfig`
holds one entry, keyed by the primary network name, carrying its `GwPriority`.
`HostConfig.NetworkMode` continues to name the same network. This is the only
change to `CreateContainer`'s signature surface: `ContainerConfig` gains an
optional `NetworkEndpoint` describing the primary endpoint, and
`buildNetworkingConfig` returns `nil` when it is absent, so a container with no
configured priority is created with exactly today's arguments.

Without this, an operator cannot stop a macvlan attachment from capturing the
default route, which breaks the return path for inbound connections arriving on
the bridge, including the `agentctl` connection the launch depends on. The
gateway priority is the difference between a working L2 configuration and one
that fails intermittently depending on network names.

## Attachment failure

A failed `NetworkConnect` leaves a created, unstarted container attached to some
but not all of its configured networks. Starting it would give the agent a
silently incomplete network, so the launch fails instead.

`createSeedAndStart` already owns this shape for the seed hook: it calls
`removeContainerAfterFailure` and returns the hook's error unwrapped. The
attachment step reuses both, so a failed attachment removes the container on the
same path a failed seed does. The error names the network that failed and wraps
the daemon's own message. Cleanup failure is logged and does not mask the
original attachment error, which is the one the operator has to act on.

## Profile editor surface

A `DockerNetworkCard` is added to
`apps/web/components/settings/profile-edit/docker-sections.tsx` and rendered
from `profile-runtime-sections.tsx` for `isDocker` profiles, between the
Dockerfile build card and the user-namespaces card.

- A primary network text input, whose empty-state helper text states that the
  daemon's own default network applies. It reads the same for both Docker
  executor types, because both resolve the same way.
- An additional-network list: each row is a name input plus an optional gateway
  priority number input, with add and remove controls. The primary network's own
  gateway priority is an optional field on the primary row.

`buildSaveConfig` in `serialize-executor-config.ts` writes `docker_network` and
`docker_network_gw_priority` with `setTextConfig` and
`docker_additional_networks` with `setJsonConfig`, all gated on `form.isDocker`,
so switching a profile to a non-Docker executor type clears them, matching the
existing treatment of `dockerfile` and `image_tag`.

All copy goes through `t()`; the card carries no string compared by equality,
and the five shipped locales are updated together.

## Persistence and compatibility

No schema change. `executor_profile.config` is an existing string map, and the
SQLite repository already round-trips arbitrary keys and treats an empty value
as absent.

A profile saved before this capability has neither key, resolves to the
install-wide default (local) or the daemon default (remote), and behaves as it
does today. Rolling the backend back leaves both keys in the config map, inert.

An upgraded install observes no behavior change at all until an operator names
a network on a profile. `docker.defaultNetwork` never had a reader, so retiring
it changes nothing that was running.

## Alternatives considered

**Creating a `kandev-network` on startup.** This was the way to keep the
shipped `docker.defaultNetwork` value meaningful. Rejected: it silently moves
every task container onto a user-defined network with embedded DNS, where
containers resolve each other by name. That changes the default isolation
posture of every install, without an operator asking, in service of a value that
never had an effect.

**Passing every network at create through `EndpointsConfig`.** Docker accepts
multiple endpoints at create from API 1.44. It is atomic and avoids the
post-create step. It was rejected because it makes the zero-additional-network
case structurally different from today's create call, it fails as one opaque
create error rather than an error naming the network that failed, and it raises
the minimum daemon API version for a path every Docker task takes. The primary
endpoint still uses `EndpointsConfig`, but only when a gateway priority is
configured, so the common path is unchanged.

**A single install-wide network, with or without a per-profile override.** This
is the issue's minimal reading. Rejected in
[Retiring the install-wide key](#retiring-the-install-wide-key).

**Allowing a macvlan primary network.** Rejected because it produces a container
that starts and is unreachable. The published `agentctl` port is how the backend
talks to the agent at all, so a configuration that discards it is not a
degraded mode, it is a failed launch with a confusing cause.

## Observability

Container creation logs the resolved primary network and the count of additional
attachments at info level, on the existing container-manager logger. A
validation rejection and an attachment failure are logged at error level with
the network name. No new metric: these are launch-path failures already visible
as launch errors, and a counter would not tell an operator anything the error
does not.
