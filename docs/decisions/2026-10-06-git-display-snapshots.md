# ADR-2026-10-06-git-display-snapshots: Preserve Git display snapshots during refresh

**Status:** accepted
**Date:** 2026-10-06
**Area:** frontend

## Context

Progressive Git observations publish current membership before their patches and counts are ready.
Replacing readable content with a loading placeholder interrupts reading and destroys editor view state.
An old patch cannot certify the contents of a newer observation.

## Decision

Keep one previous display representation beside the authoritative live snapshot for each eligible file and layer.
Label its freshness independently. Keep the existing viewer mounted during refresh and identical completion.
Retained content can support reading, but cannot authorize patch mutations or new line-based review actions.
Current membership and scope replacement govern removal and eligibility.
The display companion remains runtime-only and follows existing Git-state cleanup.

This extends [tracker-owned publication](2026-09-30-progressive-workspace-git-refresh.md) without changing source ordering or backend enrichment.
See [display continuity](../specs/platform/system-design/git-refresh-continuity.md) for its implementation contract.

## Consequences

Readers retain content and position while the toolbar identifies refresh.
Display selectors carry both provenance and current readiness.
The browser retains one bounded previous representation, with no revision history or persistent cache.
Patch-dependent controls need an explicit readiness guard.

## Alternatives considered

1. Merge old patches into new live snapshots. This would present unvalidated details as current and break source ordering guarantees.
2. Wait for enrichment before publishing membership. This would delay new files and undo progressive publication.
3. Preserve content only in each viewer's local ref. This would miss file counts, sibling consumers, and a viewer opened during refresh.
4. Keep loading placeholders. This preserves freshness but interrupts the requested reading continuity.
