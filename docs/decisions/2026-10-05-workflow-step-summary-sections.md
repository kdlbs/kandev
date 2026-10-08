# ADR-2026-10-05-workflow-step-summary-sections: Expand workflow step summaries inline

**Status:** accepted
**Date:** 2026-10-05
**Area:** frontend, workflow
**Supersedes:** 2026-09-06-inline-workflow-step-tabs

## Context

The inline step editor groups many options behind tabs. Those tabs dominate
the panel and hide the step's configured behavior until an author switches
views. The existing workflow card and step strip remain the desired shell.

## Decision

Replace step tabs with independent Agent, Instructions, Automation, Board
behavior, and Advanced disclosures. Each header shows a live draft summary.
Several sections can stay open together. Actions expand inline beneath their
rows while the other event groups remain visible.

Preserve the shared save coordinator, workflow wire format, existing controls,
and runtime semantics. Desktop and phone layouts share draft state; phone
summary rows stack their label and summary with touch-safe targets.

## Consequences

Authors can read a step before editing and compare several settings together.
Expanding many sections increases vertical scrolling. Closing a section must
not discard edits or bypass capability-resolution checks.

## Alternatives considered

- Event-first layout: emphasizes automation over agent instructions.
- Two-column step sheet: efficient for frequent editing but visually denser
  and requires a separate narrow-screen composition.
- Compact tabs: still hide summaries and prevent simultaneous comparison.
