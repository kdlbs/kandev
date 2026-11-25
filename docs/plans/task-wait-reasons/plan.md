---
created: 2026-11-07
status: done
requirements:
  - REQ-TASKS-TASK-WAIT-REASONS-001
system_design:
  - ../../specs/tasks/system-design/task-wait-reasons.md
legacy_specs: []
---

# Implementation Plan: Recorded task wait reasons

## Overview

Tasks waiting for a board slot, session capacity, or a provider quota deadline
can look like ordinary work in progress. Project the recorded quota wait into
the task status summary and show the resolved wait on the task list, board,
pipeline graph, desktop sidebar, and mobile switcher.

## Scope

In scope: the quota-wait projection, its SQLite read, rebuild and revisioned
repair, the frontend wait-reason precedence and icons, and localized labels.

Out of scope: provider block policy, model order, the control-only
waiting-reasons projection, and a dedicated stale indicator for a failed quota
read.

## Requirement and design references

| Work order | Acceptance criteria | System design |
| --- | --- | --- |
| Task 01 | `AC-TASKS-TASK-WAIT-REASONS-001.1` – `.4` | [Recorded task wait reasons](../../specs/tasks/system-design/task-wait-reasons.md) |

## Work orders

- [x] [Task 01: Distinguish recorded waiting reasons](task-01-distinguish-recorded-waits.md)

## Risks

- PostgreSQL behavior and the complete runtime update path remain unverified.
- A failed quota read retains the last known value without a dedicated stale
  indicator.
