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

## Remaining increments

Keep-model schema and idle-boundary reevaluation, round-robin/pace selection and
usage inputs remain separate original work. Their stored metadata remains opaque
and preserved; unsupported runtime modes are not silently executed as order.

## Implementation plans

- [Ordered tiers](../../../plans/ordered-profile-tiers/plan.md)
