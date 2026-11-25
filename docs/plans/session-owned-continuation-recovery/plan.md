---
created: 2026-11-08
status: done
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-003
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
legacy_specs: []
---

# Implementation Plan: Session-owned continuation recovery

## Overview

After `continue_from_history`, recovered sessions could keep stale continuation
notices, ancestry, and native-state fences, and workspace restore could borrow a
sibling session's execution in a shared environment. Settle the
context-continued state when the continuation commits, and keep workspace
recovery owned by the session being restored.

## Scope

In scope: SQLite context-continuation settlement, session-owned workspace
restore, claim-keyed launch coalescing for held workspace-recovery claims,
settlement of superseded predecessor attempts, cold native recovery of continued
history, the legacy history-recovery route, and web restore/panel actions that
follow the selected session.

Out of scope: execution-less stop settlement, new recovery UI, and resend of an
interrupted instruction.

## Requirement and design references

| Work order | Acceptance criteria | System design |
| --- | --- | --- |
| Task 01 | `AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.7`, `.9`, `.24` | [Interrupted prompt recovery](../../specs/platform/system-design/interrupted-prompt-recovery.md) |
| Task 01 | `AC-AGENTS-HARNESS-SESSION-CONTINUITY-003.1`, `005.1` | [Harness session continuity](../../specs/agents/system-design/harness-session-continuity.md) |

## Work orders

- [x] [Task 01: Settle continued sessions and keep workspace recovery session-owned](task-01-settle-continuation-and-own-restore.md)

## Risks

- The SQLite settlement and route-claim paths are covered by adjusted existing
  tests and a local run, without a dedicated permanent test.
- Launch coalescing keyed by a recovery claim must keep the plain session key
  and its duplicate-execution protection for callers without a claim.
