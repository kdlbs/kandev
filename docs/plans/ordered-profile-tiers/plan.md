---
created: 2026-10-15
status: implemented
requirements:
  - REQ-AGENTS-ORDERED-TIERS-001
system_design:
  - ../../specs/agents/system-design/ordered-profile-tiers.md
legacy_specs: []
---

# Ordered profile tiers

Deliver one complete ordered grouping/editor/engine increment before adding
usage-aware ranking. Reuse current dynamic profiles, RulesJSON, generation
claims, circuits and failure policy. Preserve future selection metadata.

## Work orders

- [x] [01: Ordered tier slice](task-01-ordered-tiers.md)

## ASCII UI preview

UI-01: Existing dynamic profile editor, enabled; same wrapping composition on
desktop/phone, existing responsive action sizes and one page scroll owner.

```text
Tier 1  Candidate A    [up] [= off/disabled] [down] [delete]
        existing enabled control and failure policies
Tier 1  Candidate B    [up] [= on]           [down] [delete]
Tier 2  Candidate C    [up] [= off]          [down] [delete]
```

OFF retains flat row numbers and hides the join control. Empty state, candidate
picker, save/conflict ownership and failure editors retain their existing flows.

## Verification

Reuse relevant existing dynamic/runtime/settings/editor/feature contract checks.
Disposable probes outside normal collection demonstrate chain restart/no-revisit,
counter separation, metadata omission and UI mutations. No permanent tests or
new validation framework. Results belong in the work order.
