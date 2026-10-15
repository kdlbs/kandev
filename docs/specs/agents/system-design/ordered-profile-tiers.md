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

`PolicyStateJSON.selection_chain` holds the logical profile, initial generation,
tried concrete IDs and last selected candidate. Same-candidate retries preserve
the chain. Permitted cross-candidate fallback starts at the failed candidate's
tier and excludes tried IDs. The existing narrow unclassified path uses the same
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
configuration or installed-runtime change. Round-robin, pace and keep-model
remain original later increments.
