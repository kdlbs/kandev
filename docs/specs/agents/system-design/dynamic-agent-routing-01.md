---
status: draft
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
created: 2026-08-13
updated: 2026-08-17
owners:
  - cfl
---
# Dynamic Agent Routing System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decision:
[ADR-2026-08-13-dynamic-agent-profile-routing](../../../decisions/2026-08-13-dynamic-agent-profile-routing.md).

Provider error classes and policy evaluation follow
[ADR-2026-08-17-provider-error-classes-and-policies](../../../decisions/2026-08-17-provider-error-classes-and-policies.md)
and [Provider Error Recovery](../../platform/requirements/provider-error-recovery.md).

Turn attribution follows
[ADR-2026-07-18-turn-configuration-snapshots](../../../decisions/2026-07-18-turn-configuration-snapshots.md).

Implementation plan:
[Dynamic Agent Routing](../../../plans/dynamic-agent-routing/plan.md).

Provider policy extension plan:
[Provider Error Policies](../../../plans/provider-error-policies/plan.md).

Rollout blocker repair:
[Dynamic Agent Routing Rollout Blockers](../requirements/dynamic-agent-routing-rollout-blockers.md).

## Why

Users often name profiles by the capability they want rather than a provider
brand. A task can use a profile named Frontier for planning and one named
Balanced for execution. The dynamic profile selects Claude, Codex, OpenCode,
or another provider from an ordered list of complete agent profiles.

Today, Office owns a separate provider router and Kanban profile changes create
or activate another task session. This makes the same intent behave differently
across workspace modes and can fragment one task across chat tabs. Kandev needs
one reusable agent profile that preserves a logical task session while routing
its work through complete existing agent profiles.

## What

### Profile configuration

- Kandev always registers one built-in virtual agent family with canonical ID
  `dynamic` and display name Dynamic. It cannot be disabled or uninstalled,
  does not expose a CLI command, and is not probed as a concrete inference
  agent. Profiles created under it have kind `dynamic`.
- Agent settings can create a dynamic profile with a user-defined name,
  description, icon, and availability scope.
- A dynamic profile references existing concrete agent profiles. It never
  copies or merges their credentials, environment, model, ACP options, flags,
  permissions, passthrough behavior, or MCP configuration.
- A dynamic profile's candidate list can reference only concrete, launchable
  profiles. It cannot reference itself, another dynamic profile, or a rich
  Office identity in the first version. An Office identity's separate
  `execution_agent_profile_id` binding can reference a concrete or dynamic
  profile.
- The profile name is the only capability label. Users can create profiles such
  as Frontier, Balanced, Economy, Review, or Security Review. Kandev stores no
  class or tier field and assigns no semantics to those names.
- Each profile has an ordered candidate list. A candidate identifies one
  concrete profile and stores separate transient-error and hard-error policies.
  Each class can wait for a trusted near reset, retry the same candidate with
  bounded exponential backoff, then either skip the candidate or stop.
- A concrete profile with `AutoFallback=true` is not an eligible dynamic
  candidate. The conductor is the only owner of cross-candidate fallback. An
  explicit `FallbackModel` remains part of the concrete profile's start-model
  policy. It does not advance the dynamic candidate list, and turn attribution
  records the model that ran.
- Dynamic profiles and their concrete candidates participate in the existing
  profile-in-use dependency dialog. A dependency lookup failure blocks the
  change. Otherwise, the user can cancel or explicitly confirm deletion or
  disabling. Confirmed changes keep durable bindings unchanged: stale selected
  profiles fail closed, while stale or disabled candidates become ineligible
  and another configured candidate can be selected.

### Transparent profile execution

- Every caller continues to select and pass one `agent_profile_id`. Task and
  workflow services, Office scheduling, utility agents, plugins, and frontend
  pickers do not branch on profile kind.
- One shared execution resolver inspects the selected profile. A concrete
  profile resolves directly, a dynamic profile resolves to the conductor,
  which chooses a concrete candidate before downstream launch.
- `execution_profile_id` is internal concrete attribution, not a second caller
  choice. It equals the selected `agent_profile_id` for a concrete profile. For
  a dynamic profile, the logical `agent_profile_id` remains dynamic and
  `execution_profile_id` identifies the concrete candidate that actually ran.
- APIs that expose profiles include a safe `kind` discriminator for rendering
  and diagnostics. Accepting a dynamic profile does not require a separate
  launch path in each caller.

### Settings interaction and mobile parity

- Desktop Agent settings adds a Dynamic profiles section beside the existing
  installed-agent profile groups. Its primary action creates a named dynamic
  profile, each row opens a dedicated profile route for candidates and rules.
- The new-profile route owns exactly one draft dynamic profile. It does not show
  Add profile or allow multiple drafts on the same page. Additional profiles
  are created from the Dynamic agents list after the current profile is saved
  or discarded.
- Phone Agent settings uses the existing fully tappable profile-row and direct
  profile-route pattern from `agent-profiles-section.tsx` and
  `mobile-agent-profile-layout.spec.ts`. The editor is dense, persistent content,
  so it uses direct navigation rather than compressing the desktop form into a
  dialog or drawer.
- On the phone profile route, name and current routing summary appear first,
  followed by ordered candidates and their Transient errors and Hard errors
  disclosures. The primary action is Add candidate. It remains visible and has
  a touch target of at least 44px.
- Candidate selection is a temporary choice and uses the existing mobile picker
  or inset bottom-drawer pattern. The candidate list remains the page's single
  vertical scroll owner, uses dynamic viewport sizing through the settings
  shell, and clears the bottom safe area. No rule editor creates document-level
  horizontal scrolling.
- Reordering is never drag-only. Phone users get visible Move up and Move down
  actions in each candidate's touch menu, desktop can additionally support
  pointer drag and keyboard reordering.
- Desktop and mobile share profile state, validation, selection logic, and the
  settings save coordinator. Only layout and temporary picker presentation
  differ.
- Error class descriptions, retry schedule, reset-wait rule, and exhausted
  outcome are visible form content. Hover help can supplement this copy but
  cannot be the only explanation.
- Chat route changes remain inline conversation events on every viewport. The
  model/options/commands controls are replaced in place, so mobile users do not
  lose composer state or navigate to another tab.
- Implementation adds a `mobile-dynamic-agent-profile.spec.ts` Playwright flow
  that enters Agent settings, creates a dynamic profile, adds and reorders
  candidates, saves it, and verifies no document horizontal overflow. A second
  mobile scenario exercises immutable per-turn provider badges, a route-change
  row, and the replaced provider controls in the existing task chat.

### Use in Kanban and workflows

- A user can select a dynamic profile anywhere a task or workflow currently
  accepts an agent profile.
- Starting a task creates one Kandev task session and one visible agent tab for
  the dynamic profile.
- A workflow step can select a dynamic profile instead of pinning a provider.
  Provider changes inside that profile do not create another task session.
- Selecting a different dynamic profile remains an explicit logical profile
  change and follows the workflow's normal profile-switch semantics.
- A workflow can still select a concrete profile explicitly when deterministic
  provider choice is required.

### Use by utility agents

- Default, built-in, custom, and plugin-invoked utility agents can select a
  concrete or dynamic profile through the existing profile picker and binding.
- A utility caller passes only the selected profile ID. A dynamic utility call
  creates a transient conductor invocation, chooses a concrete candidate, and
  returns the same result shape as a concrete utility call. It does not create a
  visible task-session tab.
- Simple ordered routing and classified pre-result failure fallback apply to
  utility calls. If a failed attempt emitted a result or performed an effect
  whose completion is ambiguous, the invocation fails closed rather than
  silently returning mixed output from two providers.
- The utility call record stores both the selected logical profile ID and the
  concrete `execution_profile_id`, model, token counts, and final outcome. A
  later profile edit does not relabel historical calls.

### Use in Office

- An Office agent such as CEO keeps its stable Office identity, role,
  instructions, skills, permissions, hierarchy, budget, and Office history.
- The Office agent editor selects an execution agent profile. It can be a
  concrete or dynamic profile.
- When it is dynamic, provider order, error actions, and health rules remain
  owned by the dynamic profile.
- Office has no separate provider-routing settings page. Onboarding selects or
  creates an execution agent profile instead of seeding workspace routing
  mappings.
- Office run, dashboard, and inbox surfaces can show route status and recovery
  actions, but they do not edit routing policy.

### Route selection

The selected concrete route is sticky. The dynamic router resolves a candidate
when the logical session starts and whenever a route-decision trigger occurs:

1. loads the configured concrete candidates in order,
2. rejects missing, disabled, incompatible, or cross-scope profiles,
3. skips candidates whose shared circuit is open,
4. applies the candidate's configured class policy to a current,
   effect-safe classified provider error, and
5. claims the next route generation with a compare-and-swap operation and
   records the selected candidate before launch.

Each candidate has one `transient` and one `hard` policy. A policy can wait for
a validated near reset, retry the same candidate with a bounded exponential
schedule, and then either skip to the next candidate or stop. Exact policy
fields, ordering, legacy normalization, validation, and unknown-error behavior
are owned by [Provider Error Recovery](../../platform/requirements/provider-error-recovery.md).
Unknown, ambiguous, stale, or effect-unsafe failures always stop automatic
recovery even if a class policy would retry or skip.

If no candidate is eligible, the task remains assigned to the dynamic profile
and enters a visible waiting or action-required state. It is not reassigned to a
different Kandev task session.

The waiting and action-required surfaces show the current class, safe reason,
retry ordinal, deadline, exhausted outcome, and actions valid for the current
route generation. `Retry now` uses the same candidate and consumes the next
retry slot. `Skip now` excludes the current candidate for that decision only.
`Cancel wait` stops the pending schedule without changing saved policy. These
actions do not open a circuit or change the profile. Core routing accepts them
only after the current turn settles. Dynamic routing does not cancel an active
turn to change routes.

### Route-decision triggers

A running dynamic session reconsiders its route only when:

- the active turn settles with a current, effect-safe classified provider
  error covered by the candidate's class policy,
- a persisted retry or reset-wait deadline becomes due,
- the user requests Retry now, Skip now, Cancel wait, or another explicit
  route action,
- the active candidate is disabled or removed from the dynamic profile, or
- restart reconciliation finds that the persisted route is no longer valid.

A normal profile edit increments the configuration version but does not itself
move a healthy session. The next route decision uses the latest committed
profile version. Disabling or removing the active candidate creates a pending
reroute at the next safe turn boundary. Reordering candidates or changing error
actions affects the next route decision and does not rewrite route history.

### One logical chat over downstream ACP sessions

- A dynamic task session has one stable Kandev session ID and chat tab.
- In Kanban, the tab and assistant author keep the dynamic profile's name and
  avatar. In Office, they keep the Office agent's name and avatar, such as CEO.
  A provider switch never presents the replacement as a new task participant.
- The runtime stores the active concrete profile and provider-native ACP session
  ID behind that logical session.
- Same-profile process recreation can use that provider's supported ACP load or
  resume behavior.
- Switching to another concrete profile always creates a fresh downstream ACP
  session. A resume token from one concrete profile is never supplied to
  another.
- Before the replacement prompt, Kandev creates a bounded continuation package
  from the task description, current workflow step, user messages and durable
  conversation summary, tool/result summary, repository status and diff
  summary, active plan or task metadata, and the failure and route reason.
- If the failed agent is still responsive, the conductor can request a handoff
  summary. Failure to obtain one cannot prevent fallback. Kandev builds the
  package from durable state.
- The replacement prompt tells the agent that it is continuing existing work,
  what was completed or uncertain, what caused the switch, and which actions
  must not be repeated without verification.
- Every assistant turn captures `execution_profile_id`, safe provider and model
  labels, and `route_generation` in the existing immutable turn configuration
  snapshot when the turn starts. The chat renders this as compact provider/model
  metadata on the assistant turn, including partial output from a failed turn.
  Later route or profile changes cannot relabel historical output.

### Route and capability events

The chat stream publishes Kandev-owned metadata around the ACP stream:

- `session.route_changing` identifies the prior concrete profile, reason, and
  route generation without claiming that the replacement is ready.
- `session.route_changed` identifies the new concrete profile, safe
  provider/model labels, route generation, and whether the downstream session
  was resumed or created fresh.
- `session.capabilities_replaced` carries the authoritative model list, current
  model, modes, ACP configuration options, and custom commands for that same
  route generation.
- `session.route_waiting` identifies when no candidate is currently eligible,
  the next probe or reset time when known, and available user actions.
- `session.route_pending` identifies an active-candidate change that will be
  enforced when the current turn settles.

Every route event carries a stable reason enum and structured parameters, such
as a semantic provider-error code or profile name. Backend events never contain
a composed user-facing English sentence. The frontend localizes the enum and
parameters.

The frontend applies route and capability changes by generation. It discards
late events from a previous downstream session, inserts a localized system row
for the structured reason, and replaces rather than merges provider-specific
  controls. The tab and message composer remain mounted. Route-change rows are
  durable and appear after reload. The current controls describe the active
  concrete route. They do not change the logical author shown on earlier turns.

### Shared health and probing

- Health belongs to the resource described by the classified evidence. A
  provider outage uses provider scope. An account quota uses provider and
  credential-binding scope. Model-specific evidence adds model scope. A
  profile-configuration error uses concrete-profile scope. The key does not add
  concrete profile identity to provider, account, or model evidence unless the
  credential binding is unknown.
- Each concrete launch adapter supplies a
  `CredentialBindingDescriptor` after it resolves the launch environment. The
  descriptor contains the canonical agent-family ID, authentication mechanism,
  credential source kind and non-secret locator identity, executor credential
  namespace, authorization scope, and workspace scope when credentials belong
  to one workspace.
- `CredentialBindingResolver` canonicalizes that versioned descriptor and
  computes an HMAC-SHA-256 fingerprint with a Kandev installation key that
  persists across restarts. Equal descriptors in one installation produce the
  same opaque key. Different descriptors do not share account-scoped health.
- The fingerprint never includes `CommandPrefix`, model, CLI flags, literal
  environment values, raw credentials, credential-file contents, prompts, or a
  raw account identifier. It is safe to persist and log as an opaque value.
- If Kandev cannot prove that two profiles use the same binding, it uses a
  conservative profile-scoped fallback fingerprint. This can reduce sharing,
  but it cannot disable an unrelated credential. Health is shared only when
  Kandev can prove the same usable binding.
- A provider-brand outage can open a broader provider circuit. An account quota
  failure opens only that account binding. A model-capacity failure does not
  disable unrelated models.
- A qualifying failure opens the circuit atomically. Other tasks and dynamic
  profiles that reference the same resource skip it immediately.
- Open circuits persist across backend restarts and have a retry deadline or an
  action-required state.
- At the deadline, one worker acquires a half-open probe lease. Other tasks keep
  using alternatives or wait. Probe success closes the circuit, failure extends
  its backoff.
- Recovery affects future route selection. A healthy active fallback session is
  not interrupted when an earlier candidate recovers.

## Data model

The durable model contains these logical records. Exact table names can follow
the repository's profile-unification conventions, but the identities and
constraints are normative.

### Dynamic profile configuration

The built-in `agents` row whose stable registry name is `dynamic` is the parent
of every dynamic `agent_profiles` row. It is seeded before profile
reconciliation and is never eligible for removal or orphan cleanup. Its profile
rows keep an empty model and do not use CLI launch fields. API DTOs expose
`kind=dynamic` when a profile belongs to this parent. All other executable
profiles expose `kind=concrete`. The discriminator identifies the execution
family. It does not classify a row's separate Office identity role.

`dynamic_agent_profiles` stores:

- `profile_id` as a primary and foreign key to `agent_profiles`,
- a version used for optimistic profile updates and route attribution.

`dynamic_agent_routes` stores:

- `dynamic_profile_id` and ordered `position` as identity,
- `execution_profile_id` referencing a concrete agent profile,
- enabled state, and
- a versioned policy document containing complete `transient` and `hard`
  policies.

The backend validates scope, candidate launchability, duplicate positions,
empty candidate lists, unsupported policy versions, incomplete class coverage,
invalid outcomes, and retry or duration bounds transactionally. The cycle
validator walks only dynamic-candidate edges. Office identity execution
bindings do not form candidate edges, and rich Office identities cannot be
dynamic candidates.

### Logical session route state

Each routed task session stores:

- stable `dynamic_profile_id`,
- active `execution_profile_id`,
- monotonic `route_generation`,
- `profile_version` used by the latest route decision,
- downstream ACP session identity and its owning concrete profile,
- latest route state (`selecting`, `starting`, `active`, `reroute_pending`,
  `switching`, `retry_wait`, `waiting_for_reset`, `retrying`,
  `action_required`, or `stopped`),
- pending route reason when an active-candidate change must wait for the current
  turn, and
- bounded continuation metadata, last route reason, classified code and class,
  catalogue version, policy snapshot, retry ordinal, absolute deadline, and
  pending exhausted outcome.

An Office run additionally stores its stable Office agent identity. That ID is
never substituted for the dynamic or concrete profile ID.

One-shot `utility_agent_calls` keep `agent_profile_id` as the selected logical
profile and add `execution_profile_id` for the concrete candidate. Dynamic
utility routing state is transient for the call, while its attempts and final
attribution are durable enough to diagnose the completed or failed invocation.

### Shared resource health

Shared health stores the opaque resource key, scope, state (`closed`, `open`, or
`half_open`), semantic failure code, confidence, failure count, retry or reset
time, probe lease and expiry, last successful probe, and updated time.
Route attempts reference both the dynamic profile version and concrete profile
used so later profile edits do not rewrite history.

Resource-health rows also store a resource-key version and safe scope fields.
The credential-binding fingerprint is stable across backend restarts. A future
key-version migration creates new rows and does not merge two bindings without
authoritative identity evidence.

Legacy `office_workspace_routing`, `office_run_route_attempts`, and
`office_provider_health` tables and their rows remain untouched during this
feature's migrations. They are not read for new route decisions and are not
shown in Office settings.

The existing `task_session_turns.metadata` configuration snapshot stores the
concrete execution profile, safe provider/model labels, and route generation
for the prompt/response cycle. Streamed messages do not duplicate this snapshot
and never derive historical attribution from current session state.

## API surface

- Existing agent-profile list and selection APIs expose `kind` and safe summary
  fields for dynamic profiles.
- Profile-consuming APIs continue to accept one selected `agent_profile_id`.
  They never require the caller to resolve or submit a concrete candidate.
- Dynamic profile CRUD uses the agent settings profile API and accepts the
  candidate and per-class policy contract as one versioned transaction. It
  returns field-addressable validation errors and always returns the normalized
  policy document.
- Candidate validation and deletion-conflict responses identify referencing
  profile IDs and safe names.
- Task-session snapshots include logical profile, active concrete profile,
  route state, route generation, pending profile enforcement, and current
  authoritative capabilities.
- The WebSocket request `session.route_action` accepts `session_id`, `action`
  (`retry_now`, `skip_now`, `cancel_wait`, or `stop`), and
  `expected_generation`. A successful response
  returns the current route snapshot. A stale generation returns a conflict
  with the authoritative snapshot. An action during an active turn returns a
  failed-precondition response.
- Route events use the contracts described above. All route-scoped user actions
  include the expected generation so a stale Retry now or Skip now action cannot
  affect a successor route.
- Office-specific routing mutation endpoints are removed before Office ships.
  Office read models consume shared route status by task-session or run ID.

## Delivery and rollout

- The feature is guarded by `features.dynamicAgentRouting` and
  `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING`. The backend is authoritative and
  rejects dynamic-profile CRUD and execution while disabled. Concrete-profile
  behavior remains unchanged. The flag ships `false` in prod, dev, and e2e
  profiles, is mutable, and requires restart. It has experimental stability and
  high risk. Development requires an explicit override until rollout.
- While the flag is disabled, Kandev retains dynamic configuration and route
  state but hides dynamic profiles from new selections. It stops probes and
  rejects create, update, launch, resume, utility, and route-action entry points
  before side effects. A persisted dynamic session enters `action_required`
  with reason `dynamic_routing_disabled`. Re-enabling the flag lets restart
  reconciliation resume it from durable state.
- With Office enabled and dynamic routing disabled, Office identities bound to
  concrete profiles continue to launch. An identity bound to a dynamic profile
  fails closed with the same action-required reason. Legacy Office routing rows
  remain hidden and unread in every flag combination.
- This feature ships the virtual family, transparent resolution, fixed order,
  classified-error fallback, shared circuits and probes, continuation, restart
  recovery, and Kanban, utility, and Office integration.
- Session-cost and subscription-usage routing are a separate future feature.
  They are not part of this feature's implementation or release criteria. See
  Dynamic Agent Telemetry Routing is deferred and has no active specification.
- Upgrade does not translate or delete existing Office routing rows. Office
  stops displaying those settings and requires a concrete or dynamic execution
  profile selection for new routing behavior.
