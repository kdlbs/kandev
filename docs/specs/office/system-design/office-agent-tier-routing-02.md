---
status: draft
system: office
requirements:
  - REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001
created: 2026-08-20
owners:
  - nova28
---
# Office per-agent and per-role tier selection System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
  replaces it entirely, per `docs/specs/office/requirements/routing.md`); OTHERWISE the effective map
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
