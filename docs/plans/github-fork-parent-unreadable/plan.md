---
created: 2026-11-15
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
legacy_specs: []
---

# Implementation plan: Unreadable fork parent in PR discovery

## Overview

PR discovery reports a permanent `unavailable` failure for a searching watch
on a fork whose parent repository the workspace credential cannot read. This
package makes that parent lookup a healthy empty result, as defined by
`AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.6` and the "Unreadable fork parent"
section of the system design.

## Root cause

1. A searching watch on a fork with no open PR in the fork reaches
   `findPRInForkParent` (`apps/backend/internal/github/service_automation_auth.go`),
   from both the per-watch path (`findPRByBranchInForkNetwork`) and the batched
   path (`lookupSearchingWatchPR` after a definitive-empty alias).
2. The fork's repository details expose the parent name even when the
   credential cannot read the parent, so `FindPRByHead` queries the parent.
3. For a private parent outside the GitHub App installation, GitHub answers
   `GET /repos/{parent}/pulls?head=...` with 404.
4. The error returns unchanged and `classifyPRDiscoveryError` maps it to
   `unavailable`, so the target stays degraded on every cycle.

Reproduction (2026-11-15, Kandev v0.101.0): a fine-grained token scoped only
to the fork reads `parent` from `GET /repos/{fork}` and receives 404 from
`GET /repos/{parent}/pulls?state=open&head={fork-owner}:{branch}`. The
installation behaves the same way: the workspace shows "PR discovery failed",
category `unavailable`, one affected target.

## Scope

In scope: not-found handling for the parent PR query, the scoped fork-parent
cache entry that suppresses repeated parent queries, and regression tests on
the per-watch and batched paths.

Out of scope: classifier changes, new health categories, frontend changes,
GraphQL batching of parent lookups, and discovery of parent PRs the credential
cannot read.

## Work orders

- [x] [Task 01: Treat an unreadable fork parent as no parent PR](task-01-unreadable-fork-parent.md)

One work order: the change is one function plus its cache entry, with one
verification boundary.

## Verification strategy

Backend unit tests in `apps/backend/internal/github`. No end-to-end test: the
outcome is the absence of a failure state that existing projection and UI
already render, and E2E has no fork-network GitHub fixture. Manual check on a
copy of a real installation is recorded in the work order results.

## Risks

- A 404 that means "parent deleted" is handled the same way. Both mean no
  readable parent PR, so the outcome is correct.
- After the credential gains access, the parent stays skipped until the cache
  entry expires (10 minutes), then the next eligible attempt searches it. An
  idle watch can therefore wait for its next scheduled poll.

## References

- [Requirement](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [System design](../../specs/integrations/system-design/github-pr-discovery-health.md)
- [Discovery health package](../github-pr-discovery-health/plan.md)
