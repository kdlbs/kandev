---
status: draft
system: tasks
created: 2026-10-12
owners:
  - kandev
---

# Deferred Start on Step Entry Requirements

## Overview

A task can carry a deferred start: a launch request recorded at creation while
a dependency, workflow WIP limit, or session capacity limit delays it. The task
system owns this contract because the record is task metadata, and the
workflow step move decides when it runs. Step agent starts are otherwise
specified in [workflow step agent start ownership](workflow-step-agent-start-ownership.md).

## Terminology

- **Deferred start:** The task's `deferred_launch` record.
- **Agentless step:** A workflow step without an `auto_start_agent` entry action,
  such as a hold or predecessor wait.

## Requirements

### REQ-TASKS-DEFERRED-START-STEP-ENTRY-001: Keep the deferred start on an agentless step

**Intent:** Moving a task into a step that cannot run an agent does not spend
its deferred start.

**User story:** As a workflow user, I want a task that waits in a hold step to
keep its pending start, so that it starts with my latest settings when it
reaches an agent step.

#### Acceptance criteria

- **AC-TASKS-DEFERRED-START-STEP-ENTRY-001.1:** When a step move enters an
  agentless step, the system shall not launch the deferred start and shall keep
  the record unchanged.
- **AC-TASKS-DEFERRED-START-STEP-ENTRY-001.2:** When a later step move enters a
  step with automatic start, the system shall consume the kept record through
  the existing deferred-launch path and launch once.
- **AC-TASKS-DEFERRED-START-STEP-ENTRY-001.3:** When the destination step cannot
  be read, the move shall keep its current deferred-launch attempt. Dependency
  resolution, queue promotion, and creation shall keep their current
  deferred-launch behavior.

## Out of scope

- The contents of the deferred-launch record.
- Deferred workflow moves, which [deferred move dispatch identity](deferred-move-onenter-dispatch-identity.md) covers.

## System design

- [Deferred start on step entry](../system-design/deferred-start-step-entry.md)
