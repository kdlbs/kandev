---
status: active
system: tasks
created: 2026-07-22
owners:
  - kandev
---
# Attach Workspace Sources Requirements

## Overview

Tasks can add repositories and folders after creation without losing their existing conversation,
state, or repository attachments. The operation is validated and materialized as one task-workspace
change.

## Requirements

### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001: Attach Workspace Sources

**Intent:** Let users and agents attach supported workspace sources while preserving task state and
providing atomic validation and materialization.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.1:** When an idle task opens workspace actions, the system shall offer source attachment and workspace-folder actions on desktop and touch surfaces, and shall preserve configured source rows while the user builds a batch.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2:** When a task submits valid repository and folder sources, the system shall persist and materialize every source, expose repository sources in repository-aware task surfaces, and keep folders file-only.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.3:** When a submission contains an invalid, inaccessible, contradictory, cross-workspace, or executor-incompatible source, the system shall reject it before changing the task or executor workspace.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4:** When any source in a multi-source submission fails during materialization, the system shall roll back all new attachments and restore any pre-existing Kandev-owned entry that the submission repointed.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5:** When attachment changes the effective task root, the system shall preserve task state, plans, conversations, sessions, and existing attachments, publish the updated workspace projection, and either retain or rehydrate the provider session according to executor capabilities.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6:** When an agent uses the legacy worktree-only add-branch action, the system shall create the new worktree as a task-root sibling, return its exact paths, refresh task projections, and leave the running agent working directory unchanged.
