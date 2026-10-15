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

## Remaining increments

Keep-model schema and idle-boundary reevaluation, cost/random/round-robin/pace
selection, usage inputs and preview remain separate original work. This increment
does not reset their stored metadata or claim to implement their behavior.

## Implementation plans

- [Ordered tiers](../../../plans/ordered-profile-tiers/plan.md)
