---
status: draft
created: 2026-05-10
owner: kandev
---

# Office Provider Routing

## Why
Office agents need a predictable way to choose between CLI providers and model strengths without manually cloning every agent profile for every provider. Users also need controlled fallback when a provider hits subscription or rate limits, while keeping important agents pinned to a specific provider when desired.

## What
- Provider routing is an advanced workspace setting and is disabled by default.
- When provider routing is disabled, Office uses the concrete CLI profile chosen for the agent exactly as it does today.
- Every Office agent has an effective model tier even when routing is disabled.
- Workspace settings provide the default model tier, initially `balanced`.
- Agents inherit the workspace default tier unless the user sets an agent-specific override.
- Workspace settings can define a global provider order, for example `claude -> codex -> opencode`.
- Workspace settings can map each provider to three model tiers: `frontier`, `balanced`, and `economy`.
- Tier labels are user-facing as Frontier, Balanced, and Economy.
- Example tier mappings include Claude `frontier=opus`, `balanced=sonnet`, `economy=haiku`, and Codex `frontier=gpt-5.5 high`, `balanced=gpt-5.4`, `economy=gpt-5.3 mini`.
- Each Office agent can inherit workspace routing or override it.
- Agent overrides can choose a model tier and provider order independently; an override order with only `claude` never falls back to `codex` or `opencode`.
- Run launch resolves a logical Office agent to an effective provider/model only when provider routing is enabled.
- Provider adapters classify launch and runtime failures into normalized provider-routing error codes using structured signals, provider-specific message patterns, and adapter phase context.
- Unknown failures in provider-owned phases can fall back as low-confidence `unknown_provider_error` events, with raw evidence preserved for later classifier updates.
- Fallback is more conservative after the agent has started task work; post-start fallback only happens for clear provider, auth, quota, rate-limit, outage, or model-availability failures.
- Detectable provider-unavailable errors can fall back to the next configured provider, including auth failures, expired credentials, provider outages, subscription limits, quota limits, and rate limits.
- A degraded provider route becomes temporarily ineligible for affected workspace launches until it reaches a retry time, passes a health check, succeeds on a launch probe, reconnects, or the user retries it from the UI.
- Auto-retryable provider-unavailable errors without a known reset time use exponential backoff before that provider route is retried.
- User-actionable provider blocks, such as missing credentials, expired auth, inactive subscription, provider not installed, provider not configured, or missing model-tier mapping, stay blocked until the user fixes configuration, reconnects, or manually retries.
- When a provider route fails again during a retry/probe, the same task immediately tries the next configured provider candidate and the failed provider route stays degraded with an increased backoff.
- If every configured provider route is unavailable, the task keeps its logical Office agent assignment and waits for provider capacity or user action instead of being reassigned.
- If at least one exhausted provider route is auto-retryable, the scheduler wakes the blocked task automatically at the earliest retry time.
- Task and run surfaces show the logical Office agent, requested tier, resolved provider/model, and fallback state when applicable.
- Run history preserves the route decision made at launch, including provider order, selected provider/model, fallback attempts, and fallback reasons.
- Run history preserves classifier evidence for fallback decisions, including normalized code, confidence, adapter phase, classifier rule ID, exit code when available, and a short raw error excerpt.
- Agent detail and agent list surfaces show the current resolved provider/model, whether the tier and route are inherited or overridden, and whether the agent is currently degraded by provider fallback.
- Provider health issues are visible in workspace settings, dashboard, inbox, and affected agent detail pages.
- Users configure provider tier mappings explicitly; the UI does not silently apply recommended presets.
- Onboarding keeps its current simple flow: the user chooses one concrete CLI profile, model, and mode for the coordinator. Office stores an effective tier through workspace default inheritance, so enabling routing later does not require editing every agent.

### Wake-reason tier policy

- Workspace settings can map specific wake reasons (heartbeat, routine_trigger, budget_alert) onto specific tiers so background work cheaps out automatically.
- The workspace policy applies to every agent in the workspace unless that agent overrides it.
- Agents can override the workspace policy with their own per-reason tier map (rare; meant for security-critical agents that need Frontier even for routine checks).
- An override map replaces the workspace map entirely — keys missing from the override fall through to the agent's normal effective tier rather than the workspace policy.
- Default workspace policy seeded at onboarding is Economy for all three reasons, mirroring the legacy cheap-profile shortcut so the out-of-the-box behaviour is unchanged when routing is enabled.
- The resolver order is: wake-reason policy (agent override > workspace policy) → agent tier override → workspace default tier.
- The wake-reason policy is the single surface for "use a cheaper model for low-stakes background runs"; the legacy `cheap_agent_profile_id` mechanism no longer exists.

## Scenarios
- **GIVEN** provider routing is disabled, **WHEN** the coordinator agent launches, **THEN** it uses the concrete profile copied during onboarding with its selected model and mode.
- **GIVEN** workspace routing is enabled with `claude -> codex -> opencode` and tier `balanced`, **WHEN** an agent without overrides launches, **THEN** it first tries the Claude balanced model and may fall back through the remaining providers on provider-limit errors.
- **GIVEN** the CEO agent overrides provider order to `claude` and tier `frontier`, **WHEN** Claude is rate-limited, **THEN** the CEO run does not try Codex or OpenCode and follows the normal failure/escalation path.
- **GIVEN** a worker agent inherits workspace routing, **WHEN** the workspace default tier changes from `frontier` to `balanced`, **THEN** future worker runs use the balanced tier without editing the worker.
- **GIVEN** routing is enabled after several agents already exist, **WHEN** those agents still inherit workspace routing, **THEN** their future runs use the workspace default tier and provider order without requiring per-agent edits.
- **GIVEN** a task run falls back from Claude to Codex because Claude is quota-limited, **WHEN** the user opens the task, run history, agent detail, or dashboard, **THEN** the UI shows the intended primary route, the actual provider/model, and the quota-limit reason.
- **GIVEN** Claude credentials expire, **WHEN** an agent with fallback providers launches, **THEN** the scheduler records the auth failure, marks Claude degraded for the workspace, and tries the next configured provider.
- **GIVEN** a provider adapter returns a known quota, auth, or rate-limit signal, **WHEN** route resolution handles the failure, **THEN** the scheduler uses the normalized error code rather than provider-specific prose.
- **GIVEN** a provider fails before session start with an unrecognized provider-owned error, **WHEN** no classifier rule matches, **THEN** the scheduler records a low-confidence `unknown_provider_error`, preserves evidence, and tries the next configured provider.
- **GIVEN** a task has already started editing or running tools, **WHEN** the provider reports an ambiguous failure, **THEN** the scheduler does not fall back unless the error clearly matches a provider-unavailable class.
- **GIVEN** a provider returns a generic provider-unavailable error without a reset time, **WHEN** the route fails, **THEN** the scheduler marks that provider route degraded, sets a short backoff retry time, and runs the same task through the next configured provider.
- **GIVEN** a degraded provider reaches its retry time, **WHEN** a health probe or task-launch probe fails again, **THEN** the task immediately tries the next candidate and the provider route receives a longer backoff.
- **GIVEN** a provider is marked degraded, **WHEN** a scheduled health check, launch probe, reconnect, or user-triggered retry succeeds, **THEN** future launches can use that provider again.
- **GIVEN** all configured providers are quota-limited or temporarily unavailable, **WHEN** a task exhausts the route list, **THEN** the task waits for provider capacity and automatically retries at the earliest route retry time.
- **GIVEN** all configured providers require user action, **WHEN** a task exhausts the route list, **THEN** the task is blocked until the user reconnects, configures a provider, fixes model mappings, or manually retries.
- **GIVEN** exhausted provider routes include both auto-retryable and user-actionable failures, **WHEN** the task is blocked, **THEN** the UI shows the earliest automatic retry and the user actions needed for the blocked routes.
- **GIVEN** a provider is unavailable for a workspace, **WHEN** the provider health state changes, **THEN** the dashboard and inbox show an actionable issue listing affected agents and routes.
- **GIVEN** onboarding creates a new workspace, **WHEN** setup completes, **THEN** no provider routing policy is enabled or required.
- **GIVEN** routing is enabled and the workspace's `tier_per_reason.heartbeat = economy`, **WHEN** a heartbeat run launches, **THEN** the resolver picks the Economy tier model regardless of the agent's default tier.
- **GIVEN** the security agent overrides wake-reason tiers with `heartbeat = frontier`, **WHEN** its heartbeat fires, **THEN** it uses Frontier even though the workspace policy says Economy.
- **GIVEN** a run reason has no policy (e.g. `task_assigned`), **WHEN** that run launches, **THEN** it uses the agent's effective tier as before, ignoring the wake-reason policy entirely.

## Out of scope
- Requiring every agent to be edited before provider routing can be enabled.
- Routing non-Office kanban sessions.
- Changing onboarding to require provider-routing setup.
- Shipping recommended provider/model tier presets.
- Cost optimization beyond user-selected tiers and provider order.
- Per-wake-reason policy for reasons outside `{heartbeat, routine_trigger, budget_alert}` in v1. Future work could extend the set.
