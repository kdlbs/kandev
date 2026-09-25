# ADR-2026-09-16-conditional-agent-plan-writes: Conditional agent plan writes

**Status:** accepted
**Date:** 2026-09-16
**Area:** protocol

## Context

An agent replaced a detailed plan with its progress checklist.
Kandev preserved history but warned only after the destructive write.
The warning required user recovery because agent tools exposed no history reads or restores.
History coalescing also means that a revision number does not identify one immutable content state.

## Decision

Agent replacement requires a durable edit version. Suspicious reductions require explicit acknowledgement before storage.
The backend changes the version on every content/title write, including browser writes and history coalescing.
Exact edits and revision recovery use the same conditional-write boundary.
Agent results provide correction guidance without requiring a task stop when no mutation occurred.
Recovery never silently replaces intervening user work.

This decision is implemented by the
[safe agent plan edits package](../plans/plan-safe-edits/plan.md).

## Consequences

Existing agent replacement calls without a version receive corrective errors.
Browser interactions retain their current contract, while browser saves invalidate stale agent versions.
A durable version field requires a replayable migration and coverage for both database dialects.
Agent history tools add recovery capability without adding automatic session orchestration.

## Alternatives Considered

- Stronger warnings alone retain the proven failure mode because mutation precedes the warning.
- Revision numbers alone miss in-place coalescing. Timestamps can also collide or change for unrelated metadata.
- Content hashes alone cannot distinguish a later return to identical content from the state the agent originally read.
- Automatic rollback after every reduction can erase intentional edits and race with another writer.
- Mandatory user approval for every edit prevents autonomous correction of routine progress updates.
- A Markdown section parser requires heading identity rules. Unique exact-text replacement meets the current checklist-edit need with fewer semantics.
