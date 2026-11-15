---
created: 2026-10-29
status: done
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001
system_design:
  - ../../specs/agents/system-design/native-code-review-recovery.md
legacy_specs: []
---

# Implementation Plan: Native code review local source fallback

## Overview

A native code review reads a session's changed files through agentctl. When that
runtime is unavailable (stopped, restarted, or a local executor without a live
agentctl), the review fails with `workspace_unavailable` even though the task's
own checkout is still on disk on the backend host. This plan adds a bounded
local Git reader and a task-bound fallback so a review can still run for a local
or worktree executor.

## Scope

In scope: `internal/review/local_git.go`, `internal/review/local_source.go`, the
reader in `internal/backendapp/review_local_source.go`, the fallback branch in
`internal/agent/handlers/review_change_source.go`, the shared untracked patch
formatter in `internal/common/unidiff`, and their regression tests.

Excluded: the caller-supplied filesystem path API (an explicit operator/admin
path), grant framework changes, schema changes, and the runtime review/utility
integration owned by its own design.

## Technical approach

`CollectLocalChanges` reads a local worktree's working tree (including staged
changes the working tree reverted, and untracked files) or a committed
`base..head` range. It enforces the same per-file, per-output, and total byte
limits as agentctl, including the staged index blob, and rejects a worktree
path that has a `..` segment, is a UNC path, or is not absolute. The backend
session reader resolves the session's persisted `Worktrees`/`WorkspacePath`
and admits only `local`, `worktree`, and `local_pc` executors. The local
fallback repeats the existing `session.exec` check before reading disk. An
explicit repository-qualified comparison target uses its exact validated ref
and fails closed when that ref is unavailable. Both readers use the shared
bounded untracked-patch formatter so the diff text and hash stay equal.

## Work orders

- [x] [Task 01: Task-bound local Git source fallback](task-01-local-source-fallback.md)

## Changed-path coverage

The work order owns the review collector, the backend session reader, the
`ReviewChangeSource` fallback, the shared untracked formatter, their regression
tests, the local-source acceptance criteria, and the matching review recovery
system design note.
