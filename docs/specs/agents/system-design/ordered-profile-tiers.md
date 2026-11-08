---
status: current
system: agents
requirements:
  - REQ-AGENTS-ORDERED-TIERS-001
---

# Ordered profile tier design

## Feature and storage boundary

`features.dynamicProfileTiers` / `KANDEV_FEATURES_DYNAMIC_PROFILE_TIERS` is a
restart-required native flag, off in every shipped profile. It is independent of
the existing dynamic-routing flag. The engine receives its effective value and
uses the existing flat path when off. Settings retain stored selection on both
paths; enabled-only join changes are rejected when the flag is off.

Candidate selection is an additive raw JSON subdocument of `Policies` in the
existing `DynamicAgentRoute.RulesJSON`. The controller merges fields by presence,
preserving unknown nested metadata. Legacy omission preserves original blocks,
recomputing joins only where original block members remain adjacent. Explicit
join edits retain each row's model data. No new table, policy engine or queue.

## Runtime and chain

The existing generation-CAS/probe/failure-policy pipeline owns selection.
Candidate runtime metadata decodes join and tier mode/direction, plus the cost
class only for cost ranking. Tiers retain saved tier order. Within a tier, order
retains row order, cost stably orders free/subscription/metered/unknown, and random
promotes a uniformly sampled eligible row. Availability is observed without a
probe; the existing final candidate check owns probe acquisition and generation
commit. Unsupported saved modes stay stored and are not
silently executed as order while the new flag is enabled.

Round-robin reads committed positive `selection_ordinal` values from the existing
task route-attempt owner. Each tier rotates saved rows after its most recently
selected current member, then applies the same eligibility and chain rules.
The anchor can be disabled or blocked; candidate deletion removes that member
from the current tier, so remaining members' committed history determines its
anchor. A tier without a valid ordinal starts at its first eligible saved row.
Legacy NULLs are not backfilled from timestamps, UUIDs or session generations.
All new committed task/session choices, including other modes, get an ordinal.

The nullable BIGINT migration precedes the profile/ordinal index. The existing
route-generation/attempt transaction allocates profile-local MAX+1 and inserts
the immutable attempt after a successful CAS. A PostgreSQL transaction advisory
lock encloses allocation; SQLite uses its existing writer owner. Failed TX and
stale CAS do not advance. One Engine mutex encloses history read, ranking and
commit. Independent backends can rank the same observation before committing:
unique choice across backends is not established by ordinal allocation alone.
An RR read failure is an error; other modes and feature OFF do not read history.

Provider-usage tiers (headroom, pace and consumption rate) reuse the last provider
reading observed through the existing `features.providerUsageLimits` snapshot. The
resolver receives a lookup keyed by concrete profile; it never fetches, probes a
model or changes a circuit. The adapter takes P2's opaque `account_id` grouping,
the most constraining observed window, its utilization, proved reset and fetch
time, and computes a consumption rate only from two comparable reads with an
unchanged reset. A missing, stale or unobserved reading stays unknown. A measured
exhausted window is unavailable; unknown ranks above exhausted but below measured
remaining capacity. Preview exposes the same usage facts without fetching, and a
selection log records candidate, tier, mode, remaining, rate, reset and pace score
without account references, provider text or credentials. While the provider usage
feature is off, saved usage modes are unsupported and are not executed as order;
the feature flag and the engine option are set from the same startup value.

Utility/review/taskless Office synthetic `utility:` routes retain their existing
state/attempt storage contract through the native owner bridge below: the attempt
row carries either the original task-session FK or a native route key, never a
fake task session or duplicate ledger. Utility and taskless Office callers
advance RR history; review-run ownership requires the existing G2 review wiring
and is not claimed connected here.

`PolicyStateJSON.selection_chain` holds the logical profile, initial generation,
tried concrete IDs and last selected candidate. Same-candidate retries preserve
the chain. Permitted cross-candidate fallback starts at the failed candidate's
tier and excludes tried IDs. If the failed candidate no longer exists in saved
order and no tried candidate locates a tier, the chain fails closed instead of
restarting at the first tier. A person's Retry of a route left waiting with no
current candidate and no exclusion or preference starts a fresh chain, so an
exhausted traversal is reconsidered from the first tier; tried attempts remain
history and open circuits or recovery guards still apply. Automatic and other
reasons keep the exhausted chain. The existing narrow unclassified path uses the same
ordered search and preserves its workflow fencing. Policy counter carry/reset
preserves the independent chain. Verified current output/effects end a chain
through a separate generation-fenced method, never the general counter helper.
Task-bound Office completion uses the same verified success boundary without the
counter feature's Office exclusion. Utility calls and taskless Office launches
already allocate a fresh `utility:` route ID per call, so a completed call's
chain is not reused by the next call. Disabled completion returns before tier
repository reads.

## Editor

Existing candidate rows keep their policy editors and actions. An enabled tier
feature inserts `=` between the up/down controls and shows derived tier numbers.
Shared draft mutations preserve source blocks on move/delete, and spread opaque
selection data before changing joins or head ownership. The first row cannot
join. OFF keeps the flat controls; stored metadata survives save/reload.

Each head has an explicit mode/failure-direction editor; every row can edit its
cost classification without changing billing identity or opaque model/window
fields. The existing single draft/save/discard/conflict flow owns these edits.

## Read-only new-selection preview

Saved-profile and create-draft dynamic preview POSTs close before body/profile
reads while off. Existing organization configuration permission applies; the
edit interlock does not block a read-only preview. Controller validation and the
resolver's shared native candidate/binding loader feed the same pure ranking as
execution. No model probe, provider usage request, route/probe claim or circuit
mutation is made. Results identify eligibility, original candidate IDs, derived
tier, mode and configured cost. Preview predicts a new chain, not an active
session switch. Random is sampled independently unless a deterministic test
picker is supplied. Draft revision, backend/auth identity and abort ownership
fence late responses. No timer, ledger or queue is added.

## Requirement mapping

AC-001.1/.5: Editor, storage and ranking. AC-001.2/.3: Runtime and chain.
AC-001.4: Feature/storage boundary. AC-001.6: Read-only preview. No operating
configuration or installed-runtime change. AC-001.7 covers task/session RR and
legacy unknown initialization. AC-001.8 covers provider-usage modes and their
unknown handling. Cross-backend choice and keep-model remain original later
increments.


## Native owner journal bridge

The existing route tables now accept either their original non-null task-session
FK or a native route key in nullable `route_id`, never both. The logical unique
key is COALESCE(route_id, session_id), including generation on attempts. The
transactional cutover copies all legacy UUIDs, payloads, timestamps, marker and
nullable selection ordinal; the task FK and delete cascade remain. There is no
synthetic task session, second ledger or installation-state change.

A native decision transaction verifies the already-created utility call, review
run or Office run-session, then performs the same generation CAS and ordinal
allocation. Missing/unrecognized owners fail closed. Utility callers create one
actual call before selection and update concrete attribution on that call;
preparation failure leaves a failed real call. Office uses its reserved actual
run-session key. Legacy/off utility routes remain transient. Task-only startup
and delayed recovery exclude native rows; native owners retain their recovery.

This increment wires utility and taskless Office callers. The native owner
validator accepts only utility-call and office-run-session keys; a review-run
key fails closed until the existing G2 review composition is wired. PostgreSQL
runtime evidence and independent-backend unique choice, keep-model and installed
acceptance remain distinct original work.
