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
Candidate runtime metadata decodes only join and tier mode/direction. Ordered
tiers use saved row order; unsupported saved modes stay stored and are not
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

## Requirement mapping

AC-001.1: Editor and storage. AC-001.2/.3: Runtime and chain. AC-001.4: Feature
and storage boundary. No operating configuration or installed-runtime change.
