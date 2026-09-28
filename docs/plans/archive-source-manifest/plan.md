---
status: done
requirements:
  - REQ-TASKS-ARCHIVE-SOURCE-MANIFEST-001
system_design:
  - ../../specs/tasks/system-design/archive-source-manifest.md
---

# Implementation Plan: Task Cleanup Source Manifest

## Overview

Retain task-scoped Git source evidence at the cleanup boundary for archive and
delete operations. Capture happens after runtime stop and is durably written
through the cleanup job claim before destructive worktree cleanup. The manifest
contains content identities, not source bytes, and remains retrievable through
the authorized task audit route.

## Work order

- [Task 01: Capture and expose task cleanup source evidence](task-01-capture-task-cleanup-source-evidence.md)

## Risks and decisions

- Worktree identity is checked against both Git's common directory and its
  registration list for the persisted repository.
- A successful empty inventory is distinguished from a not-yet-captured retry.
- Staged-index records are hashed through Git's read-only listing command,
  including unmerged stages. Dirty submodules use a
  digest over sorted working-tree paths and identities.
- Older archived cleanup rows are not reconstructed or described as clean.

## Verification strategy

- Run focused worktree capture and service cleanup lifecycle tests.
- Run the task handler authorization test for the retrieval route.
- Run docs specification validation and linked-delivery coverage.

## Verification Results

Local implementation checks passed; PR #3905 receives a fresh CI and review
snapshot after the fixup commit is pushed.
