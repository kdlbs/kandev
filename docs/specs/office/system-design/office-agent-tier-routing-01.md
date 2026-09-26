---
status: draft
system: office
requirements:
  - REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001
created: 2026-08-20
owners:
  - nova28
---
# Office per-agent and per-role tier selection System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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

## Verified current state

Verified 2026-08-20 against `~/.kandev/data/kandev.db` (workspace
`95542bf3-37e9-4fbc-9dbe-e2c774a5e7f6`) and the tree at `0b10edfa2`.

### The reported symptom reproduces

`office_run_route_attempts` holds ten attempts; every one resolved
`tier=balanced`, and the five most recent `claude-acp` launches used
`execution_profile_id=b139adcd-…` (profile "Sonnet", `model=sonnet`).

`office_workspace_routing` for that workspace:

```text
enabled           = 1
default_tier      = balanced
provider_order    = ["claude-acp","codex-acp"]
tier_per_reason   = {"budget_alert":"economy","heartbeat":"economy","routine_trigger":"economy"}
```

### Why it happened — the override exists and was never set

| Agent | `role` | `agent_profiles.model` | `settings` | Ran as |
|---|---|---|---|---|
| CEO | `ceo` | `opus[1m]` | `{"routing":{"provider_order_source":"inherit","tier_source":"inherit"}}` | sonnet |
| Critic | `specialist` | `opus[1m]` | `{}` | sonnet |
| Analyst | `specialist` | `sonnet` | `{}` | sonnet |
| Tech Lead | `specialist` | `sonnet` | `{}` | sonnet |
| Product Manager | `assistant` | `sonnet` | `{}` | sonnet |

No agent carried `tier_source: "override"`. Every run therefore fell through to the
workspace default, which is the documented and correct behaviour.

A per-agent tier override is already implemented on every layer:

- **Type + validation** — `routing.AgentOverrides{TierSource, Tier}` and
  `ValidateAgentOverridesAgainstWorkspace` (`internal/office/routing/types.go`).
- **Resolution** — `effectiveTier()` (`internal/office/routing/resolver.go`) already
  honours `ov.TierSource == "override"` above the workspace default.
- **Persistence** — `routing.WriteAgentOverrides` onto `agent_profiles.settings`.
- **API** — `PATCH /office/agents/:id` with a `routing` body key
  (`internal/office/agents/handler.go` → `applyRoutingOverride`).
- **UI** — "Override workspace tier" toggle plus a tier toggle group in
  `apps/web/app/office/agents/[id]/components/agent-routing-card.tsx`.

**The card's premise that no per-agent override exists is incorrect.** The defect is
discoverability, the absent role lever, and the misleading `model` field.

### A role → tier map cannot solve the reported problem

The card proposes a `role -> tier` map and calls it the better option. The verified
org makes that insufficient on its own:

| `role` | agents |
|---|---|
| `ceo` | CEO |
| `assistant` | Product Manager |
| `specialist` | **Analyst, Tech Lead, Critic** |

Analyst and Critic share `role = specialist`. The card's motivating case is exactly
"the Critic must not run the same model as the Analyst it checks", and a role → tier
map cannot express that. Roles are a fixed seven-value enum
(`ceo, worker, specialist, assistant, security, qa, devops`;
`internal/agent/settings/models/agent_attributes.go:16-24`) and are not user-extensible.

Per-role is therefore specified here as a **bulk default**, never as the mechanism that
satisfies the Critic case. Per-agent remains the precise lever.

### The `model` field really is ignored

`models.AgentInstance` is a type alias for `settingsmodels.AgentProfile`
(`internal/office/models/models.go:57`) — Office identities and execution profiles are
rows in the same `agent_profiles` table, separated by `role != ''` and
`workspace_id != ''`.

At launch the resolver takes the tier's execution-profile ID and reads **that** row's
`Model`. It explicitly refuses an Office identity in that position:
`resolveExecutionProfile` returns `profile %q is an Office agent identity, not an
execution profile` when `profile.Role != ""` (`resolver.go`). So an Office agent's own
`model` column can never reach a launch.

Scope of the misleading surface, verified:

- **Not shown in Settings → Agents.** `filterGlobalProfiles`
  (`internal/agent/settings/controller/agent_crud.go:99`) drops rows with
  `WorkspaceID != ""`, and all five Office identities are workspace-scoped.
- **Not shown on the Office agent page.** `agent-route-strip.tsx` and `agent-card.tsx`
  render `preview.current_model` / `preview.primary_model` — the resolved model.
- **Is exposed by the API.** `AgentResponse` returns `*models.AgentInstance` verbatim,
  and `AgentProfile.Model` carries `json:"model"`, so `GET /office/agents/:id`
  advertises `"model": "opus[1m]"` for the Critic. `PATCH` already rejects
  `agent_profile_id` with *"agent_profile_id no longer selects an Office runtime"*,
  but says nothing about `model`.

The accurate defect statement is therefore: **the Office agent API advertises a `model`
field that Office routing ignores**, not "the UI displays it".

## Precedence contract

### Deviation from the card's stated acceptance — read this first

The card's acceptance asks for `per-agent > per-role > tier_per_reason > workspace`.
This spec deliberately specifies `tier_per_reason > per-agent > per-role > workspace`
instead. Reasons, in order of weight:

1. **The card's own goal does not need the reversal.** The Critic and Analyst both ran
   with reason `task_assigned`, for which no `tier_per_reason` key exists. The frozen
   spec already guarantees that case falls through to the agent's effective tier
   (`docs/specs/office/requirements/routing.md`, final wake-reason AC). Per-agent already wins where
   the card needs it to win.
2. **The reversal breaks a shipped, documented guarantee.**
   `docs/specs/office/requirements/routing.md` states: *"the resolver picks the Economy tier model
   regardless of the agent's default tier"* for `tier_per_reason.heartbeat = economy`.
   Reversing precedence silently voids that line.
3. **It would regress cost control.** `tier_per_reason` exists to cheap-out predictable
   background work. Under the card's order, one agent pinned to `frontier` runs
   `opus[1m]` on every heartbeat forever — the exact blowup the feature prevents. The
   verified workspace maps all three wake reasons to `economy`.
4. **The card's intent already has a supported expression.** An agent that must stay on
   Frontier even for heartbeats sets its own `tier_per_reason` override
   (`TierPerReasonSource = "override"`), which is shipped, validated, and documented.

This is a contract decision made under the board's "prefer a defensible decision"
rule rather than parking the card. **Spec Review: if this reasoning is rejected, the
correct disposition is NEEDS RETHINK back to Spec, not a Build-time edit.**

### The contract

For one run, the effective tier is the first of these that yields a non-empty tier:

1. **Wake-reason policy** — agent `tier_per_reason` override when
   `tier_per_reason_source == "override"`, otherwise workspace `tier_per_reason`,
   keyed by the run's reason. Skipped entirely when the run reason is empty or absent
   from the map. *(unchanged)*
2. **Per-agent tier** — `routing.tier` when `tier_source == "override"` and `tier` is
   non-empty. *(unchanged)*
3. **Per-role tier** — the workspace `role_tiers` entry for the agent's `role`. *(new)*
4. **Workspace `default_tier`.** *(unchanged)*

Only step 3 is added. Steps 1, 2 and 4 keep their current order and semantics.

## Data model

One new column on the existing `office_workspace_routing` row. No new table, and no
column on `agent_profiles` — keeping routing policy in one place, per the card's own
recommendation and consistent with how `tier_per_reason` is stored.

```text
office_workspace_routing
  role_tiers  TEXT NOT NULL DEFAULT '{}'   -- JSON map: role -> tier
```

- Keys are restricted to the seven `AgentRole` values. Any other key is rejected.
- Values are restricted to `frontier | balanced | economy`.
- An entry whose value is the empty string is treated as absent and is dropped before
  persistence.
- `{}` means "no role policy" and is the default for every existing and new workspace.
- The Go field is `WorkspaceConfig.RoleTiers` (`internal/office/routing/types.go`,
  beside `TierPerReason`), tagged `json:"role_tiers,omitempty"`. The workspace routing
  PUT binds directly into `WorkspaceConfig` — there is no separate request DTO — so the
  wire contract and the in-memory contract are the same type.

Migration is additive with a default, so existing rows need no backfill and behaviour
is unchanged until an operator writes a map.
