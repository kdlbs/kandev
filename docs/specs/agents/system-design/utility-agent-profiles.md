---
status: draft
system: agents
requirements:
  - REQ-AGENTS-UTILITY-AGENT-PROFILES-001
created: 2026-08-08
owners:
  - kandev
---
# Profile-backed Utility Agents System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-UTILITY-AGENT-PROFILES-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-UTILITY-AGENT-PROFILES-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decision: [ADR-2026-08-08-utility-agent-profile-execution](../../../decisions/2026-08-08-utility-agent-profile-execution.md)

Safety decision: [ADR-2026-08-08-utility-profile-dependency-safety](../../../decisions/2026-08-08-utility-profile-dependency-safety.md)

Default inheritance repair: [ADR-2026-08-12-empty-utility-bindings-inherit-default](../../../decisions/2026-08-12-empty-utility-bindings-inherit-default.md)

Implementation plan: [utility-agent-profiles](../../../plans/utility-agent-profiles/plan.md)

Latest repair plan: [utility-agent-unavailable-repair](../../../plans/utility-agent-unavailable-repair/plan.md)

## Why

Utility agents run unattended one-shot jobs, but choosing only an agent family and model omits the
permissions and launch configuration that make that agent safe and reliable. Users need utility
jobs to run with an agent profile they have already configured, so a job does not stop midway for a
permission choice that its caller cannot answer.

## What

- Settings > Utility Agents has one **Default utility agent profile** selection. The choice is an
  eligible concrete or dynamic global agent profile, not an agent family/model pair.
- Each built-in utility action either inherits the default utility profile or selects one eligible
  profile as an override.
- Every custom utility agent selects one eligible concrete or dynamic profile. A custom utility
  agent cannot be created or saved without a profile.
- A built-in utility action without a profile override inherits the default utility
  profile. A stale override remains **unconfigured** after its profile is deleted or
  disabled and cannot run until the user repairs it.
- An empty `unconfigured` built-in binding is normalized to `inherit`. Selecting Default in an
  action picker persists the same inherited state and never submits an empty explicit binding.
- Eligible choices are enabled, non-deleted, global concrete profiles for ACP inference-capable
  agents and global dynamic profiles with at least one valid candidate. CLI-passthrough profiles and
  workspace-scoped Office profiles are not eligible.
- A utility invocation resolves its effective profile at the start of the call. It uses that
  profile's agent, model, mode, dynamic config options, enabled CLI flags, command prefix,
  environment/secret references, and permission policy. Editing the profile affects the next call;
  an in-flight call keeps the configuration resolved when it started.
- A dynamic selection resolves through the shared dynamic conductor. The caller submits the same
  profile ID as for a concrete selection. The call record retains that logical profile ID and the
  concrete execution profile that produced the final result.
- The UI shows profile names and their parent agent names. It does not expose separate utility-only
  model or permission controls.
- Built-in action overrides and the default profile participate in the shared Settings save/discard
  flow. Custom utility-agent create/edit continues to save through its dialog.
- When a profile is disabled and utility agents reference it, the profile settings page shows a
  dependency warning before it saves the disabled state. The user can cancel or confirm. A confirmed
  disable keeps the bindings and makes new utility calls fail closed until the bindings are repaired.
- When a profile is deleted and utility agents reference it, the existing profile-in-use dialog lists
  those utility agents. The user must cancel or explicitly confirm the destructive action. Confirmed
  deletion keeps the bindings as stale IDs; it does not silently select another profile.
- Plugin configuration continues to select a built-in or custom utility-agent ID. Invoking that
  utility agent uses its effective agent profile.
- On-demand code review continues to use the `code-review` utility agent when no workflow profile is
  specified, but its reviewer runtime comes from the effective utility profile.

## Data model

`utility_agents` stores:

| Field              | Type   | Constraint                   | Meaning                                                                        |
| ------------------ | ------ | ---------------------------- | ------------------------------------------------------------------------------ |
| `agent_profile_id` | string | empty for `inherit`, required for `explicit`, retained for stale `unconfigured` bindings | Concrete or dynamic logical profile reference. |
| `profile_binding_state` | enum | `inherit`, `explicit`, or `unconfigured` | Whether the row inherits the default, names a profile, or needs repair. |

`inherit` is valid only for built-in rows. `explicit` requires an eligible profile. `unconfigured`
is used for a custom row without a profile and for a built-in row whose profile binding is
stale or unavailable. Only a built-in row whose persisted state is `inherit` uses the selected
default. A built-in row with an empty profile ID in either the legacy `explicit` or `unconfigured`
state is normalized to `inherit`. A stale profile ID remains `unconfigured` so the user can
repair or replace the known override explicitly.

The legacy `agent_id` and `model` columns may remain temporarily as migration inputs, but they are
not execution inputs after this feature ships and are not writable through the utility-agent API.

Portable user settings store `default_utility_agent_profile_id: string`. Empty means no default is
configured. The legacy `default_utility_agent_id` and `default_utility_model` values are migration
inputs only.

`utility_agent_calls` stores the effective logical `agent_profile_id`, concrete
`execution_profile_id`, and resolved model. For a concrete selection the two
profile IDs match. For a dynamic selection they differ. Historical calls
created before either field exists keep an empty value.

### Legacy migration

The backend first adds the new profile fields and keeps the legacy agent/model fields as read-only
migration inputs. It then runs an idempotent migration after agent-profile seeding and reconciliation.
For each legacy default, built-in override, or custom utility agent, the backend considers enabled,
non-deleted, non-passthrough global profiles whose parent agent matches the legacy agent identifier
and whose configured model matches the legacy model (including an explicit empty model). Exactly
one match is copied to `agent_profile_id` and sets the row state to `explicit`.

Zero matches or multiple matches set a custom row to `unconfigured`. A built-in row with no matched
profile ID becomes `inherit`, including an empty `unconfigured` row written by an older release.
The backend does not pick the first profile, infer from a display name, or silently use a provider
default. Stale built-in IDs remain `unconfigured`. Custom utility agents that cannot be
migrated remain stored and editable but are not executable until the user selects a profile. The
legacy values remain available to a later retry or diagnostic report, but the new state is
authoritative after the migration.

## API surface

- `GET /api/v1/utility/agents` and `GET /api/v1/utility/agents/:id` return
  `agent_profile_id` and `profile_binding_state` on every utility agent.
- `POST /api/v1/utility/agents` requires `agent_profile_id` for a custom utility agent.
- `PATCH /api/v1/utility/agents/:id` accepts `agent_profile_id` and `profile_binding_state`. A
  built-in uses `inherit` with an empty ID or `explicit` with an eligible profile. A custom utility
  agent must use `explicit` when it is saved by the user.
- User-settings read and patch payloads expose `default_utility_agent_profile_id`.
- `POST /api/v1/utility/execute` keeps its request shape: callers select a utility-agent ID, and the
  backend resolves the effective profile. Successful responses and call-history responses keep the
  resolved model and expose logical and concrete profile attribution in call history.
- Plugin `Host.InvokeUtilityAgent` and its plugin manifest selector keep selecting a utility-agent
  ID; there is no plugin-facing profile-ID field.

## Permissions

- Reading and changing utility-agent selections uses the existing Settings authority. Agent-profile
  creation and mutation retain the operator-owned launcher-settings boundary.
- Utility execution never grants more access than the selected profile configures. It does not
  overlay utility-specific allowlists or copy permissions from the active task's profile.
- A profile with auto-approval enabled receives the same automatic ACP approval behavior as a
  normal launch. When auto-approval is disabled, any permission request not prevented or answered by
  the profile's own CLI policy is rejected promptly because utility calls have no interactive
  permission-response UI.
- Secret-backed profile environment values are resolved through the existing profile-secret path
  and are never returned by utility APIs or stored in utility call history.

## Failure modes

- With no effective profile, invocation fails before a call is dispatched and tells the user to
  select a profile in Settings > Utility Agents.
- A missing, deleted, disabled, CLI-passthrough, workspace-scoped, or otherwise ineligible profile
  binding fails closed. An empty built-in binding is normalized to `inherit` and uses the saved
  global default. A concrete selection never falls back to another profile. A dynamic selection may
  select another configured candidate under its routing policy, but never the active task profile,
  a raw agent/model pair, or the first available agent.
- A profile launch-policy error (invalid command prefix, unresolved required secret, invalid config
  option, or unavailable agent runtime) is surfaced as the utility call failure. The runner does not
  retry without that setting.
- A profile disable or delete dependency lookup failure blocks the profile change. The backend does
  not risk leaving an unknown utility binding behind.
- An ACP permission request that is not auto-approved is rejected and surfaced as an actionable
  non-interactive-permission failure; it does not remain pending for the normal interactive timeout.
- A failed Settings save leaves the saved profile selections authoritative and keeps the page dirty
  for retry through the shared save coordinator.

## Persistence guarantees

- The default profile and utility-agent overrides survive backend and browser restarts through
  backend-owned settings/database storage.
- Profile edits are not copied into utility-agent rows. Each new invocation reads the current
  profile; already-running calls keep their start-time resolution.
- Disabling or deleting a selected explicit profile does not rewrite the selection to another
  profile. The stale ID remains diagnosable and invocation fails closed until the user repairs it.
  The warning dialog is confirmation only; it does not perform reassignment. Built-in actions that
  inherit the default remain inherited when the default profile is deleted, so selecting a new
  default repairs them without editing each action.
- Built-in rows left as empty `unconfigured` by an older release are repaired idempotently to
  `inherit`. Selecting Default in the action picker also persists `inherit` and survives reload.
- Utility call history retains the logical profile ID, concrete execution profile ID, and resolved
  model even if either profile is later edited or deleted.

## Scenarios

- **GIVEN** an enabled Codex profile with a selected model and permission flags, **WHEN** the user
  selects it as the default utility profile and runs `enhance-prompt`, **THEN** the one-shot process
  starts with that profile's model, flags, environment, command prefix, config options, and
  permission policy.
- **GIVEN** a built-in action that inherits the default profile, **WHEN** the default profile is
  changed and saved, **THEN** the action's next invocation uses the newly selected profile without
  editing the action.
- **GIVEN** a built-in action with an explicit profile override, **WHEN** the default profile is
  changed, **THEN** that action continues to use its override.
- **GIVEN** a custom utility agent, **WHEN** the user opens its create/edit dialog, **THEN** one
  profile picker replaces the separate agent and model controls and saving is gated on a profile.
- **GIVEN** a utility agent selects a dynamic profile, **WHEN** its first candidate fails with a
  classified quota error before returning a result, **THEN** the shared conductor tries the next
  configured candidate and call history retains the dynamic logical profile plus the successful
  concrete execution profile.
- **GIVEN** a selected profile with auto-approval enabled, **WHEN** its agent emits an ACP permission
  request, **THEN** the utility call approves it using the profile policy and continues without user
  interaction.
- **GIVEN** a selected profile with auto-approval disabled whose CLI still emits a permission
  request, **WHEN** the utility call runs, **THEN** the request is rejected promptly and the caller
  receives an actionable failure instead of a pending approval prompt.
- **GIVEN** a selected profile is later disabled or deleted, **WHEN** a utility action or plugin
  invokes the bound utility agent, **THEN** execution is not dispatched and the caller receives a
  configuration failure naming the unavailable profile.
- **GIVEN** a profile is used by one or more utility agents, **WHEN** the user disables the profile,
  **THEN** the profile page lists the affected utility agents and requires cancel or explicit
  confirmation before saving the disabled state.
- **GIVEN** a profile is used by one or more utility agents, **WHEN** the user deletes the profile,
  **THEN** the existing profile-in-use dialog lists the affected utility agents and deletion needs
  explicit confirmation; after confirmation the utility bindings remain stale and fail closed.
- **GIVEN** a dependency lookup fails, **WHEN** the user tries to disable or delete the profile,
  **THEN** the profile change is rejected and the user can retry without data loss.
- **GIVEN** a legacy agent/model selection that matches exactly one eligible profile, **WHEN** the
  backend upgrades, **THEN** that profile becomes the saved selection and utility behavior is
  preserved with the profile's launch policy.
- **GIVEN** a legacy built-in action still has the migration state's explicit binding with an empty
  profile ID and its agent/model selection matches zero or multiple eligible profiles, **WHEN** the
  backend upgrades, **THEN** the row state becomes `inherit` and the action uses the selected default
  utility profile. A custom utility agent with the same migration result becomes `unconfigured` and
  remains unavailable until the user chooses a profile.
- **GIVEN** an older release left a built-in action `unconfigured` with an empty profile ID,
  **WHEN** the backend runs utility binding migration, **THEN** the row is persisted as `inherit` and
  resolves through the saved global default profile.
- **GIVEN** an unavailable built-in action and a valid global default, **WHEN** the user selects
  Default and saves, **THEN** the save succeeds, the row persists `inherit` with an empty profile ID,
  and the default profile remains selected after reload.
- **GIVEN** an inherited built-in action and a deleted default profile, **WHEN** the user selects a
  new eligible default profile, **THEN** the action uses the new default without an action-level
  edit.
- **GIVEN** Settings > Utility Agents on a phone viewport, **WHEN** the user changes the default or an
  action override, **THEN** the full profile name and save state remain reachable without horizontal
  page overflow or a desktop-only interaction.
- **GIVEN** a saved default and action override, **WHEN** the page reloads, **THEN** both profile
  selections render from backend state and no legacy agent/model picker appears.

## Utility Agents settings discoverability

The Utility Agents page must explain the scope of these helpers and place related settings in the
order that users need them.

- The page description must explain that utility agents are one-shot helpers for Kandev UI actions,
  such as commit and PR text generation or prompt enhancement. It must also state that they are not
  the agents that run inside task sessions.
- The **Configuration Chat Agent** card appears directly below the **Default utility agent model**
  card. The **Actions** card follows it, and the custom utility-agent card remains after the action
  overrides.
- The **Actions** card has a description that says it overrides the profile for a specific Kandev UI
  action. The description must make clear that these actions are not task-session agent work.
- Every agent-profile selector on this page uses the same searchable control. This includes the
  default utility profile, each action override, the custom utility-agent dialog, and the
  Configuration Chat Agent selector.
- The selector trigger and every option show the parent agent icon next to the profile label. The
  icon uses the existing agent-logo component and must keep the profile name readable when the
  label is long.
- Typing in the selector filters by the visible profile label and parent agent name. Keyboard focus,
  arrow navigation, Enter selection, Escape close, and the selected value must work on desktop and
  phone viewports.
- A saved unavailable profile keeps its repair state and remains visible as an unavailable option.
  Search must not hide that selected value when the selector opens.
- The selector keeps the existing eligibility rules for utility profiles. Search changes only how
  the eligible list is found; it does not add workspace, passthrough, disabled, or non-inference
  profiles.

### Settings discoverability scenarios

- **GIVEN** the Utility Agents page is open, **WHEN** the user scans the cards, **THEN** the order is
  default utility profile, Configuration Chat Agent, Actions, and custom utility agents.
- **GIVEN** a user who does not know the term utility agent, **WHEN** they read the page heading and
  description, **THEN** they understand that the helpers run one Kandev UI action at a time and are
  separate from task agents.
- **GIVEN** a profile selector with several agent profiles, **WHEN** the user types an agent or
  profile name, **THEN** only matching options remain and each option still shows its agent icon.
- **GIVEN** a phone viewport, **WHEN** the user opens any profile selector and types a filter,
  **THEN** the control stays within the viewport, the list can scroll, and the same icon and filter
  behavior is available without horizontal document overflow.

## Out of scope

- Adding executor/worktree selection to utility agents; sessionless calls remain host-run in their
  isolated temporary workspace.
- Making CLI-passthrough agents support one-shot structured inference.
- Changing the raw `/api/v1/agent-capabilities/:type/prompt` diagnostic API to require a profile.
- Adding a utility-specific permission editor or copying profile settings into utility-agent rows.
- Automatically replacing stale selections after a profile is disabled or deleted.
