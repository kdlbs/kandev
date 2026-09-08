---
status: draft
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
