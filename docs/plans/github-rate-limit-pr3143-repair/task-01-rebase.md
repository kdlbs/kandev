---
id: "01-rebase"
title: "Reconcile the branch with main"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-001.4
  - AC-INTEGRATIONS-GITHUB-RATE-002.1
  - AC-INTEGRATIONS-GITHUB-RATE-003.1
  - AC-INTEGRATIONS-GITHUB-RATE-004.1
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 01: Reconcile the branch with main

## Summary

Complete a history-preserving rebase onto the fetched main. Preserve the final contributor behavior and the local draft package.

## In scope

- Preserve this unstaged package before replay. Use an explicit backup and tracked stash ownership or a user-approved local commit.
- Reconcile obsolete intermediate commits with current client, authentication, MCP, and specification layouts.
- Compare the rebased final diff with the saved contributor head. Explain every dropped or changed patch.
- Preserve the shared auth circuit and current workspace authorization.
- Do not restore obsolete specification indexes only to satisfy an old patch.

## Out of scope

Other work orders, external GitHub writes, and unrelated refactoring.

## Regression evidence

No new behavioral test is required for a pure rebase. Run the affected package checks after conflict resolution and record any base-only failure separately.

## Acceptance

- Current main is an ancestor of the resulting branch, with no unmerged paths.
- Every contributor change is retained, deliberately superseded, or documented as unrelated.
- The complete draft package returns to the worktree without overwriting user edits.

## Verification

Run from the repository root after implementation:

```bash
git merge-base --is-ancestor origin/main HEAD
git diff --name-only --diff-filter=U
git range-diff 04b722121ad50f0dcb41c7c16d8bd6ca5c4b93df..ffd4f76be095db4291cdf762082c8365097d5478 origin/main..HEAD
git diff --check
(cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/mcp/handlers ./internal/mcp/server -count=1)
```

## Files likely touched

- The current PR's changed files, limited to conflict reconciliation.
- apps/backend/internal/github/gh_client.go
- apps/backend/internal/github/graphql.go
- apps/backend/internal/github/pat_client.go
- docs/decisions/INDEX.md (historical conflict target)
- docs/specs/integrations/README.md

## Dependencies

None. Execute sequentially.

## Risks

Do not squash the contributor history or use blanket ours/theirs resolution without explicit approval. Refresh the base and contributor head before replay.

## Parallelism

`sequential`

## Inputs

The recorded failed rebase and immutable heads in plan.md. Existing client and authorization tests protect conflict resolution.

## Results

Completed 2026-09-27. Replayed the branch with `git rebase --rebase-merges origin/main`.
The result is `0bf380a401c1848a36284cf22bff4614227eaede`, with
`origin/main` (`359b5ffdbb6e25592bc3a46d88db1dbf94ff103c`) as an ancestor and
no unmerged paths. Preserved the pre-rebase draft package in
`stash@{0}` (`23767826f2675a976fe9b7aab2c2eeced3d6366c`) and restored it
without conflicts. The backup branch remains at `fa960ef6`.

`git range-diff` matched all listed contributor patches; the final two commits
were recreated as `857e87311` and `0bf380a40`. The LSP conflict was resolved
to current `origin/main`, matching the original merge result. The separate
LSP-only commit remains for Task 06 scope review. No runtime tests apply to
this history-only step.

Task 06 later rewrote local history to remove unrelated commit `a6254608b` and
the artifact retry change from `ffd4f76be` (replayed as `0bf380a40` during this
step). Current local head is `95a30d761b88bab0fd6795e97ba6a5146cf0e6f7`;
`backup/pr-3143-before-scope-cleanup-20260927` preserves the pre-cleanup head.
The earlier rebase result remains recorded above as historical receipt.
