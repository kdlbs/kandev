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
3. **`no_kandev_credential`.** Check the stated source set for a Kandev
   credential. `Check` does not reproduce the launch environment resolver and
   the lifecycle package is not imported (ARCH-RUNTIME-IMPORT); it reads these
   sources, each through a narrow interface implemented by an adapter in
   `internal/backendapp/coordinator.go` (a file with no lifecycle import, so
   the `runtime_import.json` baseline is unchanged):
   - the executor profile's `EnvVars` (keys and values, secret ids);
   - the agent profile's environment (keys and values, secret ids);
   - the agent profile's `ProviderAPIKeySecretID`;
   - the credentials manager: the adapter loads the agent through
     `GetAgent(ctx, profile.AgentID)`, takes its `Name`, resolves
     `registry.Registry.Get(name)` and, for each key in the agent runtime's
     `RequiredEnv`, calls `credentials.Manager.GetCredentialValue`. An error or
     empty value is skipped, exactly as the launch skips it
     (`appendRequiredCredentialDefinitions`); a failed agent or registry lookup
     is `unreadable`.

   Each `SecretID` is revealed through the secrets store (`revealGlobalSecret`,
   or the workspace reveal for a workspace secret). Every definition is
   evaluated on its own, so a token that a later definition overrides still
   fails. A key matches only when it equals `KANDEV_API_KEY` or
   `KANDEV_RUN_TOKEN` exactly (case-sensitive); a value matches when the whole
   resolved value begins with `kandev_pat_`. Detail is `KANDEV_API_KEY`,
   `KANDEV_RUN_TOKEN` or `pat_value`; when several definitions match, the first
   in the order above names the detail.

   Repository-binding environment is not resolvable through an exported reader
   outside the lifecycle package. When the coordinator's conversation task
   carries ANY repository binding environment variable (the adapter reads the
   task's repositories and their secret bindings), the condition is not met with
   detail `unverified_source`; a coordinator with no conversation task, or a
   task with no bindings, has an empty source. `OpenConversation` creates the
   conversation task with no repositories (`coordinator/conversation.go`), so the
   source is empty by construction today; the check is kept so a later change
   that attaches one fails closed. The adapter's task-repository reader and
   credentials-manager reader are wired in `internal/backendapp/coordinator.go`
   (that wiring is in scope, and a test asserts both are non-nil in production
   wiring); a nil reader is `unverified_source`. A secret that cannot be resolved,
   or any other read error fails with detail `unreadable` (the condition never
   calls `runtimeenv`, so no `ConflictError` arises); an empty source set is met. A match outranks
   `unverified_source`, which outranks a met result; `unreadable` outranks both.
   The resolved values are compared and discarded; they are never logged or
   returned. Lifting `unverified_source` needs the lifecycle resolver exported,
   which is recorded under [Out of scope](#out-of-scope).
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
conversation task whose session is not `CREATED`, conditions 3 and 4 also
fail, with detail `changed_since_launch`, if the `updated_at` of any source
that feeds the condition is later than that session's `started_at`:

- condition 3: the executor profile, the agent profile, and every secret
  revealed for condition 3 (`secrets.Secret.UpdatedAt`);
- condition 4: the agent profile's MCP configuration row `updated_at` when the
  row exists (an MCP patch bumps only that row, not the agent profile); the
  agent profile's `updated_at` only when there is no MCP row.

A session in state `CREATED` has not launched, so the rule does not apply to
it, decided by the session state and not by `started_at` (the row's `started_at`
is set to the row's creation time and a resumed session keeps it, as in
`task/repository/sqlite/session.go`); a source that has the field but a missing
`updated_at` is `unreadable`. Without
a session there is nothing launched and the rule does not apply. The fix is the
phase 1 way to start a new conversation, which launches from the current
settings. Sources with no timestamp are residuals ([Out of scope](#out-of-scope)).

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
`session_turn_id`, as unattended ([wake turn end](wake.md#turn-end)). Delivery
binds `session_turn_id` at the agentctl acceptance boundary through the
existing `onAccepted(turnID)` hook of `promptTaskOptions`
([wake Delivery](wake.md#delivery)); until that binding lands the column is
empty. The window is real: the active turn is registered before `onAccepted`
runs, so a permission request can arrive with the column still empty. Such a
request fails closed: it is denied and counted like any other unattended
permission, so the conversation never waits for a person.

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
  unattended turn row for that session whose `session_turn_id` equals the
  request's active session turn id, or is still empty (the unbound window
  above) and whose `reserved_turn_id` is null or equals the request's active
  turn id ([orphan turn](wake-recovery.md#orphan-turn));
- otherwise, and only when the permission message was stored (a failed message
  write leaves the request to a person: nothing is recorded, counted or
  resolved), runs one transaction that re-reads the row with `outcome IS NULL`,
  inserts `(turn_id, pending_id)` into `coordinator_unattended_denials` with `ON
  CONFLICT DO NOTHING` and, only when the insert added a row, runs `UPDATE
  coordinator_unattended_turns SET denied_permissions = denied_permissions + 1
  WHERE id = ? AND outcome IS NULL`. A row settled between the lookup and the
  insert makes the transaction record nothing, and the request waits for a
  person. `turn_id` is the unattended turn ROW's `id`, which is also the turn id
  in the audit; the request's session turn id is used only for the match above.
  The count key stays `(turn_id, pending_id)`: `pending_id` is the durable
  identity of one request, so a redelivery of it, with any `request_id`, counts
  once;
- after that commit, and whether or not the insert added a row (a redelivery
  after a failed first resolution must still resolve), resolves the request
  through an exported orchestrator method,
  `Service.ResolveUnattendedPermission(ctx, taskID, sessionID, pendingID,
  unattendedTurnID string) error`, before the handler returns. It reads the live
  snapshot (`ListPendingAgentPermissions(ctx, taskID, sessionID)`), finds the
  entry whose `pending_id` matches and takes the `request_id` and the options
  from that entry. A missing entry is success (already resolved or gone). When
  the entry offers a reject option (`pickRejectOption`), it calls
  `ResolveAgentPermission` with that option id, the entry's `request_id`, the
  `pending_id` and the audit below, which restores the session to running; when
  it offers none, it calls `cancelAgentPermission` with the same identity and
  the method then calls `markSessionRunningAfterPermission` itself. The audit is
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
  not left in `WAITING_FOR_INPUT` by this request. An already-resolved,
  stale or claim-in-progress result is success. Any other failure is logged at
  warn, leaves the recorded denial in place, and is retried by
  `Service.ReresolveRecordedDenials(ctx, coordinatorID)`.

`ReresolveRecordedDenials` is scoped to one coordinator and is exported by the
coordinator package. It selects the recorded denials whose turn row belongs to
`coordinatorID` (a join through `coordinator_unattended_turns.coordinator_id`,
since the denials table has no coordinator column) and whose turn row is still
open. For each, it takes the task and session from the turn row
(`conversation_task_id` and `session_id`), reads the live snapshot, and calls
`ResolveUnattendedPermission` with the recorded `pending_id`, without counting
again. The request id comes from the live snapshot, never from storage, so a
restart or a re-issued request cannot leave a denial unresolvable. The
resolver is injected once when the service is wired, never per request. A
denial whose turn has settled stays as recorded and is never re-resolved: its
request is an attended-mode request. An empty selection, an entry no longer in
the snapshot, and an already-resolved result are success. The
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
`containment` block of the autonomy read ([wake](wake-screens.md#autonomy-read)) when
it opens and on **Check again**, and lists the four conditions with Met or
Not met and a fix line:

| Condition | Fix text |
| --- | --- |
| `executor_isolated` | "Choose a Docker, remote Docker, Sprites or Kubernetes executor profile." |
| `auth_enabled` | "Turn on Kandev authentication (features.auth) and finish setup." |
| `no_kandev_credential` | "Remove Kandev tokens from this coordinator's executor and agent profile environment." |
| `no_extra_tools` | "Remove extra MCP servers from this coordinator's agent profile." |

The `no_kandev_credential` fix names the source that matched: the detail
`KANDEV_API_KEY`, `KANDEV_RUN_TOKEN` or `pat_value` shows the line above; the
executor-profile, agent-profile and provider-key sources share it. Three details
replace the condition's own fix line while they hold:

| Detail | Fix text |
| --- | --- |
| `changed_since_launch` | "Start a new conversation with this coordinator so it launches with the current settings." |
| `unreadable` | "Kandev could not read this setting. Check the profile and try again." |
| `unverified_source` | "This coordinator's conversation task carries a repository binding that Kandev cannot verify. Remove the binding from the task, or use a coordinator without one." |

`changed_since_launch` names the new-conversation action, not a session
restart, because the phase 1 conversation is what launches from current
settings. All three are added in all six locales.

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
Admission (task 05) calls only the helper in its counting mode
(`AdmitCounting`, used by delivery) and `Check` in its read-only mode
(`AdmitReadOnly`, used by the autonomy read; see
[wake](wake.md#admission)); this card owns and tests the
counter and log with a fake caller. The autonomy read and the settings display
call `Check` directly, without counting, so opening settings never moves the
counter. The autonomy read is a no-count path: its `Check` call is not
`CheckForAdmission`, never increments the counter, never writes the state-change
log and never changes the previous key.

`coordinator_containment_failed_total{condition}` counts each unmet condition
of a `Check` the helper runs. A structured zap log at info records the
coordinator id and the failing condition name when the containment state
changes between helper calls. The comparison key is `(Contained, name of the
first unmet condition)`; a change in either logs, and a change only in a later
unmet condition or in a detail does not. The previous key is kept in memory per
coordinator id, guarded by a mutex; the first call after a restart logs no
change.

## Out of scope

Launch currency reads only sources that carry an `updated_at`. Two
sources feed launch and have none, so an edit to them after launch is not
detected: the credentials-manager values read for `RequiredEnv`, and the
agent-runtime default definitions (`appendAgentRuntimeDefaults`), which `Check`
does not read at all because runtime defaults carry no Kandev credential. They are named
residuals, not silent gaps; the fix is a new conversation, as for any edit.

Exporting the lifecycle launch environment resolver (the definition assembly
in `resolveStrictEnvironment`) as one read-only function shared by launch and
`Check` is not part of this card. It is the way to lift `unverified_source`,
because it would resolve repository-binding environment the same way launch
does. The conductor tracks it as a follow-up; until then any repository binding
on the coordinator's conversation task makes condition 3 not met.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Opt-in authentication](../../../decisions/2026-07-24-opt-in-authentication.md)
- [Live agent permission authority](../../../decisions/2026-08-11-live-agent-permission-authority.md)
