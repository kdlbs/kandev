---
id: "02-enforce-selection"
title: "Enforce base selection before launch"
status: done
wave: 2
depends_on:
  - "01-record-selection-intent"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.25
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.26
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.28
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
  - ../../specs/workspaces/system-design/worktree-base-selection.md
---

# Task 02: Enforce base selection before launch

## Summary

Carry attachment policy through preparation and reject every unauthorized
different-base fallback. Prove the reported native Git scenario through launch
admission, then document deliberate repair and retry.

## In scope

- Derive policy from the exact attachment in executor `repoInfo`. Preserve the
  distinction between an unset default and an unproven session branch.
- Add typed policy fields to single- and multi-repository launch/preparation
  structures. Wire backendapp adapters, resume/workspace-only paths,
  `prepareOneRepo`, direct branch materialization, and reconciliation copies.
- Guard configured fallback, live remote-default discovery, provider-refreshed
  substitution, numbered-PR fallback, and explicit branch replacement.
- Preserve required-refresh, cancellation, qualified-PR, local-ref, reuse, and
  authenticated empty-remote behavior. Unknown policies are strict.
- Return existing bounded preparation/launch errors naming ref and repository.
  Verify no wrong-base worktree, setup callback, ready environment, fallback
  self-healing, or agent startup for the failing selection.
- Update intentional fallback fixtures to request their separate policy.
- Prove multi-attachment admission and deliberate repair/retry with real Git.
- Update `docs/public/git-operations.md` preparation and troubleshooting text
  with the verified contract. Keep public documentation unchanged until this
  implementation work order.

## Out of scope

- New fetch/retry/credential behavior, branch resets, and a remote-default
  lookup after strict failure with refresh disabled.
- Source-policy inference from branch names, parent IDs, or a PR number.
- Prepared-route PR-base overwrite from #4477, new UI, or remote executor scripts.

## Acceptance

1. The reported missing explicit branch fails before workspace/agent admission,
   including early refresh helpers and live-default discovery paths.
2. The policy remains attachment-specific through every owned adapter.
   Known inherited/default recovery and existing local-ref/qualified-PR controls pass.
3. Real backend launch integration proves failure and successful deliberate
   repair/retry. Exact checks and public documentation validators pass.

## TDD and regression matrix

First add
`worktree/manager_explicit_base_test.go::TestCreateWorktree_ExplicitMissingBaseFailsClosed`.
Use the [recorded reproduction](plan.md#confirmed-root-cause-and-evidence):
publish the selected branch after the cache clone. Keep different commits and
sentinel files on the selected branch and `main`. Require a missing-base error,
nil worktree, no new Git worktree/branch/record, and no creation/setup callbacks.
This test must fail against the pre-enforcement manager behavior.

Then add `TestBaseBranchSelection_*` cases for:

- An explicitly selected default name that is absent while another default is
  discoverable. Strict handling performs no live-default discovery.
- Missing configured fallback, `RemoteSyncHandled`, and numbered-PR refresh.
  Strict policy must reach neither early substitution helper nor final fallback.
- Valid explicit bases with refresh disabled and enabled. Existing same-ref
  local recovery remains valid after auth/network/missing-remote errors.
- Known inherited/default metadata that permits the existing warning fallback.
  Old non-empty unknown inputs and mismatched metadata remain strict.
- Manual same-name reselection, policy-backed bases, and qualified PR targets.
- Branch replacement needing an unavailable base. Preserve the original record
  and checkout. Do not require a still-existing base for valid recorded-branch reuse.
- Two attachments with different policies and base availability. Repeat with
  two rows for the same repository and both preparation orders.
- Cancellation, empty-remote bootstrap, and retained/reused checkout controls.

Use `TestBaseBranchSelection_Propagation` in the relevant executor, lifecycle,
and adapter test packages. Exercise actual conversion functions, including
`topLevelLaunchRepoSpec`, `buildEnvPrepareRequest`, `RepoSpecs`, and
`prepareOneRepo`. A permissive top-level value must not replace a strict spec.

Add
`backendapp/base_branch_selection_integration_test.go::TestBaseBranchSelection_LaunchRejectsWrongBase`
and `TestBaseBranchSelection_RepairAndRetry`. Start from service-created
persisted inputs, use actual adapters and worktree preparation, and observe
the final launch-failure storage and external-agent start counter.
Only the external agent/transport boundary can be fake.
Cover fresh agent launch and workspace-only preparation, a valid sibling, and
same-repository multiple attachments. A deliberate fixture fetch followed by
retry must retain task/session/base identity and use the selected commit.

Existing compatibility fixtures include `manager_base_branch_fallback_test.go`,
`manager_live_default_test.go`, `manager_stacked_pr_base_fallback_test.go`,
`manager_pull_fallback_test.go`, and `manager_recreate_recovery_test.go`.
Opt in only scenarios that represent inherited/default recovery.
Keep qualified PR and local-only refresh assertions intact.

## Verification

Run from the repository root. First capture the expected RED result:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^TestCreateWorktree_ExplicitMissingBaseFailsClosed$' -count=1 -v)
```

After enforcement, repeat that command and run:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/backendapp ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/worktree -run 'BaseBranchSelection|ExplicitMissingBase|BaseBranchFallback|LiveDefault|StackedPR|PRBase|PullEnabledUsesLocalOnlyBase|EmptyRemote|Cancel' -count=1)
bash scripts/lint-go-changed $(git diff --name-only -- '*.go') $(git ls-files --others --exclude-standard -- '*.go')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/explicit-worktree-base-selection
```

Run the [exact coverage preflight](plan.md#package-validation) as well.
The lint command assumes this package's whitespace-free Go paths.
Inspect its file list. Include any additional changed suite in the test command.
All new regression functions must match `BaseBranchSelection` or
`ExplicitMissingBase`, so the focused command executes them.
These checks do not require live provider accounts or a browser.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor.go`.
- `apps/backend/internal/orchestrator/executor/executor_resume.go`.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`.
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`.
- `apps/backend/internal/backendapp/adapters.go`.
- `apps/backend/internal/backendapp/branch_materializer.go`.
- `apps/backend/internal/agent/runtime/lifecycle/types.go`.
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer.go`.
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`.
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`.
- `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go`.
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`.
- `apps/backend/internal/worktree/worktree.go`.
- `apps/backend/internal/worktree/manager_lifecycle.go`.
- `apps/backend/internal/worktree/manager_git.go`.
- `apps/backend/internal/worktree/manager_replacement.go`, only if gate wiring requires it.
- New `base_branch_selection_test.go` files alongside the affected adapters.
- `apps/backend/internal/worktree/manager_explicit_base_test.go` (new).
- `apps/backend/internal/backendapp/base_branch_selection_integration_test.go` (new).
- Existing fallback compatibility tests listed in the matrix.
- `docs/public/git-operations.md`.
- This package's status/results and the selection design's implementation status.

Audit `orchestrator/task_title_branch_runtime.go` and all remaining
`FallbackBaseBranch` producers. Default unproven policy to strict rather than
inventing provenance. Record any necessary additional copy in this file list.

## Dependencies

Task `01-record-selection-intent` must pass before enforcement.

## Risks

- Clearing the fallback string misses the manager's live-default path.
- Guarding only final branch lookup misses provider/PR substitution.
- A copied request can carry one attachment's permission into another.
- Changing valid reuse or same-ref local recovery broadens the fix.
- Legacy missing-base failures are a deliberate compatibility cost.
- A fabricated launch request cannot prove creation-to-preparation wiring.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), including the companion package inventory and recorded failure.
- [Selection design](../../specs/workspaces/system-design/worktree-base-selection.md).
- [Refresh design](../../specs/workspaces/system-design/worktree-base-refresh.md).
- [Criteria](../../specs/workspaces/requirements/worktree-base-refresh.md).
- Existing `executor_inventory_branch_continuity_integration_test.go` for persisted
  executor fixtures and `env_preparer_worktree_multi_test.go` for real Git rollback.
- `apps/backend/AGENTS.md`, `/tdd`, and `/docs-maintainer`.

## Results

Implemented and verified on 2026-11-08.

- Carried the typed policy through executor repo info, repo specs, lifecycle
  launch and environment preparation, workspace repository specs, direct
  branch materialization, and worktree creation.
- The worktree resolver rejects every different-base substitution for strict
  selections: configured fallback, live remote-default discovery,
  provider-refreshed fallback, and numbered-PR fallback all consult the policy.
  Same-ref local recovery and authenticated empty-remote bootstrap keep their
  behavior.
- `TestCreateWorktree_ExplicitMissingBaseFailsClosed` proves the reported
  native Git scenario: no worktree, no record, no callback, and no checkout
  from the repository default.
- `TestBaseBranchSelection_LaunchRejectsWrongBase` and
  `TestBaseBranchSelection_RepairAndRetry` drive service-created attachments
  through the real executor, lifecycle, and worktree preparation with an
  observable external-runtime fake. Multi-attachment admission fails closed and
  rolls back only worktrees created by the attempt.
- Existing inherited/default recovery fixtures opt into
  `BaseBranchFallbackPolicyRepositoryDefault`; qualified PR and local-refresh
  controls remain strict.
- `docs/public/git-operations.md` documents the verified selection contract.
- `TestBaseBranchSelection_ResumePreservesPolicy` drives the actual resume
  request builder: a single-repository inherited missing base keeps its
  repository-default recovery and candidate, while an explicit base stays
  strict. Ambiguous or unbound session bases stay strict.
- `TestBaseBranchSelection_MultiAttachmentSameSessionRetry` reproduces the
  partial multi-repository failure, repairs the missing branch, and retries the
  same task and session. Rolled-back worktrees evict their cache aliases, so the
  retry admits both attachments instead of recreating a checkout with no
  inventory row.
- Full worktree, lifecycle, executor, and backendapp suites pass, along with
  public-doc validation, specification lint, and `lint-go-changed`.
