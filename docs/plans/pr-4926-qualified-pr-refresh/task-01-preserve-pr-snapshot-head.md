---
id: "01-preserve-pr-snapshot-head"
title: "Preserve PR snapshot and checkout HEAD during refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-REPOSITORY-BRANCH-002
acceptance_criteria:
  - AC-EXECUTORS-REPOSITORY-BRANCH-002.2
system_design:
  - ../../specs/executors/system-design/repository-branch-resolution.md
---

# Task 01: Preserve PR snapshot and checkout HEAD

## Summary

Use the fully qualified GitHub pull request source ref for refreshes. Preserve the managed checkout's existing `HEAD` while the private PR snapshot ref advances.

## Scope

Document the refresh helper's ref ownership. Strengthen the real-Git regression test so each repeated refresh proves that checkout `HEAD` stays at its initial commit.

No API, persistence, checkout-selection, or credential changes.

## Acceptance

1. Three successive PR head revisions update `refs/kandev/pull/42/head` to the fetched commit.
2. After each refresh, the managed checkout `HEAD` equals the commit recorded before the first refresh.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -run '^TestRefreshWorkspaceRepositoryTracksUpdatedPullRequest$' ./internal/repoclone -count=1)
bash scripts/lint-go-changed apps/backend/internal/repoclone/clone.go apps/backend/internal/repoclone/clone_pr_refresh_test.go
git diff --check
```

## Results

- `go test -trimpath -tags fts5 -run '^TestRefreshWorkspaceRepositoryTracksUpdatedPullRequest$' -count=1 ./internal/repoclone`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/repoclone/clone.go apps/backend/internal/repoclone/clone_pr_refresh_test.go`: passed with 0 issues.
- The PR documentation coverage validator accepted the linked work order.
- `git diff --check`: passed.
