# ADR-2026-10-06-composable-sidebar-sort-rules: Composable sidebar sort rules

**Status:** accepted
**Date:** 2026-10-06
**Area:** frontend, backend, protocol

## Context

The original proposal offers one running-first activity preset. The user also
needs color priority between runtime state and activity. Fixed combinations do
not support changing that precedence or adding another rule.

Sidebar ordering runs on complete covered local data or bounded server pages.
Task colors are personal settings and currently resolve in frontend presentation.
Color ranking must work before page selection and match the visible marker.

## Decision

Extend the existing primary sort object with a flat, ordered `then_by` list.
Keep legacy primary key and direction fields. Each criterion breaks only ties
from earlier criteria; implicit canonical tiebreaks run after the full chain.

Offer Running and preferred named-color criteria alongside existing ordinary
sort fields. Preserve Custom as a standalone manual mode. Color uses the row's
effective automatic/manual marker, without inheriting a child's color.

Evaluate authenticated color settings in the server ranking query. Share
color-resolution conformance fixtures with the existing frontend resolver.
Keep personal colors in user settings and runtime/activity state in task summaries.

## Consequences

Users can build Running first, Red first, and Newest activity first, then change
that precedence without new presets. Existing saved sorts remain valid.

Backend ranking needs color facts, settings-aware cache identity, and all-rule
projection analysis. This increases delivery scope beyond the original preset.
Colors remain personal presentation priority and never change shared task priority.

## Alternatives considered

- More fixed presets: smaller individual changes, but every combination requires
  another preset and cannot express arbitrary precedence.
- Replace `sort` with a new top-level array: clearer standalone schema, but
  breaks existing single-sort writers and requires a broader saved-view conversion.
- Rank only manual colors: simpler SQL, but can rank a hidden manual red marker
  when an automatic rule makes the visible marker blue.
- Sort returned pages in the browser: cannot rank candidates on other pages and
  breaks the existing bounded global-order contract.

## References

- [Requirements](../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [System design](../specs/ui/system-design/sidebar-running-first-activity-sort.md)
