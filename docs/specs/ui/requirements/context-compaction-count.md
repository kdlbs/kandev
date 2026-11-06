---
status: active
system: ui
created: 2026-08-02
updated: 2026-11-05
owners:
  - kandev
---

# Context Compaction Count Requirements

## Overview

The context-window surface shows a session's inferred compaction history.
This count helps users assess how often a conversation loses older context.
Usage changes can describe different agent contexts, so every decrease is not evidence of compaction.

This revision corrects the existing capability for mixed parent and subagent usage.
The existing requirement ID and presentation contract remain canonical.
The [system design](../system-design/context-compaction-count.md) defines persistence and provider compatibility.

## Terminology

- **Eligible sample:** A usable observation from a stream that Kandev does not know to mix unidentified agent contexts.
- **Excluded sample:** A usable observation from a stream known to mix parent and subagent contexts without sufficient attribution.
- **Lifetime count:** The inferred compaction count for one task session, including its previous executions.

## Requirements

### REQ-UI-CONTEXT-COMPACTION-COUNT-001: Context compaction count

**Intent:** Users can inspect an approximate session history without treating switches between agent contexts as compactions.

#### Acceptance criteria

- **AC-UI-CONTEXT-COMPACTION-COUNT-001.1:** The context-window surface shall show a non-negative compaction count for the active agent session.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.2:** When adjacent persisted samples are eligible, a strict decrease in used tokens shall increment that session's count once.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.3:** The first eligible sample shall establish a baseline without incrementing the count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.4:** Equal or increasing eligible usage shall leave the count unchanged.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.5:** A successful explicit context reset or model change shall clear the comparison baseline and preserve the lifetime count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.6:** The info control shall explain that the count uses eligible usage drops, excludes known mixed-context streams, and remains approximate.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.7:** Desktop pointer and keyboard access, and mobile tap access, shall expose the count and its explanation.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.8:** When a session receives its first usable sample, Kandev shall show zero compactions without incrementing the count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.9:** Excluded samples shall never increment the count. Copilot ACP usage without parent/child attribution shall remain excluded, including during subagent execution and after reconnect.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.10:** Excluding a sample from compaction inference shall preserve normal context-window updates, token accounting, and cost accounting.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.11:** An eligible sample immediately after an excluded sample shall establish a new baseline without incrementing the count.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.12:** Refresh, backend restart, and execution recovery shall retain the stored count and exclusion state. Duplicate deliveries shall not create additional increments.
- **AC-UI-CONTEXT-COMPACTION-COUNT-001.13:** The correction shall preserve existing lifetime counts. It shall not subtract historical false positives without attributable evidence.

## Compatibility and accuracy

Copilot ACP currently interleaves parent and child usage without identifying the context owner.
Its usage drops cannot reliably distinguish compaction from a context switch.
Exclusion also prevents genuine compactions in that stream from increasing the inferred count.

Other provider streams retain their existing inference behavior unless evidence establishes the same ambiguity.
Missing observations and provider resets can still cause undercounts or overcounts in eligible streams.
The count does not represent a provider-confirmed compaction total.

A fresh session starts at zero.
An existing session can retain a non-zero historical count after the correction.
Invalid context-window reports retain the existing visibility rules.
A persistence failure shall expose neither a new baseline nor a derived increment.

## Out of scope

- Counting explicit ACP compaction events or changing their separate timeline presentation.
- Inferring context identity from token thresholds, timing, tool names, or open subagent cards.
- Repairing historical counts, adding a schema migration, or aggregating counts across sessions.
- Changing the context ring's layout, touch interaction, or placement.
- Changing provider accounting or suppressing usage observations.

## Implementation plans

- [Original implementation](../../../plans/context-compaction-count/plan.md).
- [Copilot inference correction](../../../plans/copilot-compaction-inference/plan.md).
