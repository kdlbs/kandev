---
status: draft
system: tasks
created: 2026-10-05
updated: 2026-10-05
owners:
  - kandev
---

# Managed clone relocation experience requirements

## Overview

This extends the existing [relocation contract](managed-clone-relocation.md).
A large checkout can take many minutes to preserve and verify. Users need to
know whether that operation still runs after they reconnect. Recovery artifacts
must remain available without crowding the task's working files.

The task system owns these outcomes because its environment owns the operation
and repository inventory. Existing authorization and preservation requirements
remain in force. This draft does not claim that the proposed behavior has shipped.

## Terminology

- **Recovery artifacts:** Retained originals, snapshots, journals, and claim files
  created by managed clone relocation.
- **Active repository:** A checkout selected by the environment's current inventory.
- **Live operation:** An accepted recovery request with a verified server runner.
- **Interrupted operation:** A previously accepted request whose runner no longer
  exists. Its persisted phase alone does not prove that work continues.

## Requirements

### REQ-TASKS-MANAGED-CLONE-RELOCATION-004: Working file presentation

**Intent:** Show working repositories clearly while retaining recovery evidence.

#### Acceptance criteria

- **AC-TASKS-MANAGED-CLONE-RELOCATION-004.1:** New relocation operations shall
  keep recovery artifacts outside the task's working file tree. They shall retain
  all copies required by the existing preservation contract.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-004.2:** Files shall label each active
  repository by its repository name. Opening, editing, searching, and attaching
  its files shall still address the authoritative checkout. Display changes shall
  not rename that checkout or create another checkout.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-004.3:** Existing adjacent recovery records
  shall remain usable after upgrade. Files and workspace search shall exclude
  only artifacts whose ownership Kandev has verified. User files with similar
  names and artifacts with uncertain ownership shall remain visible.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-004.4:** Desktop and phone shall expose
  the same active repositories and retained-copy explanation. Details shall state
  that backups remain available. Hiding an artifact shall not delete or move it.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-004.5:** Replacement publication shall
  refresh Files and search against the new inventory. Old responses shall not
  restore a stale checkout label or root. Task-root user files shall remain reachable.

### REQ-TASKS-MANAGED-CLONE-RELOCATION-005: Durable recovery progress

**Intent:** Make long recovery operations understandable across browser sessions.

#### Acceptance criteria

- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.1:** After accepting explicit
  relocation, Kandev shall expose its current repository, repository position,
  phase, and last progress update. Progress shall not invent a percentage or
  estimated completion time.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.2:** Reload, reconnect, task reopening,
  and a second browser shall recover the current server status. A lost response
  shall not imply that the operation failed or authorize another migration.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.3:** While the operation is live,
  all recovery surfaces shall disable actions that compete with it. A repeated
  request shall identify the current operation without starting another transfer.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.4:** For multiple repositories,
  progress shall distinguish the current slot from completed slots. Migration
  shall complete only after every selected slot validates. Agent readiness shall
  be reported separately from file migration completion.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.5:** A failed or interrupted operation
  shall show its actual state and safe next action. A backend restart shall not
  show an endless running state. It shall not automatically retry, expire a claim,
  discard a retained copy, or start an agent in a partial environment.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.6:** Delayed status and action responses
  shall not replace a newer operation, environment binding, or failure. Progress
  shall preserve session state, error history, and provider conversation identity.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.7:** Desktop shall reuse the existing
  recovery card. Phone shall use its existing focused card with one scroll owner,
  controls of at least 44 pixels, and no horizontal page overflow. Status changes
  shall be accessible without announcing every heartbeat.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-005.8:** Reading progress shall not start
  recovery, reconstruct a workspace, or inspect file contents. It shall remain
  available while the selected workspace is locked for migration.

## Out of scope

- Automatic deletion, expiry, or bulk movement of retained legacy artifacts.
- A backup management screen or install-wide migration job.
- Faster copying, hardlinked snapshots, or exclusion of ignored dependencies.
- A new cancel control, session lifecycle state, or runtime feature flag.
- Changes to generic metadata recovery or non-Worktree executors.

## Related contracts

- [Existing relocation requirements](managed-clone-relocation.md), especially
  REQ-TASKS-MANAGED-CLONE-RELOCATION-001, -002, and -003.
- [Extension system design](../system-design/managed-clone-relocation-experience.md)
- [Existing relocation design](../system-design/managed-clone-relocation.md)

## Implementation plans

- [Recovery progress and workspace presentation](../../../plans/managed-clone-recovery-experience/plan.md)
