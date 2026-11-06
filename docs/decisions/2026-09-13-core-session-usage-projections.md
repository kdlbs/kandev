# ADR-2026-09-13-core-session-usage-projections: Core session usage projections

**Status:** accepted
**Date:** 2026-09-13
**Area:** backend, frontend, protocol

## Context

Session Cost calculates usage through tokscale. Its current refresh does not persist those results.
The user requires native token statistics and future native collection sources.
Kandev already records native events in `task_usage_events` and maintains task-session rollups.
Those event records have immutable identities. Tokscale reports cumulative transcript totals instead.

## Decision

Kandev owns source-aware session measurements, dated projections, and the Token Usage statistics page.
Plugins submit measurements through capability-gated Host APIs and a shared accounting service.
Future native adapters use the same service with host-assigned source identities.

External cumulative measurements do not become additive events in `task_usage_events`.
They do not increment its task-session rollups.
The existing ledger remains immutable and supplies a native candidate to the new projection.
The projection selects non-duplicated coverage and reports incomplete attribution explicitly.

Plugin removal preserves core usage history. Collection settings belong to the collector.
The native page queries saved data and does not require plugin code to render.

This decision follows the typed service boundary in [ADR 0043](0043-plugin-host-data-api.md).
The broader [generic Host proposal](2026-08-31-generic-plugin-host-boundary.md) remains proposed.
This package does not silently implement its separate exact-operation protocol.

## Consequences

Core storage supports indexed aggregation and multiple sources.
The plugin can show saved values during a new calculation.
Source selection and coverage validation become accounting responsibilities in Kandev.
Existing ledger APIs keep their current event-based semantics during this additive change.
The new view identifies source and coverage so those surfaces need not imply identical totals.

## Alternatives Considered

- Plugin state and a plugin stats slot: couples native history to plugin storage and uninstall behavior.
- Add every tokscale report to the event ledger: counts cumulative usage repeatedly and duplicates native observations.
- Replace the native ledger: changes an existing audit contract and exceeds the requested collection work.
- Read the database directly from the plugin: bypasses supported permissions, services, and database portability.

## Related specifications

- [Token Usage requirements](../specs/costs/requirements/token-usage.md)
- [Token Usage design](../specs/costs/system-design/token-usage.md)
