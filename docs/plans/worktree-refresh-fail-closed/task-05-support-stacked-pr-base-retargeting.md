---
id: "05-support-stacked-pr-base-retargeting"
title: "Support stacked PR base retargeting"
status: completed
wave: 5
depends_on:
  - "02-reject-stale-fallbacks"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.12
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 05: Support Stacked PR Base Retargeting

## Summary

Keep PR-backed task bases aligned with GitHub and recover a deleted stacked-PR
parent branch through a verified fallback. Preserve fail-closed behavior for
transport, authentication, timeout, and other unproven fetch failures.

## In scope

- Resolve a GitHub pull request's current base during task repository launch.
- Propagate observed base changes from GitHub polling to the matching task
  repository.
- Fetch and verify a configured fallback only for a proven missing remote base.
- Surface the branch substitution and classify unrecoverable missing refs.
- Add focused executor, GitHub service, and worktree tests.

## Out of scope

- Pull-request head fetching or checkout changes.
- Credential, SSH, or remote-origin behavior changes.
- Event-driven worktree recreation after provider updates.

## Acceptance

- A retargeted stacked pull request materializes against its current live base.
- Polling updates the matching persisted task repository without making PR sync
  depend on that best-effort update.
- A proven deleted base uses only a successfully refreshed fallback and records
  a warning; missing fallback and all transient failures remain fatal.

## Verification

```bash
cd apps/backend && go test ./internal/worktree/... ./internal/orchestrator/executor/... ./internal/github/...
make -C apps/backend test
make -C apps/backend lint
make -C apps/backend build
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/orchestrator/executor/`
- `apps/backend/internal/github/`
- `apps/backend/internal/backendapp/`
- `docs/specs/workspaces/`
- `docs/public/git-operations.md`

## Dependencies

Task 02 established the required-refresh and verified-ref contracts extended by
this work.

## Risks

- A broad fallback could hide auth or transport failures; eligibility must use
  only the explicit missing-remote-ref classifier.
- Multi-repository tasks require exact repository association matching.

## Parallelism

`sequential`

## Inputs

- The pull-request base reconciliation and missing-base fallback sections in
  the workspace system design.
- Existing worktree fallback, executor resolver, and GitHub task-PR sync tests.

## Results

- Launch materialization now resolves a GitHub pull request's live base without
  making GitHub availability a launch dependency.
- PR polling projects a changed base to the matching task repository through
  the existing task-service update contract.
- Required base sync classifies missing remote refs explicitly and uses a
  verified fallback with a user-visible warning when one is available.
- Focused tests, the complete backend suite, backend lint and build, and all
  specification and public-documentation validators passed.
