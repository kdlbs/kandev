---
id: coordinator-containment-design
title: Containment for unattended turns design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-30
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
3. **`no_kandev_credential`.** Resolve the environment a coordinator session
   would launch with through one exported, read-only function in the lifecycle
   package, `ResolveLaunchEnvironmentDefinitions(ctx, executorProfileID,
   agentProfile, taskID)`. It returns the resolved definitions in resolver order
   (executor-profile `EnvVars`, agent-profile environment, the standard
   definitions, required credentials and repository secret bindings). The launch
   path calls this same function: it is a behaviour-preserving extraction of the
   definition assembly that `resolveStrictEnvironment` performs today, existing
   launch tests pass unchanged, and one new lifecycle test asserts that launch
   and `Check` see the same set. Nothing else in the launch path is refactored.
   `Check` passes the coordinator's own conversation task id, the one its
   session launches with; with no conversation task the repository-binding
   source is empty. The check also resolves the agent profile's
   `ProviderAPIKeySecretID` and applies the same key and value test to it.
   `Check` reaches the function through a narrow interface. Each `SecretID` is
   revealed through the secrets store (`revealGlobalSecret`, or the workspace
   reveal for a workspace secret). Every definition is evaluated on its own, so
   a token that a later definition overrides still fails. A key matches only
   when it equals `KANDEV_API_KEY` or `KANDEV_RUN_TOKEN` exactly
   (case-sensitive); a value matches when the whole resolved value begins with
   `kandev_pat_`. Detail is `KANDEV_API_KEY`, `KANDEV_RUN_TOKEN` or `pat_value`;
   when several definitions match, the first in resolver order names the
   detail. A secret that cannot be resolved, a `runtimeenv` `ConflictError` or
   any other read error fails with detail `unreadable`; an empty definition set
   is met. The resolved values are compared and discarded; they are never logged
   or returned.
4. **`no_extra_tools`.** Load the agent profile's MCP configuration
   (`AgentProfileMcpConfig`). A profile with no MCP row (`sql.ErrNoRows`) has
   no configured servers and is met, and its currency source is the agent
   profile's `updated_at`; only another read error is `unreadable`.
   Met when the configuration is disabled or its `servers` map is empty. Kandev's own
   server is injected by the session, not configured there, so any entry in
   `servers`, including one named `kandev`, fails: the requirement's "no MCP
   server other than Kandev's" means the injected one. Detail is the count of
   configured servers, or `unreadable`.

**Launch currency (conditions 3 and 4).** A live conversation session's
environment and MCP servers were fixed when it launched, and editing a
profile's contents does not end the conversation. When the coordinator has a
conversation task with a session, conditions 3 and 4 also fail, with detail
`changed_since_launch`, if the `updated_at` of any source that feeds the
condition is later than that session's `started_at`:

- condition 3: the executor profile, the agent profile, every secret revealed
  for condition 3 (`secrets.Secret.UpdatedAt`) and every repository secret
  binding in the resolved set (`RepositorySecretBinding.UpdatedAt`);
- condition 4: the agent profile's MCP configuration `updated_at`. A missing
  MCP row uses the agent profile's `updated_at`. A deleted or disabled
  configuration is compared by the agent profile's `updated_at` against
  `started_at`.

Without a session there is nothing launched and the rule does not apply. The
fix is the phase 1 way to start a new conversation, which launches from the
current settings. A missing `updated_at` on a source that has the field, or a
missing `started_at`, is `unreadable`. Sources with no timestamp are residuals
([Out of scope](#out-of-scope)).

Reads are not held in one snapshot: each condition reads current state, so a
concurrent edit can be seen by some conditions and not others. Every admission
runs a new check, and an edit between check and dispatch is bounded by the same
launch-currency rule on the next admission. A condition is evaluated even when an
earlier one failed, so `Result.Conditions` always has four entries; the first
unmet in order is the reported one.

`Check` is pure over its reads and is called fresh by every `Admit`, by the
autonomy read, and by the settings display; nothing caches its result
(`AC-COORDINATOR-CONTAINMENT-001.3`). A condition whose read fails is not met
(`AC-COORDINATOR-CONTAINMENT-001.2`). Admission reports the first unmet
condition's name as the detail of reason `containment`. A nil coordinator, a
missing executor or agent profile and a cancelled context are `unreadable` on the
conditions they feed.

Containment is not checked for attended turns: the phase 1 conversation open
and message send paths are unchanged (`AC-COORDINATOR-CONTAINMENT-002.2`).

## Unattended permissions

The unattended turn row ([wake](wake.md#store)) marks one session turn, its
`session_turn_id`, as unattended ([wake turn end](wake.md#turn-end)).

Phase 1's exact-name auto-approve runs in agentctl
(`autoApproveCoordinatorPermission` and `coordinatorAutoApprovedNames` in
`internal/agentctl/server/process/manager_permission_policy.go`): a request
it grants never leaves agentctl. Every other request arrives in the backend at
`orchestrator.Service.handlePermissionRequest`
(`internal/orchestrator/event_handlers_git.go`), which stores the permission
message with the session's active turn id and then calls
`failAutomationRunOnPermission`. Phase 3 adds a sibling call there, after the
message is stored: an optional `UnattendedPermissionHandler` the coordinator
package registers on the orchestrator only while phase 3 is effective. It
receives the `watcher.PermissionRequestData` and the active turn id, and:

- does nothing unless the task is of `coordinator` origin and holds an open
  unattended turn row whose `session_turn_id` equals the request's active
  session turn id;
- otherwise, and only when the permission message was stored (a failed message
  write leaves the request to a person: nothing is recorded, counted or
  resolved), runs one transaction that re-reads the row with `outcome IS NULL`,
  inserts `(turn_id, pending_id)` into `coordinator_unattended_denials` with `ON
  CONFLICT DO NOTHING` and, only when the insert added a row, runs `UPDATE
  coordinator_unattended_turns SET denied_permissions = denied_permissions + 1
  WHERE id = ? AND outcome IS NULL`. A row settled between the lookup and the
  insert makes the transaction record nothing, and the request waits for a
  person. A redelivered request with the same `pending_id` counts once. In the
  denials table `turn_id` is the unattended turn ROW's `id`, which is also the
  turn id in the audit; the request's session turn id is used only for the
  match above;
- after that commit, resolves the request through a resolver function the
  orchestrator passes to the handler, before the handler returns. When the
  request offers a reject option (`pickRejectOption`), it resolves through the
  same path as `ResolveAgentPermission`, which restores the session to
  running; when it offers none, it cancels through `cancelAgentPermission`, and
  the handler then restores the session to running itself
  (`markSessionRunningAfterPermission`). Either way the audit is
  `PermissionResolutionAudit` of actor kind
  `models.PermissionActorCoordinatorUnattended` (`coordinator_unattended`),
  source `models.PermissionSourceCoordinatorWake` (`coordinator_wake`) and the
  unattended turn row's id. Both constants are new. `ResolveAgentPermissionRequest`
  gains two optional fields, `ActorKind` and `UnattendedTurnID`; when `ActorKind`
  is set, `claimAgentPermission` uses it in place of the kind
  `permissionAuditActor` derives from the context, and `UnattendedTurnID` is
  stored in the new optional `PermissionResolutionAudit.UnattendedTurnID`
  (`json:"unattended_turn_id,omitempty"`). The audit is stored as message
  metadata JSON, so no schema column is added. "Let the turn continue"
  is observable as: the permission message finalized `rejected` and the session
  not left in `WAITING_FOR_INPUT` by this request. An already-resolved or
  claim-in-progress result is success. Any other failure is logged at warn,
  leaves the recorded denial in place, and is retried by
  `Service.ReresolveRecordedDenials(ctx, coordinatorID)`.

`ReresolveRecordedDenials` is scoped to one coordinator and is exported by the
coordinator package. It selects the recorded denials whose turn row belongs to
`coordinatorID` (a join through `coordinator_unattended_turns.coordinator_id`,
since the denials table has no coordinator column) and whose turn row is still
open, reads each denial's permission message, and re-resolves every one still
pending through the same resolver and audit, without counting again. The
resolver and the message reader are injected once when the service is wired,
never per request. A denial whose turn has settled stays as recorded and is
never re-resolved: its request is an attended-mode request. An empty selection,
a message already finalized, and an already-resolved result are success. The
[backstop](wake.md#backstop) calls it once per coordinator it visits, as its
last turn and setting duty, whatever `autonomy_enabled` reads; a coordinator
that leaves the visit set is picked up again on its next visit. Wiring it into
the tick is task 04's, and this design's tests call the method directly.

The handler is registered once at startup and only while phase 3 is
effective; that condition cannot change while the process runs (the flag needs
a restart), so no open turn ever loses its handler.

Which requests reach this handler is fixed by the bound tool list: agentctl
builds its allowlist from the conversation's binding, and an unattended turn
runs with that binding unchanged, so containment removes capability and never
adds it. The denial is required because agentctl leaves a request the list
does not approve pending for a person, and no person is present
([integration](integration.md#tool-list),
`AC-COORDINATOR-INTEGRATION-003.2`).

A request whose turn id is not the unattended turn's reaches the panel as in
phase 1. `coordinator_unattended_denials` rows are deleted with their turn row.

`coordinator_unattended_denials` columns (created by task 01):

| Column | Type | Notes |
| --- | --- | --- |
| `turn_id` | text not null | `coordinator_unattended_turns.id`, no foreign key |
| `pending_id` | text not null | the permission request's pending id |
| `created_at` | timestamp not null | UTC, set at insert |

Primary key `(turn_id, pending_id)`; no other index. The table has no
`coordinator_id` and no `workspace_id`: it is deleted through its turn
([wake](wake.md#store)).

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

A condition whose detail is `changed_since_launch` shows the fix "Restart the
coordinator session so it launches with the current settings." A condition
whose detail is `unreadable` shows "Kandev could not read this setting. Check
the profile and try again." Both replace the condition's own fix line while the
detail holds, and both are added in all six locales.

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

`containment.go` exports a helper, `CheckForAdmission(ctx, coordinator,
recorder)`, that runs `Check`, counts and logs, and returns the `Result`.
Admission (task 05) calls only the helper; this card owns and tests the
counter and log with a fake caller. The autonomy read and the settings display
call `Check` directly, without counting, so opening settings never moves the
counter.

`coordinator_containment_failed_total{condition}` counts each unmet condition
of a `Check` the helper runs. A structured zap log at info records the
coordinator id and the failing condition name when the containment state
changes between helper calls. The comparison key is `(Contained, name of the
first unmet condition)`; a change in either logs, and a change only in a later
unmet condition or in a detail does not. The previous key is kept in memory per
coordinator id, guarded by a mutex; the first call after a restart logs no
change.

## Out of scope

Launch currency reads only sources that carry an `updated_at`. These
contribute to condition 3 and have none, so an edit to them after launch is not
detected: agent-runtime default definitions (`appendAgentRuntimeDefaults`) and
required-credential values from the credentials manager. They are named
residuals, not silent gaps; the fix is a new conversation, as for any edit.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Opt-in authentication](../../../decisions/2026-07-24-opt-in-authentication.md)
- [Live agent permission authority](../../../decisions/2026-08-11-live-agent-permission-authority.md)
