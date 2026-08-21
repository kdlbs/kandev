---
status: draft
created: 2026-08-20
owner: nova28
umbrella: office
extends: docs/specs/office/routing.md
---

# Office per-agent and per-role tier selection

Office agents in one workspace must be able to run on different model tiers. Today
they can — a per-agent tier override already exists end to end — but the capability is
undiscoverable, the org has no way to express a tier as a property of a role, and the
Office agent record still carries a `model` field that routing ignores. The result is
an operator who configures a Critic on `opus[1m]`, watches it run on `sonnet`, and has
nothing in the product that explains the difference.

This spec extends `docs/specs/office/routing.md`. That spec remains authoritative for
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
   (`docs/specs/office/routing.md`, final wake-reason AC). Per-agent already wins where
   the card needs it to win.
2. **The reversal breaks a shipped, documented guarantee.**
   `docs/specs/office/routing.md` states: *"the resolver picks the Economy tier model
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

## Acceptance criteria

Written EARS-style. Each is observable through the API, the database, or the UI.

### Per-role resolution

- **AC-1** — GIVEN workspace `role_tiers = {"specialist":"frontier"}` and an agent with
  `role = specialist` and no tier override, WHEN a run launches with a reason carrying
  no wake-reason policy, THEN the resolved tier is `frontier` and
  `office_run_route_attempts.tier` records `frontier`.
- **AC-2** — GIVEN the same config and an agent with `role = assistant` absent from
  `role_tiers`, WHEN a run launches, THEN the resolved tier is the workspace
  `default_tier`.
- **AC-3** — GIVEN `role_tiers = {"specialist":"economy"}` and a `specialist` agent
  whose settings carry `tier_source = "override"`, `tier = "frontier"`, WHEN a run
  launches with no wake-reason policy, THEN the resolved tier is `frontier` — the
  per-agent override outranks the role entry.
- **AC-4** — GIVEN `role_tiers = {"specialist":"frontier"}`, workspace
  `tier_per_reason = {"heartbeat":"economy"}`, and a `specialist` agent, WHEN a
  heartbeat run launches, THEN the resolved tier is `economy` — the wake-reason policy
  outranks the role entry.
- **AC-5** — GIVEN `role_tiers = {"specialist":"frontier"}` and a `specialist` agent
  carrying `tier_source = "override"`, `tier = "balanced"`, and workspace
  `tier_per_reason = {"heartbeat":"economy"}`, WHEN a heartbeat run launches, THEN the
  resolved tier is `economy`, demonstrating the full four-level order in one case.
- **AC-6** — GIVEN an agent whose `role` is the empty string, WHEN a run launches,
  THEN `role_tiers` is not consulted and resolution proceeds to `default_tier`.
- **AC-7** — GIVEN `role_tiers = {}`, WHEN any run launches, THEN the resolved tier is
  identical to the tier resolved before this feature existed, for every agent in the
  workspace.

### Validation

- **AC-8** — WHEN a workspace routing write carries a `role_tiers` key outside the
  seven `AgentRole` values, THEN the write is rejected with HTTP 400 and a
  `ValidationError` whose `Field` is `role_tiers` and whose `Details` carry one
  `ValidationDetail` per offending key.
- **AC-8a** — The rejection's **wire shape** is the structured 400 that
  `respondRoutingValidation` (`internal/office/agents/handler.go`) and the dashboard
  routing handler already emit, and no other:

  ```json
  {"error": "<ValidationError.Message>", "field": "<ValidationError.Field>", "details": [...]}
  ```

  `ValidationDetail` is `{ProviderID, Field, Message}` (`internal/office/routing/types.go`)
  — there is **no** role member. The offending role therefore goes in
  `ValidationDetail.Field` and the reason in `ValidationDetail.Message`, with
  `ProviderID` left empty for `role_tiers` entries. This is pinned because "`Details`
  name the offending key" does not by itself say which member carries it, and a builder
  cannot read that off the struct.
- **AC-9** — WHEN a write carries a `role_tiers` value outside
  `frontier | balanced | economy`, THEN the write is rejected with HTTP 400 in the
  AC-8a shape, and the offending **role** and **value** both appear in the response —
  the role in `ValidationDetail.Field`, the value quoted in `ValidationDetail.Message`.
  The message must contain neither the string `default_tier` nor the prefix
  `routing config invalid:`; AC-10a explains why both are live risks rather than
  stylistic notes.
- **AC-10** — WHEN a write carries a `role_tiers` entry whose tier is mapped by no
  provider in the workspace `provider_order`, THEN the write is rejected with HTTP 400
  rather than failing at launch. The structural precedent is
  **`checkTierPerReasonMapped`**, *not* `checkTierMapped`: `role_tiers` is a **map** and
  may hold several bad entries at once, so it needs the `[]ValidationDetail`
  accumulation `checkTierPerReasonMapped` performs. `checkTierMapped` validates a
  **single** value (`ov.Tier`) and returns `Field: "routing.tier"` with no `Details` at
  all — copying it collapses N bad entries into one message and reports the wrong field.
  Both are called from `ValidateAgentOverridesAgainstWorkspace`, so naming that function
  alone does not pick the right one; this AC names the callee deliberately.
- **AC-10a** — `role_tiers` validation **requires a new field-parameterised validator**,
  because the obvious existing one cannot satisfy AC-8. `validateTier(t Tier)`
  (`internal/office/routing/types.go`) hardcodes `Field: "default_tier"` in the
  `ValidationError` it returns and carries neither the role nor any `Details`. It is
  already reused for the per-agent override (`validateTier(ov.Tier)`), so the
  wrong-field pattern is **established in this repo** — reusing it for `role_tiers`
  looks correct and is not. Two consequences:
  1. The `role_tiers` value validator takes the field name as a **parameter**, exactly
     as `validateTierPerReason(m TierPerReason, field string)` does, and emits
     `Field: "role_tiers"`.
  2. `validateTier`'s error must not be embedded in the message verbatim.
     `ValidationError.Error()` renders as
     `routing config invalid: default_tier: invalid tier "x"`, and
     `validateTierPerReason` wraps exactly that string into its own `Message` — so even
     the correct structural sibling leaks `default_tier` into user-facing text today. A
     `role_tiers` message that inherits that wording violates AC-9.
- **AC-11** — WHEN a write carries a `role_tiers` entry with an empty-string value,
  THEN the entry is dropped and the persisted map omits that key; the write succeeds.
- **AC-12** — GIVEN routing is disabled for the workspace (`enabled = 0`), WHEN a
  `role_tiers` write arrives, THEN it is validated and persisted exactly as when
  enabled; `enabled` gates automatic fallback, not tier selection.
- **AC-12a** — GIVEN `enabled = 0` and a non-empty `role_tiers`, WHEN a run launches,
  THEN the role level still participates in tier resolution. `Resolve` computes the
  effective tier before it branches on `Enabled`, and the single evaluated provider
  (`provider_order[0]`) is looked up at the role-supplied tier.

### The ignored `model` field

- **AC-13** — WHEN `GET /office/agents/:id` returns an Office identity
  (`role != ''`), THEN the `model` field is **omitted from the response body**. It is
  not emitted as an empty string, and it is not emitted alongside a marker flag.
  Omission is chosen over a marker so no consumer can read a value that has no effect.
  For a row with `role == ''` (an execution profile) the field is unchanged.
- **AC-13a** — The omission is implemented as a **dedicated response DTO in
  `internal/office/agents`** that **embeds** `models.AgentInstance` and **shadows** the
  `model` key with its own field:

  ```go
  type agentResponseBody struct {
      *models.AgentInstance
      Model *string `json:"model,omitempty"`
  }
  ```

  Go resolves the JSON name collision by depth: the outer `Model` (depth 0) wins over
  the embedded `AgentProfile.Model` (depth 1), so the embedded field never reaches the
  wire. Embedding a type alias compiles, and the embedded field is referred to by the
  alias name as written — `agentResponseBody{AgentInstance: p}`.

  The projection sets `Model` **only when `role == ''`** (pointing it at the row's
  value) and leaves it **nil** for an Office identity, so `omitempty` drops the key.
  This is what lets AC-13's two halves hold in one type, without the spec having to
  assert which rows reach which handler.

  **The shadow field is `*string`, not `string`, and this is load-bearing.** With a
  plain `string`, an execution profile (`role == ''`) whose `model` is the empty string
  would have its `model` key **dropped** by `omitempty` — a silent shape change on
  exactly the rows AC-13 promises are unchanged, because the shared struct tags `Model`
  as `json:"model"` with **no** `omitempty` and therefore emits `"model":""` today. A
  `*string` distinguishes "absent" (nil) from "present and empty" (non-nil), so the DTO
  is shape-preserving by construction whether or not an empty `model` is reachable in
  practice. Verified against the real tag set: with `*string`, `role != ''` emits no
  `model` key and `role == ''` emits `"model":""` for an empty value, matching the
  unwrapped struct.

  **Embedding is mandated over a field-by-field projection.**
  `settingsmodels.AgentProfile` carries **43 JSON-tagged fields**, several computed or
  `db:"-"`, with inconsistent `omitempty` usage. Hand-mirroring them is 43 chances to
  drop a field or mistype a tag, and every such slip is a silent response-shape change
  for execution-profile and kanban consumers. Embedding copies zero tags and so cannot
  drift when a field is later added to the shared struct — which the
  `RouteAttemptDTO` / `routeAttemptToDTO` pattern in
  `internal/office/dashboard/routing_dto.go` does not give us here, because that DTO
  mirrors a 17-field model this feature owns, not a 43-field struct shared with two
  other subsystems.

  Two alternatives remain **explicitly forbidden**: a custom `MarshalJSON` on
  `settingsmodels.AgentProfile` (a type alias shared with execution profiles and kanban
  — it would silently change their JSON too), and a `map[string]any` post-process (it
  drops compile-time field checking). The shared struct and its `json:"model"` tag are
  not edited.

  **The existing wrappers change element type, not shape.** `AgentResponse.Agent`
  becomes `*agentResponseBody` and `AgentListResponse.Agents` becomes
  `[]*agentResponseBody` (`internal/office/agents/dto.go`). Their JSON keys stay `agent`
  and `agents`; no new wrapper type is introduced and no envelope key is renamed. A nil
  agent stays a nil **wrapper field**, serialising as `"agent": null` exactly as today —
  do not substitute a zero-valued DTO, which would serialise as `{}` because a nil
  embedded pointer contributes no promoted fields.

  **Key ORDER changes and that is accepted.** Embedding emits the shadowing `Model`
  where the outer field is declared, so for `role == ''` the `model` key moves to the
  end of the object. The key SET and every value are identical. JSON object member
  order is not semantically significant, no in-repo consumer depends on it, and this is
  why AC-13c compares decoded maps rather than bytes. Earlier revisions of this AC said
  "byte-identical"; that was never achievable by either mechanism and is corrected here.
- **AC-13c** — The shape-preservation property AC-13a asserts is **observable, not
  merely asserted**. A Go test in `internal/office/agents` marshals the **same**
  `models.AgentInstance` value twice — once directly, once through the response DTO —
  decodes both into `map[string]any`, and asserts:
  - for a row with `role != ''`: the DTO map equals the direct map **with the `model`
    key deleted** (so no other key is added, dropped, or altered), and the DTO map
    contains no `model` key;
  - for a row with `role == ''`: the DTO map **equals the direct map exactly**,
    including a `model` key whose value is `""` when the row's model is empty.

  The comparison is over decoded maps, never raw bytes, per AC-13a's key-order note.
  This AC exists because a mechanism whose whole job is "change one key and nothing
  else" is worthless without a test that can see "nothing else"; it is the standing
  guard for AC-13, AC-13a and AC-13b together.
- **AC-13b** — The omission applies to **every Office-identity agent payload the
  `internal/office/agents` handler emits**, not only `GET /office/agents/:id`. Five
  handlers emit the agent struct: `listAgents` (via `AgentListResponse`), `createAgent`
  (201), `getAgent`, `updateAgent`, and `updateAgentStatus`. All five return the DTO.
  A response from any of the five that still carries a `model` key for a row with
  `role != ''` fails this AC.
- **AC-14** — WHEN `PATCH /office/agents/:id` carries a `model` field for an Office
  identity, THEN the request is rejected with HTTP 400 and a message directing the
  caller to workspace tier profiles or the agent routing override. The rejection is
  *worded* like the existing `agent_profile_id` rejection, but is **not implemented the
  same way** — see AC-14a for why the mechanisms necessarily differ, and AC-14c for the
  response shape, which AC-14a does not cover.
- **AC-14c** — The rejection's **wire shape** is the AC-8a structured 400, produced by
  `respondRoutingValidation` with
  `&routing.ValidationError{Field: "model", Message: "<the AC-14 wording>"}` — and
  **not** the bare `gin.H{"error": ...}` form the neighbouring `agent_profile_id`
  rejection uses. Two rejections in one handler function therefore carry different
  envelopes, and that is intended rather than an oversight: the `agent_profile_id` bare
  form is the older shape, `respondRoutingValidation` is already called by
  `applyRoutingOverride` a few lines away in the same file, and AC-8/AC-9/AC-10 commit
  the rest of this feature to the structured form. A caller that receives this 400 can
  then read `field == "model"` instead of pattern-matching prose.

  **AC-14's "worded like" governs the sentence only, never the envelope.** The existing
  `agent_profile_id` rejection is **not** changed to match: rewriting it is a separate
  API change this feature is not authorised to make.
- **AC-14a** — `UpdateAgentRequest` has no `model` member, and Gin's `ShouldBindJSON`
  uses stdlib `encoding/json`, which ignores unknown keys — so today a `model` key in a
  PATCH body is silently discarded. Detection is therefore implemented by buffering the
  request body **once** with `io.ReadAll(c.Request.Body)`, then decoding those same
  bytes twice: once into `map[string]json.RawMessage` to test for **presence of the
  `"model"` key specifically**, and once into `UpdateAgentRequest`. The handler must
  therefore stop calling `c.ShouldBindJSON`, which consumes the body and would leave the
  second decode empty; binding from the buffered bytes replaces it, and the existing
  malformed-JSON 400 behaviour is preserved.

  **On precedent — read this before going looking for one.** Three handlers already
  buffer a Gin request body with `io.ReadAll(c.Request.Body)`:
  `internal/office/routines/handler.go`, `internal/office/channels/handler.go` and
  `internal/system/frontenderrors/handler.go`. They are precedent for **the buffering
  step only**. **None of them does key-presence detection**, and copying any of them
  wholesale produces the wrong thing:
  - `routines/handler.go` buffers for HMAC verification, then unmarshals to
    `map[string]interface{}` to build webhook variables — no typed struct, no presence
    test.
  - `channels/handler.go` buffers for signature verification and treats the body as raw
    text; it never JSON-decodes it at all.
  - `frontenderrors/handler.go` decodes into a typed struct with
    **`DisallowUnknownFields()`**, then decodes a second time expecting `io.EOF` — a
    trailing-garbage guard, and it uses the very technique forbidden two paragraphs
    below. It is the closest-looking and the most misleading of the three.

  The `map[string]json.RawMessage` presence check exists nowhere in this backend today.
  **It is new code**, and the spec says so rather than sending a builder to find a
  pattern that is not there.

  `json.Decoder.DisallowUnknownFields` is **explicitly forbidden**: it would reject
  every unknown key on this endpoint, a far broader breaking API change than this
  feature is authorised to make. That `frontenderrors/handler.go` uses it is not a
  licence — that endpoint accepts one closed payload shape; this one does not. The
  existing `agent_profile_id` check stays a plain nil-check on the declared
  `AgentProfileID *string` field; that path is unchanged.
- **AC-14b** — Presence, not value, triggers the rejection. `{"model": "opus[1m]"}`,
  `{"model": ""}` and `{"model": null}` are **all rejected** with the same HTTP 400,
  because each proves the caller believes the field is honoured. A body that omits the
  `model` key entirely is accepted. For a row with `role == ''` the key is not rejected.
- **AC-15** — GIVEN an existing Office identity row with a non-empty `model` value,
  WHEN this feature ships, THEN that stored value is left untouched in the database and
  continues to have no effect on routing. No destructive migration runs.

### Discoverability

- **AC-16** — GIVEN an agent inherits its tier (no per-agent override, no role entry),
  WHEN the operator views that agent's routing card, THEN the card names the tier in
  force **and** the level that supplied it. The four wire values are exactly
  `wake_reason | override | role | workspace` (AC-18); the card renders their
  translated display labels. Wire values and display labels are distinct — the wire
  value is never shown raw and is never translated.
- **AC-16a** — The same rule binds the **workspace routing preview table**
  (`app/office/workspace/routing/components/agent-preview-table.tsx`), which today
  renders `{a.tier_source}` raw in a user-facing cell. WHEN that table renders a row,
  THEN it shows the translated display label for the row's `tier_source`, never the raw
  wire value. Without this AC the widening ships the new literals `role` and
  `workspace` untranslated into a shipped table, violating AC-16's own principle and
  AC-19's five-locale requirement on a surface AC-16 does not reach.
- **AC-17** — GIVEN a per-agent tier override is shadowed by a wake-reason policy for
  some reason, WHEN the operator views the agent's routing card, THEN the card states
  that the override does not apply to those reasons and names them.
- **AC-17a** — The reasons named are the keys of the agent's **effective** wake-reason
  map, which is the same map `wakeReasonTier` consults: WHEN
  `tier_per_reason_source == "override"`, the effective map is the agent's own
  `tier_per_reason` **and the workspace map is not consulted at all** (an override
  replaces it entirely, per `docs/specs/office/routing.md`); OTHERWISE the effective map
  is the workspace `tier_per_reason`. The card never shows the union of the two, and
  never shows the workspace keys when an override is in force. Keys whose value is
  empty are excluded, since they do not shadow anything. GIVEN the effective map is
  empty, THEN the card shows no shadowing notice at all.
- **AC-17b** — The shadowing notice is **not** limited to a per-agent override. A
  role-supplied tier is shadowed by wake-reason policy identically (AC-4), and AC-18b
  guarantees a preview never reports `wake_reason`, so without this AC an agent on a
  role tier would see a card asserting the role tier is in force while its heartbeat
  runs actually take Economy, with nothing said. WHEN the agent's effective tier comes
  from the **role** level or the **workspace default**, and the effective wake-reason
  map is non-empty, THEN the card states that the named reasons do not use the
  displayed tier, using the same notice and the same key set as AC-17a. No new data
  path is required: `agent-routing-card.tsx` already holds the workspace map via
  `useWorkspaceRouting` and the agent's own map via the persisted `overrides` blob.
- **AC-18** — WHEN the routing preview (`PreviewItem`) is produced for an agent, THEN
  `TierSource` reports exactly one of `wake_reason | override | role | workspace`,
  widening the current two-valued `override | inherit`.
- **AC-18a** — The value `override` keeps its current meaning and spelling, so an
  existing consumer testing `tier_source === "override"` is unaffected. The value
  `inherit` is **removed from the computed preview field only** and replaced by `role`
  or `workspace`.

  **The widening is scoped by TYPE, not by file.** Two unrelated fields are both spelled
  `tier_source`, and exactly one of them changes. Several files contain both, so
  "update file X" is not a safe instruction:

  | | Type | Widens? |
  |---|---|---|
  | **Computed preview** — CHANGES | Go `routing.PreviewItem.TierSource` → Go `dashboard.AgentRoutePreview.TierSource` → TS `AgentRoutePreview.tier_source` | **YES** — becomes `wake_reason \| override \| role \| workspace` |
  | **Persisted override** — UNCHANGED | Go `routing.AgentOverrides.TierSource` (`json:"tier_source,omitempty"`) → TS `AgentRoutingOverrides.tier_source` | **NO** — stays `inherit \| override \| ""` |

  **Sites that MUST change** (the computed chain, in dataflow order):
  1. `internal/office/routing/provider.go#tierSourceForAgent` — today the **only** site
     that emits the literal `"inherit"`; it must instead yield `role` / `workspace`,
     which requires the agent's role and the workspace `role_tiers` map. Per **AC-20d**
     it does not decide this itself: it delegates to the widened `effectiveTier`, so
     that preview and audit share one producer. "Sole producer" describes the situation
     before this feature, not after it.
  2. `internal/office/routing/provider.go` where the result is set on `PreviewItem`.
  3. `internal/office/dashboard/handler_routing.go` where it is copied into
     `AgentRoutePreview` (`previewItemsToDTOs`).
  4. `internal/office/dashboard/routing_dto.go` — the doc comment on `AgentRoutePreview`
     currently states the two-valued contract and becomes false.
  5. TS `AgentRoutePreview.tier_source` in `lib/state/slices/office/types.ts`.
  6. `agent-preview-table.tsx`, per AC-16a.
  7. Tests fixturing the computed value — **both** of them, Go and TypeScript:
     `internal/office/dashboard/handler_routing_test.go`, and
     `apps/web/lib/state/slices/office/office-routing.test.ts`, whose
     `setRoutingPreview` case fixtures a preview row with `tier_source: "inherit"`.
     That TS fixture is the **computed** preview, not the persisted override, so it
     stops compiling the moment item 5 lands. It is easy to mistake for a
     MUST-NOT-CHANGE site because the same file name pattern appears in that list
     below; it is not one.

  This list is maintained by hand and has been wrong before. It is a starting point,
  not a proof of completeness — **AC-18c is the proof**. Work the list, then run the
  typecheck gate and treat whatever it reports as in scope.

  **Sites that MUST NOT change** (persisted state — changing these is a regression):
  - `routing.AgentOverrides.TierSource` and TS `AgentRoutingOverrides.tier_source`.
  - `agent-routing-card.tsx` where it **writes** `tier_source: on ? "override" :
    "inherit"` and reads `overrides.tier_source !== "override"` — that is the persisted
    override blob being PATCHed, not the preview.
  - `e2e/helpers/office-api-client.ts`, whose `tier_source` sits inside `overrides`.
  - `internal/office/onboarding/service.go#writeAgentInheritMarkers`, which stamps
    `TierSource: "inherit"` as the CEO's onboarding marker.
  - `e2e/tests/office/office-routing-disabled.spec.ts`, which asserts
    `route.overrides.tier_source === "inherit"`.

  A stored settings blob containing `tier_source: "inherit"` keeps its persisted
  spelling and continues to mean "not an override".
- **AC-18b** — GIVEN a preview is produced through `Provider.Preview` /
  `Provider.PreviewAgent`, WHEN resolution runs, THEN it runs with an empty run reason,
  so the wake-reason level cannot apply and `TierSource` never reports `wake_reason` in
  a preview. AC-17 (override case) and AC-17b (role / workspace case) are what surface
  wake-reason shadowing in that view; between them every level a preview can report
  carries a shadowing notice.
- **AC-18c** — WHEN the `tier_source` widening has been applied, THEN
  `pnpm --filter @kandev/web typecheck` passes with no error mentioning `tier_source`.

  This AC exists because AC-18a's site list is maintained by hand and has been
  incomplete in successive reviews — the union of "files that mention `tier_source`" is
  not knowable by reading, and the type checker knows it exactly. Narrowing
  `AgentRoutePreview.tier_source` from `"inherit" | "override"` to
  `"wake_reason" | "override" | "role" | "workspace"` makes every stale `"inherit"`
  fixture or comparison a compile error, so the gate is exhaustive by construction where
  a list can only ever be a best effort.

  Any site the gate reports is **in scope for this feature**, whether or not AC-18a
  names it — with one exception that is a genuine failure, not a site to update: an
  error on a MUST-NOT-CHANGE site from AC-18a means the **persisted** override type was
  widened by mistake. Fix that by reverting the persisted type, never by widening the
  fixture to match.

  This mirrors the shape AC-19 already uses — delegate completeness to a checker that
  can see everything, rather than to a list that cannot. AC-13c is the same idea for
  the agent-payload half.
- **AC-19** — WHEN new user-facing copy is added for AC-16 through AC-18, THEN it is
  routed through `t()` / `<Trans>` and present in all five locales
  (`en`, `pt-pt`, `zh-cn`, `zh-hk`, `zh-tw`), with `pnpm run i18n:check` passing.

### Observability

- **AC-20** — WHEN a run resolves its tier, THEN the supplying level is persisted on
  the route attempt in a new `office_run_route_attempts.tier_source` column
  (`TEXT NOT NULL DEFAULT ''`), holding one of
  `wake_reason | override | role | workspace`.
- **AC-20c** — `''` means **"the supplying level is not recorded"**, and it is never
  interpreted as `workspace`. It arises from exactly two causes, which a consumer
  separates by reading the sibling `tier` column:

  | `tier` | `tier_source` | meaning |
  |---|---|---|
  | non-empty | `''` | row written before this column shipped |
  | `''` | `''` | the attempt never resolved a tier |
  | non-empty | non-empty | normal post-migration row |

  The second case is real and is **not** a pre-migration artefact: the
  `max_attempts_exceeded` attempt appended by
  `internal/office/scheduler/dispatch_routing.go` sets no `Tier` today and will
  likewise set no `TierSource`. The invariant this AC fixes is therefore
  **`tier_source` is non-empty only when `tier` is non-empty**; the converse does not
  hold, because legacy rows carry a tier and no source. A migration that back-fills
  `tier_source` for legacy rows is forbidden — the level that produced those tiers was
  never recorded and cannot be reconstructed.

  **The widened `effectiveTier` preserves this invariant at its own fallthrough.** Its
  last step returns `cfg.DefaultTier`; when that value is empty it returns an empty tier
  and an **empty source**, never `workspace`. A source is reported only when a level
  actually supplied a tier. The case is defensive rather than live — `validateTier`
  rejects an empty tier and the column is declared
  `default_tier TEXT NOT NULL DEFAULT 'balanced'`, so no supported write reaches it —
  but AC-20c states its invariant without qualification, so the fallthrough is pinned
  here rather than left for the builder to decide.
- **AC-20b** — The stored value is **surfaced, not write-only**. `tier_source` is added
  to `dashboard.RouteAttemptDTO` as `json:"tier_source,omitempty"`, mapped in
  `routeAttemptToDTO` beside the existing `Tier` field. It therefore reaches both
  payloads that already carry `RouteAttemptDTO` — `GET /runs/:id/attempts`
  (`RouteAttemptsResponse`) and the run-detail response (`RunRouting.Attempts`) — with
  no new endpoint. The `omitempty` tag means a row whose level is unrecorded (`''`,
  either cause in AC-20c) omits the key rather than reporting a false level.

  The **TypeScript client contract widens with it**: `RouteAttempt` in
  `apps/web/lib/state/slices/office/types.ts` gains
  `tier_source?: "wake_reason" | "override" | "role" | "workspace"`. That one type backs
  every consumer the field must reach — `RouteAttemptsResponse` for
  `GET /runs/:id/attempts` and the run-detail `attempts` array (both in
  `apps/web/lib/api/domains/office-runs-api.ts`), and the WS payload
  (`apps/web/lib/ws/handlers/office.ts`). Without this the field is on the wire but
  unreadable from TypeScript, and "surfaced, not write-only" is false in practice.
  Note this is the **route-attempt** `tier_source`, a third type spelled the same way;
  AC-18a's table governs the other two and is unaffected.

  **The `route_attempt_appended` WS event gains the field too, and that is intended.**
  `publishRouteAttemptAppended` (`internal/office/scheduler/routing_events.go`)
  serialises `models.RouteAttempt` **whole** as `payload["attempt"]`, so the field
  AC-20f adds to that struct appears on the event automatically, at all three publish
  sites. This is additive and `omitempty`-guarded, so no existing consumer breaks. Build
  must **not** suppress it: the two suppression routes available — dropping the JSON tag
  AC-20f specifies verbatim, or introducing a separate WS-only DTO — are both forbidden
  here, the first because it contradicts AC-20f and the second because it would create
  the second producer AC-20d exists to prevent.

  **Explicitly out of scope for this AC:** any new **UI rendering** of the value — no
  control, no column, no badge. The field becomes available to the run-detail UI and to
  WS consumers; presenting it is a later change.
- **AC-20d** — The value has **one producer, shared by the resolve path and the preview
  path**, so an agent's audit record and its preview can never disagree about the level.
  `effectiveTier` (`internal/office/routing/resolver.go`) is widened to return the tier
  **and** its source — it already receives `cfg` and `ov`, and gains the agent's `role`
  for the new level — and `tierSourceForAgent` (`provider.go`, today the preview-only
  producer of the literal `"inherit"`) is reimplemented to delegate to it rather than
  deciding independently. Two independent producers are **explicitly forbidden**: that
  is the defect this AC exists to prevent.
- **AC-20e** — The source is carried out of resolution on
  `routing.Resolution` as a new `TierSource string` field beside the existing
  `RequestedTier` (`resolver.go`). `Resolution` is the only carrier; the source is not
  threaded through `ResolveOptions`, not recomputed by the scheduler, and not re-derived
  from the agent at write time.
- **AC-20f** — The persistence path is, in order:
  1. `internal/office/models/models.go` — `RouteAttempt` gains a `TierSource string`
     field beside `Tier`, carrying the JSON tag `tier_source,omitempty` and the db tag
     `tier_source`:

     ```go
     TierSource string `json:"tier_source,omitempty" db:"tier_source"`
     ```
  2. `internal/office/repository/sqlite/base_migrations.go` — the additive column is
     added with the replayable form already used one line above for this same table:
     `r.migrate.Apply("office_run_route_attempts.tier_source", "ALTER TABLE
     office_run_route_attempts ADD COLUMN tier_source TEXT NOT NULL DEFAULT ''")`.
     **The migration for this column does not live in `workspace_routing.go`** — that
     file holds the `office_workspace_routing` read/write SQL only, and the `role_tiers`
     migration likewise belongs in `base_migrations.go`.
  3. `internal/office/repository/sqlite/route_attempts.go` — `tier_source` is added to
     the `AppendRouteAttempt` INSERT column list and its parameter list, and to the
     `ListRouteAttempts` SELECT list so `StructScan` populates it.
  4. `internal/office/scheduler/dispatch_routing.go` — the **three**
     `AppendRouteAttempt` call sites set `TierSource` wherever they already set `Tier`
     — and they do **not** divide evenly. Each of the three needs a different edit:

     | site | `res` in scope? | sets `Tier` today? | edit |
     |---|---|---|---|
     | `parkRunMaxAttempts` | **no** | no | **none** — sets neither field, per AC-20c |
     | `parkRunBlocked` | yes (`res *routing.Resolution` parameter) | yes, `Tier: string(res.RequestedTier)` | add `TierSource: res.TierSource` beside it |
     | `recordAttemptStart` | **no** — receives a bare `tier routing.Tier` | yes, from that parameter | **thread** a source parameter, supplied at the call site |

     So the split is **one no-op, one direct read, one threaded parameter** — there is
     no second direct-read site, and a builder who goes looking for one is looking for
     something that does not exist. `parkRunMaxAttempts` in particular takes no
     `*routing.Resolution` at all; threading one into it to "complete the pattern" is
     forbidden by this AC and by AC-20g.

     For the threaded site: `recordAttemptStart`'s signature today ends in a bare
     `tier routing.Tier` parameter and gains a source parameter alongside it, supplied
     from `res.TierSource` at its call site, where `res` **is** already in scope.
     Deriving the source inside `recordAttemptStart` instead is forbidden by AC-20d.
- **AC-20g** — Two nearby sites are **explicitly NOT write sites** and must not be
  changed: the `prior = append(prior, models.RouteAttempt{…})` in
  `dispatch_routing.go` is an in-memory mirror used for fallback bookkeeping by
  `latestFailedExecutionProfile`, not a persistence call; and the attempt-finalisation
  path updates outcome columns on an existing row and does not write `tier_source`.
- **AC-20a** — `office_run_route_attempts` carries no `agent_id` column; attribution is
  via `run_id` to the owning run. This feature adds no agent column, so a
  "which tier did agent X get" query remains a join through `runs`.

## Forced-to-invent pass

Decisions a builder would otherwise have to guess. Each is a contract.

### Ordering and tiebreak

- **AC-21** — The four precedence levels are evaluated in the fixed order in
  [The contract](#the-contract). The order is not configurable.
- **AC-22** — `role_tiers` is a map keyed by a unique role, so no two entries can
  apply to one agent and **no tiebreak is required**. An agent has exactly one `role`
  (a single non-null column), so at most one entry matches.
- **AC-23** — WHEN `role_tiers` is serialised to JSON for persistence or API response,
  THEN keys are emitted sorted ascending by the role string using byte order, so a
  round-trip is byte-stable and diffs are reviewable. `map` iteration order must not
  reach the wire.
- **AC-24** — WHEN the routing settings UI lists roles, THEN they are ordered by the
  declaration order of the `AgentRole` constants
  (`ceo, worker, specialist, assistant, security, qa, devops`), not alphabetically and
  not by map order.

### Idempotency and retry

- **AC-25** — WHEN the same `role_tiers` payload is written twice, THEN the second
  write succeeds and leaves the persisted value byte-identical. `updated_at` still
  advances; it records the write, not a value change.
- **AC-26** — A `role_tiers` write is a whole-map replacement, not a merge. WHEN a
  payload omits a role that is currently mapped, THEN that role's entry is deleted.
  This matches how `tier_per_reason` and `provider_order` already behave.
- **AC-26a** — An **absent `role_tiers` key** on a routing write means the same as
  `{}`: the stored map is cleared. It does not mean "preserve the current value". This
  follows the endpoint's existing whole-config semantics — `TierPerReason` is likewise
  `omitempty`, unmarshals to nil when the key is absent, and is written unconditionally
  by the `office_workspace_routing` upsert — and making one field alone
  absent-means-preserve would be a new and surprising rule on a PUT where every other
  field is replaced. The consequence is that a client which does not yet know about
  `role_tiers` erases it, so both in-repo writers are updated to round-trip the field:
  the workspace routing page, and the `cfg` parameter type of the routing-config PUT
  helper in `apps/web/e2e/helpers/office-api-client.ts`. (This is distinct from that
  file's `tier_source`, which sits inside `overrides` and stays untouched per AC-18a.)
- **AC-27** — WHEN a routing write fails validation, THEN no part of it is persisted;
  `role_tiers` and every other field are written in one transaction.
- **AC-28** — Tier resolution is a pure read. Retrying a failed launch re-resolves from
  current config; a tier is never cached on the run and never frozen at enqueue time.
  A config change between attempt N and attempt N+1 therefore takes effect on N+1, and
  the two attempts may legitimately record different tiers.

### Concurrency

- **AC-29** — GIVEN two callers write `role_tiers` for the same workspace
  concurrently, WHEN both succeed, THEN the last committed write wins in full and the
  row is never left holding a mixture of the two maps. `office_workspace_routing` is
  keyed by `workspace_id`, so this is a single-row update.
- **AC-30** — GIVEN a `role_tiers` write commits while a run is mid-resolution, WHEN
  that resolution completes, THEN it uses whichever value its own read observed and is
  never retried on that basis, and the run is not aborted. Both halves are observable:
  the resolved tier is recorded on the route attempt, and a retry would appear as a
  second attempt row.

  *Design note, not an acceptance criterion:* no lock is taken across the launch. That
  is a statement about the implementation, not an outcome any test, API response, or DB
  query can witness — this section's preamble promises every AC is observable, and as an
  AC clause it would not have been.
- **AC-31** — GIVEN an agent's `role` changes while it has queued runs, WHEN those runs
  launch, THEN each resolves against the role in force at its own resolution time. Role
  changes are not applied retroactively to already-resolved runs.

### Nil, empty and error behaviour

- **AC-32** — WHEN the `role_tiers` column holds `''`, `'{}'`, or SQL `NULL`, THEN it
  decodes to an empty map and resolution proceeds to `default_tier`. None of the three
  is an error.
- **AC-32a** — **AC-32 outranks AC-33 on the empty string.** `''` is simultaneously
  "empty" under AC-32 and "JSON that fails to decode" under AC-33, because
  `json.Unmarshal([]byte(""), ...)` returns *unexpected end of JSON input*. AC-32 wins
  because it names `''` explicitly; AC-33 governs only **non-empty** bytes that fail to
  parse (`{`, `not json`, a truncated object).

  **The adjacent `tier_per_reason` decode does NOT behave this way, and copying it is
  the failure mode.** `loadWorkspaceRouting`
  (`internal/office/repository/sqlite/workspace_routing.go`) selects
  `COALESCE(tier_per_reason, '{}')` and unmarshals the result. `COALESCE` substitutes
  for SQL `NULL` **only** — a literal `''` passes through untouched and then errors in
  `json.Unmarshal`. The pattern sitting one line from where `role_tiers` will be read
  therefore implements **AC-33's behaviour on AC-32's input**. The `role_tiers` decode
  needs an explicit empty-string short-circuit the precedent lacks: treat a zero-length
  raw value as `{}` **before** unmarshalling, then unmarshal.

  This is a defect in the precedent as a template, not in `tier_per_reason` as shipped —
  that column is also `NOT NULL DEFAULT '{}'` and no supported write puts `''` in it.
  `role_tiers` inherits the same protection, so this too is defensive. It is specified
  because AC-32 states its contract unconditionally, and a builder extending the
  neighbouring line would break it without ever seeing a failure.
- **AC-33** — WHEN the `role_tiers` column holds JSON that fails to decode, THEN
  loading the workspace routing config returns an error and the launch is refused with
  the existing blocked-route path. It does **not** silently fall back to
  `default_tier`: a corrupt policy must be visible, not quietly ignored.
- **AC-34** — WHEN an agent's `role` holds a value not in the seven-value enum (a row
  written by an older build or by hand), THEN `role_tiers` is not consulted for it,
  resolution proceeds to `default_tier`, and the launch is not refused.
- **AC-34a** — WHEN a persisted `role_tiers` map holds an entry with an empty-string
  tier (bypassing AC-11, e.g. written by hand), THEN that entry is treated as absent at
  resolution time and the agent falls through to `default_tier`. It is not an error and
  does not refuse the launch; AC-33's refusal is reserved for JSON that fails to decode.
- **AC-34b** — WHEN a persisted `role_tiers` map holds a key outside the seven-value
  enum, THEN that entry is ignored at resolution time rather than refusing the launch.
  AC-8 keeps such keys out on the write path; the read path stays permissive so a role
  removed from the enum in a future build cannot brick every launch in the workspace.
- **AC-35** — WHEN the role-supplied tier is mapped by no provider in the effective
  order at launch time, THEN the existing `missing_model_mapping` skip path applies
  unchanged; this feature adds no new blocked-route status.

### Defaults and boundaries

- **AC-36** — The default value of `role_tiers` is `{}` for every workspace, existing
  and new. Onboarding does not seed it.
- **AC-37** — `role_tiers` holds at most seven entries, bounded by the enum. A payload
  with more keys than the enum has values is rejected by AC-8 before size matters, so
  no separate length limit is specified.
- **AC-38** — Writing `role_tiers` for a role that currently has no agents is valid and
  persists; the map is a policy, not a join, and it applies when such an agent is later
  created.
- **AC-39** — WHEN an agent's per-agent tier override is cleared, THEN its effective
  tier falls to the role entry if one exists, and to `default_tier` otherwise, with no
  further operator action.

## Precedent citations

This spec cites in-repo code as precedent about a dozen times. Three review rounds have
now each found at least one citation where **copying the named precedent produces
behaviour that violates an AC in this same spec**: AC-14a's three body-buffering
handlers (round 3), then AC-32a's `tier_per_reason` decode and AC-10a's `validateTier`
(round 4). Those instances are fixed above. This section exists to close the class.

**The standing rule.** Every precedent named in this spec is cited for a **named step**,
never wholesale, and each citation states what copying it produces. A citation reading
only "matching X" or "as Y already does" is incomplete — treat it as a defect in this
spec to be routed back, not as licence to copy X or Y.

Why prose alone is not enough: all three rounds' traps were already *adjacent to* text
telling the builder to be careful, and a warning is only read by a builder who suspects
there is something to look for. The precedents in question look correct, compile, and
pass their own existing tests. So the guarantee is delegated to checks that go red.

- **AC-40** — For every AC whose behaviour a cited precedent would violate, there is a
  test **that the precedent fails**. This feature ships at minimum these four:
  1. a decode test feeding the `role_tiers` column a literal `''` and asserting an empty
     map and **no error** (AC-32/AC-32a) — fails if the `COALESCE`-only pattern is
     copied;
  2. a validation test asserting the rejection for a bad `role_tiers` **value** carries
     `field == "role_tiers"` and a message containing neither `default_tier` nor
     `routing config invalid:` (AC-9/AC-10a) — fails if `validateTier`'s error is
     returned or wrapped verbatim;
  3. a validation test asserting that **two** bad `role_tiers` entries produce **two**
     `Details` entries (AC-8/AC-10) — fails if `checkTierMapped`'s single-value shape is
     copied;
  4. a handler test asserting the PATCH `model` rejection body carries
     `field == "model"` (AC-14c) — fails if the bare `gin.H{"error": ...}` form is copied
     from the `agent_profile_id` rejection.

  Four is a floor, not a ceiling: any further precedent the builder chooses to follow
  earns the same treatment. AC-40 is the same delegation AC-18c and AC-13c already make
  — move the guarantee off a list a human maintains and onto a check a machine runs.

## Out of scope

Named exclusions. Each is a deliberate contract, not an omission.

- **Reversing the wake-reason / per-agent precedence.** Justified in
  [Deviation](#deviation-from-the-cards-stated-acceptance--read-this-first). If it is
  wanted, it is a separate change to `docs/specs/office/routing.md`.
- **User-defined roles.** `AgentRole` stays a fixed seven-value enum. Making roles
  extensible would give the Critic case a role-based answer, but it is a much larger
  change to Office identity and is not attempted here.
- **A `tier` column on `agent_profiles`.** The card offers it as an alternative;
  rejected so routing policy stays in `office_workspace_routing`, matching
  `tier_per_reason`.
- **Removing or backfilling the `model` column on Office identity rows.** AC-15 keeps
  the stored value. Dropping the column touches the shared `agent_profiles` table used
  by execution profiles and kanban, and is not in this scope.
- **Per-agent or per-role provider *order* defaults.** Only tier selection gains a role
  level. `provider_order` keeps its existing two levels.
- **Per-project, per-task, or per-skill tier selection.**
- **Automatic tier suggestions**, or any heuristic that picks a tier for a role without
  the operator saying so.
- **Changing which model IDs the tiers map to.** `provider_profiles` is untouched.
- **Cost reporting or budget enforcement changes** arising from role-differentiated
  tiers.

## Surfaces touched (E2E decision input)

- **Backend** — `internal/office/routing/{types,resolver,provider}.go` (incl.
  `provider.go#tierSourceForAgent`, which after AC-20d delegates to the widened
  `effectiveTier` rather than deciding the source itself),
  `internal/office/repository/sqlite/workspace_routing.go` (`role_tiers` read/write SQL),
  `internal/office/repository/sqlite/base_migrations.go` (**both** additive migrations —
  `office_workspace_routing.role_tiers` and `office_run_route_attempts.tier_source`;
  this is where the replayable `ALTER` precedent for these tables lives),
  `internal/office/models/models.go` (`RouteAttempt.TierSource`, AC-20f),
  `internal/office/repository/sqlite/route_attempts.go` (INSERT + SELECT, AC-20f),
  `internal/office/scheduler/dispatch_routing.go` (three `AppendRouteAttempt` sites,
  which need **three different edits** — see AC-20f's table),
  `internal/office/dashboard/handler_routing.go`,
  the AC-40 precedent-trap tests (decode in
  `internal/office/repository/sqlite`, validation in `internal/office/routing`, the
  PATCH rejection in `internal/office/agents`),
  `internal/office/dashboard/routing_dto.go` (`AgentRoutePreview` doc comment +
  `RouteAttemptDTO` per AC-20b), `internal/office/agents/handler.go` and a new
  embedding response DTO in `internal/office/agents` (AC-13a) with its shape-preservation
  test (AC-13c). **Not edited but affected:**
  `internal/office/scheduler/routing_events.go` needs no change yet publishes the new
  field automatically, because it serialises `models.RouteAttempt` whole (AC-20b).
- **API** — workspace routing GET/PUT gains `role_tiers`; all five Office-identity agent
  payloads drop `model` (AC-13b); `PATCH /office/agents/:id` rejects a `model` key
  (AC-14); `GET /runs/:id/attempts` and the run-detail payload gain `tier_source`
  (AC-20b); the `route_attempt_appended` WS event gains it additively as a consequence,
  which AC-20b accepts rather than suppresses.
- **Frontend** — `app/office/workspace/routing/` gains a role-tier card;
  `agent-preview-table.tsx` renders a translated source label (AC-16a);
  `app/office/agents/[id]/components/agent-routing-card.tsx` and `agent-route-strip.tsx`
  gain the four-level source label; TS `AgentRoutePreview` in
  `lib/state/slices/office/types.ts` widens and TS `RouteAttempt` in the same file gains
  `tier_source?` (AC-20b) — TS `AgentRoutingOverrides` does **not** (AC-18a);
  `lib/state/slices/office/office-routing.test.ts` updates its `setRoutingPreview`
  fixture (AC-18a item 7); TS `WorkspaceRouting` gains `role_tiers` and the
  routing-config PUT helper in `e2e/helpers/office-api-client.ts` gains it too (AC-26a).
  This list is indicative; **AC-18c's typecheck gate is authoritative** for the
  `tier_source` widening.
- **Explicitly NOT touched** (persisted-state sites, per AC-18a) —
  `internal/office/onboarding/service.go#writeAgentInheritMarkers`; the `tier_source`
  **inside the `overrides` blob** in `e2e/helpers/office-api-client.ts`; and the
  persisted-override reads/writes in `agent-routing-card.tsx`. Note the same e2e helper
  *is* edited elsewhere, for an unrelated field: its routing-config PUT `cfg` type gains
  `role_tiers` per AC-26a. The untouched thing is the override `tier_source`, not the
  file.
- **i18n** — new copy in five locales (AC-19), including the four source labels and the
  AC-16a table label.

User-visible UI changes in the Office routing settings and the agent routing card mean
**E2E coverage is warranted**, scoped to the existing Office routing specs
(`apps/web/e2e/tests/office/office-routing-*.spec.ts`) rather than a new suite.
