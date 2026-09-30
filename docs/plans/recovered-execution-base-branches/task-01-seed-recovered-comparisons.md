---
id: "01-seed-recovered-comparisons"
title: "Seed recovered comparisons before readiness"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001
acceptance_criteria:
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.1
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.2
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.3
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.5
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.6
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.7
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.8
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.9
  - AC-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001.10
system_design:
  - ../../specs/workspaces/system-design/workspace-base-branch-propagation.md
---

# Task 01: Seed Recovered Comparisons Before Readiness

## Summary

Reuse valid base-branch metadata, synchronously hydrate it when absent, and
complete configuration pushes before readiness becomes visible. Prove that
the first recovered Git result contains only task changes.

## In scope

- Creation metadata hydration through the existing task provider.
- Base-branch/comparison-target readiness ordering and client lease cleanup.
- Regression tests at lifecycle, executor request, and Git HTTP boundaries.

## Out of scope

New API readiness states, mandatory hydration, UI, SDK, flags, schema, and
candidate/truncation changes. No unrelated refactoring.

## Acceptance

1. Missing, nil, empty, or unusable base metadata invokes the configured provider
   before `CreateInstance`. Usable metadata survives without provider lookup;
   typed and JSON-restored maps retain root and multi-repo keys. Neither input
   map is mutated. No-task/no-provider and hydration errors preserve best-effort
   startup, with warnings on errors.
2. Readiness remains false while either push is blocked. Ready events and cached
   poll-mode flushing follow both attempts. HTTP readiness failure and push
   failure release leases and respect cancellation; push errors remain nonfatal.
3. First Git API responses from three seeded `origin/develop` repositories
   exclude unrelated `origin/main` history and return zero task commits. Adding
   one task commit returns only it; a mixed configured/unconfigured inventory
   proves independent resolution. Explicit comparison precedence remains covered
   by existing tests.

## TDD regression

Start with `TestPrepareExecutionCreateRequest_HydratesBaseBranches`: use
`newTestManager`, absent metadata, and a provider returning three keyed
`origin/develop` values. Assert those values in the outgoing metadata. It fails
before the fix because the provider is never called. Add
`TestWaitForAgentctlReady_SeedsBeforeReady` using a blocked HTTP setter and assert
the execution is not ready. It fails because readiness currently flips first.
Use channels and existing test fixtures, not timing sleeps.

Add `TestRecoveredBaseBranches_FirstGitResponses` at the API boundary. Feed
the seeded instance map into `process.NewManager`; invoke real HTTP Git routes
against real repositories. Keep setup/cleanup compatible with existing test
helpers and backend leak checks. Capture `CreateInstance` metadata in a
`GetOrEnsureExecution` regression so eventual pushes cannot mask missing
creation data.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle/... ./internal/agentctl/server/api/... ./internal/agentctl/server/process/... -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use focused `-run` invocations for red/green first, then the block above once.
Record each command and its result. Re-run documentation coverage preflight
through `.github/scripts/pr-docs.cjs`'s `validateCoverage` export with the changed
work order and referenced files. Do not publish from this work order.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_base_branches.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovered_execution_base_branches_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git_recovered_base_branches_test.go` (new)
- This package and its linked requirement/design for status and results.

## Dependencies

None.

## Risks

See the [plan risks](plan.md#risks). The original package's readiness-time
best-effort behavior remains the error-path contract. Plugin transport
capabilities are not expanded.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/workspace-base-branch-propagation.md)
- [Design](../../specs/workspaces/system-design/workspace-base-branch-propagation.md)
- `base_branches_metadata_test.go`, `manager_lifecycle_test.go`,
  `git_multi_repo_review_test.go`, and existing executor forwarding tests.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference.

## Results

Completed 2026-09-30. Creation now reuses a copied persisted map or hydrates it
before the executor boundary. Base and comparison-target pushes complete before
readiness, ready events, and the cached poll-mode flush.

- Red/green coverage: `TestPrepareExecutionCreateRequest_HydratesBaseBranches`,
  `TestGetOrEnsureExecution_SeedsBaseBranchesAtCreation`, and
  `TestWaitForAgentctlReady_SeedsBeforeReady` failed before the correction and
  pass after it.
- Additional lifecycle coverage: map ownership and JSON/root/multi-repo shapes,
  absent/malformed metadata, empty/error provider results, optional providers,
  best-effort HTTP push failures, health cancellation, and released client leases.
- `TestRecoveredBaseBranches_FirstGitResponses` passes with real repositories:
  configured `origin/develop` excludes integration history, zero task commits
  stays zero, one new task commit appears alone, and a no-base sibling uses its
  own fallback without affecting configured siblings. Log, fresh status, and
  cumulative diff all agree on the per-repository anchor.
- Combined verification command: API/process/probe/skill PASS; lifecycle hit an
  existing resolver-test environment failure. It reproduces with all changed
  production files replaced by HEAD. Rerunning lifecycle with only the inherited
  bundle override removed PASS (92.168s; skill 1.221s; 3,533 passing test events).
  Exact rerun: `(cd apps/backend && env -u KANDEV_BUNDLE_DIR go test -race
  ./internal/agent/runtime/lifecycle/... -count=1 -json)`.
- `python3 scripts/list-docs.py validate`: PASS.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- `git diff --check`: PASS.
- Actual changed-file preflight via `.github/scripts/pr-docs.cjs.validateCoverage`:
  PASS, `covered`, no errors.

See [the plan's recorded commands and results](plan.md#verification-results)
for package durations and environment evidence. Publication remains outside
this work order.
