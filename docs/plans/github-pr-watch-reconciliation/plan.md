---
created: 2026-09-13
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
legacy_specs: []
---

# Implementation Plan: Stable GitHub PR watch reconciliation

## Overview

Repair repeated branch rewrites and watch recreation in one sequential backend
work order. Extend the existing discovery requirement with AC .4 and its design;
ACs .1-.3 continue to govern association, identity, and unlink preservation.
The completed discovery-health package remains historical evidence and is not
reopened: this package changes target reconciliation only.

## Scope

### In scope

- Watch-aware branch resolution shared with creation's target source.
- Multi-repository, multi-branch, and redirected-owner regression coverage.
- Preserve genuine renames and existing association safety.

### Out of scope

- Main-instance mutation, restarts, deployment, database repair, or delegation.
- GitLab, provider quotas, new polling loops, UI changes, or new public APIs.
- Attribution of the user's original missing PR, which was not identified.

## Evidence and root cause

Main instance build `509a1bb9b` repeatedly logged branch changes followed by
recreation on the previous branch every minute. For example, on September 13
at 17:02:54 Lisbon time, one session's watches changed to the primary
branch and recreated their former branch for two repositories. Retained September 11-13 logs contained over 110,000 occurrences
of each operation. GitHub status was healthy; rate limits were not implicated.

`refreshStaleBranches` uses session-only resolution through the attributed
task's primary repository. Creation uses source-session repository/worktree
targets and redirects association ownership separately. Refresh therefore
collapses legitimate targets; reconciliation sees them missing and recreates
them. Collision deletion can repeat the cycle without a SQL error.

## Technical approach

Follow the watch target reconciliation section of the discovery system design.
Update `TaskBranchProvider`, its orchestrator implementation, and test doubles
together. Pass repository and existing branch identity to the resolver; derive
source task identity from the session. Preserve exact matches before considering
a unique same-repository replacement. Keep existing atomic update guards.
Extract focused resolver helpers/tests if needed to respect file limits.
The creation/refresh regression also reproduced inventory expansion assigning
one repository's branch to another. Deduplicate session inventory rows and
resolve all targets without that unqualified branch fallback.

## Tests

- AC .4: `TestResolvePRWatchBranchForWatch` in new orchestrator
  `event_handlers_github_watch_reconciliation_test.go`, using real target
  resolution with multiple repositories, multiple branches, group ownership,
  missing data, ambiguous replacements, and genuine single-branch renames.
- AC .4: `TestReconcileWatches_PreservesBranchTargetsAcrossCycles` in new GitHub
  `poller_branch_reconciliation_test.go`. With real SQLite watch storage, run
  at least three reconciliation cycles and assert unchanged IDs, branches,
  row counts, and no repeated create/update calls for unchanged targets.
- ACs .1-.3: retain numbered-watch, collision, detached-association, and
  multi-branch discovery tests in the existing GitHub suite.

## End-to-end evidence

Backend integration coverage carries a secondary-branch watch through
reconciliation, canned provider discovery, persisted task-PR association, and
the task-PR event. Include a numbered sibling to prove mixed-state preservation.
Use existing GraphQL/poller fixtures; do not call GitHub or the main instance.
No browser test is needed because no rendering contract changes.

## Work orders

- [x] [Task 01: Preserve watch branch identity](task-01-preserve-watch-identity.md)

## Verification results

Design validation passed on 2026-09-13:

- `python3 scripts/list-docs.py validate`: 267 decisions and 868 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Scoped `git status --short`: both specification edits and the new package present.

Focused regressions passed after reproducing both defects. The full GitHub
suite passed in 284.480s. The full orchestrator suite passed in 202.462s with
both temporary-directory variables pointing at owned tmpfs storage, following
the documented disk-I/O timeout. The targeted race check passed in 1.293s. Exact commands and results are
recorded in the completed work order. Public documentation remains
unchanged because this package records repair intent without new user operations.

## Risks

- Association owner and source session task differ in shared workspaces.
- Picking the first branch would hide ambiguity and repeat the defect.
- Synthetic review worktrees must retain configured checkout precedence.
- Existing corrupted watches may need one normal convergence cycle; no bulk
  repair is included. Explicit unlink and numbered-watch state must survive.

## Related contracts

- [Discovery requirements](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [Multi-branch decision](../../decisions/0013-multi-branch-tasks.md)
- [Earlier discovery package](../github-pr-discovery-health/plan.md)
