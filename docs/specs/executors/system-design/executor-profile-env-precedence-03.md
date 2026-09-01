---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001
created: 2026-08-17
owners:
  - tbd
---
# Executor-Profile Environment Precedence System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Acceptance criteria

Observable behaviour. Each criterion is pass/fail against the resolver's return value, the emitted
override records, or the resulting environment map.

### Precedence

- **AC-1** WHEN a key has one literal definition from origin `executor profile` and one literal
  definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key
  to the `executor profile` value and SHALL NOT return an error.
- **AC-2** WHEN a key has one literal definition from origin `managed runtime` and one literal
  definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key
  to the `managed runtime` value and SHALL NOT return an error.
- **AC-3** WHEN a key has one literal definition from origin `managed credentials` and one literal
  definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key
  to the `managed credentials` value and SHALL NOT return an error.
- **AC-4** WHEN the resolver receives one literal definition from origin `agent profile` and one
  literal definition from origin `managed agent defaults` for the same key with different values,
  THE SYSTEM SHALL resolve that key to the `agent profile` value and SHALL NOT return an error.
  This is a resolver-level criterion. In the assembled launch path the pair cannot occur, because
  `appendAgentRuntimeDefaults` drops the agent-runtime default before resolution (AC-17). Tier 3
  exists so the resolver's table is total and the assembly-time skip is a redundant optimisation
  rather than the only thing preventing a conflict.
- **AC-4a** WHEN a definition is dropped at assembly time by `appendAgentRuntimeDefaults`, THE
  SYSTEM SHALL NOT emit an override record for that key. Assembly-time filtering is not an
  override; only a resolver tier decision is.
- **AC-5** WHEN a key has literal definitions from two different Tier 1 origins with different
  values, THE SYSTEM SHALL return a `*ConflictError` whose `Origins` lists every participating
  Tier 1 origin sorted ascending, and SHALL NOT resolve the key.
- **AC-5a** WHEN a key has an `executor profile` literal and an `agent profile` literal carrying
  THE SAME value, plus a `managed runtime` literal carrying a DIFFERENT value, THE SYSTEM SHALL
  return a `*ConflictError` naming all three origins sorted ascending, and SHALL NOT resolve the
  key. The merged `executor profile`/`agent profile` identity classifies as Tier 1 via its
  strongest contributing origin (procedure step 6), so the top tier holds two identities and AC-5
  applies. THE SYSTEM SHALL NOT classify that merged identity by the surviving `Definition.Origin`
  field alone, which would rank it Tier 2 and silently discard the `executor profile` value.
  This case is production-reachable and is the reason step 6 says "strongest": `AGENT_MODEL` is
  emitted as `managed runtime` from `profileInfo.Model` (`environment_resolution.go:65`) and is
  not a reserved key (`profile_crud.go:867-868` reserve only `TASK_DESCRIPTION` and `KANDEV_*`),
  so a user who duplicated it onto both profiles — the workaround this feature replaces — hits it.
- **AC-5b** WHEN a key has an `executor profile` literal and an `agent profile` literal carrying
  THE SAME value, plus a differing literal from a Tier 3 origin, THE SYSTEM SHALL resolve the key
  to the merged value and emit ONE `OverrideRecord` whose `WinningOrigins` holds BOTH
  `agent profile` and `executor profile` sorted ascending and whose `WinningTier` is
  `TierAuthoritative`. This pins that `WinningOrigins` may span tiers while `WinningTier` records
  only the strongest. This is a resolver-level criterion; in the assembled launch path
  `appendAgentRuntimeDefaults` drops the Tier 3 definition first (AC-17).
- **AC-6** WHEN a key has two definitions carrying the same `Origin` string and different
  `definitionIdentity` values, AND no definition for that key sits in a STRONGER tier than that
  origin, THE SYSTEM SHALL return a `*ConflictError`. Where a stronger tier does hold exactly one
  surviving identity, procedure step 6 governs instead: the stronger tier wins and the ambiguous
  weaker-tier origins are discarded into `LosingOrigins`, because a set of definitions that is
  being thrown away wholesale need not be self-consistent first. That discarded origin appears in
  `LosingOrigins` EXACTLY ONCE even though it contributed two identities, because both origin
  lists are de-duplicated sets ("The override record"); AC-8d observes this case directly.
  Same-origin ambiguity WITHIN the winning tier still conflicts, which is the case that matters:
  two differing `executor profile` definitions, or two differing `managed runtime` definitions,
  block the launch exactly as today (AC-5).
  REACHABILITY IS PER-ORIGIN, AND ONE ORIGIN REACHES THIS IN PRODUCTION. An earlier draft claimed
  the assembled launch path "cannot produce two same-origin definitions that disagree, because
  duplicate keys within one profile are rejected at save time (`profile_crud.go:908`)". That is
  true for `agent profile` and false for `executor profile`: the duplicate-key check is
  agent-profile-only (see "The two profile kinds are NOT validated the same way"), so one executor
  profile may legitimately hold two entries with the same key and different values. Per origin:
  - `agent profile` — NOT reachable; duplicate keys rejected at save time (`profile_crud.go:908`).
  - `managed agent defaults` — NOT reachable; built from a map.
  - `managed runtime` — reachable in principle, since `req.Env` and `appendStandardDefinitions`
    both emit this origin, though a collision there is a managed-value bug rather than user input.
  - `executor profile` — REACHABLE from ordinary user input, with no validation preventing it.
  The behaviour is nonetheless unchanged from today: every one of those origins is Tier 1, so with
  no stronger tier present the key returns `*ConflictError` exactly as it does now. AC-6a requires
  the test for the reachable case.
- **AC-6a** WHEN one executor profile holds TWO entries with the same key and different non-empty
  values, and no definition for that key sits in a stronger tier, THE SYSTEM SHALL return a
  `*ConflictError` naming `executor profile`, and SHALL NOT resolve the key by picking either
  value.
  This is the ASSEMBLED-PATH case AC-6 says is reachable, and it SHALL be tested through
  `Executor.taskEnvironmentSources` rather than only against the resolver, because the claim being
  pinned is that a real executor profile can produce the pair. `executor profile` is Tier 1, so
  this is AC-5's same-tier behaviour and is unchanged from today; the AC exists so the reachable
  case has a test rather than resting on a false save-time guarantee.
- **AC-7** WHEN a key is defined only by origin `agent profile` and no executor profile is
  selected for the session, THE SYSTEM SHALL resolve that key to the `agent profile` value.
- **AC-8** WHEN a key is defined only by origin `agent profile` and the selected executor profile
  declares no environment variables, THE SYSTEM SHALL resolve that key to the `agent profile`
  value.
- **AC-8a** WHEN a key has two Tier 1 definitions that share one `definitionIdentity` and one
  differing literal Tier 2 definition, THE SYSTEM SHALL merge the Tier 1 pair first, then apply
  tier precedence, resolving the key to the merged Tier 1 value and emitting a single
  `OverrideRecord` whose `WinningOrigins` holds BOTH contributing Tier 1 origins sorted ascending,
  whose `WinningTier` is `TierAuthoritative`, and whose `LosingOrigins` holds the one Tier 2
  origin. Merging precedes tier comparison (procedure step 3 before step 6). This is the case that
  makes `WinningOrigins` a slice rather than a string.
- **AC-8b** WHEN a key has two differing literal Tier 1 definitions and one differing literal
  Tier 2 definition, THE SYSTEM SHALL return a `*ConflictError` whose `Origins` names all three
  origins, sorted ascending. An ambiguous top tier is not resolved by falling through to a lower
  tier.
- **AC-8c** WHEN a key has one literal Tier 1 definition and differing literal definitions in both
  Tier 2 and Tier 3, THE SYSTEM SHALL resolve the key to the Tier 1 value and emit ONE
  `OverrideRecord` whose `LosingOrigins` holds both the Tier 2 and the Tier 3 origin, each paired
  with its own tier, sorted by `Origin` ascending. Per AC-24 this one record produces TWO counter
  increments, one per losing origin.
- **AC-8d** WHEN a key has ONE literal Tier 1 definition and TWO literal definitions that carry
  THE SAME lower-tier `Origin` string as each other but DIFFERENT `definitionIdentity` values,
  THE SYSTEM SHALL resolve the key to the Tier 1 value and emit ONE `OverrideRecord` whose
  `LosingOrigins` has `len(LosingOrigins) == 1` — a single entry for that origin paired with its
  own tier — and which produces EXACTLY ONE counter increment.
  This is the losing-side de-duplication pin, and it is the case AC-6's stronger-tier carve-out
  creates: one origin contributing two discarded identities is ONE loser, not two. A test asserting
  `len(LosingOrigins) == 2` or two increments is asserting the rejected bag reading.
  This one IS genuinely resolver-level, and unlike AC-6 the reason survives the per-origin check in
  AC-6a. The ambiguous origin here is a LOSING origin, so it necessarily sits in Tier 2 or Tier 3 —
  a Tier 1 origin can never lose, and an ambiguity inside Tier 1 conflicts instead (AC-5, AC-6a).
  The only Tier 2 origin is `agent profile`, whose duplicate keys ARE rejected at save time
  (`profile_crud.go:908`), and the only Tier 3 origin is `managed agent defaults`, which is built
  from a map. `executor profile`, the one origin that can produce disagreeing duplicates, is Tier 1
  and therefore cannot appear in `LosingOrigins` at all.
- **AC-8e** WHEN a key has ONE Tier 1 value contributed by TWO definitions that share BOTH an
  `Origin` string and a `definitionIdentity`, plus a differing literal definition in a lower tier,
  THE SYSTEM SHALL emit ONE `OverrideRecord` whose `WinningOrigins` has
  `len(WinningOrigins) == 1` — that origin listed once, not twice.
  This is the winning-side de-duplication pin. It matters beyond symmetry: today's
  `selectDefinitions` already accumulates contributing origins into a `map[string]struct{}`, so the
  bag reading would be a silent behaviour change to existing merge-origin reporting. Contrast with
  AC-8a, where two DISTINCT origins contribute one identity and both are listed.

### Secret veto

- **AC-9** IF a key has definitions with different `definitionIdentity` values and at least one of
  them has a non-empty `SecretID`, THEN THE SYSTEM SHALL return a `*ConflictError` and SHALL NOT
  apply tier precedence, even when the definitions sit in different tiers.
- **AC-10** WHEN a key has a secret-backed `executor profile` definition and a literal
  `agent profile` definition, THE SYSTEM SHALL return a `*ConflictError`.
- **AC-11** WHEN a key has a literal `executor profile` definition and a secret-backed
  `agent profile` definition, THE SYSTEM SHALL return a `*ConflictError`.
- **AC-12** WHEN a key has a secret-backed `repository <name>` definition and any differing
  definition from any other origin, THE SYSTEM SHALL return a `*ConflictError`, preserving
  ADR-2026-08-03.
- **AC-12a** WHEN a key has a secret-backed `repository app` definition and a secret-backed
  `executor profile` definition that carry THE SAME `SecretID`, plus a differing literal
  `agent profile` definition, THE SYSTEM SHALL return a `*ConflictError` whose `Origins` names ALL
  THREE origins — `agent profile`, `executor profile`, `repository app` — de-duplicated and sorted
  ascending, and SHALL NOT resolve the key.
  The two secret-backed definitions share the identity `"secret:<id>"` and MERGE at procedure
  step 3, so a single surviving `Definition` carries one `Origin` field between them. THE SYSTEM
  SHALL NOT enumerate `Definition.Origin` per surviving identity, which would report only two
  origins and hide from the user one of the two places the secret is bound.
  This is the veto-path counterpart of AC-5a, and it is the MORE production-reachable of the two:
  ADR-2026-08-03 makes every repository binding secret-only, so real secret disagreements arrive
  at step 5 rather than step 6. It preserves today's behaviour — `selectDefinitions` already folds
  the merged identity's accumulated origin set into the conflict set — and
  `TestResolveEnvironmentSources_ReportsEveryConflictingOrigin` already asserts against this field.
- **AC-13** WHEN a `*ConflictError` is returned, its message and `Origins` SHALL contain no secret
  value, no `SecretID`, and no literal value.

### No regression for currently-succeeding tasks

- **AC-14** WHEN a key has exactly one definition, THE SYSTEM SHALL resolve it to that
  definition's value, unchanged from today.
- **AC-15** WHEN a key has multiple definitions that all share one `definitionIdentity`, THE
  SYSTEM SHALL merge them, resolve the key to that shared value, record every origin as it does
  today, and SHALL NOT emit an override record. Nothing was overridden.
- **AC-16** WHEN a definition set produced no error before this change AND its result was
  DETERMINISTIC under the current three-column sort — that is, its outcome did not depend on input
  slice order — THE SYSTEM SHALL produce the same environment map after this change.
  The determinism qualifier is not a hedge; it is required, and AC-29a states the same bound for
  the sort. Two changes land together, and only one of them is precedence:
  - PRECEDENCE re-routes only sets that previously returned `*ConflictError`, so it cannot alter
    any set that already succeeded.
  - THE FIVE-COLUMN SORT (AC-29) additionally settles ties the three-column sort left to input
    order. One such tie already succeeds today: a same-`Key`/`Origin`/`SecretID` pair differing
    only in `WorkspaceID` merges (identity is `"secret:" + SecretID`), and whichever definition
    happens to come first supplies the `WorkspaceID` that `resolveEnvironmentDefinition` branches
    on to choose `RevealForWorkspace` over a global reveal (`environment_resolution.go:188-195`).
    Its revealed VALUE is therefore input-order-dependent today, and AC-29b now pins it. For such
    a set the post-change map may differ from one of its pre-change orderings, deliberately.
  A differential test against the current `selectDefinitions` as oracle MUST therefore restrict
  its generator to conflict-free sets that are order-INDEPENDENT today, or assert per-ordering
  agreement only for sets with no `WorkspaceID` tie. Generating the tie class and demanding
  equality would be testing a promise this AC does not make.
- **AC-17** WHEN `appendAgentRuntimeDefaults` runs, it SHALL continue to skip any key already
  defined by an earlier definition, preserving
  `TestAppendAgentRuntimeDefaultsFillsOnlyMissingKeys`.

### Preflight consistency

- **AC-18** THE shape-only preflight at `task_environment.go:118` SHALL apply the same tier and
  secret-veto rules as the final resolve, so it never rejects a definition set that the final
  resolve would accept.
- **AC-19** WHILE the preflight definition set contains no `agent profile` definitions, its
  accept/reject verdict SHALL be identical to today's for every input.
- **AC-19a** BOTH assembly sites SHALL derive their origin strings from one shared set of exported
  constants declared in the `environment` package, and the tier table SHALL be defined once, in
  that same package. The orchestrator site currently hardcodes `"managed runtime"` and
  `"executor profile"` (`task_environment.go:63`, `:67`) while the lifecycle site uses unexported
  constants (`environment_resolution.go:17-21`); a rule keyed on origin cannot depend on two
  independent spellings staying in sync.
- **AC-19c** THE `environment` package SHALL export exactly two classification functions,
  `TierForOrigin(origin string) Tier` and `IsRepositoryOrigin(origin string) bool`, and SHALL NOT
  export the tier table itself. `TierForOrigin` SHALL be the only reader of that table.
  `IsRepositoryOrigin` SHALL be the only implementation of the normative repository-origin rule
  ("What counts as a repository origin"), and BOTH `TierForOrigin` and `NormalizeOriginLabel`
  (AC-25) SHALL call it rather than re-deriving it — one spelling, per AC-19a.
  A test SHALL assert `IsRepositoryOrigin` against the worked boundary table in that section:
  true for `repository`, `repository app`, `repository  app` (two spaces) and `repository\tapp`;
  false for `repositoryapp` and the empty string. A test SHALL also assert that
  `TierForOrigin` returns `TierAuthoritative` for an origin the table does not name (AC-35).
  Keeping the table unexported is observable at compile time and is the point: an exported map
  could be mutated by any importer, which would defeat the single-source-of-truth guarantee AC-19a
  asks for.
- **AC-19b** `Validate` SHALL keep the signature `func Validate([]Definition) error` and SHALL
  emit no override record, no override log, and no counter increment. A test SHALL assert that a
  definition set which WOULD produce an override when passed to `Resolve` produces no observable
  override side effect when passed to `Validate`.

### Observability

- **AC-20** WHEN tier precedence resolves a key that would previously have produced a
  `*ConflictError`, THE SYSTEM SHALL produce exactly one `OverrideRecord` for that key, populated
  as declared in "The override record": `Key`, `WinningOrigins` (every DISTINCT origin that
  contributed to the merged winning identity, each listed ONCE, sorted ascending, at least one),
  `WinningTier`, and `LosingOrigins` (every DISTINCT origin appearing among the discarded
  identities, each listed ONCE with its tier, sorted by `Origin` ascending, at least one).
  BOTH lists are DE-DUPLICATED SETS: no origin string appears twice in either. One entry per
  ORIGIN, never one per contributing or discarded identity. AC-8d and AC-8e observe the two
  de-duplication cases directly.
- **AC-20a** WHEN `Resolve` returns a non-nil error, it SHALL return a nil map and a nil
  `[]OverrideRecord`, even if tier precedence had already resolved other keys before the failing
  key was reached, AND the caller SHALL emit no override log record and SHALL NOT increment the
  counter for that resolve. A test SHALL cover both error paths: (a) one key resolved by tier
  precedence plus a second key that conflicts, and (b) one key resolved by tier precedence plus a
  secret that fails to reveal, which is the more reachable case because step 7 reveals only after
  every key is conflict-free. Nothing was applied, so nothing is reported.
- **AC-21** An `OverrideRecord` SHALL NOT contain any literal value, any `SecretID`, or any
  decrypted secret. The type has no field capable of carrying one.
- **AC-22** THE resolver SHALL return override records to its caller rather than logging them
  itself. The `environment` package holds no logger and must stay a pure function of its inputs.
  Records reach the caller as `Resolve`'s new third return value.
- **AC-23** WHEN a launch applies one or more overrides, `Manager.resolveStrictEnvironment` SHALL
  emit one structured log record at Info level per `OverrideRecord`, with the message
  `environment override applied` and these fields:

  | Field | Type | Source |
  | --- | --- | --- |
  | `env_key` | string | `OverrideRecord.Key` |
  | `winning_origins` | []string | `WinningOrigins`, in record order |
  | `winning_tier` | int | `WinningTier` |
  | `losing_origins` | []string | `LosingOrigins[].Origin`, in record order |
  | `losing_tiers` | []int | `LosingOrigins[].Tier`, index-aligned with `losing_origins` |
  | `task_id` | string | `LaunchRequest.TaskID` |
  | `session_id` | string | `LaunchRequest.SessionID` |

  Origins are logged UNNORMALISED, so a reader sees which repository was involved. Normalisation
  (AC-25) applies to the metric label only, where cardinality matters.
- **AC-24** THE SYSTEM SHALL increment an expvar counter map named
  `environment_override_applied_total` once per entry in `LosingOrigins`, so the number of
  increments for one record equals `len(LosingOrigins)` exactly. Counting is POST-MERGE and
  strictly per-ORIGIN, never per input definition and never per discarded identity: ALL discarded
  definitions sharing one origin collapse into a single `LosingOrigins` entry and are counted
  ONCE, **whether or not they share a `definitionIdentity`**. Two differing `agent profile`
  literals discarded together are ONE increment, not two (AC-8d).
  The earlier wording conditioned that collapse on sharing "one origin AND one
  `definitionIdentity`", which read as one entry per identity and contradicted this AC's own
  per-ORIGIN rule; the condition is the ORIGIN alone, because `LosingOrigins` is a de-duplicated
  set (AC-20, "The override record"). One key overriding one origin increments its label pair
  exactly once, so the counter measures overrides rather than internal identity groupings.
  The record and the log record remain one per key (AC-20, AC-23); only the counter fans out per
  losing origin, because one label pair cannot represent several losers.
- **AC-24a** THE counter SHALL live in a new neutral package
  `apps/backend/internal/agent/runtime/envmetrics`, following the existing convention in
  `apps/backend/internal/workflow/signalmetrics`: an `expvar.NewMap` at package scope, incremented
  through an exported `RecordOverrideApplied(winningOrigin, losingOrigin string)` helper, with the
  label key built as `k1=v1;k2=v2` so a downstream Prometheus translation splits on the same
  delimiters. The label key names SHALL be exactly `winning_origin` and `losing_origin`, giving
  map keys of the form `winning_origin=executor profile;losing_origin=agent profile`.
  `RecordOverrideApplied` SHALL receive values that are ALREADY NORMALISED per AC-25 and SHALL
  perform no normalisation of its own. `envmetrics` stays a neutral recorder with no dependency on
  `environment`, exactly as `signalmetrics` is neutral today.
- **AC-24b** THE counter SHALL be incremented by the lifecycle caller, in the same place as the
  AC-23 log, and SHALL NOT be incremented inside `selectDefinitions`, `Resolve`, or `Validate`.
  The `environment` package must not import `envmetrics`; keeping the resolver pure is what makes
  AC-27's repeatability testable.
- **AC-25** THE normalised origin used as a metric label SHALL be derived as follows, and this
  normalisation applies to BOTH label values:
  - an origin for which `IsRepositoryOrigin` reports true collapses to the single value
    `repository`, so repository names cannot make metric cardinality unbounded;
  - every other origin, INCLUDING an origin the tier table does not recognise, is used verbatim,
    with no case folding and no whitespace substitution. Origins are produced by code constants,
    so the value set is bounded without further processing.
  - WHEN `WinningOrigins` holds more than one origin, the `winning_origin` label value SHALL be
    each origin normalised, de-duplicated, sorted ascending, and joined with `+` (for example
    `executor profile+managed runtime`).
    The join's de-duplication is a POST-NORMALISATION step and is separate from the set rule on
    `WinningOrigins` itself: AC-20 already guarantees no origin appears twice in the field, but two
    DISTINCT repository origins (`repository app`, `repository web`) both normalise to
    `repository`, so the collapse can still produce a duplicate that the join must remove. That
    specific pair is resolver-level only, exactly as AC-4 and AC-6 are: repository definitions are
    secret-only by ADR construction, and the veto (AC-9) bars any secret-backed definition from
    reaching `WinningOrigins` beside a differing definition. The join is de-duplicated anyway so
    the label rule is total and cannot depend on that reachability argument staying true.

  THE normalisation SHALL be owned by the `environment` package and exposed as two exported
  functions beside the tier table: `NormalizeOriginLabel(origin string) string` for a single
  origin, and `JoinOriginLabels(origins []string) string` for the collapse-dedupe-sort-join over
  `WinningOrigins`. `NormalizeOriginLabel` SHALL make its repository decision by calling
  `IsRepositoryOrigin` (AC-19c), never by re-deriving the rule. The lifecycle caller calls these
  two and passes the results to `RecordOverrideApplied`; because AC-24 fans the counter out per
  losing origin, the `losing_origin` value is always a single origin and `NormalizeOriginLabel`
  alone produces it. `environment` owns this because the repository rule is the same knowledge the
  tier table already consults (AC-19a): two independent spellings of "what counts as a repository
  origin" is precisely the drift AC-19a exists to prevent, which is why exactly one predicate
  exists and both callers share it.
- **AC-26** WHEN no override is applied during a launch, THE SYSTEM SHALL emit no override log
  record and SHALL NOT increment the counter.
  THE RETURN CONVENTION IS PINNED: when no override is applied, `Resolve` SHALL return `nil` for
  `[]OverrideRecord`, never a non-nil empty slice. `nil` and `[]OverrideRecord{}` are not
  `reflect.DeepEqual`, so leaving the choice open would make the AC-27 and AC-30 determinism
  tests fail or pass on an irrelevant detail. Callers SHALL nonetheless treat an empty slice and
  `nil` identically — `len(records) == 0` is the correct caller-side test, so a future change to
  the convention cannot silently start emitting logs.
