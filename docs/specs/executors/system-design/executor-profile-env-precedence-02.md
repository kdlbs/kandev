---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001
created: 2026-08-17
owners:
  - tbd
---
# Executor-Profile Environment Precedence System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Resolution procedure

The order of these steps is part of the contract. A different order produces different answers for
mixed secret and multi-origin cases.

For each environment key, in a deterministic TOTAL sort order over named columns.

The sort that exists today at `environment.go:97` compares `Key`, then `Origin`, then `SecretID`.
That is not a total order, and the gap is load-bearing rather than theoretical: two definitions
agreeing on all three but differing in `WorkspaceID` share one `definitionIdentity` (identity is
`"secret:" + SecretID` for a secret), so they MERGE, and which one survives is decided by input
order — while `WorkspaceID` is exactly what `resolveEnvironmentDefinition` branches on to choose
between a global reveal and `RevealForWorkspace`. Input order could therefore change the revealed
value, which AC-30 forbids.

The sort therefore gains two further NAMED columns and becomes, all ascending:

    Key, then Origin, then SecretID, then WorkspaceID, then Literal

Adding columns can only break ties that were previously left to input order, so no definition set
whose outcome is already deterministic changes (AC-16, AC-29a).

For each key, in that order:

1. **Skip blanks.** Discard any definition whose `Key` is empty or whitespace-only.
2. **Group** the remaining definitions by `Key`.
3. **Merge identical identities.** Definitions sharing one `definitionIdentity` collapse to a
   single surviving definition, recording every contributing origin, exactly as today. Merging
   happens **before** any tier or secret evaluation. The merged group RETAINS THE FULL SET of
   contributing origins; step 6 classifies the group by that set, not by the single surviving
   `Definition.Origin` field.
4. **If one identity survives**, that is the value. No conflict, no override record.
5. **If more than one identity survives, apply the secret veto first.** If any surviving
   definition for that key has a non-empty `SecretID`, return `*ConflictError` whose `Origins`
   holds **every CONTRIBUTING origin of every surviving identity** (per "The secret veto"),
   de-duplicated and sorted ascending. Tier rank is not consulted.
6. **Otherwise compare tiers.** Classify each surviving IDENTITY — not each surviving
   `Definition` — into a tier by `TierForOrigin`. **An identity's tier is the STRONGEST
   (numerically smallest) tier among ALL of its contributing origins.**
   - If the highest occupied tier holds more than one surviving identity, return
     `*ConflictError` whose `Origins` holds every CONTRIBUTING origin of every surviving identity,
     including origins from lower tiers, de-duplicated and sorted ascending.
   - Otherwise the single identity in the highest occupied tier wins, every identity in a lower
     tier is discarded, and one override record is emitted. Both origin lists in that record are
     DE-DUPLICATED SETS (see "The override record"): `WinningOrigins` holds each distinct
     contributing origin of the winning identity once, and `LosingOrigins` holds each distinct
     origin appearing anywhere among the discarded identities once — one entry per ORIGIN, never
     one per discarded identity.
7. **Reveal** secrets only for surviving winners, after the whole key set is conflict-free,
   preserving today's ordering in `Resolve`.

Step 5 preceding step 6 is deliberate: a secret and a literal that disagree must block even when
they sit in different tiers.

Steps 5 and 6 use ONE enumeration rule between them — every contributing origin of every surviving
identity, de-duplicated and sorted. Stating it once per step rather than only on the precedence
path is deliberate: merging happens at step 3, before either, so BOTH paths can face a surviving
identity whose single `Definition.Origin` field under-reports the origins that produced it.

Step 6's "including origins from lower tiers" is deliberate: when the top tier is itself
ambiguous, the error should name every origin the user has to look at, matching today's behaviour
of reporting the complete participating set.

**Step 6 classifies the IDENTITY by its strongest contributing origin, and that word "strongest"
is load-bearing.** Classifying by the surviving `Definition.Origin` instead would silently
discard a Tier 1 value in the most common real configuration this feature exists to serve.
Worked case: key `AGENT_MODEL` carries `executor profile` literal X, `agent profile` literal X
(the SAME literal — precisely the duplicate-the-value workaround the Why section says users
employ today), and `managed runtime` literal Y from `profileInfo.Model`
(`environment_resolution.go:65`). Step 3 merges the X pair into one identity. Under the
five-column sort the surviving `Definition` is the `agent profile` one, because
`"agent profile" < "executor profile"` — so classifying by that single field would put identity X
in Tier 2, leave Y as the sole Tier 1 identity, and hand the key to Y while silently dropping an
`executor profile` literal that disagrees with it. AC-5 requires that pair to conflict. Under the
strongest-contributing-origin rule, identity X is Tier 1 (via `executor profile`), the top tier
holds two identities, and the key conflicts — which is what AC-5 says and what AC-5a pins.

## The override record

The record is a declared Go type, not a shape left to the builder. It lives in the `environment`
package beside the tier table, and it carries identity and rank only — never a value.

```go
// Tier ranks an origin. LOWER IS STRONGER: Tier 1 beats Tier 2 beats Tier 3.
type Tier int

const (
    TierAuthoritative       Tier = 1
    TierProfileDefault      Tier = 2
    TierAgentRuntimeDefault Tier = 3
)

// OriginTier pairs an origin string with the tier it classified into.
type OriginTier struct {
    Origin string
    Tier   Tier
}

// OverrideRecord describes one environment key whose value was chosen by tier
// precedence instead of blocking the launch. It contains no literal value, no
// SecretID, and no decrypted secret.
type OverrideRecord struct {
    Key            string       // the environment key that was overridden
    WinningOrigins []string     // SET: >= 1 entry, each distinct origin ONCE,
                                // sorted ascending; MAY span tiers
    WinningTier    Tier         // the STRONGEST tier among WinningOrigins — the tier the
                                // winning identity was classified into by procedure step 6
    LosingOrigins  []OriginTier // SET: >= 1 entry, each distinct origin ONCE,
                                // sorted by Origin ascending
}
```

`WinningOrigins` is a SLICE, not a string. A merged winner can legitimately have more than one
contributing origin — that is exactly the case AC-8a describes, where two Tier 1 definitions share
one `definitionIdentity`, merge, and then beat a Tier 2 literal.

### BOTH ORIGIN LISTS ARE SETS, NOT BAGS

This is a contract, not an implementation detail, because it decides observable test assertions
and observable counter values:

> **Every origin string appears AT MOST ONCE in `WinningOrigins`, and at most once across all of
> `LosingOrigins`.** Both lists are de-duplicated by origin string, then sorted. Neither list ever
> contains two entries carrying the same `Origin`.

Concretely, per list:

- **`WinningOrigins`** holds each DISTINCT origin that contributed to the winning identity. Two
  definitions that share both an origin and a `definitionIdentity` contribute ONE entry, not two.
- **`LosingOrigins`** holds each DISTINCT origin appearing anywhere among the discarded
  identities, paired with that origin's own tier. One entry per ORIGIN — **never one per discarded
  identity.** So when a single losing origin contributes SEVERAL differing identities, which is
  exactly what AC-6's stronger-tier carve-out produces, that origin still yields ONE
  `LosingOrigins` entry and therefore ONE counter increment (AC-8d).

Because `Tier` is functionally determined by `Origin` through `TierForOrigin`, de-duplicating by
origin can never discard distinct tier information: two entries sharing an `Origin` are always
identical `OriginTier` values.

**The two lists are de-duplicated INDEPENDENTLY, and one origin MAY appear in both.** The rule
above bounds each list on its own; it does not make them disjoint, and THE SYSTEM SHALL NOT
subtract `WinningOrigins` from `LosingOrigins`. Worked case: `executor profile` = X,
`agent profile` = X, `agent profile` = Y. Identity X is contributed by both origins and classifies
Tier 1; identity Y is contributed by `agent profile` alone and classifies Tier 2; X wins. Then
`WinningOrigins` is `[agent profile, executor profile]` and `LosingOrigins` is
`[{agent profile, TierProfileDefault}]` — the same origin on both sides, because it genuinely both
contributed to the winner and had a distinct value discarded. The metric label pair is
`winning_origin=agent profile+executor profile;losing_origin=agent profile`, which is correct
rather than a defect to normalise away.
This is resolver-level only, for the same reason AC-6's ambiguous case is: reaching it needs one
origin contributing two differing identities, and the only origin that can do that is
`executor profile` (AC-6a), which is Tier 1 and therefore never a loser. It is pinned because the
rule is otherwise silent on the interaction and a builder could reasonably "clean up" the
duplicate, changing an observable counter value with no AC to catch it.

Three reasons this is the SET reading and not the bag reading, in order of weight:

1. **It preserves today's behaviour.** `selectDefinitions` already accumulates contributing
   origins in a `map[string]struct{}` (`origins[key][origin] = struct{}{}`) and already emits
   `ConflictError.Origins` de-duplicated and sorted. The bag reading would be a silent behaviour
   CHANGE to merge-origin reporting on a path this feature was not asked to touch.
2. **It is what the counter means.** AC-24 counts "per-ORIGIN, not per input definition". Under the
   bag reading, one key overriding one origin could increment the same
   `winning_origin=…;losing_origin=…` label pair twice, which makes the metric a count of internal
   identity groupings rather than of overrides.
3. **It makes AC-28's ordering total.** With origins unique, `Origin` alone totally orders
   `LosingOrigins`; under the bag reading it does not, and AC-27's deep-equality requirement would
   rest on an undefined order between indistinguishable entries.

**The winning origins may span tiers, and `WinningTier` is the strongest of them.** A merge groups
definitions by `definitionIdentity`, which is the hash of the literal (or the secret ID) and says
nothing about origin, so one identity can be contributed by an `executor profile` (Tier 1) and an
`agent profile` (Tier 2) that happen to carry the same value. `WinningTier` therefore records the
tier the identity was CLASSIFIED into by procedure step 6 — the numerically smallest tier among
`WinningOrigins` — not a property shared by every member. Consumers that need the per-origin
detail read `WinningOrigins`; `WinningTier` answers only "which tier won this key".

Note the remaining asymmetry with `LosingOrigins`, which pairs each origin with its OWN tier via
`OriginTier`. That is deliberate and is not affected by the set rule above: losers are enumerated
per origin because AC-24 fans the counter out per losing origin, whereas the winner is a single
classified identity with a single rank.

### Signatures

```go
// CHANGED: gains a third return value.
func Resolve(ctx context.Context, definitions []Definition, reveal RevealFunc) (map[string]string, []OverrideRecord, error)

// UNCHANGED.
func Validate(definitions []Definition) error
```

`Resolve` gains a return value. That is a deliberate, named change to the function signature; the
exported FIELDS of `Definition`, `ConflictError` and `SecretError` are untouched. There are only
two call sites to update: `Manager.resolveStrictEnvironment`
(`environment_resolution.go:51`, the sole production resolve site) and `resolveEnvironmentSources`
(`task_environment.go:40`, which today is reached only from tests).

**What each call site does with the new value.** `Manager.resolveStrictEnvironment` consumes the
records: it emits the AC-23 log and the AC-24 counter increments. `resolveEnvironmentSources`
DISCARDS them (`resolved, _, err := ...`) and SHALL NOT log or count. That is not laziness about a
test helper — it is AC-46's rule applied consistently: emission is tied to an actual launch
resolution, and this function is reached from no production path (5 test callers, 0 production).
A test helper that incremented the shared expvar map would make the counter a count of test runs.
If a future change gives it a production caller, that caller must take over the emission and this
clause must be revisited.

**On any error, `Resolve` returns NO override records.** When the third return value is non-nil the
second SHALL be `nil` and the first SHALL be `nil`, matching today's `return nil, err` on every
failure path. This is all-or-nothing, not partial (AC-20a).

The case is reachable and is not hypothetical. Records are computed for every key before any
secret is revealed, because step 7 reveals only after the whole key set is conflict-free. So a set
where key `A` is resolved by tier precedence while key `B` conflicts, or where `A` overrides and
any secret then fails to reveal, has already produced a record for `A` at the moment the error is
returned. Handing those records back would let `Manager.resolveStrictEnvironment` log
`environment override applied` and increment `environment_override_applied_total` for a launch
that failed and applied nothing — corrupting the counter AC-24 defines and contradicting AC-46's
"tied to an actual resolution". A launch that does not start has not overridden anything.

`Validate` keeps its `error`-only signature and emits no records, no log, and no counter
increment. This is safe rather than merely convenient: the preflight set is assembled by
`Executor.taskEnvironmentSources` from `managed runtime`, `executor profile` and
`repository <name>` definitions only — every one of them Tier 1 — because agent-profile
definitions are not appended until `resolveStrictEnvironment` runs at the lifecycle site. A
Tier-1-only set can never occupy two tiers, so it can never produce an override, so there is
nothing for `Validate` to return. If a future change adds a Tier 2 or Tier 3 definition to the
preflight set, this AC must be revisited (AC-19b).
