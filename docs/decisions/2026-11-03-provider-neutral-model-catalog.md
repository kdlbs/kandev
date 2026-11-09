# ADR-2026-11-03-provider-neutral-model-catalog: Preserve provider model structure in one shared catalog

**Status:** accepted
**Date:** 2026-11-03
**Area:** backend, agentctl, frontend, session configuration

## Context

Issue #5026 requires grouped navigation and verified composed-model selection across settings and live sessions.
Current ACP converters flatten groups and erase their identities.
Devin Fusion motivates composition, but existing evidence does not define a durable Lead/Sidekick decoder contract.
Mode, model dimensions, and model-dependent options have different authority.

## Decision

Extend the existing normalized select option with an optional bounded catalog.
Use neutral shared types for groups, exact choices, and verified composition bindings.
Retain a derived flat compatibility view and the current owning configuration snapshot.
Keep provider interpretation in backend compatibility normalization and keep the renderer provider-neutral.

Dimensions resolve only to exact current catalog entries.
Thinking and Speed remain authoritative provider configuration options.
Preserve current mutation, restoration, comparison baseline, and turn-attribution owners.
Multiple ACP changes retain partial-application semantics without claiming an atomic transaction.

Require a separate provider qualification gate for Fusion bindings.
Unknown or ambiguous entries remain ordinary choices.
Fallback does not satisfy verified Fusion composition acceptance.

## Consequences

Probe and live projection paths must preserve optional catalog data, including boot hydration and existing snapshots.
Flat providers and older records retain their current behavior.
Another provider can use the same UI without vendor-specific branches.
Qualification can delay Fusion completion after ordinary grouping works.

## Alternatives considered

- Keep a flat list: preserves lookup but loses supplied navigation and leaves pairing selection cumbersome.
- Preserve group headings only: improves navigation but cannot express dimension compatibility.
- Parse Devin names in React: couples interpretation to presentation without establishing trustworthy bindings.
- Add another selector store or metadata expression language: duplicates authority and exceeds bounded presentation needs.
- Treat Apply as atomic: misreports confirmed state when a later option request fails.

## Related artifacts

- [Requirements](../specs/agents/requirements/dynamic-provider-options.md)
- [System design](../specs/agents/system-design/grouped-model-catalog.md)
- [Combined delivery plan](../plans/first-draft-profile-discovery/plan.md)
