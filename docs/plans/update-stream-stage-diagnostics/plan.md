---
created: 2026-11-07
status: done
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
legacy_specs: []
---

# Implementation Plan: Update stream stage diagnostics

## Overview

A stalled turn can leave the backend unable to distinguish a quiet adapter from
an update stream or projection worker that stopped making progress. Extend the
existing first-stall diagnostic record with bounded ages and counters for
adapter updates, agentctl stream writes, backend reads, and completed durable
projection.

## Scope

In scope: bounded stage counters and ages in agentctl and the lifecycle
manager, an opt-in internal heartbeat that agentctl sends only when the
connecting backend requests it, per-stream-owner projection progress, and the
stall diagnostic fields that carry these measurements.

Out of scope: active-stream reattachment, accepted-queue successor recovery,
stop-wait snapshots, changes to durable replay order, MCP messages, the
five-minute threshold, or the one-record-per-generation diagnostic rule.

## Requirement and design references

| Work order | Acceptance criteria | System design |
| --- | --- | --- |
| Task 01 | `AC-AGENTS-AGENT-STALL-RECOVERY-001.9`, `.13`, `.15` | [Agent stall recovery](../../specs/agents/system-design/agent-stall-recovery.md#diagnostic-snapshot) |

## Work orders

- [x] [Task 01: Report bounded update stream stages](task-01-report-update-stream-stages.md)

## Risks

- Stage metadata must never count as a turn event or prompt progress, or it
  would extend the inactivity clock that `AC-AGENTS-AGENT-STALL-RECOVERY-001.9`
  protects.
- Older agentctl peers omit the heartbeat; missing or stale measurements stay
  unknown rather than zero.
