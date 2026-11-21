# ADR-2026-10-28-retained-usage-history: Retain usage independently of tasks and collectors

**Status:** accepted
**Date:** 2026-10-28
**Area:** backend, protocol, frontend

## Context

The user requires years of usage history after task archive, deletion, or collector replacement.
Current external measurements cascade from tasks. Native statistics also depend on live task joins.
Read-time overlap scans repeat work and can grow quadratically with observation count.
Organization access controls do not establish historical actor attribution by themselves.

## Decision

Core owns retained usage subjects, source observations, canonical facts, and rebuildable daily summaries.
Live tasks and sessions provide optional navigation links, not the lifetime or identity of accounting history.
Core snapshots trusted scope and actor evidence when it accepts usage.
Collector provenance remains distinct from the underlying work identity and model provider.

Retain native normalized observations atomically with their existing ledger transaction.
Keep `task_usage_events` and task-session rollups under their existing task-scoped contract.
External cumulative snapshots never become additive events in that ledger.
An accepted retained observation survives later ledger deletion.
Migration preserves old observations before deletion can destroy their only remaining copy.

Resolve sources within indexed work and coverage groups when accepted observations change.
Publish canonical facts and daily summaries through atomic revisions and restart-safe projection generations.
Chart queries use dated summaries. They do not resolve the full workspace history on every read.
Ordinary task deletion retains history. Existing explicit workspace and organization erasure remains authoritative.

This extends [core usage projections](2026-09-13-core-session-usage-projections.md).
It replaces the task-deletion policy of external projections without replacing native event arithmetic or task-level ledger APIs.

## Consequences

Historical totals no longer depend on task content, plugin installation, or live ownership joins.
Different collectors can supply evidence for one usage identity without adding duplicate consumption.
The database stores compact summaries plus sufficient observations to correct and rebuild them.
Retention increases storage and requires registered erasure, bounded migration, and cross-database concurrency tests.
Unknown actors, providers, or dates remain unknown rather than inferred from the current task owner.
Coarse daily reports cannot support exact arbitrary-timezone rebucketing.

## Alternatives Considered

- Keep task cascades: loses historical consumption during routine cleanup.
- Retain every task forever: couples accounting to task content and prevents normal deletion.
- Keep only daily sums: loses the evidence needed for corrections and collector deduplication.
- Reconcile everything on reads: repeats archive-wide work for every chart request.
- Store history in plugin state: ties shared history and its schema to one collector's installation.
- Treat plugin IDs as work identities: replacement collectors can duplicate the same underlying consumption.

## Related specifications

- [Retained history requirements](../specs/costs/requirements/usage-history.md)
- [Retained history design](../specs/costs/system-design/usage-history.md)
- [Task event ledger](../specs/task-cost-ledger/spec.md)
