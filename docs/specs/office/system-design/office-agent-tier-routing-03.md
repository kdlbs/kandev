---
status: draft
system: office
requirements:
  - REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001
created: 2026-08-20
owners:
  - nova28
---
# Office per-agent and per-role tier selection System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-OFFICE-AGENT-TIER-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
  wanted, it is a separate change to `docs/specs/office/requirements/routing.md`.
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
