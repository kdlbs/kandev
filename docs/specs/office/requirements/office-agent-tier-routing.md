---
status: active
system: office
created: 2026-08-20
owners:
  - nova28
---
# Office per-agent and per-role tier selection Requirements

## Overview

# Office per-agent and per-role tier selection

Office agents in one workspace must be able to run on different model tiers. Today
they can — a per-agent tier override already exists end to end — but the capability is
undiscoverable, the org has no way to express a tier as a property of a role, and the
Office agent record still carries a `model` field that routing ignores. The result is
an operator who configures a Critic on `opus[1m]`, watches it run on `sonnet`, and has
nothing in the product that explains the difference.

This spec extends `docs/specs/office/requirements/routing.md`. That spec remains authoritative for
tiers, provider order, execution profiles, provider health, and wake-reason policy.
Nothing here changes those contracts except where explicitly named in
[Precedence](#precedence-contract).

## Requirements

### REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001: Office per-agent and per-role tier selection

**Intent:** # Office per-agent and per-role tier selection Office agents in one workspace must be
able to run on different model tiers. Today they can — a per-agent tier override already exists end
to end — but the capability is undiscoverable, the org has no way to express a tier as a property of
a role, and the Office agent record still carries a `model` field that routing ignores. The result
is an operator who configures a Critic on `opus[1m]`, watches it run on `sonnet`, and has nothing in
the product that explains the difference. This spec extends `docs/specs/office/requirements/routing.md`. That
spec remains authoritative for tiers, provider order, execution profiles, provider health, and
wake-reason policy. Nothing here changes those contracts except where explicitly named in
[Precedence](#precedence-contract).

#### Acceptance criteria

- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.1:** **AC-1** — GIVEN workspace `role_tiers = {"specialist":"frontier"}` and an agent with `role = specialist` and no tier override, WHEN a run launches with a reason carrying no wake-reason policy, THEN the resolved tier is `frontier` and `office_run_route_attempts.tier` records `frontier`.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.2:** **AC-2** — GIVEN the same config and an agent with `role = assistant` absent from `role_tiers`, WHEN a run launches, THEN the resolved tier is the workspace `default_tier`.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.3:** **AC-3** — GIVEN `role_tiers = {"specialist":"economy"}` and a `specialist` agent whose settings carry `tier_source = "override"`, `tier = "frontier"`, WHEN a run launches with no wake-reason policy, THEN the resolved tier is `frontier` — the per-agent override outranks the role entry.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.4:** **AC-4** — GIVEN `role_tiers = {"specialist":"frontier"}`, workspace `tier_per_reason = {"heartbeat":"economy"}`, and a `specialist` agent, WHEN a heartbeat run launches, THEN the resolved tier is `economy` — the wake-reason policy outranks the role entry.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.5:** **AC-5** — GIVEN `role_tiers = {"specialist":"frontier"}` and a `specialist` agent carrying `tier_source = "override"`, `tier = "balanced"`, and workspace `tier_per_reason = {"heartbeat":"economy"}`, WHEN a heartbeat run launches, THEN the resolved tier is `economy`, demonstrating the full four-level order in one case.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.6:** **AC-6** — GIVEN an agent whose `role` is the empty string, WHEN a run launches, THEN `role_tiers` is not consulted and resolution proceeds to `default_tier`.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.7:** **AC-7** — GIVEN `role_tiers = {}`, WHEN any run launches, THEN the resolved tier is identical to the tier resolved before this feature existed, for every agent in the workspace.
- **AC-OFFICE-OFFICE-AGENT-TIER-ROUTING-001.8:** **AC-8** — WHEN a workspace routing write carries a `role_tiers` key outside the seven `AgentRole` values, THEN the write is rejected with HTTP 400 and a `ValidationError` whose `Field` is `role_tiers` and whose `Details` carry one `ValidationDetail` per offending key.

## System design

The migrated technical source is split into [part 1](../system-design/office-agent-tier-routing-01.md), [part 2](../system-design/office-agent-tier-routing-02.md), [part 3](../system-design/office-agent-tier-routing-03.md).

## Decision: the agent record's `model` does not select a model

Routing is authoritative. The precedence order (wake_reason > per-agent
override > role_tiers > workspace default) governs which tier — and
therefore which model — a run launches on. An Office agent's own `model`
field (`agent_profiles.model`) is not part of that precedence chain and
never selects the launched model; a resolver test pins this at
`apps/backend/internal/office/routing/agent_model_ignored_test.go`.

The lever for an operator who wants a specific agent on a specific tier is
the per-agent tier override (`tier_source = "override"`), or a `role_tiers`
entry for agents sharing that role. Setting `model` on the agent record has
no effect on resolution: the field predates workspace tier routing, is
excluded from the agent response DTO, and a `PATCH` that attempts to set it
is rejected with a `ValidationError`.

### ISSUE-9 (Office Beta, 2026-09-25) disposition

A Beta test reported the Critic agent (workspace `95542bf3`) configured
`model = "opus[1m]"` running on Sonnet across every observed run
(`d8a6f6d4`, `8e0996b9`, reason `review_started`). Investigation found: the
operator never set a per-agent tier override, so resolution fell through to
the workspace `default_tier = "balanced"` exactly as designed; the agent
list and detail API carry no `model` key; and the UI (agent header and
Configuration tab) render the effective model, tier, and tier source, with
no surface showing "Opus" for this agent in the running build. The Beta
report's `opus[1m]` observation came from reading the agent record's
`model` column directly, not from the product. Closed as working-as-designed
with no change to resolution or precedence.
