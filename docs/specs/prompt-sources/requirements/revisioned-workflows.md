---
status: active
system: prompt-sources
created: 2026-10-15
owners: [backend]
---

# Revisioned workflow source requirements

## Overview

Workspace owners need to reconcile declared workflow prompts without replacing
the task, session or history that already belongs to an existing workflow.
A template identifies an immutable revision. An instance binds that revision
and its string variables to one workspace workflow.

## Requirements

### REQ-PROMPT-SOURCES-REVISION-001: Revisioned native workflow instances

**Intent:** Keep declarative source and native execution in agreement while
preserving the existing workflow's identity and work.

**User story:** As a workspace owner, I want to import and compare revisioned
workflow sources, so that prompt updates preserve ongoing work and its history.

#### Acceptance criteria

- **AC-PROMPT-SOURCES-REVISION-001.1:** When an authorized owner imports valid
  templates and workspace instances, the system shall return stable instance
  and workflow IDs, applied revision, source lock and changed fields.
- **AC-PROMPT-SOURCES-REVISION-001.2:** When the same source is imported again,
  the system shall report no drift and preserve workflow, step and instance
  timestamps. An explicitly reimported prior revision shall support rollback.
- **AC-PROMPT-SOURCES-REVISION-001.3:** When a workflow changes, same-named steps
  shall retain their IDs. Existing tasks, sessions and history shall remain.
  A removed step with assigned or queued tasks, including archived or ephemeral
  tasks, shall prevent both preview approval and import.
- **AC-PROMPT-SOURCES-REVISION-001.4:** When an instance is source locked,
  ordinary definition writes shall be rejected. An explicitly unlocked instance
  shall permit editing and expose the resulting drift, including a renamed
  occupied step whose removal cannot currently be applied.
- **AC-PROMPT-SOURCES-REVISION-001.5:** When input, revision or observed state
  conflicts, or a write fails, the import shall leave all requested instances
  unchanged. Preview shall perform no persistent writes.
- **AC-PROMPT-SOURCES-REVISION-001.6:** Import and diff shall follow existing
  workspace visibility rules. Import and preview shall additionally require the
  existing native workflow definition-write permission; visibility alone shall
  not grant reconciliation. Missing source variables, unknown step fields and
  unavailable direct profile bindings shall be rejected rather than ignored.
  The exact native `{{task_prompt}}` placeholder in a step prompt shall remain
  available for runtime task-description expansion.

## Out of scope

Task movement or archive, external source assembly, agent profile creation,
model order changes, scheduling, operating deployment and activation.
