---
status: active
system: executors
created: 2026-09-19
owners:
  - kandev
---

# Task executor status indicators

## Overview

Executor indicators let users inspect the current execution environment without
opening a task. Executors owns the status contract; task lists consume it.
This document extracts the compact indicator contract from the
[Kubernetes foundation](../../kubernetes-executor/spec.md) and extends it with
continued automatic refresh. Task-page executor controls remain in that foundation.

## Requirements

### REQ-EXECUTORS-TASK-STATUS-001: Fresh, inspectable task indicators

Sidebar and Kanban remote executor indicators shall expose current exact-session
status without requiring pointer interaction.

#### Acceptance criteria

- **AC-EXECUTORS-TASK-STATUS-001.1:** A valid rendered indicator shall read its
  exact executor/task/session status before hover. While the document is visible
  and the indicator remains mounted, it shall start another read within 90 seconds
  of the previous read settling, without user interaction. Returning to a visible
  document shall refresh an expired result. Timing excludes browser suspension.
- **AC-EXECUTORS-TASK-STATUS-001.2:** Duplicate indicators for the same exact scope
  shall share an in-flight read and recent result. Invalid scope shall issue no
  request and show unavailable. A changed session shall never display a late
  result belonging to its predecessor. Externally supplied status remains authoritative.
- **AC-EXECUTORS-TASK-STATUS-001.3:** Hover or keyboard focus shall open an anchored,
  viewport-contained details surface and refresh status, joining any in-flight
  read. Escape shall dismiss it. The summary shall identify the executor and show
  labelled state, creation and check times, plus Kubernetes restart count and
  workspace mode when available. Errors shall be sanitized, visible as text, and
  retry automatically; loading shall retain previously known facts.
- **AC-EXECUTORS-TASK-STATUS-001.4:** Touch users shall open the same facts in a
  bounded bottom drawer through an effective target of at least 44 px, without
  selecting or opening the underlying task. The drawer shall support dismissal,
  focus return, internal scrolling, and bottom safe-area clearance.

## Exclusions

No executor lifecycle changes, new backend APIs, persisted live status, credential
changes, or continuous reads while the document is hidden or no consumer remains.
