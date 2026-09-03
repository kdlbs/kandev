---
status: draft
system: tasks
created: 2026-09-03
owners:
  - kandev
---

# Missing Workspace Rehome Requirements

## Overview

The task system owns recovery when a retained task environment can no longer be
reused because its physical workspace disappeared. Recovery must keep the same
Kandev task and workflow position, avoid duplicate launches, preserve durable
Kandev artifacts, and make possible repository-data loss explicit.

## Terminology

- **Rehome:** Replace an unusable task environment binding with a freshly
  materialized binding for the same task.
- **Loss assessment:** Durable evidence describing whether repository work in
  the old workspace is known recoverable, known unique, or unknown.

## Requirements

### REQ-TASKS-MISSING-WORKSPACE-REHOME-001: Safe automatic task rehome

**Intent:** Continue a task after its remote workspace disappears without a
successor task or duplicate execution.

#### Acceptance criteria

- **AC-TASKS-MISSING-WORKSPACE-REHOME-001.1:** When step-entry auto-start or an
  explicit restart or resume receives the typed missing-workspace reuse-unsafe
  condition, and the latest loss assessment proves that the old workspace has
  no unique repository work, the system shall create one fresh workspace
  binding for the same task and retry the intended launch exactly once.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-001.2:** A successful rehome shall retain
  the task ID, current workflow step, repository and base-branch selection,
  server-stored conversation and plan artifacts, and the intended agent and
  executor profile selection.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-001.3:** Concurrent or replayed recovery
  attempts for the same failed binding and launch intent shall converge on one
  replacement workspace and one replacement live session.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-001.4:** Rehome shall retry only the
  original failed launch once. A second preparation, reuse, or launch failure
  shall remain visible with both the original and recovery error and shall not
  trigger another automatic rehome.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-001.5:** If replacement provisioning or
  launch fails, the task shall expose a durable pending-action or terminal
  failure after reload; it shall not leave a replacement session appearing as
  an unexplained `CREATED` session.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-001.6:** Launches against an available,
  reusable workspace shall retain their existing behavior and shall not create
  a rehome operation, replacement environment, or replacement session.

### REQ-TASKS-MISSING-WORKSPACE-REHOME-002: Repository loss guard

**Intent:** Never hide the possibility that rehoming abandons unique repository
work.

#### Acceptance criteria

- **AC-TASKS-MISSING-WORKSPACE-REHOME-002.1:** When Kandev cannot prove that the
  missing workspace contained no unique unpushed or uncommitted work, the
  system shall not rehome automatically and shall expose an explicit data-loss
  warning with a human-authorized fresh-rehome action.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-002.1a:** Proof is complete only when
  every repository in the current environment inventory has a phase-current,
  completion snapshot from the launching session showing a clean tree, a
  configured remote branch, and no commits ahead of that branch. Missing,
  stale, partial, or live-monitor-only evidence is unknown and fails closed.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-002.2:** When durable repository evidence
  identifies unique work, automatic rehome shall remain blocked even when the
  workflow phase completed successfully.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-002.3:** When a completed planning or
  specification phase has durable server-side plan and conversation artifacts
  and repository evidence proves no unique work, its next-step auto-start shall
  be eligible for automatic rehome.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-002.4:** A human-authorized rehome shall
  be scoped to the current task, failed binding, workflow step, and error stamp;
  stale or foreign authorization shall fail without mutation.
- **AC-TASKS-MISSING-WORKSPACE-REHOME-002.5:** Desktop and mobile task surfaces
  shall expose the same loss warning, failure detail, and recovery outcome, with
  the authorization action available without horizontal overflow.

## Out of scope

- Recovering bytes that no longer exist on the executor host.
- Automatically pushing, force-pushing, or deleting repository refs.
- Treating every workspace-reuse error as evidence that a physical directory
  disappeared.
- Creating a successor Kandev task.
