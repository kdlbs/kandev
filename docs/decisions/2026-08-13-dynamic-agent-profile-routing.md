# ADR-2026-08-13-dynamic-agent-profile-routing: Unify Provider Routing Behind Dynamic Agent Profiles

**Status:** accepted (amended 2026-08-17)
**Date:** 2026-08-13
**Area:** backend, frontend, protocol, workflow

## Amendment (2026-08-17)

[ADR-2026-08-17-provider-error-classes-and-policies](2026-08-17-provider-error-classes-and-policies.md)
replaces generic dynamic candidate actions with separate transient and hard
policies. Each policy can wait for a trusted near reset, retry with bounded
exponential backoff, then skip or stop. The shared routing ownership and
continuation boundaries in this decision remain unchanged.

## Context

Kandev's first provider-routing design belongs to Office workspaces. It maps
model tiers to concrete execution profiles, tracks provider health, and falls
back between providers. Kanban tasks need the same capability, including one
logical chat session while the concrete provider changes.

Keeping routing in Office creates two policy engines and two settings surfaces.
It also creates different recovery behavior for the same user intent. Routing
then depends on where a task runs rather than on the profile that
the user selected. Office has not been enabled for users, so its ownership
boundary can change before compatibility is required.

ACP gives Kandev a common transport, but a provider-native session cannot be
loaded by a different provider. A routed agent therefore needs a Kandev-owned
logical session above several provider-native ACP sessions. The initial feature
must work from provider errors and shared health without new plugin contracts.
Subscription and cost routing are a separate future feature.

## Decision

### Dynamic profiles own routing

Add a permanent built-in virtual agent family with the canonical ID `dynamic`.
The family is always registered, cannot be disabled or uninstalled, does not
advertise an ACP command, and is excluded from concrete-agent probing. User
profiles created under that family are dynamic profiles. This preserves the
existing `agent_profiles.agent_id` ownership invariant without making a
removable synthetic agent the parent of durable routing policy.

A dynamic profile is a virtual agent configuration and is not itself a provider
CLI launch configuration. It owns:

- a user-defined name and an ordered set of concrete agent-profile candidates,
- actions for classified provider errors, and
- route-health, circuit-breaker, probing, and recovery policy.

A dynamic candidate must be a concrete, launchable profile. Rich Office
identities and dynamic profiles cannot be candidates. An Office identity's
execution binding can select a concrete or dynamic profile because that binding
is not a candidate edge.

Names such as Frontier, Balanced, Economy, Review, or Security Review express
the user's intent. Kandev does not persist a class or tier field and does not
interpret those names.

Dynamic profiles are configured with the normal agent settings. Office no
longer owns a provider-routing settings page or a workspace-specific routing
policy.

### Profile selection is transparent to callers

Task creation, workflow steps, Office execution bindings, utility agents, and
other profile consumers continue to pass one selected agent-profile ID. A
shared execution resolver inspects that profile behind the selection boundary:

- a concrete profile resolves directly to its own launch configuration, and
- a dynamic profile resolves to the conductor, which selects and records a
  concrete execution profile before launching downstream ACP.

Callers do not branch on profile kind and do not supply a concrete
`execution_profile_id` for a dynamic profile. Internally,
`execution_profile_id` always identifies the concrete profile that actually
launched. It equals the selected profile ID for a concrete selection and differs
for a dynamic selection. This distinction keeps the abstraction transparent
without treating the non-launchable virtual profile as a CLI configuration.

Kanban tasks and workflow steps can select a dynamic profile wherever they can
select an agent profile. An Office agent selects either a concrete or dynamic
profile as its execution agent profile. Office wake-reason policy can request a
different execution agent profile for a run, but it cannot define provider
order, mappings, health policy, or fallback rules outside the selected
dynamic profile.

Utility agents also accept concrete or dynamic profiles. A one-shot utility
invocation has no visible task-session tab, but it uses the same resolver and
dynamic conductor. Its durable call record retains the selected logical profile
and the concrete execution profile that produced the result.

### Keep identity, policy, and execution separate

A routed Office run has three distinct identities:

1. The Office agent identity owns its role, instructions, skills, permissions,
   budget, hierarchy, and Office history.
2. The assigned dynamic profile owns routing policy and is the logical agent
   shown for the task session.
3. The resolved concrete profile owns the provider CLI, account environment,
   model, mode, ACP options, flags, permissions, and MCP configuration for the
   current downstream session.

A routed Kanban run has the latter two identities. A concrete-profile run has
no separate routing identity.

The runtime and persistence contracts must carry these identities explicitly.
They must not infer one identity from another or construct a hybrid launch by
overlaying one provider's settings on another profile.

### The dynamic runtime is a virtual ACP conductor

The task backend, workflow scheduler, and task UI continue to see one Kandev
task session and one chat tab. A dynamic-profile runtime owns the downstream
ACP lifecycle behind that session.

When a route changes, the conductor:

1. settles or stops the failed downstream attempt,
2. persists the route transition and invalidates its provider-native resume
   token for the successor,
3. starts a new ACP session with the selected concrete profile,
4. builds a bounded continuation prompt from durable task state, run state,
   recent conversation, repository state, and an optional handoff summary, and
5. publishes a route-change event followed by an authoritative replacement of
   models, modes, configuration options, and custom commands.

The provider-native session is not exposed as another Kandev task session and
does not create another tab. Same-profile restart can use that provider's
`session/load` or `session/resume`. Cross-profile continuation always starts a
fresh provider-native session.

These events are Kandev extensions around ACP, not claims that standard ACP can
import one provider's history into another provider.

### Routing policy follows profile kind

Provider-error classification remains shared and provider-neutral. The
selected profile kind chooses the recovery owner:

- A concrete profile uses conservative interactive same-profile recovery. It
  never changes account, model, or provider automatically.
- A dynamic profile uses its configured retry and fallback policy in Kanban or
  Office. Selecting it is prior authorization to use its eligible routes.

The dynamic conductor is the only owner of cross-candidate fallback. A concrete
profile with legacy `AutoFallback` enabled is not a valid dynamic candidate. An
explicit `FallbackModel` remains an intra-profile start-model policy and does
not select another dynamic candidate.

This replaces the earlier rule that Kanban and Office necessarily have
different routing owners.

### Profile edits use the next routing decision

Dynamic profile configuration is versioned. Normal edits affect a running
logical session at its next routing decision. They do not churn an otherwise
healthy active route. Disabling or removing the active candidate schedules a
reroute at the next safe turn boundary. Historical route decisions retain the
profile version that produced them.

### Logical identity and concrete attribution coexist

The Kanban tab and assistant author keep the dynamic profile's identity. In
Office, they keep the Office agent identity, such as CEO. Every assistant turn
stores immutable concrete profile, provider, model, and route generation
attribution and displays it as compact metadata. A route change also
creates a durable system row, while the live model, mode, options, and commands
always come from the active concrete route.

Turn attribution extends the immutable snapshot boundary from
[Attribute Runtime Configuration to Turns](2026-07-18-turn-configuration-snapshots.md),
historical messages never inherit the session's latest route.

### Health is shared by the resource that failed

Route health uses a scope-specific resource key. Provider outages use provider
scope. Quota failures use provider and credential-binding scope. Model evidence
adds model scope. Profile configuration errors use concrete-profile scope.

Each concrete launch adapter produces a versioned
`CredentialBindingDescriptor` after environment resolution. It includes the
canonical family, authentication mechanism, non-secret credential source,
executor namespace, authorization scope, and applicable workspace scope.
`CredentialBindingResolver` canonicalizes the descriptor and computes an
HMAC-SHA-256 fingerprint with a persistent Kandev installation key. It cannot
include raw credentials, credential-file contents, literal environment values,
`CommandPrefix`, CLI flags, models, prompts, or raw account IDs. If an adapter
cannot prove a shared binding, the resolver uses a conservative profile-scoped
identity.

A classified hard or long-horizon failure atomically opens the shared circuit.
Other dynamic sessions that reference the same resource skip it while the
circuit is open. One half-open attempt owns the probe lease. Successful probes
close the circuit, failed probes extend its deadline. Existing sessions on a
fallback route are not moved back automatically when an earlier candidate
recovers. Recovery affects new selections and explicit safe-boundary reroutes.

### Telemetry routing is a separate feature

This decision implements the virtual family, transparent resolver, fixed
candidate order, classified-error fallback, shared circuits and probes, the ACP
conductor, continuation, restart recovery, and Kanban, Office, and utility
integration. It does not add session-cost or subscription-usage plugin
contracts. It does not add telemetry-backed rules or turn interruption.

Those behaviors belong to a separate deferred Dynamic Agent Telemetry Routing
feature and implementation package. An implementation of this decision must
not implement that deferred package.

### Legacy Office routing data is retained but inactive

Office has not been released as an enabled user workflow, so Kandev does not
translate legacy Office workspace routing into dynamic profiles. Existing
Office routing tables and rows remain intact across upgrade. New code stops
displaying and mutating those settings, and users configure dynamic profiles
when they want routing. A future cleanup requires a separate retention decision,
but this migration does not drop or rewrite the legacy rows.

The handoff removes the live Office `StartTaskWithRoute` and `RouteOverride`
path, including legacy model, flag, and environment overlays. Dynamic routing
always launches one complete concrete profile. `KANDEV_MOCK_PROVIDERS` remains
generic E2E infrastructure, but the Office-specific routable-provider catalogue
does not remain a production routing authority.

## Consequences

### Positive

- Kanban and Office share one routing model, runtime, settings surface, health
  service, and observability contract.
- Office configuration becomes smaller: an agent chooses an execution agent
  profile.
- One task session can survive provider changes without creating misleading
  session tabs.
- The initial feature has no dependency on new plugin contracts.
- A failed account is suppressed across concurrent tasks without disabling
  healthy accounts from the same provider.

### Costs

- The dynamic conductor becomes a stateful runtime boundary that must persist
  route generation, active concrete profile, downstream session identity, and
  circuit state.
- The UI needs explicit route-transition and capability-replacement events.
- Cross-provider continuation is semantic rather than lossless. It depends on
  durable task and repository state plus a bounded resume prompt.
- Existing Office routing runtime ownership and settings UI must be removed
  before Office is enabled. Existing table rows remain stored but inactive.

## Alternatives Considered

### Keep separate Kanban and Office routers

Rejected. Profile selection has different behavior in each workspace mode.
This alternative also duplicates routing policy, health state, UI, and tests.

### Keep routing in the workflow or Office scheduler

Rejected. A task-level scheduler must not manage downstream ACP sessions or
provider-native resume tokens. It also creates another task session or tab
when a provider changes.

### Treat the dynamic profile as an ordinary ACP provider

Rejected. No downstream ACP agent can import another provider's session, and
standard ACP does not describe Kandev's logical route identity or atomic
capability replacement. The conductor must own those extensions.

### Include plugin telemetry in the initial delivery

Rejected for this package. Provider-error fallback and ordered routing deliver
the dynamic-profile value without a new plugin contract. Telemetry needs a
separate contract, rollout flag, persistence model, and test package.

## Relationship to Prior Decisions

This decision supersedes
[Separate Office identity from routed execution profiles](2026-07-15-office-agent-execution-profile-routing.md)
as the owner and scope of routing. It retains that decision's separation
between Office identity and concrete execution configuration, then adds a
dynamic routing-policy identity between them.

This decision amends
[Separate Agent Error Evidence From Recovery Policy](2026-08-08-provider-neutral-agent-error-recovery.md):
the recovery policy is now selected by concrete versus dynamic profile kind,
not by Kanban versus Office workspace mode.

This decision also amends
[Attribute Runtime Configuration to Turns](2026-07-18-turn-configuration-snapshots.md):
a routed turn's immutable configuration snapshot includes its concrete
execution profile and route generation.

Product behavior is specified in
[Dynamic Agent Routing](../specs/agents/requirements/dynamic-agent-routing.md).
Deferred telemetry behavior has no active specification.
