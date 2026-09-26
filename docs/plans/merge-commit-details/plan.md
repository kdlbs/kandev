---
spec: docs/specs/ui/requirements/merge-commit-details.md
created: 2026-08-02
status: completed
---

# Implementation Plan: Repair merge commit details

## Overview

Make the agentctl commit-detail path request a first-parent patch for merge
commits, matching GitHub and the existing commit-list interpretation. Prove the
regression with a real temporary Git graph before changing production code,
then run the focused process-package tests. The API response and frontend
consumer remain unchanged.

## Root cause

`GitOperator.ShowCommit` invokes `git show --format= --stat --numstat -p <sha>`.
For the reported two-parent commit `45f2c92d2a129c06a266f45865ab62d4f63a5d8c`,
Git emits 649 numstat rows but no `diff --git` patch sections under its default
merge-diff behavior. `parseCommitDiff` only constructs file entries from
`diff --git` sections, so it returns an empty map and the frontend renders its
empty state. Running the same command with `--first-parent` emits 649 patch
sections and the expected +45,312/-3,983 first-parent diff.

## Backend

### Commit diff selection

- `apps/backend/internal/agentctl/server/process/git_log.go`: add
  `--first-parent` to the patch-producing `git show` invocation in
  `GitOperator.ShowCommit`. Keep metadata lookup, SHA validation, parsing,
  response fields, and uncapped single-commit behavior unchanged.
- Do not use `-m`, which would produce one diff per parent rather than the
  single first-parent view required by the repair spec.

## Frontend

No frontend code changes are planned. `useCommitDiff` and
`CommitDetailPanel` already render every file returned by the existing
`CommitDiffResult.files` contract on desktop and mobile.

## Tests

- **What:** a clean two-parent merge returns the incoming first-parent file,
  patch, and stats instead of an empty file map.
  **File:** `apps/backend/internal/agentctl/server/process/git_test.go`.
  **How:** add `TestShowCommit_MergeCommitUsesFirstParentDiff` using
  `setupTestRepo`, create divergent feature/main commits, merge main into the
  feature branch with `--no-ff`, and call the real `GitOperator.ShowCommit`.
  The RED run must fail because `result.Files` is empty. The GREEN assertions
  must show the incoming main-side file, exclude a feature-only file already
  present in the first parent, and verify the parsed additions/deletions.
- **What:** existing ordinary and large single-commit diffs remain present and
  uncapped.
  **File:** existing tests in
  `apps/backend/internal/agentctl/server/process/git_test.go`.
  **How:** run the focused `ShowCommit` regression tests, followed by the full
  process package.

## E2E Tests

No new Playwright test is planned. The defect is fully reproduced at the real
Git process boundary, and the existing commit-click E2E already verifies that a
non-empty `files` response is rendered. This repair changes no browser logic,
layout, interaction, or mobile composition.

## Verification Results

- RED: `rtk go test -v -run '^TestShowCommit_MergeCommitUsesFirstParentDiff$'
  ./internal/agentctl/server/process` failed as expected with `files=[]` and
  `merge diff missing incoming first-parent change`.
- GREEN: the same focused command passed with 1 test passed.
- Package: `rtk go test ./internal/agentctl/server/process` passed with 550
  tests in 1 package (exit code 0).
- Hygiene: `git diff --check` passed. The test repositories are temporary and
  removed by `t.Cleanup`; no browser sessions, instances, or external state
  were started or modified.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [Task 01: First-parent merge commit diff](task-01-first-parent-merge-diff.md) — completed.

There are no parallel candidates: the production change and regression test
share one package and form a single TDD unit.

## Risks and out of scope

- First-parent semantics intentionally show what the merge added to the branch
  that received it; parent selection and combined-diff UI remain out of scope.
- The response can still be large because `ShowCommit` is intentionally
  uncapped; this repair does not change that established contract.
- No public API, persistence, authorization, frontend, or documentation
  contract changes are introduced.
