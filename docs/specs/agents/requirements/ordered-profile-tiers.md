---
status: active
system: agents
created: 2026-10-15
owners:
  - kandev
---

# Ordered dynamic profile tiers

### REQ-AGENTS-ORDERED-TIERS-001: Adjacent tier selection

#### Acceptance criteria

- **AC-AGENTS-ORDERED-TIERS-001.1:** An independently enabled native feature shall let users join adjacent candidate rows with `=`, split them and move/delete rows while retaining candidate identities, model metadata and originating tier policies. Tier numbers shall follow current adjacency rather than become saved identities.
- **AC-AGENTS-ORDERED-TIERS-001.2:** Ordered selection shall traverse tiers and their members in saved order after the existing failure retry/wait/stop policy permits a transition. An automatic transition shall not revisit a tried candidate or an earlier tier within its durable chain, including after restart.
- **AC-AGENTS-ORDERED-TIERS-001.3:** Counter resets, classified failures, startup readiness and workflow rejection shall not reset a selection chain. Verified current output/effects or an explicit new-chain boundary may end it. Existing generation and workflow fences remain authoritative.
- **AC-AGENTS-ORDERED-TIERS-001.4:** Omitted and partially supplied selection data shall preserve stored model/window/unknown metadata. Unrelated edits shall preserve original route/session history and saved settings. Disabled mode shall retain the existing flat UI/runtime without deleting stored tier data.
- **AC-AGENTS-ORDERED-TIERS-001.5:** Each tier shall expose order, configured cost-class and random selection. Cost ranks free, subscription, metered, then unknown with saved-order ties; random samples only eligible untried candidates within that tier. Existing retry, failure direction, generation and chain boundaries remain authoritative.
- **AC-AGENTS-ORDERED-TIERS-001.6:** A saved or unsaved profile shall offer an explicit read-only new-selection preview using the same pure ranking. Preview identifies the candidate, tier, mode, cost class and exclusions without saving, claiming a generation/probe, changing circuit health or starting work. Failed or stale previews preserve the draft. Random preview is a sample, not a reservation of the next execution.
- **AC-AGENTS-ORDERED-TIERS-001.7:** Round-robin shall choose the eligible candidate after the last committed current tier member in current saved order, wrapping within that tier. Positive selection ordinals, not wall clocks or usage counts, identify the last choice. Legacy unknown order shall remain unknown and initialize at the first eligible saved row. Failed transactions, stale generation claims and previews shall not advance selection. Existing eligibility, chain, failure direction and explicit same-candidate retry remain authoritative.

## Remaining increments

Keep-model schema and idle-boundary reevaluation, pace selection and
usage inputs remain separate original work. Their stored metadata remains opaque
and preserved; unsupported runtime modes are not silently executed as order.
Round-robin is initially implemented for committed task/session routes in one
backend engine. Utility, review and taskless Office routes still require the
original native owner bridge; their synthetic route IDs have no task-session FK
owner for the existing attempt table. This increment does not complete that scope
or cross-backend atomic choice.

## Implementation plans

- [Ordered tiers](../../../plans/ordered-profile-tiers/plan.md)
