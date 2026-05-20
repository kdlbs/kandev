# 0006: Tier routing vs cheap_agent_profile_id coexistence

**Status:** superseded (2026-05-11) by the wake-reason tier policy patch
**Date:** 2026-05-11
**Area:** backend

> **Supersession (2026-05-11).** This ADR's "keep both mechanisms" call
> has been reversed. `cheap_agent_profile_id`, `resolveModelProfile`,
> and the per-task `model_profile` override no longer participate in
> the launch path — every Office launch goes through the routing
> resolver. The cheap variant's three trigger contexts (heartbeats,
> routine triggers, budget alerts) are now expressed as a workspace
> `TierPerReason` policy with optional per-agent override
> (`routing.AgentOverrides.TierPerReason`). Onboarding seeds
> `Economy` for all three reasons so the default behaviour matches
> the legacy cheap-profile shortcut without an agent edit. See the
> updated spec/plan under `docs/specs/office-provider-routing/` for
> the unified model.

## Context

Office provider routing (`docs/specs/office-provider-routing/`) introduced a workspace-scoped routing surface that maps every Office agent to a *logical* provider order + model tier (frontier / balanced / economy). The launch path picks the first eligible provider for the agent's tier and falls back across providers on classified provider-unavailable errors.

A pre-existing field, `agent_settings.cheap_agent_profile_id` (`apps/backend/internal/agent/settings/models/models.go`), solves a different but overlapping problem: it lets a single agent profile name a *cheaper variant* profile to use on specific wake reasons (heartbeats today; potentially other low-stakes triggers later). The variant is a concrete `AgentProfile` row with its own provider/model/mode — orthogonal to the workspace tier mapping.

Both mechanisms shape the per-launch (provider, model) tuple, so a naive reading of either suggests the other is redundant.

## Decision

Keep both mechanisms in v1. They solve overlapping but distinct problems:

- **Tier routing** is workspace-scoped, fallback-aware, and indifferent to wake reason. It answers: "given this agent's logical provider order + tier, which concrete (provider, model) should this launch use right now?"
- **`cheap_agent_profile_id`** is agent-scoped and wake-reason-driven. It answers: "this is a heartbeat — should the agent run in a cheaper variant entirely, regardless of routing?"

Today there is no caller-level interaction: routing is gated on `office_workspace_routing.enabled`, and the cheap-variant lookup runs only on heartbeat-style wake reasons. When routing is disabled, the cheap variant works exactly as before. When routing is enabled, the resolver picks the route for the *effective* agent profile (the cheap variant when wake reason calls for it, otherwise the base profile).

## Consequences

- Two surfaces continue to exist in the codebase. New contributors must understand both before changing the launch path.
- The cheap-variant feature is preserved without migration churn — existing `cheap_agent_profile_id` references keep working.
- A future unification would need an ADR of its own. Plausible directions: (a) replace `cheap_agent_profile_id` with a per-agent routing override keyed on wake reason; (b) introduce a wake-reason-aware "cost tier" that the resolver consumes; (c) deprecate `cheap_agent_profile_id` as a no-op and surface its intent purely through agent overrides. Each comes with a migration plan for existing profiles. None of these are scoped for v1.
- Removing `cheap_agent_profile_id` now would be a real design call, not a cleanup. The field is documented in `models.go` with a back-reference to this ADR so future readers find this trade-off before touching it.

## Alternatives considered

1. **Remove `cheap_agent_profile_id` and emulate it via agent routing overrides.** Rejected for v1 because the existing field is referenced by the prompt builder and per-agent settings UI; rewriting that surface in the same patch as the routing-bug fix-up batch would expand scope beyond the bug list. Worth revisiting once the routing surface is exercised by users in production.
2. **Fold the cheap variant into routing as a new tier (e.g. `economy_for_heartbeat`).** Rejected because tier semantics in the spec are about *strength*, not *trigger context*. Mixing wake-reason metadata into the tier vocabulary would muddy both abstractions.

## References

- `docs/specs/office-provider-routing/spec.md` — routing feature spec
- `docs/specs/office-provider-routing/plan.md` — routing implementation plan
- `apps/backend/internal/agent/settings/models/models.go` — `AgentSettings.CheapAgentProfileID`
