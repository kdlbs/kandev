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


### REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002: Complete repository association replacement

**Intent:** A caller can replace a task's complete repository association set without losing its
previous associations when the replacement fails. This contract also applies to repository-input
updates and fresh-branch persistence; it does not extend source materialization rollback.

For concurrent operations, the original set is the canonical set at the serialized replacement
boundary, after prior committed writers. Failure means this operation makes no association change;
it does not undo another independently committed mutation after the request began. A preparation
failure makes no association write at all.

#### Acceptance criteria

- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1:** When preparation, validation, storage, or cancellation
  prevents a replacement from committing, the system shall retain every original association with
  its exact ID, task and repository identity, branches, position, metadata, immutable branch-policy
  fields, and timestamps, and shall retain none of the attempted replacement rows. This covers a
  failure after any proper subset of replacement rows has been inserted, including the second row.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2:** When all inputs are valid and storage commits, the
  system shall expose exactly the complete requested association set in input order, including
  legitimate multiple branches of the same repository. It shall replace the prior set rather than
  append to it. New association IDs and timestamps may be assigned, as in existing replacement.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.3:** When an update omits repositories or supplies null,
  the system shall leave associations unchanged. When an update supplies an explicit empty list,
  the system shall commit an empty association set. An explicit direct replacement with an empty
  or nil list shall likewise clear the set, including an already-empty set.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.4:** When a replacement retains a selected branch policy,
  the system shall preserve the existing immutable snapshot under the existing matching and
  explicit-snapshot precedence, even if the policy was later edited or deleted. It shall preserve
  existing checkout-option omission, branch matching, ambiguity rejection and environment-created
  immutability behavior for task updates, and existing direct replacement semantics. New rows shall
  retain the requested supported checkout, PR and contribution metadata; replacement does not
  promise preservation of arbitrary old metadata omitted from the request.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.5:** When complete replacements overlap on one task,
  their canonical association reads and writes shall serialize. A later replacement shall decide
  inherited fields from the complete set committed by its predecessor; neither a partial union nor
  an intermediate cleared set shall be exposed by a single committed-set read. This is scoped to
  participating complete replacements and existing explicitly task-serialized writers, not a new
  synchronization guarantee for every legacy writer or for a multi-query reader's whole lifetime.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.6:** When a registered REST or WebSocket task update's
  replacement fails, it shall return the existing error classification and emit no successful
  task-update evidence for that failed replacement. A successful response and its ordinary event
  shall reflect a complete committed set under existing postcommit-read semantics. No global
  revision, cross-request event order, or delivery guarantee is added. Prior task-creation evidence
  in the fresh-branch flow and repository-entity resolution events remain separate operations.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.7:** When resolving inputs, the system shall retain
  workspace and provider trust boundaries, typed resolution errors, duplicate-key rejection and
  policy validation. Association replacement shall not introduce runner-mutability gating or
  change launch eligibility. Existing authorization of each entry point remains applicable. Task
  updates shall validate requested assignees and changed parents before preparation can create
  repository entities; a rejection shall create neither an entity nor its creation success event.
- **AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.8:** When replacement fails after other task fields or
  external repository/Git operations have succeeded, the association set shall be preserved without
  promising rollback of those separate operations. On restart the database shall contain the old
  complete set or the committed new complete set. Cancellation before commit is failure; cancellation
  after a completed commit does not undo that commit. An indeterminate commit acknowledgement
  requires a canonical read, not a claim that the prior set necessarily survived.

## Complete replacement boundary

The task-owned attachment lifecycle owns this contract. Workspace repository entities, physical
worktrees and launch-time inventory retain their own lifecycles. The focused technical extension is
[Complete repository association replacement](../system-design/attach-workspace-source-replacement.md).
The existing source-attachment requirement and its materialization guarantees remain unchanged.
