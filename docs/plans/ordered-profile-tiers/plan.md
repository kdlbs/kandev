---
created: 2026-10-15
status: in_progress
requirements:
  - REQ-AGENTS-ORDERED-TIERS-001
system_design:
  - ../../specs/agents/system-design/ordered-profile-tiers.md
legacy_specs: []
---

# Ordered profile tiers

Deliver one complete ordered grouping/editor/engine increment, then provider-usage
ranking on the same engine from the last observed provider usage snapshot. Reuse
current dynamic profiles, RulesJSON, generation claims, circuits and failure
policy. Preserve future selection metadata.

## Work orders

- [x] [01: Ordered tier slice](task-01-ordered-tiers.md)
- [x] [02: Cost/random ranking and read-only preview](task-02-cost-random-preview.md)
- [x] [03: Committed task/session round-robin](task-03-round-robin.md)
- [x] 04: Provider-usage modes (headroom, pace, consumption rate) from the
  provider usage snapshot; see the system design.

## ASCII UI preview

UI-01: Existing dynamic profile editor, enabled; same wrapping composition on
desktop/phone, existing responsive action sizes and one page scroll owner.

```text
Tier 1  Candidate A    [up] [= off/disabled] [down] [delete]
        existing enabled control and failure policies
Tier 1  Candidate B    [up] [= on]           [down] [delete]
Tier 2  Candidate C    [up] [= off]          [down] [delete]
Tier head: [Order / Cost / Random / Round-robin / Headroom / Pace / Consumption rate]
           [Same tier / Next tier]
Each row:  [Unknown / Free / Subscription / Metered cost]
[Preview new selection] Candidate / tier / mode / cost / usage / exclusions
```

OFF retains flat row numbers and hides the join control. Empty state, candidate
picker, save/conflict ownership and failure editors retain their existing flows.

## Verification

Reuse relevant existing dynamic/runtime/settings/editor/feature contract checks.
Disposable probes outside normal collection demonstrate chain restart/no-revisit,
counter separation, metadata omission and UI mutations. No permanent tests or
new validation framework. Results belong in the work order.

The task/session RR increment follows actual committed selection ordinals.
Utility and taskless Office advance the same history through the native owner
bridge; review-run ownership remains with the existing review lane. Provider-usage
modes use only the last observed provider usage snapshot and never fetch during
selection or preview. Cross-backend choice and keep-model persistence/idle-boundary
behavior remain original feature work; saved opaque metadata is retained.
