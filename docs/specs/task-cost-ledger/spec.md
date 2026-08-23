---
status: draft
created: 2026-08-23
owner: nova28
---

# Task Cost & Token Ledger

Umbrella: [costs](../costs/) · Related: [office/costs](../office/costs.md) -
this spec amends four named passages in that document and Build lands the
amendment in the same change; see *Out of scope*. Everything else there is
unchanged.

## Why

An operator running Kandev today cannot answer "what did this task cost?".
Token counts for each turn are captured but buried in a JSON metadata blob with
no cost attached and no way to aggregate; dollar cost is computed only by the
Office subscriber, and Office ships disabled in production
(`KANDEV_FEATURES_OFFICE: prod: "false"`). Every Kanban-only install — the
shipped default — accumulates real spend with no durable, queryable record of
it.

## What

This feature adds a production-owned, append-only usage ledger: one immutable
row per observed agent usage event, carrying token counts, resolved dollar
cost, and the inputs needed to re-derive that cost. It runs regardless of the
Office feature flag, and it is the single writer of the per-session usage
rollup. It also adds the first read surface over that ledger: per-task and
per-session usage totals.

### Recording

- **AC-1** (Ubiquitous) The system SHALL persist one ledger row for every
  agent usage event it observes, in every installation, independent of the
  `features.office` toggle. This feature introduces **no new runtime feature
  toggle**: it is additive, it cannot alter agent behaviour (see *Failure
  modes*), and gating it would reproduce the exact defect it exists to fix —
  a shipped default that records nothing.
- **AC-2** (Event-driven) WHEN a usage event is observed for a known task, the
  system SHALL record the event's input tokens, cached-read tokens,
  cached-write tokens, output tokens, thought tokens, total tokens, resolved
  cost in subcents, cost source, and estimation flag as a single new row.
- **AC-3** (Ubiquitous) Ledger rows SHALL be immutable once written. The system
  SHALL provide no operation that updates or partially rewrites a recorded row.
  Correction is expressed by recording a further row, never by mutating one.
- **AC-4** (Event-driven) WHEN a usage event carries an output-token count that
  the adapter did not observe, the system SHALL record output tokens as *not
  recorded* (NULL) rather than as zero, so an unobserved sample is
  distinguishable from a measured zero.
- **AC-30** (Ubiquitous) Of the four nullable token columns, only `tokens_out`
  is reachable as *not recorded* from the current producer, and the writer
  SHALL be specified accordingly rather than inventing a rule.
  `streams.PromptUsage` (`internal/agentctl/types/streams/agent.go`) carries a
  presence bit for output tokens alone - `OutputTokensPresent bool`, always
  serialized, which is what makes AC-4 implementable - while
  `CachedReadTokens`, `CachedWriteTokens` and `ThoughtTokens` are plain `int64`
  with `omitempty`, so an absent field and a measured zero are byte-identical
  on the wire. For those three the writer SHALL record the decoded numeric
  value, which is `0` when the field is absent, and SHALL NOT map absence to
  NULL: that would write *not recorded* over a genuine measured zero, the same
  conflation AC-4 forbids in the other direction, and it would do so for the
  overwhelming majority of rows. The three columns nonetheless stay nullable,
  so a transport that later carries presence bits can record the distinction
  without a migration; AC-12's explicit coalesce therefore remains required,
  both for `tokens_out` today and for those columns under any such transport.
  Consequently a NULL in `tokens_cached_read`, `tokens_cached_write` or
  `tokens_thought` is unreachable through the event path today. Scenarios
  asserting NULL handling for those three are forward-compatibility cover over
  a directly inserted row and SHALL say so; the reachable NULL path is
  exercised through `tokens_out`.
- **AC-5** (Ubiquitous) Each row SHALL carry the contract version under which it
  was written, so a later contract change can be detected per row rather than
  inferred from a date.
- **AC-23** (Ubiquitous) `tokens_total` SHALL be computed by the writer as
  `tokens_in + tokens_cached_read + tokens_cached_write + tokens_out +
  tokens_thought`, each *not recorded* value contributing zero. It SHALL NOT be
  copied from the payload. A provider-reported total that disagreed with the
  components would make a single API response self-contradictory; the
  components are authoritative and a differing payload total is ignored, not
  recorded. An overflowing sum is dropped as `overflow`.
- **AC-24** (Ubiquitous) The ledger writer SHALL consume the prompt-usage event
  that already exists on the internal bus: subject
  `events.BuildSessionPromptUsageWildcardSubject()`
  (`session_prompt_usage.updated.*`, declared in `internal/events/types.go`),
  published by the orchestrator's `publishPromptUsage`
  (`internal/orchestrator/event_handlers_streaming.go`). This feature
  introduces no new event, subject, or publisher, and does not change what the
  orchestrator emits. That publisher is **not** unconditional, and this spec
  SHALL NOT be read as claiming it is: it returns early when the session id is
  empty, when the event bus is nil, or when `payload.Data.Usage` is nil. A turn
  whose agent reported no usage object therefore produces no event, and so no
  ledger row and no drop counter - it is invisible to this feature rather than
  dropped by it. The ledger records exactly what is published and no more.
  Widening the publisher's precondition would change the orchestrator's
  published contract and is named in *Out of scope*.
- **AC-25** (Ubiquitous) The subscriber and writer SHALL live in a new package
  `internal/task/usage`, in the task tier that owns the table, and SHALL NOT
  import any `internal/office/**` package. It SHALL decode the bus payload into
  a struct it declares itself, whose JSON tags mirror
  `lifecycle.SessionPromptUsageEventPayload` - exactly as
  `office/service.PromptUsageData` already does for the same event. The wire
  contract is the JSON, not a shared Go type. This is what keeps the dependency
  direction the backend requires: `internal/agent/runtime/lifecycle` imports
  `internal/task/models`, so importing the lifecycle payload type from the task
  tier would point an edge back up the stack.
- **AC-34** (Ubiquitous) The subscriber SHALL NOT do its work on the bus
  callback goroutine. `MemoryEventBus.Publish` delivers to every regular
  subscription **synchronously**, on the publisher's own goroutine - its call
  site says so ("deliver to all synchronously to preserve ordering") - and the
  publisher here is `publishPromptUsage`, on the orchestrator's streaming path.
  A blocking handler would therefore put a database write, and under AC-32 up
  to three attempts with backoff between them, directly in front of agent
  streaming, contradicting the guarantee in *Failure modes* that recording
  never affects an agent run. The bus callback SHALL decode the payload, hand
  the result to a single long-lived worker over a bounded buffered channel, and
  return `nil` immediately. Specifically:
  - The channel SHALL hold **1024** events. A send that would block SHALL be
    abandoned rather than waited on, counted `dropped:overflow` (AC-27), and
    logged at warn with the task id. Blocking on a full channel reintroduces
    exactly the coupling this criterion removes, and silently discarding
    without a counter reproduces the invisible-writer failure AC-27 exists to
    bound.
  - Exactly **one** worker SHALL drain the channel, serially. That is what
    makes `occurred_at` assignment monotonic and therefore what makes AC-16's
    `(occurred_at, id)` order match arrival order, and it keeps AC-32's backoff
    off every other goroutine in the process. A goroutine per event - the shape
    `office/service`'s `maybeAsync` uses for this same subject - SHALL NOT be
    used: it is unbounded under burst and it makes `occurred_at` acquisition
    racy against arrival, so two events could be ordered opposite to the order
    they were published in.
  - On shutdown the writer SHALL stop accepting new events, drain what is
    already buffered, and return once the buffer is empty or after **5
    seconds**, whichever comes first. Events still buffered at that deadline
    are counted `dropped:drain_timeout` and logged. An unbounded drain would hang
    process shutdown behind a wedged database; a drain of zero would discard
    every buffered event on every restart.
  - An event offered by the bus callback **after** the drain has begun SHALL be
    refused rather than queued, counted `dropped:shutdown` (AC-27), and logged;
    the callback SHALL still return `nil` immediately, because the bus delivers
    on the publisher's goroutine and a closing writer may no more block an
    agent turn than a busy one. `shutdown` marks a closed writer, not queue
    backpressure.
  - The event **in flight** at the deadline - the one the worker is writing, as
    distinct from those still queued behind it - SHALL be resolved too. The
    worker's per-event context SHALL derive from a context cancelled at the
    drain deadline, and AC-32's backoff sleeps SHALL honour that cancellation
    rather than run to completion. A cancelled in-flight event is counted
    `dropped:error`, its transaction rolls back, and **no ledger row or rollup
    increment is written after the drain returns**. Left unspecified, a writer
    may commit into a pool the process is closing, which is a use-after-close
    rather than a late row; and "shutdown returns within 5 seconds" becomes
    untestable, because the assertion cannot distinguish a drain that returned
    from a worker still running behind it.
  - The writer SHALL be constructed, started, and subscribed to the bus in
    `internal/backendapp/main.go` **outside** the `if !cfg.Features.Office`
    early return that today encloses `RegisterEventSubscribers` - the
    Office-disabled install is the one this feature exists for - and its drain
    SHALL be registered through the same `addCleanup` mechanism as the other
    shutdown steps, at a point **after** the repository cleanups are
    registered. `runCleanups` runs registrations in **LIFO** order, so
    registering later means draining earlier, which is what puts the drain
    ahead of the database pool's close. Registered before them it would run
    after the pool closed and every drained event would fail on a closed pool -
    the exact failure the bounded drain exists to prevent, reached by getting
    the ordering backwards.

  Decoding on the callback and writing on the worker is deliberate: a decode
  failure is attributable to the event that caused it (`dropped:decode_error`)
  and costs no queue slot, while the expensive and retry-prone part is what
  gets detached.
- **AC-35** (Event-driven) WHEN a usage event is recorded, the writer SHALL
  carry `model`, `agent_type` and `agent_profile_id` onto the row from the
  correspondingly named payload fields, writing `''` for one only when the
  payload's own field is empty. AC-2 enumerates the token and cost values and
  stops there; without this criterion none of these three is required by
  anything, all three are `NOT NULL DEFAULT ''`, and a writer that leaves all
  three empty satisfies every other criterion in this spec. It would also make
  AC-7's audit columns useless: the row would record the rates and the
  catalogue version that priced it, but not what was priced, so the cost could
  be neither re-derived nor attributed to a model or a profile. `agent_type` is
  additionally an input AC-31 derives `provider` from, so leaving it empty
  silently degrades that derivation to its model-prefix tier.
- **AC-27** (Ubiquitous) The writer SHALL export two expvar maps, visible at
  `/debug/vars`, mirroring the shape of
  `internal/office/service/cost_metrics.go`:
  `task_usage_events_written_total` keyed
  `source=<cost_source>;provider=<provider>`, and
  `task_usage_events_dropped_total` keyed `reason=<reason>` where reason is one
  of `decode_error`, `unattributable`, `invalid`, `duplicate`, `overflow`,
  `drain_timeout`, `shutdown`, `error`. Every
  terminal transition in *State machine* SHALL increment exactly one counter, so
  a writer that has silently stopped is observable and the drop classes are
  distinguishable from each other. The stages SHALL run in the order **decode,
  admit, validate, ownership, insert** - each stage's failure terminal, so
  exactly one counter is incremented however many stages an event could have
  failed. Admit is AC-34's queue hand-off, placed after decode so a malformed
  payload costs no queue slot: an event refused because the queue is full
  counts `overflow`, and one refused because the shutdown drain has already
  begun counts `shutdown` (AC-34). Uniqueness is deliberately **not** a stage
  of its own - duplicates are detected by the unique constraint at insert,
  which is the last stage (AC-13, AC-32) - so an event that is both a
  redelivery and invalid counts `invalid`, because it never reaches the insert.
  Validation likewise precedes AC-33's session-ownership check, so an event
  that is both invalid and session/task-mismatched counts `invalid` and is not
  recorded at all. The reason is not preference: validation reads only the
  decoded payload, while the ownership check costs a database read, and an
  event already known unrecordable SHALL NOT cause one.

  Within the validate stage the sub-checks SHALL themselves run in a fixed
  order, so an event failing more than one is classified deterministically:
  (1) `usage_event_id` present and non-empty, else `invalid`; (2) `task_id`
  present and non-empty, else `unattributable`; (3) no token or cost value
  negative, else `invalid`. That order follows the schema's own requiredness -
  the idempotency key, then the mandatory attribution column, then value sanity
  - and only the (2)-versus-(3) pairing is observable, since (1) and (3) share
  a label: an event missing `task_id` **and** carrying a negative token count
  counts `unattributable`, not `invalid`. Left unstated, two conforming
  implementations disagree on that event's counter and both satisfy this
  criterion.

  These counters observe the **subscriber**, not the producer. They
  bound the failure "the writer received events and stopped recording them";
  they deliberately do not bound "the producer stopped publishing", because per
  AC-24 an unpublished turn increments neither counter. No surface SHALL
  present them as proof that a task's recorded spend is complete.

### Cost resolution

Cost resolution reuses the two-layer contract already proven in
[office/costs](../office/costs.md) and records which layer produced the number.

- **AC-6** (Event-driven) WHEN the usage event carries a provider-reported cost
  and its presence flag is set, the system SHALL record that amount verbatim
  (including an explicit zero) with `cost_source = provider_reported`, and
  SHALL NOT perform a pricing lookup.
- **AC-7** (Event-driven) WHEN no provider-reported cost is present and pricing
  is resolvable for the event's model, the system SHALL compute cost from the
  token counts and record it with `cost_source = models_dev_list`, together
  with the per-million rates and pricing-catalog version used. The computation
  is exactly:

  ```
  cost_subcents = (tokens_in           * rate_input_per_million
                 + tokens_cached_read  * rate_cached_read_per_million
                 + tokens_cached_write * rate_cached_write_per_million
                 + tokens_out          * rate_output_per_million) / 1000000
  ```

  All four products are summed in 64-bit integer arithmetic **before** the
  single division, and that division truncates toward zero. Overflow records
  an `unpriced` row with zero cost. Rounding happens
  once, on the summed numerator, never per token class: rounding four times
  drifts from rounding once, and two writers that disagree about where to round
  produce different money for identical input. A *not recorded* (NULL) token
  count contributes zero to the numerator. `tokens_thought` is not priced.
  This reproduces `costs.CalculateCostSubcents`
  (`internal/office/costs/pricing.go`) exactly, so a row priced here and a row
  priced by Office from the same tokens and rates are identical to the
  subcent.
- **AC-8** (Unwanted behaviour) IF no provider-reported cost is present and
  pricing cannot be resolved, THEN the system SHALL record the row with
  `cost_subcents = 0` and `cost_source = unpriced`, and SHALL NOT drop the row.
  Token counts are still recorded. A lookup that has not returned within a
  writer-imposed deadline of **2 seconds** counts as "cannot be resolved": the
  writer SHALL bound every pricing call with its own context deadline and treat
  a breach exactly as a miss. The deadline is the writer's, not the lookup's,
  because the lookup's own default is far larger -
  `internal/office/costs/modelsdev/client.go` constructs its HTTP client with
  `Timeout: 30 * time.Second`, six times AC-34's entire 5-second drain - and
  because the injected interface (AC-26) admits any implementation. Unbounded,
  the degradation this criterion promises does not happen during the outage it
  exists for: the single serial worker (AC-34) stalls per event, the 1024-slot
  queue fills behind it, and `dropped:overflow` becomes the steady state
  exactly where `unpriced` rows were specified. 2 seconds is chosen to sit
  comfortably inside the 5-second drain, so an event being priced when shutdown
  begins can still finish and commit rather than being abandoned.
- **AC-9** (Ubiquitous) `estimated` SHALL reflect the usage sample's authority
  as reported by the adapter and SHALL be recorded independently of
  `cost_source`. An estimated row counts toward totals at face value.
- **AC-26** (Ubiquitous) The pricing arithmetic of AC-7 SHALL be extracted to
  `internal/common/costs` - the rate struct and the cost function - with
  `internal/office/costs` delegating to it, so both writers compute identically
  and neither imports the other's tier. The models.dev catalogue lookup SHALL be
  constructed in `internal/backendapp/main.go` outside the `features.office`
  branch that holds it today (the `SetPricingLookup` / `SetModelInfoLookup`
  wiring) and injected into the ledger writer through a narrow interface the
  writer declares itself, so no office type crosses the boundary.
  **Only the construction moves.** Both existing setter calls stay inside the
  `if cfg.Features.Office` block: `services.Office.SetPricingLookup` must,
  because `services.Office` is nil in a gated install, and
  `orchestratorSvc.SetModelInfoLookup` SHALL as well. That second call feeds a
  wholly unrelated capability - the orchestrator's context-window fallback,
  per its own doc comment in `internal/orchestrator/model_info.go` - which is
  gated today and which this card does not touch. It shares only the
  `pricingLookup` variable and the enclosing `if`, so a literal reading of
  "construct it outside the gate" would carry it out too and silently enable
  context-window fallback for every Office-disabled install, which is the
  shipped default this feature exists for. The block therefore splits: an
  always-run construction whose value is injected into the ledger writer, and
  a still-gated pair of setter calls. Changing when `SetModelInfoLookup` runs
  is named in *Out of scope*. That
  interface SHALL return the rates and the catalogue version from a **single
  call**, so both come from one snapshot - mirroring
  `shared.PricingLookupWithVersion` (`internal/office/shared/interfaces.go`),
  which exists for exactly this reason and which `lookupPricingWithVersion`
  type-asserts. Two independent calls SHALL NOT be used: a catalogue that
  refreshes between them writes a new `pricing_catalog_version` beside the
  previous `rate_*_per_million` values, and the resulting row is
  indistinguishable from a correct one while being unreproducible, which
  defeats the only purpose those columns have. IF that wiring
  is absent, THEN AC-8 applies and rows record as `unpriced`. That degradation
  bounds build order; it is not permission to leave the lookup unwired.
- **AC-31** (Ubiquitous) The `provider` value SHALL be derived by the writer,
  because the bus payload does not carry one:
  `lifecycle.SessionPromptUsageEventPayload` has `agent_type`, `agent_id` and
  `model`, and no provider field. Derivation SHALL be the first non-empty of,
  in order: the CLI-id mapping applied to `agent_type`; the same mapping
  applied to `agent_id`; a model-prefix match; then `''`. The CLI mapping is
  `claude-acp` to `anthropic`, `codex-acp` and `openai-acp` to `openai`,
  `gemini` and `gemini-acp` to `google`. The two CLI tiers are not redundant
  with the prefix match and SHALL NOT be collapsed into it: claude-acp reports
  logical aliases (`sonnet`, `haiku`) that no prefix match resolves, which is
  the documented reason `providerFromCLI` exists. Both helpers SHALL be
  extracted to `internal/common/costs` alongside the rate struct above -
  `ProviderForModel` from `internal/office/costs/pricing.go` and
  `providerFromCLI` from `internal/office/service/event_subscribers.go` - with
  Office's `resolveProvider` delegating to the extracted versions, so the two
  writers label the same event identically. Copying the mapping into
  `internal/task/usage` SHALL NOT be used to satisfy AC-25's no-office-import
  rule: `internal/common/` is where this backend puts cross-tier shared code,
  and a second copy is how the two PTY implementations drifted. An unresolved
  provider is the empty string, which is a valid value for a
  `NOT NULL DEFAULT ''` column and a valid expvar label.

### Rollup

- **AC-10** (Ubiquitous) The system SHALL maintain cumulative
  `tokens_in`, `tokens_cached_in`, `tokens_out`, and `cost_subcents` on the
  owning `task_sessions` row, and this ledger writer SHALL be the only writer
  of those columns.
- **AC-21** (Event-driven) WHEN `features.office` is enabled, the Office cost
  subscriber SHALL NOT increment the `task_sessions` rollup columns. The
  increment is removed from `recordCostEventAndRollup`
  (`internal/office/service/event_subscribers.go`), which continues to insert
  its own `office_cost_events` row exactly as today; the ledger writer performs
  the increment for both. This is the concrete edit AC-10's sole-writer rule
  requires. Without it an Office-enabled install double-counts every turn, and
  Office is enabled in the `dev` and `e2e` profiles, so the defect ships to
  every developer before it reaches an operator. The tests that pin the removed
  increment, and the shared-transaction plumbing it leaves behind, are resolved
  by AC-36.
- **AC-29** (Ubiquitous) `Repository.BackfillSessionTokensCachedIn`
  (`internal/task/repository/sqlite/base_migrations.go`) and its
  composition-root call SHALL be removed. It is a third writer of the AC-10
  columns, and left in place it destroys ledger data on every restart. It is an
  **assignment**, not an increment: it sets `task_sessions.tokens_cached_in` to
  `COALESCE((SELECT SUM(tokens_cached_in) FROM office_cost_events WHERE
  session_id = task_sessions.id), 0)` for every row matching
  `EXISTS (SELECT 1 FROM office_cost_events ...) OR tokens_cached_in <> 0`. Its
  own doc comment records that it runs unconditionally on every boot with no
  run-once tracking, and `internal/backendapp/storage.go` calls it on every
  startup with **no `features.office` gate** - it sits immediately after
  `office.Provide`, which is itself ungated. In the Office-disabled install this
  feature exists for, `office_cost_events` is empty, so the `OR tokens_cached_in
  <> 0` arm matches every session this ledger has just written and assigns zero;
  a failure there also aborts boot. Removal SHALL also delete the
  `sessionCachedTokenBackfiller` interface, the `backfillSessionCachedTokens`
  helper, and the construction-order comment that ties repository provisioning
  to it. Rows already reconciled by it keep their current values and are not
  recomputed, consistent with the no-backfill rule in *Out of scope*. Gating it
  behind `features.office` SHALL NOT be used instead: after AC-21 the Office
  subscriber no longer maintains those columns in any install, so
  `office_cost_events` is no longer their source of truth anywhere and
  re-deriving from it is wrong everywhere, not merely misplaced. Seven existing
  tests pin the surface this criterion deletes; their disposition is AC-36, and
  Build SHALL NOT decide it ad hoc.
- **AC-36** (Ubiquitous) Every production surface removed or changed by AC-21
  and AC-29 SHALL have the tests that pin it resolved in the same change, and
  this criterion states which, because deleting or skipping a test to make a
  build green is otherwise forbidden and Build has no authority to grant itself
  the exception. Two of the tests below assert as correct the exact behaviour
  those criteria identify as destructive, so they cannot be left passing
  either. The rule is: a test whose subject no longer exists is deleted with
  it, which is not a weakening; a test whose subject moved to a new owner is
  rewritten against that owner, never deleted; and where a deleted test carried
  an assertion nothing else reproduces, that assertion SHALL be re-established
  against the ledger writer **before** the deletion lands, so no round of this
  change is net-negative on coverage.

  | Surface removed or changed | Tests pinning it | Disposition |
  |---|---|---|
  | `Repository.BackfillSessionTokensCachedIn` (AC-29) | `task/repository/sqlite/session_usage_test.go`: `TestBackfillSessionTokensCachedIn_SumsLedger`, `_CorrectsDriftedRollup`, `_ZeroesRollupWithNoMatchingLedgerRows`, `_SkipsAlreadyCorrectUntouchedSessions` | **Deleted with the function.** Their subject is gone. `_ZeroesRollupWithNoMatchingLedgerRows` in particular asserts the zeroing AC-29 identifies as data loss, so it cannot be retained or adapted. AC-12 is the surviving statement of the rollup-equals-ledger invariant these defended, and is separately tested. |
  | same, Postgres parity (AC-29) | `task/repository/sqlite/session_usage_postgres_test.go`: `TestPostgresBackfillSessionTokensCachedIn_SumsLedger`, `_HandlesValuesBeyondInt32Range` | **Deleted with the function**, with one precondition: `_HandlesValuesBeyondInt32Range` is the only beyond-`int32` rollup coverage in the suite, so AC-28's equivalent boundary assertion against the ledger writer's rollup SHALL land first. |
  | `backfillSessionCachedTokens` call site, helper, and `sessionCachedTokenBackfiller` interface in `backendapp/storage.go` (AC-29) | `backendapp/storage_cached_tokens_test.go`: `TestProvideRepositoriesBackfillsSessionCachedTokens` | **Deleted.** It asserts the boot path calls the backfiller, which is precisely the behaviour being removed. Its replacement observable already exists: AC-29's two-boot restart scenario, which passes only because nothing recomputes the rollup. |
  | the rationale comment in `backendapp/storage_telemetry_test.go` citing that test as its precedent | (comment only) | **Rewritten** to cite a surviving neighbour. A comment pointing at a deleted test is a dangling reference inside the suite, and this one explains why an unrelated boot-order assertion exists. |
  | rollup increment inside `recordCostEventAndRollup` (AC-21) | `office/service/session_usage_reconcile_test.go`: `TestPromptUsage_RollupReconcilesWithCostLedger` | **Deleted.** It asserts Office's rollup equals the sum of `office_cost_events`, which AC-21 makes false by design in every install. |
  | same (AC-21) | `office/service/event_subscribers_cost_atomicity_test.go`: `TestPromptUsage_RollupFailureRollsBackLedgerInsertThenRetrySucceeds` | **Rewritten against the ledger writer.** The behaviour it encodes - a rollup failure rolls the insert back, a retry then succeeds - is still required, by AC-11 and AC-32; only its owner changes. Deleting it would drop the one existing test of that interaction. |
  | the shared-transaction plumbing (`IncrementTaskSessionUsageTx` and the repository-sharing helpers) | `office/repository/sqlite/cost_event_tx_atomicity_test.go`: `TestCostEventTxAtomicity_RealReposShareTransaction` | **Retained, repointed.** The abstraction is explicitly NOT removed: AC-11 requires exactly this capability for the ledger writer, so AC-21 removes a *caller*, not the mechanism. The test SHALL exercise the ledger writer's repositories instead of Office's. |

- **AC-32** (Event-driven) WHEN a ledger insert fails, the writer SHALL classify
  the failure before reacting, and SHALL NOT distinguish failure kinds by
  matching the driver's message text uniformly across dialects: SQLite reports
  `FOREIGN KEY constraint failed` without naming the constraint, while Postgres
  names it, so one string match is wrong on one dialect. Classification SHALL
  use dialect-aware helpers added to `internal/db` beside the existing
  `IsDuplicateColumnError` / `IsAlreadyExistsError` and following their shape
  (a `pgconn.PgError` SQLSTATE check with a SQLite-specific fallback):
  `IsForeignKeyViolation` (Postgres `23503`) and `IsTransientError` (Postgres
  `40001` serialization failure and `40P01` deadlock; SQLite `SQLITE_BUSY` and
  `SQLITE_LOCKED`).

  A unique violation is classified by a **third** detector, which deliberately
  does NOT live in `internal/db`: it SHALL be constraint-specific, declared in
  the task repository package (`internal/task/repository/sqlite`) beside the
  table it guards, matching Postgres on `pgErr.Code == "23505" &&
  pgErr.ConstraintName == "uniq_task_usage_events_usage_event_id"` and SQLite
  on the constraint-bearing message
  `UNIQUE constraint failed: task_usage_events.usage_event_id`, and SHALL
  surface to its caller as a package-level sentinel error matched with
  `errors.Is`. This is not a new idea in this repo: it is the shape of
  `isExternalIDUniqueViolation` / `uniq_tasks_external_id`
  (`internal/task/repository/sqlite/task_external_id.go`) and of
  `isUsageEventUniqueViolation` / `ErrDuplicateUsageEvent` /
  `uniq_office_cost_usage_event`
  (`internal/office/repository/sqlite/costs.go`), and it is already a stated
  contract in a prior spec of this feature's own tier -
  `docs/specs/tasks/external-id-idempotency/spec.md`, "Unique-violation
  classification across dialects" - which gives the reason: "A bare `23505`
  check is insufficient: it matches any unique violation, so a primary-key
  failure would be misread as a Found outcome." Here the misread is worse than
  a wrong label: `duplicate` is the one drop class this spec declares *not* an
  error, so a violation of some other constraint classified as a duplicate is
  swallowed silently and counted as expected behaviour.

  Detection SHALL be by the constraint **at insert**, NOT by a pre-insert
  `SELECT`. A read-then-insert pair is not atomic: two concurrent deliveries of
  the same deterministic identifier can both read before either writes, both
  find nothing, and both attempt the insert - so the constraint decides the
  outcome regardless, after the read has already misled the writer about which
  branch it is on. The reaction SHALL be:
  - **transient** - re-run the whole unit of work, unchanged, in a **fresh
    transaction** per attempt, to a maximum of three attempts in total. "The
    insert" here means the AC-11 transaction entire - the ledger insert *and*
    the rollup increment - not the INSERT statement alone. Postgres aborts the
    whole transaction on both `40001` and `40P01`, so re-issuing one statement
    inside the failed transaction returns `25P02 current transaction is
    aborted` instead of reproducing the original condition, which would make
    the three-attempt guarantee unreachable on that dialect while appearing to
    work on SQLite. The row keeps its `session_id` and its rollup increment.
    Backoff between attempts SHALL be **50 ms then 100 ms** - fixed
    exponential, base 50 ms, factor 2, no jitter, no cap needed at two waits -
    bounding a retried event at roughly 150 ms of added latency. That latency
    lands on the writer's own worker goroutine (AC-34) and never on the
    agent's, which is what makes a blocking sleep acceptable here at all.
    Jitter is deliberately omitted: with a single serial writer (AC-34) there
    is no fleet of peers to synchronize with, so it would add variance without
    reducing contention. Exhausted attempts are `dropped:error`.
  - **foreign key** - retry exactly once with `session_id = NULL` and no rollup
    increment. This deliberately requires no session-versus-task
    discrimination: if the violated key was `task_id`, the retry is rejected
    too and the row is `dropped:error`, which is already the specified outcome
    for that case.
  - **unique violation on `usage_event_id`** - no retry, no rollup increment,
    counted `dropped:duplicate` (AC-14), never `dropped:error`. This is the
    only insert failure that is an expected outcome rather than a fault, and it
    is what makes AC-14 reachable at all: without this bucket the "anything
    else" branch below claims every redelivery, so a correctly deduplicating
    ledger would report a climbing error count and a permanently zero duplicate
    count, and *Failure modes*' "not an error" row would be unimplementable.
  - **anything else** - `dropped:error`, no retry.

  A transient failure SHALL NOT take the foreign-key path. Nulling the session
  of a row that would have committed on a second attempt silently discards
  attribution the writer actually had, and does so precisely under load, where
  it is least likely to be noticed.
- **AC-33** (Event-driven) WHEN a usage event carries both a task id and a
  session id, the writer SHALL verify that the referenced session belongs to
  that task before inserting. IF they disagree, THEN the row SHALL be recorded
  under the payload's `task_id` with `session_id = NULL` and no rollup
  increment, and a warning SHALL be logged. It is not dropped: the spend is
  real, the task attribution is what the caller asserted, and the session
  reference is the field proven inconsistent. This is deliberately the same end
  state as an absent session row, so the two share one code path. Without this
  rule the writer can create rows the read side is specified to refuse - a
  session-scoped read returns 404 when the session does not belong to the task
  in the path - and `IncrementTaskSessionUsageTx` updates by session id alone,
  so it would have incremented another task's session rollup.
- **AC-11** (Event-driven) WHEN a ledger row is recorded, the ledger insert and
  the rollup increment SHALL be applied in a single transaction, so no
  committed state exists in which one landed and the other did not.
- **AC-12** (Ubiquitous) For any session whose usage was recorded by this
  ledger, the rollup columns SHALL equal the sum of that session's ledger rows, where `tokens_cached_in` sums
  `cached_read + cached_write`. Every nullable token column contributes zero
  when it is *not recorded* (NULL): this applies to `tokens_cached_read`,
  `tokens_cached_write`, `tokens_out`, and `tokens_thought` alike, in the rollup
  increment and in every read aggregation. The zero SHALL be produced by an
  explicit coalesce, not left to SQL, where `NULL + n` is `NULL` and one
  unrecorded sample would null out a whole session's total. Consequently the
  session output total is a lower bound when any of its rows has an unrecorded
  output count. The equality is a forward guarantee. Sessions whose rollup was
  written by Office before AC-21 removed that increment have no ledger rows
  behind those values, are not backfilled (see *Out of scope*), and are outside
  this criterion.
- **AC-28** (Ubiquitous) `task_sessions.cost_subcents`, `task_sessions.tokens_in`
  and `task_sessions.tokens_out` SHALL be widened to `BIGINT`, so all four
  rollup columns satisfy the same 64-bit rule as the ledger. The migration is
  idempotent and dialect-aware in the established `runMigrations()` shape: on
  Postgres it alters the column type; on SQLite it is a no-op, because SQLite's
  `INTEGER` is already 64-bit.

### Idempotency and ordering

- **AC-13** (Ubiquitous) The externally minted usage event identifier SHALL be
  unique across the ledger and SHALL be the idempotency key.
- **AC-14** (Unwanted behaviour) IF an event bearing a **deterministic**
  identifier is redelivered with an identifier already present in the ledger,
  THEN the system SHALL record no new row, apply no rollup increment, and
  report the drop as a duplicate rather than as an error.
- **AC-22** (Ubiquitous) Deduplication SHALL be exactly as strong as the
  identifier the producer minted, and SHALL NOT be described as stronger.
  `usageEventIDFor` (`internal/orchestrator/event_handlers_streaming.go`)
  derives a deterministic UUIDv5 from
  `(session_id, execution_id, prompt_generation)` only when all three are
  present; when any is missing it returns a fresh random UUID, which
  [office/costs](../office/costs.md) already documents deliberately:
  "Generation-less transports receive a random per-publish key, so those rows
  are intentionally not deduplicated." For that transport class the system
  SHALL record a redelivered event as a NEW row and SHALL apply its rollup
  increment. The ledger records what it was told. AC-14 is therefore a
  guarantee about identifiers, not a guarantee that spend is never counted
  twice, and no read surface may present it as the latter. Changing the minting
  rule is out of scope (see *Out of scope*). A test that exercises AC-14 with a
  hand-chosen fixed identifier proves nothing about this path; the redelivery
  scenarios below cover both classes deliberately.
- **AC-15** (Ubiquitous) A single turn MAY produce multiple ledger rows.
  `(session_id, turn_id)` is an aggregation key, NOT a uniqueness constraint.
- **AC-16** (Ubiquitous) Ledger reads SHALL be ordered by `occurred_at`
  ascending with the monotonic `id` column as the tiebreak. This pair is a
  total order: no two rows compare equal, so repeated reads return an identical
  sequence. `id` orders rows by the instant the writer acquired it, which under
  concurrent commits may differ from commit order; the contract is determinism
  and stability, not a claim that `id` reproduces commit sequence.

  This ordering binds a named read. The repository SHALL expose
  `ListTaskUsageEvents(ctx, taskID string, limit int) ([]TaskUsageEvent, error)`
  returning that task's rows in `(occurred_at, id)` ascending order, `limit` of
  zero or less meaning no limit. An unknown task and a task with no rows both
  return an empty slice and a nil error, never an error and never a nil slice.
  It is repository-internal: it backs the append-only and ordering assertions
  in *Scenarios*, and *Out of scope* keeps it off the HTTP surface. Without a
  named method this criterion would constrain a read no builder is required to
  write, and the two specified routes return aggregates whose `MIN`/`MAX` and
  `SUM` are order-independent - they cannot observe it.
- **AC-17** (Unwanted behaviour) IF a usage event arrives with no turn
  identifier, THEN the system SHALL still record the row with a null `turn_id`
  and SHALL still apply the rollup. A missing turn identifier degrades
  per-turn grouping only; it never costs a row. In the sampled database this is
  the majority case (1,135 of 1,962 rows), so dropping such events would
  discard most recorded spend.

### Reading

- **AC-18** (Ubiquitous) The system SHALL expose per-task usage totals and
  per-session usage totals over HTTP, in every installation, independent of the
  `features.office` toggle.
- **AC-19** (Ubiquitous) Totals SHALL report tokens by kind, total cost in
  subcents, the number of contributing events, and whether any contributing row
  was estimated or unpriced, so a caller can tell a confident total from a
  partial one. `tokens_total` in a response SHALL be the sum of the stored
  `tokens_total` column over the contributing rows, never recomputed from the
  per-kind columns at read time. The response SHALL report every kind AC-23
  sums - `tokens_in`, `tokens_cached_read`, `tokens_cached_write`, `tokens_out`
  and `tokens_thought` - so that "the two agree by construction" is a property
  the caller can actually verify. Because AC-23 computes the stored column on
  write from exactly those kinds and the response neither recomputes nor omits
  one, a response cannot contradict itself. Omitting a summed kind would make
  `tokens_total` exceed the visible kinds for every reasoning-heavy model while
  the spec claimed the opposite.
- **AC-20** (Ubiquitous) A task or session with no recorded usage SHALL return
  zeroed totals with a zero event count, not an error and not an empty body.
  `output_tokens_complete` SHALL be `true` in that case: no contributing row has
  an unrecorded output count, so the zero total is exact rather than a lower
  bound. `estimated_event_count` and `unpriced_event_count` are `0` and both
  timestamps are null.

## Data model

### `task_usage_events` (new)

Owned by the task repository (`internal/task/repository/sqlite`), alongside
`task_step_transitions`, whose append-only-log conventions it follows:
dialect-aware monotonic integer primary key, a `contract_version` column, and
`(occurred_at, id)` ordering.

```
task_usage_events
  id                       integer   PK, monotonic
                                     (SQLite AUTOINCREMENT / Postgres BIGSERIAL)
  usage_event_id           text      NOT NULL — idempotency key (AC-13)
  task_id                  text      NOT NULL, FK -> tasks(id) ON DELETE CASCADE
  session_id               text      nullable, FK -> task_sessions(id)
                                     ON DELETE SET NULL
  turn_id                  text      nullable, no FK
  agent_profile_id         text      NOT NULL DEFAULT ''
  agent_type               text      NOT NULL DEFAULT ''  -- CLI engine slug
  model                    text      NOT NULL DEFAULT ''
  provider                 text      NOT NULL DEFAULT ''
  tokens_in                bigint    NOT NULL DEFAULT 0
  tokens_cached_read       bigint    nullable — NULL = not recorded
  tokens_cached_write      bigint    nullable — NULL = not recorded
  tokens_out               bigint    nullable — NULL = not observed (AC-4)
  tokens_thought           bigint    nullable
  tokens_total             bigint    NOT NULL DEFAULT 0 - computed, AC-23
  cost_subcents            bigint    NOT NULL DEFAULT 0
  cost_source              text      NOT NULL — provider_reported
                                     | models_dev_list | unpriced
  estimated                integer   NOT NULL DEFAULT 0
  rate_input_per_million         bigint   nullable
  rate_cached_read_per_million   bigint   nullable
  rate_cached_write_per_million  bigint   nullable
  rate_output_per_million        bigint   nullable
  pricing_catalog_version  text      nullable
  contract_version         integer   NOT NULL
  occurred_at              timestamp NOT NULL
  created_at               timestamp NOT NULL

  UNIQUE INDEX uniq_task_usage_events_usage_event_id (usage_event_id)
  INDEX (task_id, occurred_at, id)
  INDEX (session_id, turn_id)
  INDEX (occurred_at)
```

The uniqueness of `usage_event_id` SHALL be that **named** unique index, not
an inline column constraint. AC-32's Postgres detector matches on
`pgErr.ConstraintName`, so the name is part of the contract: an auto-generated
name would differ between a fresh install and a migrated one and make the
detector unportable between them. It is created the way
`uniq_office_cost_usage_event` is
(`internal/office/repository/sqlite/base_migrations.go`) and named the way
`uniq_tasks_external_id` is.

Units and nullability rules:

- `cost_subcents` is hundredths of a cent (int64). A caller renders dollars by
  dividing by 10,000. This matches
  `office_cost_events.cost_subcents` and `task_sessions.cost_subcents`; no new
  unit is introduced. There is no `cost_usd` column — a float dollar amount
  would make sums non-associative and is deliberately not stored.
- A nullable token column means *not recorded*; zero means *measured zero*.
  The two are never conflated (AC-4). Which columns can actually carry *not
  recorded* is bounded by what the wire can express: only `tokens_out` today
  (AC-30). In every arithmetic context - the rollup
  increment, the read aggregations, and the AC-7 pricing numerator - a *not
  recorded* value contributes zero and MUST be coalesced explicitly. SQL's
  `NULL + n = NULL` would otherwise let one unrecorded sample null out an entire
  session's total (AC-12).
- Every token and cost column is 64-bit (`BIGINT`), never a 32-bit `INTEGER`.
  This is not defensive sizing: a single sampled turn reports 8,203,943
  cached-read tokens, and the merged per-session cached-input total has been
  measured at 98,805,109 on one already-completed task. On Postgres `INTEGER`
  is `int4` (ceiling 2,147,483,647), so a long-running session would abort the
  insert and take the whole transaction with it. SQLite's `INTEGER` is already
  64-bit; the widening exists for Postgres parity.
- `occurred_at` is the ingest instant in UTC, acquired **once, before the
  first insert attempt**, and reused byte-for-byte by every AC-32 retry of that
  event. It SHALL NOT be re-read on a later attempt: a retried event would
  otherwise carry a timestamp later than events that arrived after it, which
  breaks the `(occurred_at, id)` total order AC-16 promises and lets
  `first_event_at` / `last_event_at` move under contention rather than under
  real event arrival. It is deliberately NOT taken from the payload's
  `timestamp` field: that value is producer-clock-derived and would let a
  skewed or replayed producer reorder the ledger. `created_at` equals
  `occurred_at` for a first write; the two are separate columns so a future
  out-of-band import can carry a true event time without lying about when the
  row was recorded.
- `contract_version` starts at `1` for every row written by this feature. It is
  bumped only when a column's *meaning* changes, not when a column is added.
- The `rate_*` and `pricing_catalog_version` columns are populated only on the
  `models_dev_list` path. They are recorded so a computed cost can be
  re-derived and audited later; without them an `unpriced`-to-priced catalog
  change is indistinguishable from a pricing bug.
- `tokens_cached_read` / `tokens_cached_write` are stored separately, not
  pre-merged. Providers charge different rates for the two, and a merged value
  cannot be decomposed afterwards. The merged sum exists only on the rollup.

Foreign-key rationale (this deviates from the task brief's stated
`ON DELETE CASCADE from task_sessions`; see *Corrections to the task brief*):

- `task_id` cascades from `tasks`. Deleting a task deletes its spend record, so
  task deletion remains a complete deletion.
- `session_id` is `ON DELETE SET NULL`, matching
  `task_step_transitions.session_id` and its stated reason: the historical fact
  must survive deletion of the thing it referred to. A ledger row whose session
  was pruned still contributes to its task's total.
- `turn_id` carries no foreign key, for the same reason.

### `task_sessions` (existing columns, newly written in production)

`cost_subcents`, `tokens_in`, `tokens_cached_in`, `tokens_out` already exist on
`task_sessions` and are already incremented atomically by the Office
subscriber. This feature makes the production ledger writer the sole writer of
those four columns (AC-10) and surfaces them on the session model
(`models.TaskSession`) and on `dto.TaskSessionDTO`, which they are not today.

`dto.TaskSessionDTO` is the **only** DTO widened. The repo has a second,
`dto.TaskSessionSummaryDTO` - declared in the same file as "a lightweight
version of `TaskSessionDTO` without snapshot fields" - which backs the
`GET /tasks/:id/sessions` list projection
(`task/handlers/task_session_summary_projection.go`), the boot payload's
`Sessions` array, and the MCP `list_task_sessions` tool. It is deliberately
left unchanged, along with every projection built from it. Widening it would
add per-session cost to an agent-facing tool and to a list endpoint this card
never scoped, and the deliverable for querying cost is the HTTP read surface
below, not a field on a list row. Naming one DTO also settles which existing
API responses and frontend types change: exactly those carrying the full
`TaskSessionDTO`.

Three of the four are declared `INTEGER`; only `tokens_cached_in` is `BIGINT`
(`base_schema.go`, and `base_migrations.go`'s `migrateSessionsAddCostColumns`).
On Postgres `INTEGER` is `int4`, and that migration's own comment records the
consequence: an overflow "would abort the single multi-column UPDATE in
`IncrementTaskSessionUsage` ... silently taking
`tokens_in`/`tokens_out`/`cost_subcents` down with it for that session". Under
AC-11 that abort would now roll back the ledger insert too, so a derived column
would destroy the source of truth. AC-28 widens all three, and the boundary
scenarios below deliberately name columns that can actually overflow rather
than the one already safe.

## API surface

Both routes register beside the existing task routes in
`internal/task/handlers` and are available regardless of `features.office`.

```
GET /api/v1/tasks/:id/usage
GET /api/v1/tasks/:id/sessions/:sessionId/usage
```

Response body (identical shape for both scopes):

```json
{
  "scope": "task",
  "scope_id": "…",
  "tokens_in": 80,
  "tokens_cached_read": 8203943,
  "tokens_cached_write": 264177,
  "tokens_out": 44979,
  "tokens_thought": 0,
  "tokens_total": 8513179,
  "cost_subcents": 79118,
  "event_count": 1,
  "estimated_event_count": 0,
  "unpriced_event_count": 0,
  "output_tokens_complete": true,
  "first_event_at": "2026-08-23T04:15:58Z",
  "last_event_at":  "2026-08-23T04:15:58Z"
}
```

- `output_tokens_complete` is false when any contributing row has an unrecorded
  output count, marking `tokens_out` as a lower bound (AC-12, AC-19). It is
  `true` for a scope with no rows at all (AC-20).
- `tokens_total` is the sum of the stored per-row `tokens_total`, which the
  writer computed from the per-kind counts (AC-23). It is never recomputed at
  read time, and all five summed kinds are reported beside it, so it cannot
  disagree with them (AC-19).
- `tokens_thought` is reported because AC-23 includes it in `tokens_total`. It
  is zero for most models and non-zero for reasoning-heavy ones; it is never
  priced (AC-7).
- `estimated_event_count` and `unpriced_event_count` let a caller distinguish a
  confident total from a partial one without reading raw rows.
- Timestamps are null when `event_count` is 0.
- Response codes: `200` with zeroed totals for a known scope with no usage
  (AC-20); `404` when the task or session does not exist; `404` when the
  session exists but does not belong to the task in the path.

The task-scoped total sums every ledger row for the task, including rows whose
`session_id` has been set to NULL by session deletion. The session-scoped total
sums only rows still bound to that session.

No new WebSocket contract. The existing `session.prompt_usage` frame is
unchanged; this feature does not alter what the frontend receives today.

## State machine

A usage event has a single, terminal lifecycle. There is no draft, no pending,
and no reconciliation state.

| From | Trigger | To | Actor |
|---|---|---|---|
| (none) | usage event observed, all required identifiers present | `recorded` | ledger writer |
| (none) | usage event observed, identifier already present | `dropped:duplicate` | ledger writer |
| (none) | usage event observed, task identifier missing | `dropped:unattributable` | ledger writer |
| (none) | usage event observed, session identifier missing | `recorded` with null `session_id`, no rollup | ledger writer |
| (none) | usage event observed, turn identifier missing | `recorded` with null `turn_id` | ledger writer |
| (none) | usage event observed, a token or cost value is negative | `dropped:invalid` | ledger writer |
| (none) | usage event observed, invalid **and** session/task mismatched (AC-27, AC-33) | `dropped:invalid` | ledger writer |
| (none) | usage event decoded, worker queue full (AC-34) | `dropped:overflow` | ledger writer |
| (none) | usage event still buffered when the shutdown drain deadline expires (AC-34) | `dropped:drain_timeout` | ledger writer |
| (none) | usage event decoded, shutdown drain already begun (AC-34) | `dropped:shutdown` | ledger writer |
| (none) | usage event in flight when the shutdown drain deadline expires (AC-34) | `dropped:error`, transaction rolled back, nothing written after the drain returns | ledger writer |
| (none) | usage event observed, no task identifier **and** a negative value (AC-27) | `dropped:unattributable` | ledger writer |
| (none) | usage event observed, `usage_event_id` missing or empty | `dropped:invalid` | ledger writer |
| (none) | bus payload fails to decode | `dropped:decode_error` | ledger writer |
| (none) | usage event observed, its session row does not exist | `recorded` with null `session_id`, no rollup | ledger writer |
| (none) | usage event observed, its session row belongs to a different task (AC-33) | `recorded` with null `session_id`, no rollup | ledger writer |
| (none) | usage event redelivered with a randomly minted identifier (AC-22) | `recorded` as a new row, rollup applied | ledger writer |
| (none) | insert fails transiently (AC-32) | retried unchanged, up to three attempts; then `recorded` or `dropped:error` | ledger writer |
| (none) | insert fails on the `usage_event_id` unique constraint (AC-32) | `dropped:duplicate`, transaction rolled back, no rollup increment | ledger writer |
| (none) | insert fails on a foreign key (AC-32) | retried once with null `session_id`; then `recorded` without rollup, or `dropped:error` | ledger writer |
| (none) | insert or rollup fails for any other reason | `dropped:error`, transaction rolled back | ledger writer |
| `recorded` | task deleted | row removed by cascade | database |
| `recorded` | session deleted | `session_id` cleared, row retained | database |

A retry is not a state: only the outcome of the final attempt is a transition,
so a retried event still increments exactly one counter.

`recorded` has no outgoing transition that changes the row's contents (AC-3).

Every terminal transition above increments exactly one AC-27 counter: a
`recorded` row bumps `task_usage_events_written_total`, and each `dropped:<x>`
bumps `task_usage_events_dropped_total` under `reason=<x>`. The stage order is
AC-27's - decode, admit, validate, ownership, insert - so the redelivery of an
invalid event is counted `invalid` rather than `duplicate`, because it never
reaches the insert where uniqueness is enforced, and an event that is both
invalid and mismatched is counted `invalid` rather than recorded with a null
session. Within validate, the fixed sub-check order makes an event that is both
unattributable and negative count `unattributable`.

## Permissions

Usage totals are visible to any caller authorized to read the task itself; they
introduce no new authorization tier. Where `features.auth` is enabled, both
routes follow the same workspace-scoped rule as the sibling
`GET /api/v1/tasks/:id/sessions` route. No caller can write to the ledger over
HTTP; the only writer is the internal event subscriber.

## Failure modes

| Condition | Behaviour |
|---|---|
| Decoded payload carries no task id | Row dropped, counted `unattributable`, warning logged. There is nothing to attribute the spend to and `task_id` is `NOT NULL`. Distinct from "task row absent" below, where the id is present and the row is gone. Defensive only: no current producer can reach it, since `publishPromptUsage` sets `task_id` alongside the session id it has already required to be non-empty. |
| Decoded payload carries a task id but no session id | Row recorded with `session_id = NULL` and no rollup increment, reaching the same end state as "session row absent" below. The spend is kept; only per-session grouping is lost. Dropping it would contradict the same principle applied to a missing `turn_id` (AC-17) and to an absent session row. Also unreachable from the current producer, which returns early on an empty session id (AC-24). |
| Payload's `session_id` names a session belonging to a different task | Row recorded under the payload's `task_id` with `session_id = NULL` and no rollup increment, warning logged (AC-33). Same end state as an absent session row, by design. |
| `usage_event_id` already present | Row dropped, counted as `duplicate`. Not an error; no rollup increment. Redelivery is expected, not exceptional. |
| Rollup increment fails after ledger insert | Whole transaction rolls back. No row, no increment. A later redelivery of the same event retries cleanly instead of colliding with a half-landed row. |
| Session row absent when the rollup runs | The writer records the row with `session_id = NULL` and applies no rollup increment; that transaction contains the insert alone, which satisfies AC-11 vacuously. Task attribution and the spend record survive, and only per-session grouping is lost. This is the same end state as a session deleted afterwards (`ON DELETE SET NULL`), so the two are deliberately indistinguishable. The earlier form of this row - insert with a non-null `session_id` and let the rollup match zero rows - is impossible: the DSN sets `_foreign_keys=on` (`internal/db/sqlite.go`), so that insert is rejected outright rather than committing. |
| Session deleted between the writer's read and its commit | The insert violates the `session_id` foreign key, classified by `IsForeignKeyViolation` per AC-32. The writer retries exactly once with `session_id = NULL` and no rollup, reaching the row above. A second failure is counted `error`. |
| Insert fails with a transient error (SQLite `SQLITE_BUSY` / `SQLITE_LOCKED`, Postgres serialization failure or deadlock) | The whole insert-plus-rollup transaction is re-run in a fresh transaction per attempt, at most three attempts, waiting 50 ms then 100 ms, keeping `session_id` and the rollup increment (AC-32). Retrying the INSERT statement alone inside the aborted transaction would fail with `25P02` on Postgres instead of retrying. Only exhausted attempts are counted `error`. This is what makes the two-concurrent-events guarantee in *Concurrency* hold under real contention rather than only in its absence, and it is why a transient failure must never take the foreign-key path. |
| Task row absent or deleted mid-write | The `task_id` foreign key rejects the insert. AC-32's single foreign-key retry, which nulls `session_id`, is rejected for the same reason, so the row is dropped and counted `error`. A ledger row is never written without a task to attribute it to - and reaching that outcome needs no session-versus-task classifier, only the retry. |
| Pricing lookup unavailable, times out, or misses | Row recorded `unpriced` with zero cost (AC-8). Never blocks, never drops, never retries inline. "Times out" is bounded by the **writer's own 2-second context deadline**, not by the lookup's, whose shipped default is 30 seconds (AC-8, AC-26). Unbounded, the single serial worker stalls per event and the queue fills behind it, turning an outage that should produce `unpriced` rows into `dropped:overflow`. |
| Pricing lookup unwired (Office off today) | Same as a miss: `unpriced`. Wiring the lookup outside the Office gate is in scope and its target is named in AC-26; a missing lookup must degrade, not fail. |
| Provider reports a negative token or cost value | Row dropped, counted as `invalid`, warning logged. Negative values are never written; an append-only ledger has no correcting update. |
| Database unavailable | Insert fails and is not transient-retryable; event dropped and counted `error`. The agent turn is unaffected: usage recording never blocks or fails an agent run. |
| Worker queue full when an event is decoded | Event abandoned rather than waited on, counted `overflow`, warning logged with the task id (AC-34). Blocking instead would push backpressure onto the orchestrator's streaming goroutine, which is the one thing this writer may never do. |
| Process shuts down with events still buffered | The writer stops accepting, drains the buffer, and returns when it is empty or after 5 seconds (AC-34). Events still buffered at the deadline are counted `error` and logged. Bounded on purpose: an unbounded drain would hang shutdown behind a wedged database. |
| Event arrives after the shutdown drain has begun | Refused at the admit stage rather than buffered, counted `shutdown`, logged; the callback still returns immediately (AC-34). A reason distinct from `overflow`, so a clean restart is not read as sustained backpressure. |
| Event in flight when the drain deadline expires | Its context is cancelled, the transaction rolls back, it is counted `error`, and nothing is written after the drain returns (AC-34). Committing into a pool the process is closing is a use-after-close, not a late row. |
| Insert rejected by the `usage_event_id` unique constraint | Counted `duplicate`, not `error`; no retry, no rollup increment (AC-32, AC-14). Classified by a constraint-specific detector, so a unique violation raised by any other constraint takes the `error` path instead of being silently swallowed as expected behaviour. |
| Ledger writer stops silently | The AC-27 expvar counters `task_usage_events_written_total` and `task_usage_events_dropped_total` are exported at `/debug/vars`, so a stopped writer is observable rather than presenting as "this task was free". |
| Producer stops publishing, or a turn reports no usage object | Not observable through this feature, by construction: the event never reaches the subscriber, so neither AC-27 counter moves and no row is written. AC-24 states the publisher's real precondition and AC-27 scopes its counters to the subscriber. This bounds what the counters prove - they show the writer is alive, never that a task's recorded spend is complete. |

The overriding rule: **recording usage is never allowed to affect the agent
run.** Every failure above degrades the record, not the work. AC-34 is what
makes that rule structural rather than aspirational: because the bus delivers
to subscribers synchronously on the publisher's goroutine, the guarantee holds
only while the writer hands off and returns. Every retry, backoff and database
wait above happens on the writer's own worker.

## Persistence guarantees

- Ledger rows survive process restart, agent crash, session deletion, and
  Office being toggled on or off. They are removed only when their task is
  deleted.
- Rollup columns survive restart and are recomputable from the ledger at any
  time, because the ledger is the source of truth and the rollup is purely
  derived. This holds only once AC-29 removes
  `BackfillSessionTokensCachedIn`, which today reassigns `tokens_cached_in`
  from `office_cost_events` on every boot, ungated: with that code in place the
  guarantee is false in exactly the install this feature targets, and it fails
  on the second boot rather than the first, so any test that starts the backend
  once will not see it.
- Nothing about this feature is cached in memory across a restart. A restart
  mid-turn loses only usage events not yet observed; already-committed rows are
  unaffected.
- No retention policy, TTL, or compaction. The ledger grows monotonically with
  usage for the life of the task.
- Usage recorded before this feature ships is not backfilled. Historical turns
  carry `prompt_usage` in `task_session_turns.metadata`; that data is left where
  it is and is not migrated.

## Concurrency

- Two events for the same session arriving concurrently each take their own
  transaction. The rollup is applied as a relative increment
  (`SET col = col + ?`), never a read-modify-write, so concurrent increments
  compose and no update is lost.
- Two deliveries of the *same* event racing each other, where the producer
  minted a deterministic identifier: exactly one commits; the other fails the
  `uniq_task_usage_events_usage_event_id` constraint at insert and is dropped
  as a duplicate through AC-32's unique-violation branch (AC-14), not as an
  error. The rollup is incremented exactly once, because the loser's increment
  shares the loser's rolled-back transaction. This is the collision the
  constraint actually has to resolve: no pre-insert read can, since both
  deliveries may read before either writes (AC-32). Where the producer minted a random identifier (AC-22) the two
  deliveries carry different keys, so both commit and the rollup is incremented
  twice. That is the honest outcome of a producer contract this card does not
  change, not a race this card loses.
- A read of task totals concurrent with an insert returns either the
  pre-insert or the post-insert total, never a torn total, because the insert
  and its rollup share one transaction.
- Ordering under concurrency is by `(occurred_at, id)`; two rows with an
  identical `occurred_at` are ordered by the monotonic `id`, so the total order
  is deterministic and stable across repeated reads.

## Scenarios

Golden path

- **GIVEN** a Kanban-only installation with `features.office` disabled, **WHEN**
  an agent turn completes and reports usage, **THEN** a `task_usage_events` row
  exists for that turn and the session's `cost_subcents` is greater than zero.
- **GIVEN** a recorded usage event with a provider-reported cost, **WHEN** the
  row is read, **THEN** `cost_source` is `provider_reported` and
  `cost_subcents` equals the provider's amount, and no pricing lookup was
  performed.
- **GIVEN** a task with three recorded usage events, **WHEN**
  `GET /api/v1/tasks/:id/usage` is called, **THEN** the response
  `event_count` is 3 and `cost_subcents` equals the sum of the three rows.

Cache-heavy turn (the dominant real shape)

- **GIVEN** a usage event reporting `input_tokens=80`,
  `cached_read_tokens=8203943`, `cached_write_tokens=264177`,
  `output_tokens=44979`, **WHEN** the row is recorded, **THEN**
  `tokens_cached_read` and `tokens_cached_write` are stored separately and the
  session rollup's `tokens_cached_in` increases by 8,468,120.

Multiple events per turn

- **GIVEN** a turn that emits five usage events over fifteen minutes, **WHEN**
  all five are observed, **THEN** five ledger rows exist sharing one `turn_id`,
  and the per-turn total is their sum.
- **GIVEN** two ledger rows sharing `(session_id, turn_id)`, **WHEN** the ledger
  is written, **THEN** no uniqueness constraint is violated.
- **GIVEN** a usage event carrying no turn identifier, **WHEN** it is observed,
  **THEN** a row is recorded with a null `turn_id`, the session rollup is
  incremented, and the row is included in the task total.

Boundaries and ordering

- **GIVEN** a session whose cumulative **output** tokens exceed 2,147,483,647 -
  a column declared `INTEGER` before this feature - **WHEN** a further event is
  recorded, **THEN** the ledger row and the rollup increment both commit on
  Postgres as well as SQLite (AC-28). Run against the pre-widening schema on
  Postgres this scenario MUST fail; a version of it that passes before the
  migration is not testing the migration.
- **GIVEN** a session whose cumulative `cost_subcents` exceeds 2,147,483,647
  (about $214,748 of recorded spend), **WHEN** a further event is recorded,
  **THEN** both writes commit on Postgres, and the ledger row is not lost to a
  rollup-column overflow.
- **GIVEN** a session whose cumulative cached-input tokens exceed 2,147,483,647,
  **WHEN** a further event is recorded, **THEN** the insert and the rollup both
  succeed on Postgres as well as SQLite.
- **GIVEN** two ledger rows with an identical `occurred_at`, **WHEN** the ledger
  is read twice, **THEN** both reads return them in the same order.
- **GIVEN** two usage events for the same session processed concurrently,
  **WHEN** both commit, **THEN** the session rollup reflects both, with neither
  increment lost.

Idempotency

- **GIVEN** a usage event already recorded whose identifier was derived
  deterministically from `(session_id, execution_id, prompt_generation)`,
  **WHEN** the identical event is redelivered, **THEN** no second row is
  written, the session rollup is unchanged, and the drop is counted as a
  duplicate rather than surfaced as an error.
- **GIVEN** a usage event published with no `execution_id` or no
  `prompt_generation`, so `usageEventIDFor` minted a random identifier, **WHEN**
  the identical event is redelivered, **THEN** a second row IS written with a
  different identifier and the rollup IS incremented twice (AC-22). This is the
  documented behaviour of the producer contract, and a test asserting the
  opposite is asserting something production does not do.
- **GIVEN** a usage event arriving with an empty `usage_event_id`, **WHEN** it
  is observed, **THEN** no row is written and the drop is counted as invalid.
- **GIVEN** an event that is both a redelivery and carries a negative token
  count, **WHEN** it is observed, **THEN** the drop is counted as `invalid`,
  not `duplicate` (AC-27).
- **GIVEN** two deliveries of the same event bearing the same deterministic
  identifier, whose inserts are made to overlap so both pass any pre-insert
  check and race at the constraint, **WHEN** both are processed, **THEN**
  exactly one row exists, the rollup was incremented exactly once, and the
  loser is counted `duplicate` and not `error` (AC-13, AC-14, AC-32,
  *Concurrency*). Only a genuine insert-time collision exercises AC-32's
  unique-violation branch; a redelivery arriving after the first has committed
  is served by any pre-insert read and proves nothing about it.
- **GIVEN** a unique violation raised by a constraint other than
  `uniq_task_usage_events_usage_event_id` - on Postgres a `23505` naming a
  different constraint, on SQLite a `UNIQUE constraint failed:` message naming
  a different column - **WHEN** it is classified, **THEN** it is NOT counted
  `duplicate` and takes AC-32's "anything else" branch. A detector matching a
  bare `23505`, or the bare substring `UNIQUE constraint failed`, passes the
  scenario above and fails this one, which is why the prior contract row in
  `docs/specs/tasks/external-id-idempotency/spec.md` exists.
- **GIVEN** an event that carries no `task_id` **and** a negative token count,
  **WHEN** it is validated, **THEN** the drop is counted `unattributable`, not
  `invalid` (AC-27). The validate stage's sub-checks share labels with each
  other except for this pairing, so it is the only one whose ordering is
  observable.

Estimation and pricing

- **GIVEN** a usage event with no provider-reported cost and a model absent from
  the pricing catalogue, **WHEN** the row is recorded, **THEN**
  `cost_source` is `unpriced`, `cost_subcents` is 0, and the token counts are
  still recorded.
- **GIVEN** a task whose events include one `unpriced` row, **WHEN** task totals
  are read, **THEN** `unpriced_event_count` is 1, so the caller can see the
  total understates actual spend.
- **GIVEN** a usage event whose adapter could not observe output tokens,
  **WHEN** task totals are read, **THEN** `output_tokens_complete` is false.
- **GIVEN** a provider that explicitly reports a cost of zero with its presence
  flag set, **WHEN** the row is recorded, **THEN** `cost_source` is
  `provider_reported` with `cost_subcents` 0, and no pricing estimate replaces
  it.

Deletion and durability

- **GIVEN** a task with recorded usage, **WHEN** its session is deleted,
  **THEN** the ledger rows remain, their `session_id` is null, and the task's
  total is unchanged.
- **GIVEN** a task with recorded usage, **WHEN** the task is deleted, **THEN**
  its ledger rows are removed.
- **GIVEN** recorded usage in an installation with `features.office` disabled,
  **WHEN** the backend restarts **twice**, **THEN** every row and every rollup
  value is unchanged after each boot, `tokens_cached_in` included (AC-29). One
  restart is not enough: the defect this covers is a boot-time reassignment, so
  a test that starts the backend once passes with the bug present. Run this
  against a build that still calls `BackfillSessionTokensCachedIn` and it MUST
  fail with `tokens_cached_in` at zero; a version that passes before the removal
  is not testing the removal.

Failure and degradation

- **GIVEN** a usage event with an empty task id, **WHEN** it is observed,
  **THEN** no row is written and the drop is counted as unattributable.
- **GIVEN** a usage event reporting a negative output-token count, **WHEN** it
  is observed, **THEN** no row is written and the drop is counted as invalid.
- **GIVEN** the rollup increment fails, **WHEN** the event is processed,
  **THEN** no ledger row is committed for that event.
- **GIVEN** the pricing lookup is not wired, **WHEN** an event without a
  provider-reported cost is observed, **THEN** the row is recorded `unpriced`
  and the agent turn completes normally.
- **GIVEN** an insert that fails with a transient contention error twice and
  succeeds on the third attempt, **WHEN** the event is processed, **THEN** the
  row is recorded with its `session_id` intact and its rollup applied, and no
  drop is counted (AC-32).
- **GIVEN** an insert that fails with a transient contention error on every
  attempt, **WHEN** the event is processed, **THEN** no row is written, exactly
  one `reason=error` drop is counted, and no row was ever written with a nulled
  `session_id` (AC-32). A transient failure taking the foreign-key path is the
  specific regression this covers.
- **GIVEN** an insert that fails because the task row is gone, **WHEN** the
  event is processed, **THEN** the single foreign-key retry fails for the same
  reason, no row is written, and exactly one `reason=error` drop is counted
  (AC-32).
- **GIVEN** an agent turn that reports no usage object, **WHEN** the turn
  completes, **THEN** no event is published, no row is written, and neither
  AC-27 counter changes. This is the specified outcome, not a defect: the
  producer's precondition is stated in AC-24 and the counters are scoped to the
  subscriber in AC-27.

Read surface

- **GIVEN** a task that has never run an agent, **WHEN** its usage is read,
  **THEN** the response is `200` with all totals zero, `event_count` 0, and
  null timestamps.
- **GIVEN** a session id that belongs to a different task, **WHEN** it is read
  under this task's path, **THEN** the response is `404`.

Session attribution and durability under a missing session

- **GIVEN** a usage event whose `session_id` names a session row that does not
  exist, **WHEN** it is observed, **THEN** a ledger row is recorded with a null
  `session_id`, no rollup increment is applied, and the row still counts toward
  the task total.
- **GIVEN** a usage event whose `session_id` names a session belonging to a
  different task, **WHEN** it is observed, **THEN** a row is recorded under the
  payload's `task_id` with a null `session_id`, no rollup increment is applied
  to either session, and a warning is logged (AC-33).
- **GIVEN** a usage event carrying a task id and an empty session id, **WHEN**
  it is observed, **THEN** a row is recorded with a null `session_id` and no
  rollup, rather than dropped as unattributable. Only a missing **task** id is
  unattributable.

Aggregation and observability

- **GIVEN** a row whose `tokens_out` is *not recorded* (NULL) and whose other
  counts are present, **WHEN** the session rollup and the task totals are read,
  **THEN** the NULL contributes zero, neither total is null, and
  `output_tokens_complete` is false (AC-12, AC-19). This is the reachable NULL
  path: the wire carries a presence bit for output tokens only (AC-30).
- **GIVEN** a row inserted directly with `tokens_cached_read` NULL - which the
  current producer cannot generate, so this is forward-compatibility cover for a
  transport that later carries presence bits (AC-30) - **WHEN** the **task and
  session total reads** are performed, **THEN** the NULL contributes zero and
  neither total is null (AC-12). The scope is the read aggregation only: a row
  inserted directly runs no rollup increment, so this scenario cannot assert
  anything about the increment's coalesce, and a test written as though it
  could would pass against a writer whose increment path has no coalesce at
  all. The increment side of AC-12 is covered on the reachable NULL path,
  through `tokens_out`, in the scenario above.
- **GIVEN** a usage event whose payload omits `cached_read_tokens` entirely,
  **WHEN** it is observed, **THEN** the row records `0`, not NULL, because the
  wire cannot distinguish absence from a measured zero for that field (AC-30).
- **GIVEN** a task with rows whose per-kind counts are known **and at least one
  row reporting non-zero `tokens_thought`**, **WHEN** totals are read, **THEN**
  `tokens_total` equals `tokens_in + tokens_cached_read + tokens_cached_write +
  tokens_out + tokens_thought` as reported in that same response, with all five
  present in the body (AC-19, AC-23). A fixture whose thought tokens are all
  zero passes even when the response omits the field, so it does not test this.
- **GIVEN** a usage event that is dropped as unattributable, **WHEN**
  `/debug/vars` is read, **THEN** `task_usage_events_dropped_total` carries a
  `reason=unattributable` entry, and a successfully recorded event increments
  `task_usage_events_written_total` under its `source` and `provider` (AC-27).
- **GIVEN** a usage event whose `agent_type` is `claude-acp` and whose `model`
  is the alias `sonnet`, **WHEN** it is observed, **THEN** the row records
  `provider` as `anthropic`, and the written counter carries
  `provider=anthropic` (AC-31). A model-prefix match alone resolves this to the
  empty string, so a fixture using a full model name would not detect the loss
  of the CLI tiers.
- **GIVEN** an installation with `features.office` enabled and one usage event,
  **WHEN** both the ledger writer and the Office subscriber derive a provider
  for it, **THEN** the two values are identical, because both call the helper
  extracted by AC-31.
- **GIVEN** the pricing catalogue is refreshed between two lookups, **WHEN** an
  event is priced, **THEN** the row's `rate_*_per_million` values and its
  `pricing_catalog_version` come from the same snapshot, so the recorded cost
  is reproducible from the recorded rates (AC-26). A writer that reads rates
  and version through two separate calls fails this.

Execution model, identity, and ordering

- **GIVEN** the ledger writer's database is made to block on insert, **WHEN** a
  prompt-usage event is published on the bus, **THEN** `Publish` returns
  without waiting for the write (AC-34). A handler that does its work on the
  callback goroutine fails this, and it is the only assertion that distinguishes
  the two designs: both record the same row eventually.
- **GIVEN** a worker queue already holding its 1024-event capacity, **WHEN**
  another event arrives, **THEN** it is abandoned rather than waited on and
  `task_usage_events_dropped_total` carries a `reason=overflow` entry
  (AC-34, AC-27).
- **GIVEN** events buffered but not yet written, **WHEN** the process shuts
  down, **THEN** the buffered events are drained and recorded, and shutdown
  returns within the 5-second deadline even if the database never responds
  (AC-34). Asserting only that buffered events are recorded would pass against
  an unbounded drain that hangs shutdown.
- **GIVEN** the writer's drain has begun, **WHEN** the bus publishes another
  prompt-usage event, **THEN** the callback returns without blocking, no row is
  written, and `task_usage_events_dropped_total` carries a `reason=shutdown`
  entry distinct from `reason=overflow` (AC-34, AC-27).
- **GIVEN** an event in flight against a database that never responds, **WHEN**
  the 5-second drain deadline expires, **THEN** the drain returns, that event
  is counted `dropped:error`, and no ledger row or rollup increment appears
  afterwards (AC-34). A writer that lets the in-flight write continue past the
  deadline passes the buffered-drain scenario above and fails this one.
- **GIVEN** a pricing lookup that never returns, **WHEN** an event is priced,
  **THEN** the row is recorded `unpriced` with zero cost within the writer's
  2-second deadline and the queue does not reach its 1024-event capacity (AC-8,
  AC-26, AC-34). Asserting only that the row is `unpriced` passes against a
  writer that waits out the lookup's own 30 seconds.
- **GIVEN** two usage events published in a known order, **WHEN**
  `ListTaskUsageEvents` is called, **THEN** they are returned in that same
  order, and a repeated call returns an identical sequence (AC-16, AC-34). A
  goroutine-per-event writer can fail this intermittently, so the fixture SHALL
  publish enough events to make an ordering inversion observable rather than
  relying on two.
- **GIVEN** a task with no ledger rows, and separately a task id that does not
  exist, **WHEN** `ListTaskUsageEvents` is called for each, **THEN** both
  return an empty slice and a nil error (AC-16).
- **GIVEN** an insert that fails transiently twice and succeeds on the third
  attempt, **WHEN** the event is recorded, **THEN** the row's `occurred_at` is
  the instant acquired before the first attempt, not the third (AC-32, AC-16).
  A fixture whose retries complete inside the timestamp's resolution cannot
  observe this and SHALL force a measurable gap.
- **GIVEN** a usage event carrying a non-empty `model`, `agent_type` and
  `agent_profile_id`, **WHEN** it is recorded, **THEN** all three appear on the
  row with those values (AC-35). A writer that defaults all three to `''`
  passes every other criterion, so this is the only assertion that catches it.
- **GIVEN** a usage event that is both invalid and whose session belongs to a
  different task, **WHEN** it is processed, **THEN** it is dropped as `invalid`
  with no row written and no ownership lookup performed (AC-27, AC-33). An
  implementation that checks ownership first records a null-session row
  instead, which is a different observable outcome.

Office coexistence

- **GIVEN** an installation with `features.office` enabled, **WHEN** an agent
  turn reports usage, **THEN** the session rollup reflects that turn exactly
  once, not twice.
- **GIVEN** an installation with `features.office` enabled, **WHEN** a turn
  reports usage, **THEN** both an `office_cost_events` row and a
  `task_usage_events` row exist for it, and the `task_sessions` rollup was
  incremented by the ledger writer only (AC-21). Asserting the rollup value
  alone is not sufficient: an Office subscriber that still increments and a
  ledger writer that does not would produce the same number.

## Corrections to the task brief

Recorded because they change the contract, and each is backed by evidence taken
from current code and a live database (503 sessions / 5,664 turns / 1,962
existing cost rows).

1. **"Nothing persists token usage per task run outside Office" is only half
   true.** `internal/orchestrator/event_handlers_streaming.go`
   (`persistPromptMetadataOnTurn`) already writes a `prompt_usage` object into
   `task_session_turns.metadata` on every turn, unconditionally — 839 turns
   carry it in the sampled database. What is missing in production is *cost*
   and *queryability*, not token capture. The gap this spec closes is a
   queryable, priced, append-only record.

2. **`(session_id, turn_id)` is not unique and cannot be the ledger key.** In
   the sampled ledger, 811 turns have one event, 4 have two, 1 has three, and 1
   has five — a single turn spanning 10:33 to 10:48 with five distinct
   provider-reported costs. This follows from the design: the idempotency key is
   derived from `(session, execution, prompt_generation)`, and one turn contains
   many prompt generations. The unique key is `usage_event_id` (827 rows, 827
   distinct values); `(session_id, turn_id)` is an aggregation index (AC-13,
   AC-15).

3. **`FK ON DELETE CASCADE from task_sessions` would defeat the durability
   requirement.** The brief asks for an append-only durable record and for a
   cascade that erases it when a session is pruned. This spec follows the
   repository's own append-only-log precedent instead:
   `task_step_transitions.session_id` is `ON DELETE SET NULL`, explicitly
   because "the historical fact ... must survive that deletion". The live
   `office_cost_events` table already holds 110 rows whose session no longer
   exists. Cascade is kept on `task_id`, so task deletion is still complete.

4. **`tokens_in` / `tokens_out` alone would misreport real turns by orders of
   magnitude.** A sampled turn reports `input_tokens=80` against
   `cached_read_tokens=8,203,943` and a provider-reported cost of $7.91. Cache
   token classes are first-class ledger columns, stored unmerged.

5. **`cost_usd` is not stored.** The established unit across
   `office_cost_events`, `task_sessions`, and the adapter wire type is integer
   subcents. Introducing a float dollar column would make sums
   non-associative and would disagree with the columns it must reconcile
   against.

6. **`internal/agent/usage` is confirmed unrelated**, as the brief states: it
   probes provider plan quota (Claude OAuth usage endpoint, Codex auth file) and
   is specified by [costs/subscription-usage](../costs/subscription-usage.md).
   It is untouched here.

7. **Scope note, per the brief's instruction to flag growth.** The single
   genuinely larger option is converging this ledger with Office's
   `office_cost_events` into one table. That is deliberately *not* attempted
   here (see *Out of scope*); the no-double-count rule (AC-10, and the Office
   coexistence scenario) bounds the cost of deferring it. Everything else fits
   the brief's stated scope.

## Out of scope

- **Converging with `office_cost_events`.** Office keeps its own ledger, budget
  policies, and cost explorer unchanged. Migrating Office's eight aggregation
  queries and 1,962 existing rows onto this table is a follow-up.

  This card **does** make one narrow amendment to
  [office/costs](../office/costs.md), and Build must land it in the same change.
  A spec cannot remove a writer while leaving another document asserting that
  writer in place, so the passages there stating that the Office subscriber
  maintains the `task_sessions` rollup are superseded by AC-10 and AC-21: the
  sentence that `cost_subcents`, `tokens_in`, `tokens_out` and
  `tokens_cached_in` are incrementally updated as cost events arrive; the
  `recordCostEventAndRollup` atomicity paragraph; the `TaskSession ... running
  totals` line; and the redelivery scenario that asserts a rollup increment.
  Everything else in that document - Office's own `office_cost_events`
  contract, its budgets, and the two-layer cost model this spec reuses - is
  unchanged and remains frozen.
- **Making generation-less transports deduplicable.** `usageEventIDFor` mints a
  random identifier when `execution_id` or `prompt_generation` is absent, and
  those rows are not deduplicated (AC-22). Changing that minting rule would
  change the orchestrator's published contract and a behaviour
  [office/costs](../office/costs.md) documents as intentional, so it is a
  follow-up rather than part of this card. The consequence is named in AC-22 and
  in the idempotency scenarios rather than left for a reader to discover.
- **Widening the prompt-usage publisher's precondition.** `publishPromptUsage`
  returns early when the session id is empty, when the event bus is nil, or when
  the usage object is nil, so a turn reporting no usage is never published and
  is invisible to this ledger (AC-24). Publishing those turns would change the
  orchestrator's published contract, which this card does not touch, and it
  would mean deciding what a usage-less turn should record - a question with no
  answer the ledger can supply. The consequence is named in AC-24, in AC-27's
  scoping of its counters, in the failure-modes table, and in a scenario, rather
  than left for a reader to discover.
- **A dedicated boot-time reconciliation of the rollup against the ledger.**
  AC-29 removes the one that exists because it destroys data; it does not
  replace it. The rollup is maintained transactionally on every write (AC-11)
  and is recomputable from the ledger on demand, so a periodic or startup
  reconciler has nothing to correct and would be a third writer again.
- **Backfilling or recomputing the rollup** for sessions written before AC-21
  removes Office's increment. Existing values stay as they are; the ledger is
  authoritative from its first row forward, and AC-12's equality is a forward
  guarantee, not a claim about pre-existing rows.
- **Budgets, limits, or enforcement.** This ledger records; it does not cap,
  warn, throttle, or block. Per-period budgets remain an Office feature.
- **A cost UI.** No dashboard, chart, or task-detail cost panel. The frontend
  store already has an unrendered `promptUsage` slice; this feature does not
  render it. The deliverable is the HTTP read surface.
- **Backfilling historical usage** from `task_session_turns.metadata` or from
  `office_cost_events`.
- **Subscription-plan spend.** Flat-fee plans report no per-token cost; those
  rows record tokens with `cost_source = unpriced`. Utilization tracking is
  [costs/subscription-usage](../costs/subscription-usage.md).
- **Per-turn or per-task cost attribution across sub-agents.** Sub-agent turns
  are attributed to the session that spawned them, as today.
- **Currency other than USD**, currency conversion, and any billing, invoicing,
  or payment integration.
- **Retention, archival, or compaction** of ledger rows.
- **Listing raw ledger rows over HTTP**, and any pagination or cursor contract
  for them. The read surface returns totals only; raw rows are reachable from
  the database and from the internal repository, not from the API.
- **Widening `dto.TaskSessionSummaryDTO`** or any projection built from it -
  the `GET /tasks/:id/sessions` list, the boot payload's `Sessions` array, and
  the MCP `list_task_sessions` tool. Only the full `dto.TaskSessionDTO` gains
  the four rollup fields. Adding per-session cost to a list row and to an
  agent-facing tool is a surface decision this card did not scope, and the
  deliverable for querying cost is the HTTP read surface.
- **Changing when `orchestratorSvc.SetModelInfoLookup` runs.** AC-26 moves the
  models.dev *construction* out of the `features.office` gate and nothing else;
  that setter, which feeds the orchestrator's context-window fallback, stays
  gated exactly as today. Ungating context-window fallback for every install is
  a separate behaviour change with its own review.
- **Tuning the AC-32 backoff or the AC-34 queue and drain constants** at
  runtime. The values are fixed in code: 50 ms/100 ms backoff, a 1024-event
  buffer, a 5-second drain deadline. Making them configurable adds a
  configuration surface before any evidence exists that the defaults are wrong.
- **Aggregation surfaces beyond task and session scope** — by model, by
  provider, by agent profile, by period. The columns support them; the routes
  are not part of this iteration.
