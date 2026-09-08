---
status: active
system: office
created: 2026-04-25
updated: 2026-08-18
owners:
  - cfl
---
# Office: Cost Tracking & Budget Management Requirements

## Overview

Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

## Requirements

### REQ-OFFICE-COSTS-001: Office: Cost Tracking & Budget Management

**Intent:** Autonomous agents consume tokens on every turn, and when multiple agents run unattended across many tasks, spending can escalate without visibility. Kandev tracks analytics (task counts, turn counts, agent usage) but has no concept of monetary cost. Users have no way to know how much a task cost, which agent is most expensive, or to set guardrails that prevent runaway spending. Without cost tracking and budgets, autonomous operation is a financial black box.

#### Acceptance criteria

- **AC-OFFICE-COSTS-001.1:** Every agent session generates cost events as it runs.
- **AC-OFFICE-COSTS-001.2:** Cost events are aggregated on read into multiple views (by agent, project, task, model, time window).
- **AC-OFFICE-COSTS-001.3:** Budget policies SHALL enforce spending limits per agent, project, or workspace with `notify_only`, `pause_agent`, or `block_new_tasks` actions.
- **AC-OFFICE-COSTS-001.4:** Cost is resolved per cost event via a two-layer lookup (provider-reported then `models.dev` cache); no static fallback table exists.
- **AC-OFFICE-COSTS-001.5:** Stale `models.dev` pricing remains readable while one coalesced refresh runs per client. Concurrent background lookups and explicit refresh calls do not start duplicate network fetches.
- **AC-OFFICE-COSTS-001.6:** For providers that emit token telemetry on the ACP wire (claude-acp, opencode-acp, gemini, codex-acp), cost events are populated from the `complete` event at end-of-turn.
- **AC-OFFICE-COSTS-001.7:** For providers without usable wire telemetry (codex per-turn split, amp), a disk-runner subsystem reads on-disk session files via pinned `@ccusage/*` packages and feeds normalized cost events into the same pipeline.
- **AC-OFFICE-COSTS-001.8:** The cost explorer UI surfaces aggregations and lets the user manage budget policies.

## System design

The migrated technical source is split into [part 1](../system-design/costs-01.md), [part 2](../system-design/costs-02.md).
