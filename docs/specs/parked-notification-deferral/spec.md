---
status: draft
created: 2026-08-09
updated: 2026-08-09
owner: kandev
---

# Parked-Session Notification Deferral

> ## Provenance — READ THIS FIRST
>
> **This spec was cut out of [`docs/specs/disambiguate-waiting/spec.md`](../disambiguate-waiting/spec.md)
> on 2026-08-09**, at Spec Review's round-4 cap, by human decision (option (c)):
> *"can u split into 2 task with 2 spec first … and we can review them independently."*
>
> **It has NOT been reviewed as a standalone contract.** Its material survived four review
> rounds *as part of a larger spec*, which is not the same thing: the surrounding sections it
> leaned on now live in a different file, and no reviewer has yet read this document on its own
> terms. Expect a first Spec Review round to find real defects, and do not treat the parent's
> review history as covering it.
>
> **No Kandev card exists for this spec.** Creating one is the human's decision, per the
> workflow's option (c). When they do, it enters at Spec Review with a fresh cap — which is the
> main thing the split buys.
>
> **It DEPENDS ON `disambiguate-waiting`, which must ship first.** Every term this spec uses —
> *parked*, `observed_detached`, the probe and its `live | settled | unknown` tri-state, the
> three-term conjunction, the sampling loop, `KANDEV_PARKED_PROBE_BUDGET`,
> `KANDEV_PARKED_PROBE_INTERVAL` — is defined there and is **not** redefined here. This spec
> adds exactly one thing: it makes the operator ping wait while a machine is still working.
>
> **Acceptance-criterion identifiers are deliberately not renumbered.** AC-28…AC-67 keep the ids
> they carried in the combined spec, so four rounds of review findings still resolve against
> them. Gaps are where the parent's ACs stayed. New criteria continue from **AC-83**
> (`disambiguate-waiting` used AC-73…AC-82).
>
> **Determinism decision `D4` keeps its identifier** from the combined spec. The parent left D4
> vacant rather than reusing the number, so a citation to "D4" is unambiguous across both files.

## Why

`disambiguate-waiting` makes a parked session *visible*. It deliberately changes nothing about
*notifications*: a session parked on a background shell still fires `session.turn_finished` at
turn end, exactly as today, and its AC-76 is the guard on that.

That leaves the originating ticket's third acceptance bullet unmet — **"a task parked on a
background shell does not ping the operator."** Row 3 of the ticket's table is the damaging one
precisely because the ping is indistinguishable from one that needs a human, and an operator who
learns the ping is unreliable stops reading it.

This spec closes that bullet, and only that bullet.

## The one live observation this design is calibrated against

Recorded in the parent as §J, reproduced here because it is the sole empirical input to *this*
spec's policy and a reader of this file alone would otherwise not have it.

Observed on a running instance on 2026-08-08: a Kandev Verify step parked on a background e2e
shell held `WAITING_FOR_INPUT` for **≈12 minutes** with a live process tree throughout, then
**self-resumed** with no operator input.

**Self-resume is unreliable rather than absent.** The agent did come back — so permanent
suppression is wrong, because sometimes it will not. But it usually will — so notifying at turn
end is also wrong. **Withhold-then-deliver-on-a-transition is the only policy consistent with
both**, and that is what this spec specifies.

It also calibrates the orphan backstop against the right quantity. A window sized to self-resume
latency would fire at, say, five minutes and ping an operator about a task that was about to
resume itself. The backstop is a bound on a *leak*, not on a self-resume, and must be sized
against how long legitimate background work runs — a full e2e suite is tens of minutes.

## Requirements

- While a session is **parked** (as `disambiguate-waiting` defines it), the
  `session.turn_finished` notification for the turn that parked it SHALL be **withheld**.
- A withheld notification SHALL NOT be lost. When the session stops being parked while still
  settled and unanswered, the withheld notification SHALL be **delivered then**.
- A withheld notification SHALL be **dropped**, not delivered, when the agent resumes itself or
  the operator acts — in both cases nobody needs telling.
- Delivery SHALL be **at most once** per turn, however the exits race.
- Every indeterminate input SHALL fail toward **delivering**: a false ping costs one
  notification, a withheld-and-never-delivered notification costs a card nobody is told about.
- This path SHALL be independent of `features.claudeBackgroundPromptHandoff`, which remains off
  and is neither read nor weakened.
- The **projection** SHALL be untouched. This spec adds no term to the parent's three-term
  conjunction, publishes no new carrier, and changes no icon.

## Why the protocol cannot supply this

Restated from the parent because it is the justification for building anything here at all.

ACP has no concept of "turn over, work continues, I will come back." `StopReason` is a closed
five-member enum — `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`
(`types_gen.go:6631-6637`). Sweeping the vendored schema at version `1.20.0` for the terms that
would express one returns **zero occurrences in both tiers** (`background`, `async`, `detach`,
`parked`, `long_running`). The only `resume` is `session/resume`, which is client-initiated.

`_meta.claudeCode.toolResponse.status: "async_launched"`
(`internal/agentctl/types/streams/tool_payload.go:296-302`) appears nowhere in the bridge's
`dist/` — it is opaque passthrough from the Claude Code CLI, not a bridge contract. **No
behaviour in this spec may depend on it.**

## Data model

No schema migration. The parent's `parkedState` record (backend process memory, per session)
gains two fields, and nothing else in the system changes:

```
parkedState  (extended by this spec)
  …                            all fields defined in disambiguate-waiting
  deferred_turn_id      string     nullable; the withheld turn_finished occurrence
  deferred_since        timestamp  nullable
```

Both are process-local, never persisted, and gone after a backend restart — the parent's
*Persistence guarantees* already covers the rest of the record and applies to these too. After a
restart a previously withheld notification is **dropped**, not delivered: the turn it belonged to
is over, the projection is not reconstructed, and re-deriving it would ping about work nobody can
still observe. That is the conservative direction here because the alternative is a ping with no
context behind it.

## API surface

**None.** This spec adds no field to any DTO, no WebSocket action, and no HTTP route. It changes
*when* an existing notification is dispatched and nothing about its content. That is deliberate:
every carrier this feature could have needed already exists because the parent added it.

## State machine

`TaskSessionState` is unchanged, and so is the parent's projection table. The only state machine
here is the deferred notification's.

| Exit | From | Trigger | To |
|---|---|---|---|
| **(a)** | withheld | probe transitions `live → settled` while the session is still settled and unanswered, **and the settle-confirmation window then elapses with that still true** | delivered, keyed on the original turn ID |
| **(b)** | withheld | the orphan backstop window elapses | delivered, keyed on the original turn ID |
| **(c)** | withheld | the session leaves `WAITING_FOR_INPUT` (the agent resumed itself, or a queued prompt was admitted) | dropped |
| **(d)** | withheld | the operator submits a prompt — **admitted or merely queued** — or the session is stopped or deleted | dropped |
| **(e)** | withheld | probe transitions `live → unknown` while the session is still settled and unanswered | delivered, keyed on the original turn ID |

**"Still settled and unanswered" — defined once**, because exits (a) and (e) both guard on it and
it visibly overlaps exit (d). A session is *settled and unanswered* when ALL of the following hold
at the instant the guard is evaluated:

- its `TaskSessionState` is `WAITING_FOR_INPUT`;
- no operator prompt for it has been **admitted or queued** since the turn that parked it settled
  — a queued-but-undelivered prompt counts as **answered**, because the operator has already acted
  and does not need to be told to;
- it has not been stopped or deleted, and its execution has not ended.

Any of these going false is exactly exit (c) or exit (d).

### The deliberate divergence from the projection — a queued prompt

A queued-but-unadmitted operator prompt **drops the deferral here** (exit (d)) but **does not
clear the parked projection** in the parent (parent AC-75). The two specs are answering different
questions and must not be made to agree:

| | question | a queued prompt answers |
|---|---|---|
| the projection (parent) | *is a machine still working?* | nothing — the workload is still alive |
| the deferral (this spec) | *does a human still need to be told?* | no — the human has already acted |

So after a queued prompt the card keeps showing the background affordance while the withheld
notification is discarded. That is correct on both sides: the operator can see work is still
running, and does not get pinged about a turn they have already responded to. An implementation
that unifies the two conditions breaks one of them. AC-86 asserts the pair together, across both
specs, so the divergence is pinned rather than rediscovered.

### The settle-confirmation window on exit (a) — why it exists

§J's common case is that background work finishes and *the agent resumes itself in response*.
Both events arrive close together and in an order Kandev does not control. Without a rule, that
race decides whether an operator is pinged about a task that is already running again, and two
acceptance criteria describing the same physical sequence — AC-29 (deliver) and AC-30a (do not) —
would be settled by a coin flip in CI.

So exit (a) does not deliver on the observation. It **arms** on the `live → settled` transition
and delivers only when `KANDEV_PARKED_SETTLE_CONFIRM` elapses with the session *still* settled and
unanswered. A self-resume inside the window is exit (c) and drops the deferral. AC-66 pins both
halves in one criterion.

**The window holds the NOTIFICATION only, never the PROJECTION.** At the `live → settled` sample
the parent's projection un-parks immediately — `last_sample` is no longer `live`, so `parked` is
false (parent AC-25), the carriers publish, and the task row stops rendering the background icon
(parent AC-62). The notification is what waits. This sentence's absence in an earlier revision was
a contradiction that took a whole review round to find; it is stated here and in the parent both.

The window applies to exit **(a) only**. Exit (e) is not held, because `unknown` means Kandev lost
the ability to observe rather than that work completed — there is no completion event for the
agent to react to, so there is no race to resolve, and holding an already indeterminate result
would only delay the fail-safe direction. Exit (b) is not held either; it is already a long
backstop.

### Why exit (e) exists

The probe is a tri-state, so from `live` it can move to `settled` *or* to `unknown` — agentctl
crashed or disconnected, the budget elapsed, the platform stopped enumerating. Under (a)–(d) alone
that transition matches nothing: the session un-parks, but (a) needs `settled` rather than
`unknown`, and the session is still `WAITING_FOR_INPUT` so (c) never fires. The withheld
notification would rest entirely on the backstop, which defaults to never, and would be lost
permanently. Treating `unknown` as "deliver" is the same fail-safe direction the rest of this spec
takes: it costs at most one notification a self-resuming agent did not strictly need, where the
alternative costs a card nobody is told about.

### The withhold happens BEFORE the occurrence is recorded, and this ordering is load-bearing

Delivery is keyed on the original turn ID, and the existing per-provider occurrence dedup
(`docs/specs/platform/notifications.md`) is what guarantees at most one delivery however exits
race. But if an implementation records the occurrence first and suppresses only the *delivery*,
the deferred delivery minutes later is deduped away as a repeat of an occurrence that was never
actually sent, and **no operator is ever pinged** — every delivering exit becomes a no-op. AC-28
cannot distinguish the two implementations.

So, explicitly: for a withheld turn, **no occurrence is recorded at withhold time**. The occurrence
is recorded at *delivery* time by whichever exit delivers, and its absence until then is what
leaves the deferred delivery reachable. AC-63 asserts the end-to-end consequence, which is the only
assertion the wrong ordering fails.

Exits (a), (b) and (e) are all deliveries keyed on the same original turn ID. Delivery is
**single-flighted** so exactly one wins; at the default backstop, (b) is out of the picture and (a)
and (e) are mutually exclusive, since one sample resolves to exactly one of `settled` or `unknown`.

## Timing and configuration

Two durations govern this spec. The parent owns `KANDEV_PARKED_PROBE_BUDGET` and
`KANDEV_PARKED_PROBE_INTERVAL`; this spec reads neither directly but both bound its behaviour, as
noted below.

| Constant | Env var | Default | **Read by** | `0` means |
|---|---|---|---|---|
| **Orphan backstop window** — exit (b) | `KANDEV_PARKED_DEFER_BACKSTOP` | `0` (never) | **backend** | exit (b) never fires; the deferral rests on exits (a)/(c)/(d)/(e) |
| **Settle-confirmation window** — the hold on exit (a) | `KANDEV_PARKED_SETTLE_CONFIRM` | `10s` | **backend** | no hold; exit (a) delivers on observation |

They follow the existing `getEnvDuration("KANDEV_ACP_IDLE_TIMEOUT", time.Hour)` convention
(`internal/agentctl/server/config/config.go:285`), whose doc comment already establishes the
"`0` disables" idiom. Both are plain env-sourced config, **not** runtime feature toggles: they are
operational tuning, not a release gate, so they do not go in `runtimeflags/registry.go`. The
"Read by" column exists because both are read by the **backend** while the convention they follow
lives in agentctl's config, and an operator who sets a backend key in agentctl's environment in a
Docker/SSH/Sprites deployment would have it silently ignored.

Note that the parent's `KANDEV_PARKED_PROBE_BUDGET` deviates from the "`0` disables" idiom and
rejects non-positive values (parent AC-81). These two keys do **not** deviate — `0` means what the
table says.

Defaults chosen, with the reasoning recorded so it is reviewable rather than arbitrary:

- **Backstop `0` = never, by default.** §J bounds any non-zero window from below at ≈12 minutes, so
  a short window would reintroduce the exact false ping this feature removes. Exit (e) closes the
  gap that would otherwise make a backstop load-bearing: a probe that stops answering `live` for
  *any* reason releases the deferral. The knob exists for operators who want a belt-and-braces
  bound, set well above their longest legitimate background job.
- **Settle-confirmation `10s`.** Long enough to cover the gap between a workload finishing and the
  agent re-invoking itself in response (§J observed the self-resume, though not its latency); short
  enough that a task genuinely needing a human is not held back noticeably, since exit (a) is
  already gated on the parent's 30s sampling interval. It is a *hold*, not a poll — one timer per
  deferral.

**`KANDEV_PARKED_PROBE_INTERVAL = 0` disables deferral entirely.** Stated once, here, and
unconditionally — it does **not** depend on the backstop's value:

> When the parent's `KANDEV_PARKED_PROBE_INTERVAL` is `0`, no periodic sample is ever taken, so
> exits (a) and (e) cannot fire. A session may still *project* `parked_on_background_work` from the
> synchronous first sample — the operator-facing affordance is the parent's business and is
> unaffected (parent AC-74) — but its `session.turn_finished` notification is **delivered
> immediately and never withheld**, whatever `KANDEV_PARKED_DEFER_BACKSTOP` is set to.

The rule is unconditional on purpose. **Kandev never withholds a notification whose release depends
on a mechanism that is switched off**, and a backstop is a bound on a deferral, not a substitute for
the observation that ends it. AC-65 asserts the rule with the backstop set to a **positive** value,
so it cannot be satisfied by a reading in which only `interval=0` *combined with* `backstop=0`
disables deferral.

## Permissions

Scoped by the existing rules; grants no new access. Notification bodies, recipients and provider
subscriptions are unchanged — only the dispatch instant moves.

## Failure modes

| Condition | Behaviour |
|---|---|
| Probe reports `unknown` at turn settle | Never parked, never withheld. The notification is delivered in the same turn-completion handling (AC-85). |
| Probe budget elapses at turn settle | Same as `unknown` — delivered, not withheld (AC-85). |
| agentctl crashes, disconnects, or is version-skewed while a session is parked | The probe call fails → `unknown` → exit (e) delivers the withheld notification. It is never left waiting on the backstop, which defaults to never. |
| Probe reports `live` but the workload is a leaked orphan | Exit (e) covers every case where the probe stops answering `live`. A genuinely immortal warm process is bounded only if `KANDEV_PARKED_DEFER_BACKSTOP` is set positive; at the default the deferral persists until the session leaves `WAITING_FOR_INPUT` (exit (c)). This is the deliberate trade recorded under the backstop default. |
| Backend restarts while a notification is withheld | The deferral record is not reconstructed and the notification is **dropped**. Conservative: no occurrence was recorded at withhold time, so nothing is duplicated, and a ping about a turn whose context is gone helps nobody. |
| Turn completion carries an empty turn ID | Not deferred — delivered immediately (D4). |
| A session has no subscribed provider | Nothing to withhold; the deferral is not created. |
| `KANDEV_PARKED_PROBE_INTERVAL` is `0` | Deferral disabled entirely; delivered immediately (AC-65). |

## Determinism, concurrency, and boundaries

### D4 — Deferral: at most one, single-flighted, never queued

*(Identifier preserved from the combined spec; the parent leaves D4 vacant.)*

- At most **one** deferred notification exists per session.
- The three delivering exits — (a), (b), (e) — can race. Delivery is single-flighted so exactly one
  wins; the per-provider occurrence dedup keyed on the turn ID is the backstop if both somehow
  proceed. At the default backstop, (b) is out and (a)/(e) are mutually exclusive.
- A new turn starting moves the session out of `WAITING_FOR_INPUT`, which drops any outstanding
  deferral through exit (c). **No queue of deferred notifications is kept**, and a superseded
  deferral is never delivered later.
- An empty or missing turn ID cannot key the occurrence dedup, so it is **not deferred** — it is
  delivered immediately, failing toward notifying. This matches `handleSemanticOccurrence`
  (`internal/notifications/service/service.go:191-194`), which already returns early on an empty
  occurrence ID.
- **Two callers, one session.** A sample-driven exit and a session-state-driven exit can be
  evaluated concurrently. The deferral record's read-decide-clear is one critical section, so the
  loser observes the record already cleared and does nothing. The record is cleared *before* the
  dispatch, so a delivering exit that then fails to dispatch does not leave a second exit able to
  re-deliver — the notification is lost rather than duplicated, which is the direction the
  occurrence dedup would enforce anyway.

### D9-N — Defaults and boundary values for this spec

*(The parent's D9 covers the projection's boundaries; these are the notification's. Named `D9-N`
rather than reusing `D9` so a citation names one table unambiguously.)*

| Field / input | Default / boundary behaviour |
|---|---|
| orphan backstop window | `0` = never (`KANDEV_PARKED_DEFER_BACKSTOP`); exit (b) does not fire at the default |
| settle-confirmation window | `10s` (`KANDEV_PARKED_SETTLE_CONFIRM`); `0` = no hold, exit (a) delivers on observation (AC-84) |
| parent's probe sampling interval `0` | deferral disabled entirely and unconditionally, independent of the backstop (AC-65) |
| a queued-but-undelivered operator prompt | counts as **answered**: the deferral is **dropped**, not delivered — while the parent's projection stays `true` (AC-86) |
| turn completion with an empty turn ID | delivered immediately, never deferred (D4, AC-43) |
| probe `unknown` or budget elapsed at settle | delivered in the same turn-completion handling, never withheld (AC-85) |
| backend restart with a notification withheld | dropped; no occurrence was ever recorded |
| deferred delivery's timestamp | the time it was dispatched, not the time the turn settled (AC-67) |

## Scenarios

- **AC-28** — **GIVEN** a provider subscribed to `session.turn_finished` and a turn that settles a
  session into the parked condition, **WHEN** the turn completes, **THEN** no `session.turn_finished`
  delivery is recorded for that turn.
- **AC-29** — **GIVEN** the withheld notification from AC-28, **WHEN** the probe transitions to
  `settled` while the session is still `WAITING_FOR_INPUT` and unanswered and the
  settle-confirmation window elapses with that still true, **THEN** exactly one
  `session.turn_finished` delivery is recorded, keyed on the original turn ID.
- **AC-30** — **GIVEN** the withheld notification from AC-28, **WHEN** the agent resumes itself and
  the session leaves `WAITING_FOR_INPUT`, **THEN** no `session.turn_finished` delivery is ever
  recorded for that turn.
- **AC-30a** — **GIVEN** the scenario of §J, reproduced with a probe that returns `live` for at
  least three consecutive samples before returning `settled`, and an agent that resumes itself on
  that transition, asserted at the **default** `KANDEV_PARKED_SETTLE_CONFIRM` with the self-resume
  landing inside the settle-confirmation window, **WHEN** the sequence runs, **THEN** zero
  `session.turn_finished` deliveries are recorded for that turn. *(This criterion previously also
  asserted the projection and rendering state at each sample point. Those clauses were the parent's
  concern and moved with the split to `disambiguate-waiting` AC-73; the straddle is recorded in the
  task plan. Nothing was dropped.)*
- **AC-31** — **GIVEN** the withheld notification from AC-28 **and `KANDEV_PARKED_DEFER_BACKSTOP`
  set to a positive duration** (at its default of `0`/never this WHEN is unreachable by
  construction, so the configuration is part of the criterion rather than something the test
  invents), **WHEN** the backstop window elapses with the probe still reporting `live`, **THEN**
  exactly one `session.turn_finished` delivery is recorded, keyed on the original turn ID.
- **AC-32** — **GIVEN** the withheld notification from AC-28, **`KANDEV_PARKED_DEFER_BACKSTOP` set
  to a positive duration**, and a probe that transitions to `settled` after the backstop already
  delivered it, **WHEN** the transition is observed, **THEN** no second delivery is recorded for
  that turn.
- **AC-33** — **GIVEN** a turn that settles a session with no attested detached launch, **WHEN** the
  turn completes, **THEN** `session.turn_finished` is delivered within the same turn-completion
  handling — not deferred — with its title and body unchanged from the values in
  `docs/specs/platform/notifications.md`.
- **AC-42** — **GIVEN** a session holding a withheld notification **and
  `KANDEV_PARKED_DEFER_BACKSTOP` set to a positive duration** (the exits can only contend when exit
  (b) is enabled), **WHEN** the `live → settled` transition's settle-confirmation window and the
  backstop-window expiry elapse simultaneously, **THEN** exactly one `session.turn_finished`
  delivery is recorded for that turn (D4).
- **AC-43** — **GIVEN** a turn completion carrying an empty turn ID, **WHEN** the notification
  decision is made, **THEN** it is delivered immediately and never deferred (D4).
- **AC-47** — **GIVEN** a session holding a withheld notification whose probe then transitions
  `live → unknown` while the session is still `WAITING_FOR_INPUT` and unanswered, **WHEN** the
  transition is observed, **THEN** exactly one `session.turn_finished` delivery is recorded, keyed
  on the original turn ID (exit (e)). Asserted with the backstop window at its default of `0`/never,
  so the delivery is attributable to exit (e) alone.
- **AC-48** — **GIVEN** the default configuration, **WHEN** the backstop window is read, **THEN** it
  is `0`, meaning exit (b) never fires; and **GIVEN** `KANDEV_PARKED_DEFER_BACKSTOP` is set to a
  positive duration, **WHEN** that duration elapses with the probe still `live`, **THEN** exit (b)
  delivers exactly once (AC-31).
- **AC-63** — **GIVEN** a turn whose `session.turn_finished` was withheld per AC-28, **WHEN** a
  delivering exit later fires, **THEN** a `session.turn_finished` delivery is **actually recorded
  and dispatched** for that turn — proving no occurrence was recorded at withhold time. An
  implementation that records the occurrence before suppressing satisfies AC-28 but fails this.
- **AC-65** — **GIVEN** the parent's `KANDEV_PARKED_PROBE_INTERVAL` set to `0` **and
  `KANDEV_PARKED_DEFER_BACKSTOP` set to a positive duration**, and a session that settles into the
  parked condition, **WHEN** the turn completes, **THEN** `parked_on_background_work` may be `true`
  but the `session.turn_finished` notification is **delivered immediately and not withheld**. The
  positive backstop is part of the criterion: it is what distinguishes the unconditional rule from a
  reading in which only `interval=0` *combined with* `backstop=0` disables deferral.
- **AC-66** — **GIVEN** a session holding a withheld notification whose probe transitions
  `live → settled` at the default `KANDEV_PARKED_SETTLE_CONFIRM`, **WHEN** the session leaves
  `WAITING_FOR_INPUT` **before** that window elapses, **THEN** zero deliveries are recorded for that
  turn; and **WHEN** instead the window elapses with the session still settled and unanswered,
  **THEN** exactly one delivery is recorded, keyed on the original turn ID.
- **AC-67** — **GIVEN** a withheld notification delivered later by any delivering exit, **WHEN** it
  is dispatched, **THEN** its title and body are identical to the immediate path's (AC-33), its
  occurrence key is the **original** turn ID, and its delivery timestamp is the time it was
  dispatched — not the time the turn settled.
- **AC-83** — **GIVEN** a session holding a withheld notification, **WHEN** the operator submits a
  prompt for it, **THEN** zero `session.turn_finished` deliveries are ever recorded for that turn.
  Asserted twice: once where the prompt is **admitted** immediately, and once where it is **queued**
  and not admitted while the session remains `WAITING_FOR_INPUT`. This is exit (d), and the queued
  case is the non-obvious half — the operator has acted, so no ping is owed even though the session
  never left `WAITING_FOR_INPUT`.
- **AC-84** — **GIVEN** `KANDEV_PARKED_SETTLE_CONFIRM` set to `0` and a session holding a withheld
  notification, **WHEN** the probe transitions `live → settled` while the session is still settled
  and unanswered, **THEN** exactly one delivery is recorded **on that observation**, with no hold;
  and **GIVEN** the same configuration, **WHEN** the agent resumes itself in the same instant,
  **THEN** at most one delivery is recorded in total. The `0` boundary is documented in D9-N and was
  observed by no criterion before this one.
- **AC-85** — **GIVEN** a turn settling on a session with an attested detached launch whose probe
  resolves to `unknown` — asserted for each of: the platform cannot enumerate; the probe budget
  elapses; the agent stream is disconnected — **WHEN** the notification decision is made, **THEN**
  the notification is **delivered within the same turn-completion handling and never withheld**.
  *(These clauses were carried by the parent's AC-27, AC-40 and AC-46 in the combined spec. The
  parent trimmed them at the split because it withholds nothing; they land here so no case lost its
  assertion.)*
- **AC-86** — **GIVEN** a parked session holding a withheld notification, **WHEN** the operator
  submits a prompt that is **queued but not admitted**, **THEN** the withheld notification is
  dropped **and** `parked_on_background_work` remains `true` with the background affordance still
  rendered. This asserts the deliberate divergence between the deferral and the projection in one
  place, across both specs, so an implementation cannot satisfy one by breaking the other. The
  parent's AC-75 asserts the projection half from its side.

## Contract amendment

`docs/specs/platform/notifications.md` gains the deferral rule from the state machine above. Its
existing table rows are unchanged, since a deferred delivery is still exactly one occurrence per
turn.

`docs/specs/platform/background-work-liveness.md` is **not** amended by this spec — the parent
amends it, and that amendment is about the visual affordance, not the ping.

Neither amendment relaxes prompt admission, and neither reads
`features.claudeBackgroundPromptHandoff`.

## Out of scope

- **Everything in [`docs/specs/disambiguate-waiting/spec.md`](../disambiguate-waiting/spec.md)** —
  the probe and its platform implementations, the `agent.background.probe` transport, the parked
  projection and its revisions and epoch, the recogniser seam, and every icon call site. This spec
  reads those and changes none of them.
- Adding a `TaskSessionState` member, or changing prompt admission or the queued-message path.
- Enabling, weakening, or removing `features.claudeBackgroundPromptHandoff`.
- Surviving a backend restart. A withheld notification is dropped; see *Failure modes*.
- Deferring any notification other than `session.turn_finished`.
- Batching, coalescing, or re-ordering notifications across turns or sessions. At most one deferral
  per session, never a queue (D4).
- Notifying differently — a distinct sound, body, or channel for a delayed ping. The delivery is
  byte-identical to the immediate one apart from its timestamp (AC-67).

## Open findings carried in from Spec Review round 4

Round 4 reviewed the combined spec and raised seventeen findings. Four concerned this half. Their
disposition at extraction time, recorded honestly:

- **f10 — exit (d)'s operator-prompt case had no AC.** **CLOSED at extraction** by the new
  **AC-83**, which asserts both the admitted and the queued case. D9-N's "counts as answered" row is
  now observed.
- **f11 — `KANDEV_PARKED_SETTLE_CONFIRM = 0` had no AC.** **CLOSED at extraction** by the new
  **AC-84**.
- **f12, deferral half — `interval=0` leaves a withheld notification with no delivering exit.**
  **Already closed** by the pre-existing **AC-65**: at `interval=0` nothing is ever withheld, so the
  hazard cannot arise. The dangerous half of f12 was the *projection* remaining `true` indefinitely,
  which is the parent's AC-74.
- **f13, deferral half — a queued prompt drops the deferral while the affordance keeps rendering.**
  **CLOSED at extraction** by stating the divergence as intentional (*State machine → The deliberate
  divergence*) and observing it in **AC-86**, which asserts both halves together.

**f8 was reassigned to the parent, deliberately, against the reviewer's suggested placement.**
Round 4 put `KANDEV_PARKED_PROBE_BUDGET = 0` here on the grounds that the budget guards the
synchronous *notification* decision. After the split it guards the synchronous *projection* sample,
which survives into the parent and runs on every attested turn end whether or not this spec ever
ships. Leaving the hazard here would have left the parent shipping an unbounded blocking call on
turn settlement with nothing owning the fix. It is closed by the parent's **AC-81**. This spec's
timing table records that the two keys defined here do **not** share that deviation.

**No round-4 finding assigned to this half was dropped.** But note the provenance header: those four
findings are the ones a reviewer found *in the combined document*. This file has never been reviewed
on its own, and its first review round should assume there are others.

## Open questions

- **OQ-1 — should the backstop default change now that it is this spec's only bound of last
  resort?** In the combined spec, `KANDEV_PARKED_DEFER_BACKSTOP = 0` was defensible because exit (e)
  covers every case where the probe stops answering `live`. That argument still holds. But the
  combined spec also shipped the projection alongside, so an operator staring at a stale spinner had
  a visible cue that something was wrong; here the failure is silent. Resolving this needs a
  judgement about how often a genuinely immortal warm descendant occurs in practice, which no
  measurement in either spec answers. Recorded rather than guessed. A defensible resolution is to
  keep `0` and rely on exit (e), which is what this spec currently specifies.
