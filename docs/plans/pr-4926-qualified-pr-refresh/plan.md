---
created: 2026-10-29
status: implemented
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
---

# PR #4926: Preserve pull request refresh state

## Outcome

Refresh the managed checkout's private PR snapshot ref after each contributor commit while preserving its current checkout.

## Work orders

- [x] [Task 01: Preserve the PR snapshot and checkout HEAD](task-01-preserve-pr-snapshot-head.md)

## Verification strategy

Use a real bare Git remote and three successive PR head revisions. Check that each refresh updates the Kandev-owned snapshot ref and that checkout `HEAD` remains at its original commit.

## Results

Implementation and focused verification results are recorded in the [work order](task-01-preserve-pr-snapshot-head.md#results).
