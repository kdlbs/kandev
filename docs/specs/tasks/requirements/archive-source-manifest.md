---
status: active
system: tasks
created: 2026-09-24
owners:
  - kandev
---

# Task Cleanup Source Manifest Requirements

## Overview

Archive and delete cleanup removes task worktrees. The task system must retain
evidence of the source state that existed immediately before removal so a
Coordinator can audit the task's terminal integrity after the checkout is gone.
The task system owns the cleanup lifecycle and durable task association; the
workspace system continues to own Git worktree registration and file capture.

## Terminology

- **Source manifest:** A task-scoped record of Git identities and changed-path
  content hashes captured for a cleanup job.
- **Cleanup boundary:** The point after all recorded runtime stop operations
  succeed and before cleanup removes any task worktree.

## Requirements

### REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001: Preserve task source evidence

**Intent:** Preserve verifiable, secret-free evidence for task source state at
the cleanup boundary.

**User story:** As a Coordinator, I want to inspect the source state associated
with an archived or deleted task, so that I can audit its terminal integrity.

#### Acceptance criteria

- **AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.1:** Before archive or delete cleanup
  removes any worktree, the cleanup job shall durably persist the source
  manifest for every captured task worktree. A retry shall reuse persisted
  evidence, and a failed capture or persistence shall leave the worktree intact.
- **AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.2:** Each manifest shall bind the exact
  task, cleanup job, environment, worktree, and repository identities. Capture
  shall verify that the path is a registered worktree of the recorded
  repository; uncertain or foreign ownership shall block cleanup.
- **AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.3:** Evidence shall include HEAD,
  staged-index identity, tracked and untracked status, and content digests for
  changed paths. It shall retain no source bytes or dereferenced symlink data.
  An unmerged index shall retain a digest of its index file. Dirty submodules
  shall have a stable digest of their working-tree contents.
- **AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.4:** Git metadata, index, status, path,
  or content capture failures shall be recorded as a recoverable cleanup error
  and shall block destructive cleanup. A disappeared untracked path shall not
  be recorded as a deletion.
- **AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.5:** An authorized workspace member
  shall be able to retrieve retained source manifests for a task, including
  after task deletion. A foreign workspace caller shall receive the existing
  task-not-found response.
- **AC-TASKS-ARCHIVE-SOURCE-MANIFEST-001.6:** Captures shall not combine or
  attribute content from another task or repository.

## Out of scope

- Reconstructing source state for tasks archived before this requirement was
  implemented.
- Retaining source file contents or changing normal clean-task lifecycle
  behavior after successful evidence capture.
