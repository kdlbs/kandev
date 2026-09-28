---
id: coordinator-containment-design
title: Containment for unattended turns design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-CONTAINMENT-001
  - REQ-COORDINATOR-CONTAINMENT-002
  - REQ-COORDINATOR-CONTAINMENT-003
---

# Containment for unattended turns System Design

## Purpose and boundaries

This design is the check that admission step 2 of [wake](wake.md#admission)
runs, the settings display of its result, and the denial of permissions
nobody can answer during an unattended turn. It reads executor profiles,
agent profiles, secrets and the auth mode; it changes none of them and adds no
sandboxing to any executor.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-CONTAINMENT-001` | [Check](#check) |
| `REQ-COORDINATOR-CONTAINMENT-002` | [Check](#check), [Settings display](#settings-display) |
| `REQ-COORDINATOR-CONTAINMENT-003` | [Unattended permissions](#unattended-permissions) |

## Why these four conditions

The agent reaches Kandev's own tools through agentctl's local `/mcp`, which
agentctl tunnels over the backend's agent-stream WebSocket; that path carries
the server-derived coordinator principal and the phase 1 guard. Every other
path to Kandev's authority is what containment closes:

| Path from the agent's own tools | Closed by |
| --- | --- |
| REST or external `/mcp` on the backend's port, from a process on the backend's host (`local`, `worktree`) | `executor_isolated` |
| The same from a container through `host.docker.internal` (`local_docker`) | `auth_enabled`: with auth enabled both require a session cookie or PAT |
| Reading or writing Kandev's database and `~/.kandev` files as the backend's OS user | `executor_isolated` |
| A Kandev PAT or Office runtime credential placed in the session's environment | `no_kandev_credential` |
| A second MCP server in the agent profile, including one pointed at Kandev's external `/mcp` with a PAT | `no_extra_tools` |

`ssh` is excluded because the target host can be the backend's own host and
nothing in its configuration proves otherwise. `plugin_remote` is excluded
because its placement is a plugin's choice. Remote Docker and Sprites have no
route to the backend's loopback. `k8s` pods do not share the backend's
filesystem.

## Check

`internal/coordinator/containment.go` exports
`Check(ctx, coordinator) Result`, where `Result` is
`{Contained bool, Conditions []Condition}` and `Condition` is
`{Name string, Met bool, Detail string}`. The conditions are always returned
in this order, and `Contained` is true only when all four are met:

1. **`executor_isolated`.** Load the coordinator's executor profile and its
   executor; met when the executor type is `local_docker`, `remote_docker`,
   `sprites` or `k8s`, or is `mock_remote` while the active runtime profile is
   `e2e`. Detail is the executor type, or `unreadable` when the profile or
   executor cannot be loaded.
2. **`auth_enabled`.** Met when `auth.Service.Mode()` is `ModeEnabled`.
   `ModeDisabled` and `ModeSetup` fail with detail `disabled` or `setup`.
3. **`no_kandev_credential`.** Resolve the launch environment the lifecycle
   manager would build for a coordinator session from the executor profile's
   `EnvVars` and the agent profile's environment, resolving each `SecretID`
   through the secrets store. Fail with detail `KANDEV_API_KEY`,
   `KANDEV_RUN_TOKEN` or `pat_value` when a key has one of those names or a
   value begins with `kandev_pat_`; fail with detail `unreadable` when any
   secret cannot be resolved. The resolved values are compared and discarded;
   they are never logged or returned.
4. **`no_extra_tools`.** Load the agent profile's MCP configuration
   (`AgentProfileMcpConfig`); met when it is disabled or its `servers` map is
   empty. Kandev's own server is injected by the session, not configured
   there. Detail is the count of configured servers, or `unreadable`.

`Check` is pure over its reads and is called fresh by every `Admit`, by the
autonomy read, and by the settings display; nothing caches its result
(`AC-COORDINATOR-CONTAINMENT-001.3`). A condition whose read fails is not met
(`AC-COORDINATOR-CONTAINMENT-001.2`). Admission reports the first unmet
condition's name as the detail of reason `containment`.

Containment is not checked for attended turns: the phase 1 conversation open
and message send paths are unchanged (`AC-COORDINATOR-CONTAINMENT-002.2`).

## Unattended permissions

The unattended turn row ([wake](wake.md#store)) marks a session as running an
unattended turn from delivery step 3 until its settle. The coordinator
package registers a hook on the orchestrator's permission-request handling for
sessions of `coordinator` origin, the same seam phase 1's exact-name
auto-approve uses:

- When the request is auto-approved by the exact-name rule, nothing changes.
- Otherwise, when the session holds an open unattended turn, the hook resolves
  the request at once through the same resolution path a person's answer
  takes, choosing the request's reject option (or cancelling when it has
  none), with a `PermissionResolutionAudit` of actor kind
  `coordinator_unattended`, source `coordinator_wake` and the turn id, and
  increments the turn row's `denied_permissions` with
  `UPDATE ... SET denied_permissions = denied_permissions + 1 WHERE id = ?`.
- Otherwise the request reaches the panel as in phase 1.

A request that arrives after the turn settles is an attended-mode request and
waits for a person. The phase 1 forcing of the profile and environment
auto-approve settings off applies to every coordinator session, so unattended
turns inherit it with no new code (`AC-COORDINATOR-CONTAINMENT-003.2`); a
test pins it for a session started by delivery.

## Settings display

The Autonomy section of the coordinator's settings (UI-04) reads the
`containment` block of the autonomy read ([wake](wake.md#autonomy-read)) when
it opens and on **Check again**, and lists the four conditions with Met or
Not met and a fix line:

| Condition | Fix text |
| --- | --- |
| `executor_isolated` | "Choose a Docker, remote Docker, Sprites or Kubernetes executor profile." |
| `auth_enabled` | "Turn on Kandev authentication (features.auth) and finish setup." |
| `no_kandev_credential` | "Remove Kandev tokens from this coordinator's executor and agent profile environment." |
| `no_extra_tools` | "Remove extra MCP servers from this coordinator's agent profile." |

A read failure of the autonomy route shows "Containment unavailable" with
Check again. Copy goes through `t()` in six locales; `features.auth` is a
verbatim config key.

## Security

- The check has no bypass: admission refuses when it fails, and `Deliver`
  is the only unattended turn start.
- Secret values are read only inside `Check` and never leave it.
- The containment result is visible to readers through the autonomy read; it
  names condition states and executor types, not secrets or values.

## Observability

`coordinator_containment_failed_total{condition}` counts failing checks at
admission; a structured zap log at info records the coordinator id and the
failing condition name when a coordinator's containment state changes between
admissions.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Opt-in authentication](../../../decisions/2026-07-24-opt-in-authentication.md)
- [Live agent permission authority](../../../decisions/2026-08-11-live-agent-permission-authority.md)
