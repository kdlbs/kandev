---
created: 2026-09-23
status: draft
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: PR-link fork launch

## Overview

Restore task startup from a fork PR link. First repair attachment-aware base
validation. Then prove the existing desktop and phone creation flows with real
Git fixtures and the mock provider. Both work orders remain pending.

## Evidence and root cause

Task `979b1b1b-3b52-44c2-8ea5-f317baf39839` failed on 2026-09-23 at
12:39:34 UTC, before agent startup. The backend reported:

```text
pull request identity did not match the task repository binding
```

The retained attachment had `base_branch=main`,
`checkout_branch=feature/archive-force-remove-csd`, and only `pr_number=3879`
in metadata. GitHub returned source `nova28/kandev` and target `kdlbs/kandev`.
The source branch was `feature/archive-force-remove-csd`. The target was `main`.

Commit `a2d4ee8c185` (#3878) added the rejecting identity check.
`resolvePRBaseForLaunch` assumes that the attached repository owns the head
unless `RemoteContribution` supplies another source. Browser creation emits
ordinary PR-link metadata. Its separate PR association runs asynchronously.
The MCP contribution coordinator does not run on this browser creation path.

Evidence came from live task metadata, the GitHub API, source history, and
backend bundle `7841647e48331f84267ca55ea09d2bbd`. The running build was
`v0.95.1-20-gf2d52f18263`. The archive was size-limited but retained creation
and failure. No permanent reproduction test exists yet. Task 01 supplies RED.

## Requirement conformance

The workspace system owns preparation identity and materialization. This is an
implementation regression against existing criteria, not a new product feature.
Reuse [REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001](../../specs/workspaces/requirements/worktree-base-refresh.md),
criteria .11 and .15 through .19. Requirements remain unchanged.
The [design clarification](../../specs/workspaces/system-design/worktree-base-refresh.md#ordinary-pr-link-launch-compatibility)
separates target-attached PR checkout from fork-attached checkout.

The user confirmed that creating a task from a fork PR must work.
No material intent question remains. This package does not convert ordinary
browser tasks into the MCP contribution workflow. That workflow has additional
collaboration and push-policy semantics owned by the task system.

## Scope

### In scope

- Validated target-attached fork PR launch with existing metadata.
- Launch before or after asynchronous PR association, and retry before preparation.
- Exact provider namespace, PR number, head branch, and repository checks.
- Compatibility with explicit contribution bindings and fork-attached checkouts.
- Desktop and phone PR-link startup evidence.

### Out of scope

- New controls, copy, API fields, persistence formats, flags, or permissions.
- Changing source remotes, push routing, or contribution collaboration policy.
- Resetting an existing checkout, repairing the live task, or automatic retries.
- Redesigning PR association ordering, GitLab behavior, or comparison reconciliation.

## Technical approach

`apps/backend/internal/orchestrator/executor/executor_resume.go` owns the
attachment-aware decision in `resolvePRBaseForLaunch` and `validPRBaseIdentity`.
An unbound target-attached checkout can accept a provider-validated fork head
only when the target, PR number, and non-empty checkout branch match.
An explicit source binding always wins. A mismatch cannot enter this legacy path.

`apps/backend/internal/backendapp/orchestrator.go` owns namespace lookup and
`selectLinkedTaskPRForBase`. Preserve rejection of ambiguous linked matches.
The no-association path queries the attached namespace. It must work during
the browser association race. Existing typed errors retain fallback decisions.

Keep the resolved source identity transient in `PRBase`. No database migration
or new durable binding is required. The target-attached checkout keeps the
ordinary target base and target PR-head fetch. Fork-attached checkouts retain
qualified upstream base handling. Do not derive permission from PR identity.

## Tests

All criterion suffixes refer to `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001`.

| Criteria | Planned evidence |
| --- | --- |
| .11, .15 | `executor_pr_base_identity_test.go`: `TestResolveTaskRepoInfo_TargetAttachedForkPRWithoutContribution` fails before the fix |
| .15, .19 | Same test file: explicit source mismatch, unrelated target, wrong branch/number, incomplete identity, same-repository compatibility |
| .15, .17 | `backendapp/pr_base_resolver_test.go`: linked row absent/present, duplicate matches, provider failure, cancellation |
| .16, .18 | `backendapp/pr_base_integration_test.go`: `TestTargetAttachedForkPRBasePreparationEndToEnd` proves exact head/base and unchanged push routing |
| .17, .19 | Integration fixture: valid sibling plus invalid target blocks preparation, without a fallback checkout |

## E2E tests

Task 02 extends `apps/web/e2e/tests/task/create-task-github-url.spec.ts`
(project `chromium`) and `mobile-create-task-remote-repo.spec.ts`
(project `mobile-chrome`). Both paste an upstream PR URL, start the task,
observe the agent, reload, and retain the PR association and selected branch.
The fixture publishes a fork head into the upstream PR ref namespace.
It must not preseed `RemoteContribution` or `ComparisonTarget` metadata.
These flows cover .11, .15, .16, and .19.

There is no rendered UI change, so no ASCII preview is required. Existing
Start Task dialog and mobile creation surface keep their controls, scrolling,
and navigation. Phone coverage uses touch submission on the existing surface.

## Work orders

- [ ] [Task 01: Repair PR attachment identity validation](task-01-attachment-identity.md)
- [ ] [Task 02: Prove browser PR-link startup](task-02-pr-link-startup.md)

Task 02 depends on Task 01. Execution is sequential. No delegation is authorized.

## Companion packages

[Fork PR base resolution](../fork-pr-base-resolution/plan.md) introduced this
regression. Its completed work and command results remain historical evidence.
This package owns the additional cases and new verification results.
[Fork PR comparison targets](../fork-pr-comparison-targets/plan.md) retains
comparison ownership and push invariants. Neither package is reopened.

## Verification results

Product implementation and tests: pending. Exact commands are in each work order.

Design-package checks on 2026-09-23:

- `python3 scripts/list-docs.py validate`: passed (299 decisions, 1124 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Relative document links and referenced acceptance IDs: validated.
- New package inventory: plan and both pending work orders present.
- Production and test code: unchanged. No product tests ran during planning.

## Risks

- Accepting any fork head would weaken binding validation. Explicit bindings
  must never fall through to the ordinary target-attached path.
- Requiring a linked row would make startup depend on asynchronous timing.
- A fixture with a same-repository head or preseeded binding would hide this defect.
- Local Git URL rewrites must remain inside disposable test repositories.

## Documentation impact

This package changes planned implementation only. Public docs remain unchanged.
Implementation restores existing PR-link behavior without new user instructions.
The existing repository-qualified comparison ADR remains authoritative.
