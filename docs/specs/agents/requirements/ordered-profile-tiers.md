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
- **AC-AGENTS-ORDERED-TIERS-001.2:** Ordered selection shall traverse tiers and their members in saved order after the existing failure retry/wait/stop policy permits a transition. An automatic transition shall not revisit a tried candidate or an earlier tier within its durable chain, including after restart. When the chain's current position is absent from saved order and no tried candidate locates a tier, selection fails closed instead of restarting at the first tier.
- **AC-AGENTS-ORDERED-TIERS-001.3:** Counter resets, classified failures, startup readiness and workflow rejection shall not reset a selection chain. Verified current output/effects or an explicit new-chain boundary may end it. Existing generation and workflow fences remain authoritative.
- **AC-AGENTS-ORDERED-TIERS-001.4:** Omitted and partially supplied selection data shall preserve stored model/window/unknown metadata. Unrelated edits shall preserve original route/session history and saved settings. Disabled mode shall retain the existing flat UI/runtime without deleting stored tier data.
- **AC-AGENTS-ORDERED-TIERS-001.5:** Each tier shall expose order, configured cost-class and random selection. Cost ranks free, subscription, metered, then unknown with saved-order ties; random samples only eligible untried candidates within that tier. Existing retry, failure direction, generation and chain boundaries remain authoritative.
- **AC-AGENTS-ORDERED-TIERS-001.6:** A saved or unsaved profile shall offer an explicit read-only new-selection preview using the same pure ranking. Preview identifies the candidate, tier, mode, cost class and exclusions without saving, claiming a generation/probe, changing circuit health or starting work. Failed or stale previews preserve the draft. Random preview is a sample, not a reservation of the next execution.
- **AC-AGENTS-ORDERED-TIERS-001.7:** Round-robin shall choose the eligible candidate after the last committed current tier member in current saved order, wrapping within that tier. Positive selection ordinals, not wall clocks or usage counts, identify the last choice. Legacy unknown order shall remain unknown and initialize at the first eligible saved row. Failed transactions, stale generation claims and previews shall not advance selection. Existing eligibility, chain, failure direction and explicit same-candidate retry remain authoritative.
- **AC-AGENTS-ORDERED-TIERS-001.8:** Provider-usage modes (headroom, pace and consumption rate) shall rank only a comparable provider/account/window reading already observed through the existing provider usage surface. Selection and preview shall not fetch provider usage. An absent, stale or unobserved reading remains unknown; a measured exhausted reading is unavailable; a consumption rate requires two comparable samples with an unchanged proved reset. The account grouping is the opaque public account identity, never a profile, cache key, credential path or secret. While the provider usage surface is off, saved usage modes are unsupported and are not executed as order.

## Remaining increments

Keep-model schema and idle-boundary reevaluation remain separate original work.
Their stored metadata remains opaque and preserved; unsupported runtime modes are
not silently executed as order. Usage modes read only the last observed provider
usage snapshot and stay unknown when that feature is off or no snapshot exists.
Round-robin is implemented for committed task/session routes and the native
utility/taskless Office owner bridge. The native owner validator accepts only
utility-call and office-run-session keys; review-run ownership requires the
existing G2 review wiring and fails closed until then. Cross-backend atomic
choice remains out of scope.

## Implementation plans

- [Ordered tiers](../../../plans/ordered-profile-tiers/plan.md)
