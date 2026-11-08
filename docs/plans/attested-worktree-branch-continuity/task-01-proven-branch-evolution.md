---
id: "01-proven-branch-evolution"
title: "Admit proven branch evolution after completed repair"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.9
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.10
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.12
system_design:
  - ../../specs/agents/system-design/workspace-inventory-repair.md
---

# Task 01: Admit proven branch evolution after completed repair

## Summary

Correct row-scoped receipt admission for a checkout that changed branch after
completed positive repair. Deliver the shared executor fix, real Resume and
prepared-launch regressions, and public recovery guidance as one vertical change.

## In scope

- Follow the design's separate historical and current proof paths.
- Add and run the plan's primary regression before production edits.
- Cover both entry points, inherited borrowers, every inventory slot, and old
  persisted receipts without introducing a migration.
- Retain strict explicit retry, negative attestation, callback safety, and
  environment claim behavior. Add no new lock for read-only completed-repair checks.
- Keep original receipt bytes unchanged and preserve current checkout content.

## Out of scope

Live installation changes, new providers, schema changes, receipt updates,
branch resets, MCP product work, new UI, and prompt-delivery reconciliation.

## Acceptance

1. Resume and prepared launch each admit one launch on the same proven checkout
   after a recorded branch rename or switch and completed positive repair.
   Files, refs, index, and historical receipt bytes remain unchanged by admission.
2. All negative and mixed-inventory cases from the plan fail before launch.
   Existing idempotency and incomplete/negative attestation regressions pass.
   A current branch mismatch cannot authorize branch evolution.
3. Public recovery guidance distinguishes ordinary admission, explicit repair
   replay, and unknown-prompt recovery. Record actual test results and preserve
   any remaining independent runtime blocker as a limitation.

## Verification

Run from the repository root. Run sequentially. The first command is the red
gate after adding its test. It must fail on the branch/ref comparison first.
Repeat it after the fix, then run the remaining commands.

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator/executor -run '^TestResumeSessionWithOptionsAllowsAttestedBranchEvolution$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -run 'WorkspaceInventory|AttestedBranchEvolution|AttestedWorkspaceInventory|InventoryClaim|InventoryEnvironment|InventoryRepair|LaunchPreparedSession|ResumeSessionWithOptions|NormalResumeCrossSession' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor -run '^TestInventoryAdmission' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestWorkspaceRecoveryBlock' -count=1)
(cd apps/backend && go test -trimpath ./internal/worktree -run '^(TestInspectPreservedCheckout|TestPreservation)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/executor/executor_workspace_inventory_recovery.go apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity.go apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity_test.go apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity_integration_test.go apps/backend/internal/orchestrator/executor/executor_inventory_branch_identity_test.go apps/backend/internal/orchestrator/executor/executor_workspace_inventory_receipt_identity_test.go apps/backend/internal/worktree/checkout_identity.go apps/backend/internal/worktree/preservation.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If file names change during implementation, update the lint list to the exact
changed Go files. The integration fixture must use the real SQLite repository
and Git branch updates, not replace the failed predicate with a mock result.
No SQL behavior changes are planned. If implementation changes a dialect-sensitive
store operation, add real PostgreSQL coverage and its exact command before
declaring the work order complete.

## Changed files

- `apps/backend/internal/orchestrator/executor/executor_workspace_inventory_recovery.go`
- `apps/backend/internal/worktree/checkout_identity.go` (new)
- `apps/backend/internal/worktree/preservation.go`
- `apps/backend/internal/orchestrator/executor/executor_inventory_branch_identity_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_workspace_inventory_receipt_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity_integration_test.go` (new)
- `docs/public/websocket-api.md`
- This plan, work order, and the linked requirement/design/decision records.

## Dependencies

None beyond current main. The MCP feature branch is evidence, not a code dependency.

## Risks

See the plan. Preserve current environment ownership when the caller is a borrower.
Do not confuse a completed receipt with permission to bypass runtime admission.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/workspace-inventory-repair.md)
- [System design](../../specs/agents/system-design/workspace-inventory-repair.md),
  especially Branch continuity after completed repair.
- [Accepted decision](../../decisions/2026-10-23-attested-worktree-branch-continuity.md)
- Existing `executor_workspace_inventory_attestation_test.go`,
  `executor_workspace_inventory_all_slots_test.go`, and
  `executor_inventory_claim_test.go` fixtures.

## Results

Implemented and verified on 23 October 2026.

- Red: the real Git/SQLite Resume regression failed for both switch and rename
  with `repaired row receipt identity no longer matches`, before production edits.
- Green: Resume, additional-session launch, owner-bound borrowed environments,
  current dirty/staged/untracked content, unchanged receipts, and a live sibling
  passed. Incomplete/negative/malformed repair evidence and unproven current
  identity refused both entry points. Mixed inventory admitted only when every
  slot was valid. Explicit same-key replay retained its conflict behavior.
- The focused executor race suite and the separate `TestInventoryAdmission`
  race suite passed. The unchanged orchestrator `TestWorkspaceRecoveryBlock`
  cases passed, preserving independent prompt-delivery recovery.
- The preservation inspector and callback-safety tests passed. The original
  broad `-run 'Preserv'` command also selected unrelated relocation tests and
  failed `TestManagedCloneRecoveryPrivateArtifactsPreserveContent`: the shell's
  `0077` umask made its fixture `0700`, although it asserted `0755`. That exact
  test passed with `umask 022`. That failure did not require a source or test change. The
  command above now selects the inspector this fix actually reuses.
- Public docs: all 63 validator tests and all 54 published pages passed.
  Catalog validation passed for 400 decisions and 1,718 specifications; all
  36 specification-linter tests and full specification lint passed.
  The changed runtime files have valid delivery-package coverage.
- Changed-file lint passed with zero issues, using golangci-lint 2.9.0 (the CI
  version) from `/tmp/kandev-mcp-assignment-tools` because it was absent from
  the default `PATH`. `git diff --check` passed.
- PR review regressions reproduced an unrecorded branch change reaching launch
  and unreadable ignored content blocking Resume. Completed-repair admission
  now checks actual Git identity for both matching and changed recorded branches,
  using shared identity checks without scanning checkout content or the index.
  Full repair and unfinished attestation retain preservation hashing. The
  executor and worktree race suites passed after these changes.
- No store or dialect-sensitive SQL changes, schema migration, new dependency,
  new lock, receipt rewrite, deployment, restart, or live task mutation.

The affected installation still needs this code installed before its branch
receipt blocker can change. Its independent unknown prompt outcome remains
subject to normal recovery. These tests do not claim that the live task resumed.
