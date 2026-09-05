---
status: draft
system: office
requirements:
  - REQ-OFFICE-ROUTING-001
created: 2026-05-10
owners:
  - cfl
---
# Office Provider Routing System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

> Superseded by [Dynamic Agent Routing](../../agents/requirements/dynamic-agent-routing.md).
> Office now selects a concrete or dynamic execution agent profile. Provider
> mappings, rules, health, and fallback belong to the reusable dynamic profile,
> not to Office workspace settings. This file is retained as the historical
> Office-only contract until its implementation is migrated.
>
> Upgrade compatibility retains the legacy Office routing tables and rows but
> no longer displays or reads them for launch decisions. They are not converted
> automatically. Users configure concrete or dynamic execution profiles for new
> behavior.

Provider routing lets a workspace map abstract model tiers (Frontier / Balanced / Economy) to concrete execution profiles, configure an ordered provider fallback chain, and degrade routes automatically when they hit auth / quota / rate / outage limits. An execution profile is a complete launch configuration, not a model-only overlay. This spec describes the routing contract: Office identity preservation, execution-profile selection, fallback, provider health, degraded-state lifecycle, and wake-reason tier policy.

## Why

Office agents need a predictable way to choose between CLI providers, accounts, and model strengths without cloning their role configuration. Users also need controlled fallback when a provider hits subscription or rate limits while preserving the Office agent's instructions, skills, permissions, budget, task, and worktree.

## What

### Enablement and tiering

- Provider routing is an advanced workspace setting and automatic fallback is disabled by default.
- Office always resolves an execution profile from the agent's effective tier, even when automatic fallback is disabled.
- When routing is disabled, Office selects only the first configured provider in the effective provider order for that tier. It does not health-filter, try a later provider, or silently use a workspace default profile.
- Short same-route transient retry remains active when routing is disabled; only
  cross-provider fallback is disabled.
- Every Office agent has an effective model tier even when routing is disabled.
- Workspace settings provide the default model tier, initially `balanced`.
- Agents inherit the workspace default tier unless the user sets an agent-specific override.

### Provider order and tier mapping

- Workspace settings can define a global provider order, for example `claude -> codex -> opencode`.
- Workspace settings map each provider and tier to an existing execution profile. The selected profile supplies the provider CLI, account environment, model, mode, ACP config options, CLI flags and permission behavior, passthrough mode, and MCP configuration.
- Tier labels are user-facing as Frontier, Balanced, and Economy.
- Example tier mappings include Claude `frontier=opus`, `balanced=sonnet`, `economy=haiku`, and Codex `frontier=gpt-5.5 high`, `balanced=gpt-5.4`, `economy=gpt-5.3 mini`.
- Each Office agent can inherit workspace routing or override it.
- Agent overrides can choose a model tier and provider order independently; an override order with only `claude` never falls back to `codex` or `opencode`.
- Users configure provider tier profiles explicitly; the UI does not silently apply recommended presets or accept a raw model string as a complete route.
- Onboarding lets the user choose one concrete CLI profile for the coordinator and one source profile for each workspace tier: Frontier, Balanced, and Economy. Office expands those source profiles into provider/model tier mappings so enabling routing later does not require editing every agent.
- Persisted routing keys use the provider's logical registry name (for example `claude-acp`), resolved from the source profile's parent agent. Database UUIDs are never valid provider IDs.
- The tier profile selectors are profile-family choices, not direct agent assignments. When the CEO creates a worker, QA, or specialist agent and assigns a tier, the agent inherits the workspace tier family selected during onboarding or later routing settings.
- Execution profile IDs are the authoritative tier mappings. Provider and model labels shown by routing are derived snapshots from the referenced profile.
- Deleting a profile that is still referenced by a workspace tier returns an in-use error until the mapping is changed.
- A referenced profile must exist, be active, belong to the same workspace or be global, carry a launchable CLI configuration, and resolve to the logical provider whose routing entry contains it.

### Run resolution

- Every Office run has two explicit identities: stable `agent_profile_id` for the Office agent and concrete `execution_profile_id` for the selected launch profile.
- Run launch resolves the effective tier in this order: wake-reason policy (agent override > workspace policy) -> agent tier override -> workspace default tier.
- It then resolves the effective provider order and loads the execution profile referenced by each provider's effective-tier mapping.
- `enabled=false` returns only the first configured candidate and disables automatic fallback. `enabled=true` returns the ordered healthy candidates and preserves the existing degradation/parking policy.
- Missing, deleted, cross-workspace, wrong-provider, or ambiguous execution-profile mappings are actionable configuration errors. They never fall through to the workspace default agent profile.
- Provider adapters classify launch and runtime failures into normalized provider-routing error codes using structured signals, provider-specific message patterns, and adapter phase context.
- Unknown failures in provider-owned phases can fall back as low-confidence `unknown_provider_error` events, with raw evidence preserved for later classifier updates.

### Fallback chain

- High-confidence short transients do not enter the fallback chain immediately.
  Network interruption, temporary provider unavailability/overload, model
  capacity, and rate-limit waits of at most 60 seconds first retry the same
  execution profile after 5, 10, and 20 seconds.
  - The short same-route retry phase is scheduler-owned. The current run's
  `retry_scheduled` route attempt records the retry owner; a
  workspace/provider/model-scoped `short_retry` cooldown makes concurrent runs
  wait instead of switching or retrying independently. It does not enter the
  long-horizon degraded state or route-cycle exclusion set until its
  three-attempt budget is exhausted. The UI shows the same provider, exact next
  retry, and attempt ordinal throughout this phase.
- Fallback is more conservative after the agent has started task work; post-start fallback only happens for clear provider, auth, quota, rate-limit, outage, or model-availability failures.
- Detectable provider-unavailable errors can fall back to the next configured provider after any required short phase, including auth failures, expired credentials, persistent provider outages, subscription limits, quota limits, and long rate limits.
- When an already-degraded provider route fails again during a health retry/probe, the same task immediately tries the next configured provider candidate and the failed provider route stays degraded with an increased backoff.
- A long-horizon failure, or exhaustion of the short same-route budget, enters
  the existing degradation and fallback behavior. A validated rate-limit wait
  longer than 60 seconds, renewable quota, auth/subscription, and invalid model
  or configuration evidence bypass the short phase.
- If every configured provider route is unavailable, the task keeps its logical Office agent assignment and waits for provider capacity or user action instead of being reassigned.
- If at least one exhausted provider route is auto-retryable, the scheduler wakes the blocked task automatically at the earliest retry time.
- When fallback selects a different execution profile after work has started, Kandev reuses the task, run, task environment, and worktree but starts a fresh provider-native session. A resume token created by one execution profile is never supplied to another.
- The fallback launch reapplies the Office agent's instructions and skills and adds a continuation instruction telling the new agent to inspect the durable task description, comments/messages, status, run state, and current git worktree before continuing.
- Provider-native chat history is not transferred across providers. Durable task state and repository state are the handoff contract.

### Provider health checks and degraded state

- A degraded provider route becomes temporarily ineligible for affected workspace launches until it reaches a retry time, passes a health check, succeeds on a launch probe, reconnects, or the user retries it from the UI.
- After the short same-route budget is exhausted, auto-retryable provider-unavailable errors without a known reset time use minute-scale exponential backoff before that degraded route is retried.
- User-actionable provider blocks, such as missing credentials, expired auth, inactive subscription, provider not installed, provider not configured, or missing model-tier mapping, stay blocked until the user fixes configuration, reconnects, or manually retries.
- Provider health issues are visible in workspace settings, dashboard, inbox, and affected agent detail pages.

### Wake-reason tier policy

- Workspace settings can map specific wake reasons (heartbeat, routine_trigger, budget_alert) onto specific tiers so background work cheaps out automatically.
- The workspace policy applies to every agent in the workspace unless that agent overrides it.
- Agents can override the workspace policy with their own per-reason tier map (rare; meant for security-critical agents that need Frontier even for routine checks).
- An override map replaces the workspace map entirely - keys missing from the override fall through to the agent's normal effective tier rather than the workspace policy.
- Default workspace policy seeded at onboarding is Economy for all three reasons, mirroring the legacy cheap-profile shortcut so the out-of-the-box behaviour is unchanged when routing is enabled.
- The resolver order is: wake-reason policy (agent override > workspace policy) -> agent tier override -> workspace default tier.
- The wake-reason policy is the single surface for "use a cheaper model for low-stakes background runs"; the legacy `cheap_agent_profile_id` mechanism no longer exists.

### Surfacing

- Task and run surfaces show the logical Office agent, requested tier, resolved provider/model, and fallback state when applicable.
- Run history preserves the route decision made at launch, including provider order, selected provider/model, fallback attempts, and fallback reasons.
- Run history preserves classifier evidence for fallback decisions, including normalized code, confidence, adapter phase, classifier rule ID, exit code when available, and a short raw error excerpt.
- Agent detail and agent list surfaces show the current resolved provider/model, whether the tier and route are inherited or overridden, and whether the agent is currently degraded by provider fallback.

## Data model

Provider routing persists the workspace routing config, concrete execution-profile decisions, per-provider health rows, and per-run routing telemetry. Agent overrides ride on the stable Office identity's `agent_profiles.settings` JSON blob (no new identity table).

### `office_workspace_routing` (per workspace)

One row per workspace. Defaulted in-code to `Enabled=false, DefaultTier=balanced, ProviderOrder=[], ProviderProfiles={}, TierPerReason={}` when the row is absent (see `internal/office/repository/sqlite/workspace_routing.go::defaultWorkspaceRouting`).

```
office_workspace_routing
  workspace_id      string  PK
  enabled           int     0|1
  default_tier      string  frontier|balanced|economy
  provider_order    text    JSON array of provider IDs
  provider_profiles text    JSON map: provider_id -> {execution_profile_ids}
  tier_per_reason   text    JSON map: wake_reason -> tier (NOT NULL, "{}" default)
  updated_at        timestamp
```

`provider_profiles[pid].execution_profile_ids` is `{frontier, balanced, economy}` with each value either an active execution profile ID or empty string ("skip this provider for this tier"). It is authoritative at runtime. The backend derives provider/model display snapshots by loading the referenced profile; routing does not persist independent mode, flags, environment, permissions, or MCP overlays.

For upgrade compatibility the decoder accepts the legacy `tier_profile_ids` key and writes the canonical `execution_profile_ids` key. Legacy `tier_map` entries are migrated only when Kandev can find exactly one active execution profile with the same logical provider and model (and mode when present). Missing or ambiguous matches surface as configuration errors that require profile selection. A workspace with no routing row is seeded from its existing Office configuration so an upgrade does not silently switch providers.

`tier_per_reason` keys are restricted to `heartbeat | routine_trigger | budget_alert` (see `routing.AllWakeReasons`); other keys are rejected by `routing.ValidateWorkspaceConfig`.

### Dual identity persistence

The stable Office identity remains in existing `agent_profile_id` columns. Concrete launch choice is stored separately:

```
task_sessions
  agent_profile_id       string  stable Office identity
  execution_profile_id   string  concrete profile for the current execution

executors_running
  agent_profile_id       string  stable Office identity where already present
  execution_profile_id   string  profile that owns the process and resume token

runs
  agent_profile_id                    string  stable Office identity
  resolved_execution_profile_id       string  concrete profile that launched

office_run_route_attempts
  execution_profile_id   string  concrete profile tried by this attempt
```

`task_sessions.execution_profile_id` and `executors_running.execution_profile_id` are required for restart recovery. A provider-native resume token is valid only when the stored execution profile matches the profile selected for the next launch. Existing non-Office sessions may use `execution_profile_id = agent_profile_id`.

### `office_provider_health` (per workspace, per provider, per scope)

```
office_provider_health
  workspace_id   string     PK part
  provider_id    string     PK part
  scope          string     PK part: provider|tier|model
  scope_value    string     PK part: "" for provider; tier name; model id
  state          string     healthy|short_retry|degraded|user_action_required
  error_code     string     normalized routingerr.Code, "" when healthy
  retry_at       timestamp  nullable; unset for user_action_required
  backoff_step   int        current step in routing.backoffSchedule
  last_failure   timestamp  nullable
  last_success   timestamp  nullable
  raw_excerpt    text       sanitized stderr/stdout snippet, nullable
  updated_at     timestamp
```

Primary key is `(workspace_id, provider_id, scope, scope_value)` so a tier-specific failure does not take the whole provider down. `ScopeFromCode` maps normalized error codes to the scope to degrade: `model_unavailable -> (model, <model>)`, `provider_not_configured` (missing tier mapping) -> `(tier, <tier>)`, every other auth/quota/rate/provider failure -> `(provider, "")`.

The resolver walks scope priority `provider -> tier -> model` and uses the first non-healthy hit. A `short_retry` hit holds matching runs on the same route until its deadline; it never walks to a later candidate. The owning run is recorded by its `retry_scheduled` route attempt. `MarkProviderDegraded` increments `backoff_step` on every degraded -> degraded transition; short-retry transitions do not consume the long-horizon backoff budget. `MarkProviderHealthy` clears `backoff_step`, `retry_at`, and `error_code`.

### `runs` routing columns (per run)

`runs` carries the per-run routing decision, short-retry phase, and parking state.
The short-retry fields apply to every Office run. Other route-decision columns
may remain nullable / zero-valued for legacy runs that never resolved an Office
execution profile.

```
runs (routing additions)
  logical_provider_order     text   JSON snapshot of effective order at launch, stable across post-start fallbacks
  requested_tier             string the tier the resolver consumed (override > default)
  resolved_provider_id       string the provider that actually launched
  resolved_model             string model on the launched provider
  resolved_execution_profile_id string concrete profile that supplied the launch configuration
  current_route_attempt_seq  int    monotonically bumped by IncrementRouteAttemptSeq
  route_cycle_baseline_seq   int    seq floor for the current retry cycle; attempts at or below the baseline are NOT in the exclude-set
  routing_blocked_status     string waiting_for_provider_capacity | blocked_provider_action_required
  earliest_retry_at          timestamp nullable; minimum auto-retry deadline across degraded routes
  scheduled_retry_at         timestamp next same-route retry or parked-run wake
```

### `office_run_route_attempts` (per run, per attempt)

One row per provider attempt inside a run. `(run_id, seq)` is the primary key. Each fallback (resolver candidate tried) appends a new row keyed by the next `current_route_attempt_seq`. Persisted by the dispatcher in `internal/office/scheduler/dispatch_routing.go`.

```
office_run_route_attempts
  run_id           string  PK part, FK -> runs.id
  seq              int     PK part
  provider_id      string
  execution_profile_id string
  model            string
  tier             string
  outcome          string  succeeded|failed|skipped
  error_code       string  routingerr.Code; nullable
  error_confidence string  high|medium|low; nullable
  adapter_phase    string  routingerr.Phase; nullable
  classifier_rule  string  the rule id that produced the classification
  exit_code        int     nullable
  raw_excerpt      text    sanitized stderr/stdout snippet
  reset_hint       timestamp nullable; provider-supplied retry deadline
  started_at       timestamp
  finished_at      timestamp nullable
```

A short transient attempt uses `retry_scheduled` and does not join the current
route cycle's provider exclusion set. Its durable owner is the run identified
by `run_id` plus `runs.current_route_attempt_seq`; its ordinal is the count of
`retry_scheduled` rows after `route_cycle_baseline_seq`. The affected health
scope and cooldown deadline live in `office_provider_health.scope`,
`scope_value`, and `retry_at`, while `runs.scheduled_retry_at` mirrors that
deadline. On restart, the latest attempt marker preserves the short-retry
cycle; a resolver seeing a live `short_retry` row holds the route and does not
fall through. The final exhausted attempt changes to the existing
provider-unavailable failure outcome before fallback resolution.

### Agent overrides (`agent_profiles.settings.routing`)

No new table. `routing.AgentOverrides` is JSON-serialized under the top-level `"routing"` key of `agent_profiles.settings`. Round-tripped by `routing.ReadAgentOverrides` / `routing.WriteAgentOverrides`. Zero blob deletes the key.

```
AgentOverrides
  provider_order_source    string  "" (inherit) | "override"
  provider_order           []ProviderID
  tier_source              string  "" (inherit) | "override"
  tier                     Tier
  tier_per_reason_source   string  "" (inherit) | "override"
  tier_per_reason          map[wake_reason]Tier
```

A `*_source` value of `"override"` replaces the workspace equivalent entirely; any other value (including the empty string default) inherits. Keys missing from an override `tier_per_reason` map fall through to the agent's normal effective tier rather than the workspace policy.

## API surface

### Go interfaces

`routing.Resolver` (`internal/office/routing/resolver.go`) is the single seam the scheduler dispatcher calls to pick a candidate.

```go
type Resolver interface {
    Resolve(ctx context.Context, workspaceID string,
        agent settingsmodels.AgentProfile, opts ResolveOptions) (*Resolution, error)
}

type ResolveOptions struct {
    ExcludeProviders []ProviderID // providers already tried in this run
    Reason           string       // wake reason; consults TierPerReason
}

type Resolution struct {
    FallbackEnabled bool             // false returns only the first configured candidate
    RequestedTier   Tier
    ProviderOrder   []ProviderID
    Candidates      []Candidate      // ordered, exclude-set-filtered
    SkippedDegraded []SkippedCandidate
    BlockReason     BlockReason      // populated when Candidates is empty
}
```

`Candidate` carries `(ExecutionProfileID, ProviderID, Model, Tier)`. `ProviderID` and `Model` are audit/display snapshots derived from the selected execution profile. The runtime resolves the complete launch configuration from `ExecutionProfileID`; routing never combines a base profile with mode/flags/env overlays from another provider.

`routingerr.ProviderProber` (`internal/agent/runtime/routingerr/probe.go`) is the optional cheap-availability check:

```go
type ProviderProber interface {
    Probe(ctx context.Context, in ProbeInput) *Error
}

// Package-level registry
func RegisterProber(providerID string, p ProviderProber)
func GetProber(providerID string) (ProviderProber, bool)
```

Probers must complete in under five seconds and MUST NOT start an agent session. Missing prober means "use the next real launch as the probe" — no default LaunchAsProbe shim exists in v1.

`routingerr.Classify(Input) *Error` is the classifier entry point used by adapters. `Input` carries `(Phase, ProviderID, ExitCode, StructuredErr, HTTPStatus, Stderr, Stdout)`; the returned `*Error` carries `(Code, Confidence, Phase, FallbackAllowed, AutoRetryable, UserAction, ClassifierRule, ExitCode, ResetHint, RawExcerpt)`. Normalized `Code` values include `auth_required`, `missing_credentials`, `subscription_required`, `quota_limited`, `rate_limited`, `network_unavailable`, `provider_unavailable`, `provider_overloaded`, `model_capacity`, `model_unavailable`, `provider_not_configured`, `unknown_provider_error`, `agent_runtime_error`, `permission_denied_by_user`, `task_error`, `repo_error`.

The provider-neutral recovery contract in
[`platform/provider-error-recovery.md`](../../platform/requirements/provider-error-recovery.md)
is authoritative for semantic classification only. `AutoRetryable`,
`FallbackAllowed`, and `UserAction` are compatibility projections of the Office
routing policy, not universal classifier truths. Office may fall through to
another configured provider for subscription or quota failures. Renewable quota
can park a run until its reset/backoff; an inactive subscription leaves that
route user-action-required with no timed retry.

### HTTP routes (workspace-level, mounted in `dashboard`)

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/workspaces/:wsId/routing` | – | `{config: WorkspaceConfig, known_providers: [ProviderID], execution_profiles: [ExecutionProfileSummary]}` |
| PUT | `/workspaces/:wsId/routing` | `WorkspaceConfig` | `204` on success, `400` with `{error, field, details[]}` on validation failure |
| POST | `/workspaces/:wsId/routing/retry` | `{provider_id}` | `{status: "probed"\|"retrying", retry_at?}` |
| GET | `/workspaces/:wsId/routing/health` | – | `{health: [ProviderHealth]}` (non-healthy rows only) |
| GET | `/workspaces/:wsId/routing/preview` | – | `{agents: [AgentRoutePreview]}` |
| GET | `/agents/:id/route` | – | `{preview, overrides, last_failure_code?, last_failure_run?}` |

The per-agent override blob is written via the existing `PATCH /agents/:id` endpoint (mounted in `internal/office/agents`): the request body's `settings` JSON round-trips the `"routing"` key. Validation runs `routing.ValidateAgentOverridesAgainstWorkspace`, which adds a cross-check that the overridden tier is mapped on at least one provider in the effective order so save-time errors surface immediately instead of at the next launch.

`PUT /workspaces/:wsId/routing` always validates the first provider's execution profile for every effective tier that can launch. Enabling automatic fallback additionally validates every provider candidate in the order. Each referenced profile must exist, be active, be a shallow runtime profile rather than a rich Office identity, be global or same-workspace, and resolve to the provider key that contains it. Material config changes (provider order, default tier, execution profile mappings) trigger `ClearAllParkedRoutingForWorkspace`, which re-queues every parked run in the workspace.

### WS event types (forwarded to clients by the office WS broadcaster)

| Event type | Payload fields | When published |
|---|---|---|
| `office.provider.health_changed` | `workspace_id, provider_id, scope, scope_value, state, error_code, retry_at` | Whenever the scheduler marks a provider health row degraded / healthy / user-action |
| `office.route_attempt.appended` | `run_id, attempt` (full `RouteAttempt` shape) | After the dispatcher writes a `RouteAttempt` row |

Internal expvar metrics (`/debug/vars` in dev): `routing_route_attempts_total`, `routing_route_fallbacks_total`, `routing_route_parked_total`, `routing_provider_degraded_total`, `routing_provider_recovered_total`. Same events are also logged as structured zap entries under `routing.metric.*`.
