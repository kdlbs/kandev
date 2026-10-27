# ADR-2026-10-26-phone-list-sidebar-views: Phone List renders sidebar saved views

**Status:** accepted
**Date:** 2026-10-26
**Area:** frontend, protocol

## Context

The user wants the actual desktop sidebar views on phone List. Independent
mobile sort choices cannot express the existing running, color, and activity
chain. Copying definitions creates competing owners and divergent saves.

The user also wants compact chrome. Temporary search/filter controls and saved
view editing need different entry points and mutation rules.

## Decision

Phone List renders the existing personal workspace sidebar view collection,
active selection, and draft. It shares query semantics and row presentation.
Desktop standalone List and Threads retain their existing preferences and views.

The top-left List control owns saved-view editing. A search icon beside the
hamburger reveals temporary result refinements. Those refinements never write
saved views. Explicit links remain temporary compatibility requests.

Reuse the bounded sidebar query path. Add request-only literal search support
without adding persisted filter dimensions or a new saved-view store.

## Consequences

Phone and desktop sidebar saves target the same workspace view. The List page
needs an early responsive consumer branch and one shared editor controller.
Temporary query identity must remain separate from persisted drafts.

Legacy links and page-local plugin facets need explicit compatibility adapters.
They cannot silently become saved-view fields or override ordinary view selection.

This narrows the proposed
[surface-owned views decision](2026-08-31-surface-owned-saved-task-views.md):
phone List is another rendering of sidebar views, not another saved collection.
Threads and integration views remain independent. No settings migration follows.

## Alternatives considered

- Add Running and Last activity to the List enum: cannot preserve actual saved
  identities, multiple color criteria, or the full shared editor.
- Copy sidebar views into a phone collection: creates separate definitions and save behavior.
- Replace desktop List and Threads collections: exceeds the requested phone scope.
- Let Filters edit the saved view: confuses result refinement with portable changes.

## References

- [Requirements](../specs/ui/requirements/mobile-list-saved-views.md)
- [Design](../specs/ui/system-design/mobile-list-saved-views.md)
- [Plan](../plans/mobile-list-layout/plan.md)
